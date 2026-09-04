// Package service provides business logic for authentication.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/vicky/url-shortner/external/cache"
	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/config"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/utils"
)

// SessionCache is the contract the service depends on for session caching.
// Implementations live in external/cache; the service never imports Redis
// or any other concrete backend.
type SessionCache interface {
	HGet(ctx context.Context, key, field string) (string, error)
	HMGet(ctx context.Context, key string, fields ...string) (map[string]string, error)
	HGetAll(ctx context.Context, key string) (map[string]string, error)
	HSet(ctx context.Context, key string, fields map[string]any, opts ...cache.CacheOption) error
	HDel(ctx context.Context, key string, fields ...string) error
	Del(ctx context.Context, key string) error
}

// NoopCache is a SessionCache implementation that always returns cache-miss.
// Used in tests and as a fallback when cache is unavailable.
type NoopCache struct{}

func (NoopCache) HGet(context.Context, string, string) (string, error) { return "", fmt.Errorf("noop") }
func (NoopCache) HMGet(context.Context, string, ...string) (map[string]string, error) {
	return nil, fmt.Errorf("noop")
}
func (NoopCache) HGetAll(context.Context, string) (map[string]string, error) {
	return nil, fmt.Errorf("noop")
}
func (NoopCache) HSet(context.Context, string, map[string]any, ...cache.CacheOption) error {
	return nil
}
func (NoopCache) HDel(context.Context, string, ...string) error { return nil }
func (NoopCache) Del(context.Context, string) error             { return nil }

// Cache key prefixes — every Redis key in the application must use one of these.
const (
	cacheKeySession   = "session:"   // session validation cache (keyed by session ID)
	cacheKeyRateLimit = "ratelimit:" // login rate-limit counter (keyed by email)
)

// apperror.ErrUnauthorized wraps ErrUnauthorized so it maps to HTTP 401, and
// uses one generic message for both unknown emails and wrong passwords to
// prevent account enumeration.

// AuthService provides authentication business logic.
type AuthService struct {
	queries gen.Querier
	db      *sql.DB
	cfg     *config.Config
	cache   SessionCache
	log     logger.Logger
}

func NewAuthService(queries gen.Querier, db *sql.DB, cfg *config.Config, cache SessionCache, log logger.Logger) *AuthService {
	if log == nil {
		log, _ = logger.New()
	}
	return &AuthService{
		queries: queries,
		db:      db,
		cfg:     cfg,
		cache:   cache,
		log:     log,
	}
}

// withAuthTx runs fn inside a database transaction. When s.db is nil (tests
// without a real DB) the fn receives the mock querier directly so unit tests
// remain simple.
func (s *AuthService) withAuthTx(ctx context.Context, fn func(q gen.Querier) error) error {
	if s.db == nil {
		return fn(s.queries)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.log.Error("failed to begin transaction", logger.Error(err))
		return apperror.ErrInternal
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(gen.New(tx)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		s.log.Error("failed to commit transaction", logger.Error(err))
		return apperror.ErrInternal
	}
	return nil
}

type Tokens struct {
	AccessToken  string
	RefreshToken string
}

// Claims is the JWT access token payload. It carries the session ID so
// the middleware can verify the session is still alive (not revoked /
// expired) on every request. SessionVersion is compared against the
// session's last_active_at to invalidate old tokens after a refresh.
type Claims struct {
	UserID         string `json:"user_id"`
	Email          string `json:"email"`
	DisplayName    string `json:"display_name"`
	Role           string `json:"role"`
	SessionID      int64  `json:"session_id"`
	SessionVersion int64  `json:"session_version"`
	jwt.RegisteredClaims
}

// Register creates a new user account.
func (s *AuthService) Register(ctx context.Context, req *payload.RegisterRequest, deviceType, deviceName, ipAddress, country, city, userAgent string) (*payload.AuthResponse, error) {
	// Validate email
	if err := utils.ValidateEmail(req.Email); err != nil {
		return nil, err
	}

	// Validate password strength
	if err := utils.ValidatePassword(req.Password); err != nil {
		return nil, err
	}

	// Check if user already exists
	_, err := s.queries.GetUserByEmail(ctx, req.Email)
	if err == nil {
		s.log.Warn("registration attempt with existing email", logger.String("email", req.Email))
		return nil, apperror.ErrEmailAlreadyExists
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, sql.ErrNoRows) {
		s.log.Error("failed to check user existence", logger.Error(err), logger.String("email", req.Email))
		return nil, apperror.ErrInternal
	}

	// Hash password
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.log.Error("failed to hash password", logger.Error(err))
		return nil, apperror.ErrInternal
	}

	var user gen.CreateUserRow
	var displayUserID string

	// Wrap CreateUser + UpdateUserDisplayID + AddPasswordHistory in a transaction
	err = s.withAuthTx(ctx, func(q gen.Querier) error {
		// Create user
		var txErr error
		user, txErr = q.CreateUser(ctx, gen.CreateUserParams{
			Email:           req.Email,
			PasswordHash:    string(passwordHash),
			DisplayUserName: utils.NullString(req.DisplayName),
		})
		if txErr != nil {
			return txErr
		}

		// Generate display user ID
		displayUserID = utils.EncodeID(user.ID, utils.UserIDPrefix, s.cfg.UserIDSecretKey)
		_, txErr = q.UpdateUserDisplayID(ctx, gen.UpdateUserDisplayIDParams{
			ID:            user.ID,
			DisplayUserID: utils.NullString(displayUserID),
		})
		if txErr != nil {
			return txErr
		}

		// Add initial password to history
		txErr = q.AddPasswordHistory(ctx, gen.AddPasswordHistoryParams{
			UserID:       user.ID,
			PasswordHash: string(passwordHash),
			IpAddress:    utils.NullIP(ipAddress),
			UserAgent:    utils.NullString(userAgent),
		})
		if txErr != nil {
			return txErr
		}

		return nil
	})
	if err != nil {
		s.log.Error("failed to create user (transaction rolled back)", logger.Error(err), logger.String("email", req.Email))
		return nil, apperror.ErrInternal
	}

	s.log.Info("user registered", logger.Int64("userID", user.ID), logger.String("email", req.Email))

	// Generate tokens
	tokens, err := s.GenerateTokens(ctx, user.ID, displayUserID, user.Email, req.DisplayName, user.Role, deviceType, deviceName, ipAddress, country, city, userAgent)
	if err != nil {
		return nil, err
	}

	return &payload.AuthResponse{
		Token: payload.RefreshTokenResponse{
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
		},
		User: payload.UserResponse{
			ID:          displayUserID,
			Email:       user.Email,
			DisplayName: req.DisplayName,
			Role:        user.Role,
		},
	}, nil
}

// Login authenticates a user and returns tokens.
func (s *AuthService) Login(ctx context.Context, req payload.LoginRequest, deviceType, deviceName, ipAddress, country, city, userAgent string) (*payload.AuthResponse, error) {
	// Validate email
	if err := utils.ValidateEmail(req.Email); err != nil {
		return nil, err
	}

	// Check login rate limiting
	if blocked, err := s.checkLoginRateLimit(ctx, req.Email); err != nil {
		return nil, err
	} else if blocked {
		return nil, apperror.ErrRateLimited
	}

	user, err := s.queries.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("login attempt with unknown email", logger.String("email", req.Email))
			return nil, apperror.ErrUnauthorized
		}
		s.log.Error("failed to get user by email", logger.Error(err), logger.String("email", req.Email))
		return nil, apperror.ErrInternal
	}

	// Block login for accounts pending deletion. Return the same generic
	// error as bad credentials so account status is not leaked.
	if user.Status == "PENDING_DELETION" {
		s.log.Warn("login attempt for account pending deletion", logger.Int64("userID", user.ID))
		return nil, apperror.ErrUnauthorized
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {

		// Record failed login attempt
		s.recordFailedLogin(ctx, req.Email)

		s.log.Warn("invalid password", logger.String("email", req.Email))
		return nil, apperror.ErrUnauthorized
	}

	// Clear failed login attempts on successful login
	s.clearFailedLogins(ctx, req.Email)

	displayUserID := ""
	if user.DisplayUserID.Valid {
		displayUserID = user.DisplayUserID.String
	}

	s.log.Info("user logged in", logger.Int64("userID", user.ID), logger.String("email", req.Email))

	// Expire any sessions past their expires_at before generating new tokens
	if expireErr := s.queries.ExpireSessionsByUser(ctx, user.ID); expireErr != nil {
		s.log.Error("failed to expire old sessions", logger.Error(expireErr), logger.Int64("userID", user.ID))
	}

	// Generate tokens
	tokens, err := s.GenerateTokens(ctx, user.ID, displayUserID, user.Email, user.DisplayUserName.String, user.Role, deviceType, deviceName, ipAddress, country, city, userAgent)
	if err != nil {
		return nil, err
	}

	// Calculate password age
	var passwordAgeDays int
	changeSuggested := false
	if user.PasswordChangedAt.Valid {
		passwordAgeDays = int(time.Since(user.PasswordChangedAt.Time).Hours() / 24)
		if passwordAgeDays >= 150 {
			changeSuggested = true
		}
	}

	return &payload.AuthResponse{
		Token: payload.RefreshTokenResponse{
			AccessToken:  tokens.AccessToken,
			RefreshToken: tokens.RefreshToken,
		},
		User: payload.UserResponse{
			ID:              displayUserID,
			Email:           user.Email,
			DisplayName:     user.DisplayUserName.String,
			Role:            user.Role,
			PasswordAgeDays: passwordAgeDays,
			ChangeSuggested: changeSuggested,
		},
	}, nil
}

func (s *AuthService) GenerateTokens(ctx context.Context, userID int64, encodedUserID, email, displayName, role, deviceType, deviceName, ipAddress, country, city, userAgent string) (*Tokens, error) {

	refreshToken, err := s.generateRefreshToken()
	if err != nil {
		s.log.Error("failed to generate refresh token", logger.Error(err))
		return nil, apperror.ErrInternal
	}

	refreshTokenHash := s.hashToken(refreshToken)

	// Calculate session expiry from config
	expiresAt := time.Now().Add(s.cfg.RefreshTokenExpiry)

	session, err := s.queries.CreateSession(ctx, gen.CreateSessionParams{
		UserID:           userID,
		RefreshTokenHash: refreshTokenHash,
		DeviceType:       utils.NullString(deviceType),
		DeviceName:       utils.NullString(deviceName),
		IpAddress:        utils.NullIP(ipAddress),
		UserAgent:        utils.NullString(userAgent),
		Country:          utils.NullString(country),
		City:             utils.NullString(city),
		ExpiresAt:        sql.NullTime{Time: expiresAt, Valid: true},
	})
	if err != nil {
		s.log.Error("failed to create session", logger.Error(err), logger.Int64("userID", userID))
		return nil, apperror.ErrInternal
	}

	// Regenerate access token with session ID embedded
	accessToken, err := s.generateAccessTokenWithSession(encodedUserID, email, displayName, role, session.ID, session.LastActiveAt.Time.Unix())
	if err != nil {
		s.log.Error("failed to generate access token with session ID", logger.Error(err))
		return nil, apperror.ErrInternal
	}

	// Eagerly populate session cache with minimal session data needed for
	// auth decisions. DB is fallback on cache miss.
	TTL := s.cfg.RefreshTokenExpiry
	sessionCacheKey := fmt.Sprintf("%s%d", cacheKeySession, session.ID)
	_ = s.cache.HSet(ctx, sessionCacheKey, map[string]any{
		"id":             session.ID,
		"user_id":        session.UserID,
		"session_status": session.SessionStatus.Int16,
		"last_active_at": session.LastActiveAt.Time.Unix(),
		"expires_at":     session.ExpiresAt.Time.Unix(),
		"refresh_token":  session.RefreshTokenHash,
		"role":           role,
	}, cache.WithExpiration(TTL))

	s.log.Info("tokens generated", logger.Int64("userID", userID), logger.Int64("sessionID", session.ID))

	return &Tokens{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) generateAccessToken(encodedUserID, email, displayName, role string) (string, error) {
	return s.generateAccessTokenWithSession(encodedUserID, email, displayName, role, 0, 0)
}

func (s *AuthService) generateAccessTokenWithSession(encodedUserID, email, displayName, role string, sessionID int64, sessionVersion int64) (string, error) {
	claims := Claims{
		UserID:         encodedUserID,
		Email:          email,
		DisplayName:    displayName,
		Role:           role,
		SessionID:      sessionID,
		SessionVersion: sessionVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.cfg.AccessTokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   encodedUserID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.cfg.JWTSecretKey))
}

func (s *AuthService) generateRefreshToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func (s *AuthService) hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// passwordReusesPrevious checks whether newPassword matches any of the user's
// recent retained password hashes (up to cfg.PasswordReuseLimit). It returns
// true if a match is found, enforcing the "last N passwords" reuse policy.
func (s *AuthService) passwordReusesPrevious(ctx context.Context, userID int64, newPassword string) (bool, error) {
	limit := s.cfg.PasswordReuseLimit
	if limit <= 0 {
		limit = 5
	}
	hashes, err := s.queries.ListPasswordHistory(ctx, gen.ListPasswordHistoryParams{
		UserID: userID,
		Limit:  int32(limit),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		s.log.Error("failed to load password history", logger.Error(err), logger.Int64("userID", userID))
		return false, apperror.ErrInternal
	}
	for _, h := range hashes {
		if bcrypt.CompareHashAndPassword([]byte(h), []byte(newPassword)) == nil {
			return true, nil
		}
	}
	return false, nil
}

// enforcePasswordHistoryLimit trims the user's password history to keep at
// most cfg.PasswordReuseLimit most recent records. Called after a new password
// is stored so the table never grows unbounded.
func (s *AuthService) enforcePasswordHistoryLimit(ctx context.Context, q gen.Querier, userID int64) error {
	limit := s.cfg.PasswordReuseLimit
	if limit <= 0 {
		limit = 5
	}
	if err := q.DeletePasswordHistoryOver(ctx, gen.DeletePasswordHistoryOverParams{
		UserID: userID,
		Limit:  int32(limit),
	}); err != nil {
		s.log.Error("failed to trim password history", logger.Error(err), logger.Int64("userID", userID))
		return apperror.ErrInternal
	}
	return nil
}

func (s *AuthService) ValidateAccessToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecretKey), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, apperror.ErrInvalidToken
}

// ValidateAccessTokenAllowExpired parses a JWT, validates the signature, but
// skips the expiry claim check. Used by the middleware for the refresh endpoint
// so that a client with an expired access token can still hit /auth/refresh
// and the sessionID/userID from the (expired but valid-signature) token are
// placed into the request context.
func (s *AuthService) ValidateAccessTokenAllowExpired(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.cfg.JWTSecretKey), nil
	}, jwt.WithoutClaimsValidation())

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok {
		return claims, nil
	}

	return nil, apperror.ErrInvalidToken
}

// ValidateSession checks whether a session is still alive (not revoked and
// not expired). It checks cache first, then falls back to the database.
func (s *AuthService) ValidateSession(ctx context.Context, sessionID int64) (bool, error) {

	sessionCacheKey := fmt.Sprintf("%s%d", cacheKeySession, sessionID)

	// Cache hit — single HMGet for all needed fields
	cached, err := s.cache.HMGet(ctx, sessionCacheKey, "session_status", "expires_at")
	if err == nil && len(cached) > 0 {
		if cached["session_status"] != "1" {
			s.log.Warn("session revoked", logger.Int64("sessionID", sessionID))
			return false, nil
		}

		if expiresAtStr, ok := cached["expires_at"]; ok {

			expiresAt, _ := strconv.ParseInt(expiresAtStr, 10, 64)

			if time.Now().After(time.Unix(expiresAt, 0)) {

				s.log.Warn("session expired", logger.Int64("sessionID", sessionID))
				_ = s.queries.ExpireSession(ctx, sessionID)
				return false, nil
			}
		}

		return true, nil
	}

	// Cache miss — query DB
	session, err := s.queries.GetSessionByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("session not found", logger.Int64("sessionID", sessionID))
			return false, nil
		}
		s.log.Error("failed to validate session", logger.Error(err), logger.Int64("sessionID", sessionID))
		return false, err
	}

	if session.SessionStatus.Int16 != 1 {
		s.log.Warn("session revoked", logger.Int64("sessionID", sessionID))
		return false, nil
	}

	if session.ExpiresAt.Valid && time.Now().After(session.ExpiresAt.Time) {
		s.log.Warn("session expired", logger.Int64("sessionID", sessionID))
		_ = s.queries.ExpireSession(ctx, sessionID)
		return false, nil
	}

	// Populate cache for next time
	_ = s.cache.HSet(ctx, sessionCacheKey, map[string]any{
		"id":             session.ID,
		"user_id":        session.UserID,
		"refresh_token":  session.RefreshTokenHash,
		"last_active_at": session.LastActiveAt.Time.Unix(),
		"session_status": session.SessionStatus.Int16,
		"expires_at":     session.ExpiresAt.Time.Unix(),
	}, cache.WithExpiration(s.cfg.RefreshTokenExpiry))

	return true, nil
}

// ValidateSessionWithVersion checks session alive + version match in a
// single HMGET round-trip, replacing the separate ValidateSession +
// GetSessionVersion calls the middleware used to make.
func (s *AuthService) ValidateSessionWithVersion(ctx context.Context, sessionID int64, tokenVersion int64) (bool, error) {

	sessionCacheKey := fmt.Sprintf("%s%d", cacheKeySession, sessionID)

	// Single HMGet for all three fields in one TCP call.
	cached, err := s.cache.HMGet(ctx, sessionCacheKey, "session_status", "expires_at", "last_active_at")
	if err == nil && len(cached) > 0 {

		if cached["session_status"] != "1" {
			s.log.Warn("session revoked", logger.Int64("sessionID", sessionID))
			return false, nil
		}

		if expiresAtStr, ok := cached["expires_at"]; ok {

			expiresAt, _ := strconv.ParseInt(expiresAtStr, 10, 64)
			if time.Now().After(time.Unix(expiresAt, 0)) {

				s.log.Warn("session expired", logger.Int64("sessionID", sessionID))
				_ = s.queries.ExpireSession(ctx, sessionID)
				return false, nil
			}
		}

		if v, ok := cached["last_active_at"]; ok && tokenVersion > 0 {

			dbVersion, _ := strconv.ParseInt(v, 10, 64)
			if dbVersion != tokenVersion {
				s.log.Warn("session version mismatch — stale token", logger.Int64("sessionID", sessionID),
					logger.Int64("tokenVersion", tokenVersion), logger.Int64("dbVersion", dbVersion))
				return false, nil
			}
		}

		return true, nil
	}

	// Cache miss — query DB
	session, err := s.queries.GetSessionByID(ctx, sessionID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("session not found", logger.Int64("sessionID", sessionID))
			return false, nil
		}
		s.log.Error("failed to validate session", logger.Error(err), logger.Int64("sessionID", sessionID))
		return false, err
	}

	if session.SessionStatus.Int16 != 1 {
		s.log.Warn("session revoked", logger.Int64("sessionID", sessionID))
		return false, nil
	}

	if session.ExpiresAt.Valid && time.Now().After(session.ExpiresAt.Time) {
		s.log.Warn("session expired", logger.Int64("sessionID", sessionID))
		_ = s.queries.ExpireSession(ctx, sessionID)
		return false, nil
	}

	if tokenVersion > 0 && session.LastActiveAt.Valid {
		dbVersion := session.LastActiveAt.Time.Unix()
		if dbVersion != tokenVersion {
			s.log.Warn("session version mismatch — stale token", logger.Int64("sessionID", sessionID),
				logger.Int64("tokenVersion", tokenVersion), logger.Int64("dbVersion", dbVersion))
			return false, nil
		}
	}

	// Populate cache for next time
	_ = s.cache.HSet(ctx, sessionCacheKey, map[string]any{
		"id":             session.ID,
		"user_id":        session.UserID,
		"refresh_token":  session.RefreshTokenHash,
		"last_active_at": session.LastActiveAt.Time.Unix(),
		"session_status": session.SessionStatus.Int16,
		"expires_at":     session.ExpiresAt.Time.Unix(),
	}, cache.WithExpiration(s.cfg.RefreshTokenExpiry))

	return true, nil
}

// DecodeUserID decodes an HMAC-encoded display user ID (e.g. "USR_abc123")
// into the internal integer user ID.
func (s *AuthService) DecodeUserID(encodedUserID string) (int64, error) {
	return utils.DecodeID(encodedUserID, utils.UserIDPrefix, s.cfg.UserIDSecretKey)
}

// GetSessionVersion returns the last_active_at timestamp for a session.
// The middleware uses this to compare against the session_version claim
// in the access token. A mismatch means the token was issued before the
// most recent refresh and must be rejected.
func (s *AuthService) GetSessionVersion(ctx context.Context, sessionID int64) (int64, error) {

	sessionCacheKey := fmt.Sprintf("%s%d", cacheKeySession, sessionID)

	// Cache hit — single HMGet for last_active_at
	cached, err := s.cache.HMGet(ctx, sessionCacheKey, "last_active_at")
	if err == nil && len(cached) > 0 {
		if v, ok := cached["last_active_at"]; ok {
			ts, _ := strconv.ParseInt(v, 10, 64)
			return ts, nil
		}
	}

	// Cache miss — query DB
	session, err := s.queries.GetSessionByID(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	if !session.LastActiveAt.Valid {
		return 0, nil
	}

	// Populate cache for next time
	TTL := s.cfg.RefreshTokenExpiry
	_ = s.cache.HSet(ctx, sessionCacheKey, map[string]any{
		"id":             session.ID,
		"user_id":        session.UserID,
		"refresh_token":  session.RefreshTokenHash,
		"last_active_at": session.LastActiveAt.Time.Unix(),
		"session_status": session.SessionStatus.Int16,
		"expires_at":     session.ExpiresAt.Time.Unix(),
	}, cache.WithExpiration(TTL))

	return session.LastActiveAt.Time.Unix(), nil
}

// getSession looks up a session by refresh token hash.
// DB lookup to resolve hash → session, then uses HGetAll on the session cache.
func (s *AuthService) getSession(ctx context.Context, refreshTokenHash string) (gen.Session, error) {

	session, err := s.queries.GetSessionByRefreshTokenHash(ctx, refreshTokenHash)
	if err != nil {
		return gen.Session{}, err
	}

	sessionCacheKey := fmt.Sprintf("%s%d", cacheKeySession, session.ID)

	// Cache miss — populate cache for next time.
	TTL := s.cfg.RefreshTokenExpiry
	_ = s.cache.HSet(ctx, sessionCacheKey, map[string]any{
		"id":             session.ID,
		"user_id":        session.UserID,
		"session_status": session.SessionStatus.Int16,
		"last_active_at": session.LastActiveAt.Time.Unix(),
		"expires_at":     session.ExpiresAt.Time.Unix(),
		"refresh_token":  session.RefreshTokenHash,
	}, cache.WithExpiration(TTL))

	return session, nil
}

// RefreshAccessToken validates the existing refresh token and issues a new
// access token. The refresh token is NOT rotated — it stays valid for its
// full 7-day lifetime. The old access token is invalidated by updating the
// session's last_active_at, which changes the session_version embedded in
// the JWT. The middleware rejects the old token because its version no
// longer matches the database.
func (s *AuthService) RefreshAccessToken(ctx context.Context, refreshToken string, sessionID int64) (*payload.RefreshTokenResponse, error) {

	var session gen.Session
	var err error

	// When the middleware placed a sessionID in context (from a valid-signature
	// access token, even if expired), the handler passes it here — try cache
	// first for a fast path. On miss, fall back to the refresh token hash
	// lookup which is the authoritative source.
	if sessionID > 0 {
		sessionCacheKey := fmt.Sprintf("%s%d", cacheKeySession, sessionID)

		// Cache hit — single HGetAll in one TCP round-trip.
		cached, cacheErr := s.cache.HGetAll(ctx, sessionCacheKey)
		if cacheErr == nil && len(cached) > 0 {
			var id, userID int64
			var status int16
			_, _ = fmt.Sscanf(cached["id"], "%d", &id)
			_, _ = fmt.Sscanf(cached["user_id"], "%d", &userID)
			_, _ = fmt.Sscanf(cached["session_status"], "%d", &status)

			session = gen.Session{
				ID:            id,
				UserID:        userID,
				SessionStatus: sql.NullInt16{Int16: status, Valid: true},
			}
		}
	}

	// Cache miss (or no sessionID) — always resolve by refresh token hash.
	if session.ID == 0 {

		refreshTokenHash := s.hashToken(refreshToken)

		session, err = s.getSession(ctx, refreshTokenHash)
		if err != nil {
			s.log.Warn("refresh failed: session lookup error", logger.Error(err))
			return nil, apperror.ErrInvalidRefreshToken
		}
	}

	if session.SessionStatus.Int16 != 1 {
		s.log.Warn("refresh failed: session revoked", logger.Int64("sessionID", session.ID))
		return nil, apperror.ErrSessionRevoked
	}

	// Check if session has expired via expires_at
	if session.ExpiresAt.Valid && time.Now().After(session.ExpiresAt.Time) {
		s.log.Warn("refresh failed: session expired", logger.Int64("sessionID", session.ID))
		_ = s.queries.ExpireSession(ctx, session.ID)
		_ = s.cache.Del(ctx, fmt.Sprintf("%s%d", cacheKeySession, session.ID))
		return nil, apperror.ErrSessionExpired
	}

	// Update last_active_at — this bumps the session version so the
	// old access token is immediately rejected by the middleware.
	err = s.queries.UpdateSessionLastActive(ctx, session.ID)
	if err != nil {
		s.log.Error("failed to update session last active", logger.Error(err), logger.Int64("sessionID", session.ID))
		return nil, apperror.ErrInternal
	}

	// Fetch user to get display ID, email, and display name for the new token
	user, err := s.queries.GetUserByID(ctx, session.UserID)
	if err != nil {
		s.log.Error("failed to get user for refresh", logger.Error(err), logger.Int64("userID", session.UserID))
		return nil, apperror.ErrInternal
	}

	encodedUserID := ""
	if user.DisplayUserID.Valid {
		encodedUserID = user.DisplayUserID.String
	}

	displayName := ""
	if user.DisplayUserName.Valid {
		displayName = user.DisplayUserName.String
	}

	// Read back the updated last_active_at so the new access token's
	// version matches the DB exactly (avoids clock precision issues).
	updatedSession, err := s.queries.GetSessionByID(ctx, session.ID)
	if err != nil {
		s.log.Error("failed to read updated session for version", logger.Error(err), logger.Int64("sessionID", session.ID))
		return nil, apperror.ErrInternal
	}

	// Generate only a new access token — same refresh token, same session
	accessToken, err := s.generateAccessTokenWithSession(encodedUserID, user.Email, displayName, user.Role, session.ID, updatedSession.LastActiveAt.Time.Unix())
	if err != nil {
		s.log.Error("failed to generate access token", logger.Error(err))
		return nil, apperror.ErrInternal
	}

	// Sync cache so the next middleware check sees the updated version.
	sessionCacheKey := fmt.Sprintf("%s%d", cacheKeySession, session.ID)
	_ = s.cache.HSet(ctx, sessionCacheKey, map[string]any{
		"last_active_at": updatedSession.LastActiveAt.Time.Unix(),
		"role":           user.Role,
	}, cache.WithExpiration(s.cfg.RefreshTokenExpiry))

	s.log.Info("token refreshed", logger.Int64("userID", session.UserID), logger.Int64("sessionID", session.ID))

	return &payload.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) RevokeSession(ctx context.Context, sessionID, userID int64) error {
	// Fetch the session to get cache keys before revoking
	session, err := s.queries.GetSessionByID(ctx, sessionID)
	if err != nil {
		s.log.Error("revokeSession: session not found", logger.Error(err), logger.Int64("sessionID", sessionID))
		return apperror.ErrInternal
	}
	if session.UserID != userID {
		s.log.Warn("revokeSession: session not owned by user", logger.Int64("sessionID", sessionID), logger.Int64("userID", userID))
		return apperror.ErrNotFound
	}

	// Delete refresh token cache

	// Delete session validation cache
	sessionCacheKey := fmt.Sprintf("%s%d", cacheKeySession, sessionID)
	_ = s.cache.Del(ctx, sessionCacheKey)

	// Revoke in DB
	return s.queries.RevokeSession(ctx, gen.RevokeSessionParams{
		ID:     sessionID,
		UserID: userID,
	})
}

func (s *AuthService) ListSessions(ctx context.Context, userID int64) ([]payload.SessionResponse, error) {
	sessions, err := s.queries.ListSessionsByUser(ctx, userID)
	if err != nil {
		s.log.Error("failed to list sessions", logger.Error(err), logger.Int64("userID", userID))
		return nil, apperror.ErrInternal
	}

	return s.buildSessionResponses(sessions), nil
}

func (s *AuthService) buildSessionResponses(sessions []gen.Session) []payload.SessionResponse {
	resp := make([]payload.SessionResponse, len(sessions))
	for i, sess := range sessions {
		resp[i] = payload.SessionResponse{
			ID:           sess.ID,
			DeviceType:   sess.DeviceType.String,
			DeviceName:   sess.DeviceName.String,
			Country:      sess.Country.String,
			City:         sess.City.String,
			LoggedInAt:   sess.LoggedInAt.Time.Format(time.RFC3339),
			LastActiveAt: sess.LastActiveAt.Time.Format(time.RFC3339),
		}
		if sess.IpAddress.Valid {
			resp[i].IPAddress = sess.IpAddress.IPNet.IP.String()
		}
		if sess.ExpiresAt.Valid {
			resp[i].ExpiresAt = sess.ExpiresAt.Time.Format(time.RFC3339)
		}
	}
	return resp
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string, sessionID int64) (*payload.RefreshTokenResponse, error) {
	return s.RefreshAccessToken(ctx, refreshToken, sessionID)
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string, userID, sessionID int64) error {

	// Remove session cache — stops the middleware from accepting the old access token.
	_ = s.cache.Del(ctx, fmt.Sprintf("%s%d", cacheKeySession, sessionID))

	err := s.queries.RevokeSession(ctx, gen.RevokeSessionParams{
		ID:     sessionID,
		UserID: userID,
	})
	if err != nil {
		s.log.Error("failed to revoke session on logout", logger.Error(err), logger.Int64("sessionID", sessionID))
		return apperror.ErrInternal
	}

	s.log.Info("user logged out", logger.Int64("userID", userID), logger.Int64("sessionID", sessionID))
	return nil
}

// ForgotPassword updates the user's password without requiring the current
// password. On success, all existing sessions for the user are revoked,
// forcing a re-login with the new password. Invalid credentials return a
// generic error to prevent account enumeration.
func (s *AuthService) ForgotPassword(ctx context.Context, req payload.ForgotPasswordRequest, ipAddress, userAgent string) error {
	if err := utils.ValidateEmail(req.Email); err != nil {
		return err
	}

	if err := utils.ValidatePassword(req.NewPassword); err != nil {
		return err
	}

	user, err := s.queries.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("forgot password: user not found", logger.String("email", req.Email))
			return apperror.ErrUnauthorized
		}
		s.log.Error("forgot password: failed to get user", logger.Error(err), logger.String("email", req.Email))
		return apperror.ErrInternal
	}

	reused, err := s.passwordReusesPrevious(ctx, user.ID, req.NewPassword)
	if err != nil {
		return err
	}
	if reused {
		s.log.Warn("forgot password: new password matches a previous password", logger.Int64("userID", user.ID))
		return apperror.ErrPasswordReuse
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.log.Error("forgot password: failed to hash new password", logger.Error(err))
		return apperror.ErrInternal
	}

	err = s.withAuthTx(ctx, func(q gen.Querier) error {
		if _, txErr := q.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{
			ID:           user.ID,
			PasswordHash: string(newHash),
		}); txErr != nil {
			return txErr
		}
		if txErr := q.AddPasswordHistory(ctx, gen.AddPasswordHistoryParams{
			UserID:       user.ID,
			PasswordHash: string(newHash),
			IpAddress:    utils.NullIP(ipAddress),
			UserAgent:    utils.NullString(userAgent),
		}); txErr != nil {
			return txErr
		}
		return s.enforcePasswordHistoryLimit(ctx, q, user.ID)
	})
	if err != nil {
		s.log.Error("forgot password: transaction failed", logger.Error(err), logger.Int64("userID", user.ID))
		return apperror.ErrInternal
	}

	// Revoke all existing sessions — user must re-login with new password.
	sessions, listErr := s.queries.ListActiveSessionsByUser(ctx, user.ID)
	if listErr == nil {
		for _, sess := range sessions {
			_ = s.cache.Del(ctx, fmt.Sprintf("%s%d", cacheKeySession, sess.ID))
			_ = s.queries.RevokeSession(ctx, gen.RevokeSessionParams{ID: sess.ID, UserID: user.ID})
		}
	}

	s.log.Info("forgot password: success", logger.Int64("userID", user.ID))
	return nil
}

// ChangePassword validates the user's current password and updates it.
// On success, ALL sessions including the current one are revoked, forcing
// a re-login with the new password.
func (s *AuthService) ChangePassword(ctx context.Context, userID int64, req payload.ChangePasswordRequest, sessionID int64, ipAddress, userAgent string) error {
	if err := utils.ValidatePassword(req.NewPassword); err != nil {
		return err
	}

	user, err := s.queries.GetUserByID(ctx, userID)
	if err != nil {
		s.log.Error("update password: failed to get user", logger.Error(err), logger.Int64("userID", userID))
		return apperror.ErrInternal
	}

	lastHash, err := s.queries.ListPasswordHistory(ctx, gen.ListPasswordHistoryParams{
		UserID: user.ID,
		Limit:  1,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("update password: no password history", logger.Int64("userID", user.ID))
			return apperror.ErrUnauthorized
		}
		s.log.Error("update password: failed to get password history", logger.Error(err), logger.Int64("userID", user.ID))
		return apperror.ErrInternal
	}
	if len(lastHash) == 0 {
		return apperror.ErrUnauthorized
	}

	if err := bcrypt.CompareHashAndPassword([]byte(lastHash[0]), []byte(req.CurrentPassword)); err != nil {
		s.log.Warn("update password: invalid current password", logger.Int64("userID", user.ID))
		return apperror.ErrInvalidCurrentPassword
	}

	reused, err := s.passwordReusesPrevious(ctx, user.ID, req.NewPassword)
	if err != nil {
		return err
	}
	if reused {
		s.log.Warn("update password: new password matches a previous password", logger.Int64("userID", user.ID))
		return apperror.ErrPasswordReuse
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.log.Error("update password: failed to hash new password", logger.Error(err))
		return apperror.ErrInternal
	}

	err = s.withAuthTx(ctx, func(q gen.Querier) error {
		if _, txErr := q.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{
			ID:           user.ID,
			PasswordHash: string(newHash),
		}); txErr != nil {
			return txErr
		}
		if txErr := q.AddPasswordHistory(ctx, gen.AddPasswordHistoryParams{
			UserID:       user.ID,
			PasswordHash: string(newHash),
			IpAddress:    utils.NullIP(ipAddress),
			UserAgent:    utils.NullString(userAgent),
		}); txErr != nil {
			return txErr
		}
		return s.enforcePasswordHistoryLimit(ctx, q, user.ID)
	})
	if err != nil {
		s.log.Error("update password: transaction failed", logger.Error(err), logger.Int64("userID", user.ID))
		return apperror.ErrInternal
	}

	// Revoke ALL sessions including the current one — must re-login.
	sessions, listErr := s.queries.ListActiveSessionsByUser(ctx, user.ID)
	if listErr == nil {
		for _, sess := range sessions {
			_ = s.cache.Del(ctx, fmt.Sprintf("%s%d", cacheKeySession, sess.ID))
			_ = s.queries.RevokeSession(ctx, gen.RevokeSessionParams{ID: sess.ID, UserID: user.ID})
		}
	}

	s.log.Info("update password: success", logger.Int64("userID", user.ID), logger.Int64("sessionID", sessionID))
	return nil
}

// RevokeOtherDevices revokes all active sessions for the user except the
// specified session. The status of each revoked session is set to revoked (0).
func (s *AuthService) RevokeOtherDevices(ctx context.Context, userID, currentSessionID int64) error {
	// Fetch all active sessions for the user (excluding current)
	sessions, err := s.queries.ListActiveSessionsByUser(ctx, userID)
	if err != nil {
		s.log.Error("revokeOtherDevices: failed to list sessions", logger.Error(err), logger.Int64("userID", userID))
		return apperror.ErrInternal
	}

	for _, sess := range sessions {
		if sess.ID == currentSessionID {
			continue
		}
		// Remove session cache
		_ = s.cache.Del(ctx, fmt.Sprintf("%s%d", cacheKeySession, sess.ID))
	}

	// Bulk revoke all sessions except current (both active and expired)
	if err := s.queries.RevokeSessionsByUserExcept(ctx, gen.RevokeSessionsByUserExceptParams{
		UserID: userID,
		ID:     currentSessionID,
	}); err != nil {
		s.log.Error("revokeOtherDevices: failed to revoke sessions", logger.Error(err), logger.Int64("userID", userID))
		return apperror.ErrInternal
	}

	s.log.Info("revokeOtherDevices: all other sessions revoked", logger.Int64("userID", userID), logger.Int64("exceptSessionID", currentSessionID))
	return nil
}

// RevokeAllSessions revokes every active session for the user, including
// the specified current session. The user must re-login on all devices.
func (s *AuthService) RevokeAllSessions(ctx context.Context, userID int64) error {
	// Fetch all active sessions for cache cleanup
	sessions, err := s.queries.ListActiveSessionsByUser(ctx, userID)
	if err != nil {
		s.log.Error("revokeAllSessions: failed to list sessions", logger.Error(err), logger.Int64("userID", userID))
		return apperror.ErrInternal
	}

	for _, sess := range sessions {
		// Remove session cache
		_ = s.cache.Del(ctx, fmt.Sprintf("%s%d", cacheKeySession, sess.ID))
	}

	// Bulk revoke all sessions for the user (both active and expired)
	if err := s.queries.RevokeAllSessionsByUser(ctx, userID); err != nil {
		s.log.Error("revokeAllSessions: failed to revoke all sessions", logger.Error(err), logger.Int64("userID", userID))
		return apperror.ErrInternal
	}

	s.log.Info("revokeAllSessions: all sessions revoked", logger.Int64("userID", userID))
	return nil
}

// checkLoginRateLimit checks if the user has exceeded max failed login attempts.
// Returns (blocked, error) where blocked=true means user is locked out.
func (s *AuthService) checkLoginRateLimit(ctx context.Context, email string) (bool, error) {
	key := cacheKeyRateLimit + email

	lockedUntilStr, err := s.cache.HGet(ctx, key, "locked_until")
	if err != nil {
		return false, nil
	}

	lockedUntil, _ := strconv.ParseInt(lockedUntilStr, 10, 64)
	if time.Now().Unix() < lockedUntil {
		return true, nil
	}

	return false, nil
}

// recordFailedLogin increments the failed login counter for an email.
// Locks the account for 30 minutes after 3 failed attempts.
func (s *AuthService) recordFailedLogin(ctx context.Context, email string) {
	key := cacheKeyRateLimit + email

	attemptsStr, _ := s.cache.HGet(ctx, key, "attempts")
	attempts, _ := strconv.Atoi(attemptsStr)

	attempts++

	var lockedUntil int64
	if attempts >= 3 {
		lockedUntil = time.Now().Add(30 * time.Minute).Unix()
	}

	_ = s.cache.HSet(ctx, key, map[string]any{
		"attempts":     attempts,
		"locked_until": lockedUntil,
	}, cache.WithExpiration(30*time.Minute))
}

// clearFailedLogins removes the failed login counter for an email.
func (s *AuthService) clearFailedLogins(ctx context.Context, email string) {
	key := cacheKeyRateLimit + email
	_ = s.cache.HDel(ctx, key, "attempts", "locked_until")
}
