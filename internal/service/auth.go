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
	HGet(key, field string) (string, error)
	HSet(key, field string, value any) error
	HSetFields(key string, fields map[string]any) error
	HSetWithTTL(key, field string, value any, ttl time.Duration) error
	HDel(key string, fields ...string) error
}

// NoopCache is a SessionCache implementation that always returns cache-miss.
// Used in tests and as a fallback when cache is unavailable.
type NoopCache struct{}

func (NoopCache) HGet(string, string) (string, error)                  { return "", fmt.Errorf("noop") }
func (NoopCache) HSet(string, string, any) error                       { return nil }
func (NoopCache) HSetFields(string, map[string]any) error              { return nil }
func (NoopCache) HSetWithTTL(string, string, any, time.Duration) error { return nil }
func (NoopCache) HDel(string, ...string) error                         { return nil }

// Cache key prefixes — every Redis key in the application must use one of these.
const (
	cacheKeyRefresh   = "refresh:"   // refresh token session cache (keyed by token hash)
	cacheKeySession   = "session:"   // session validation cache (keyed by session ID)
	cacheKeyRateLimit = "ratelimit:" // login rate-limit counter (keyed by email)
)

// errInvalidCredentials is returned when email/password verification fails.
// It wraps ErrUnauthorized so it maps to HTTP 401, and uses one generic
// message for both unknown emails and wrong passwords to prevent
// account enumeration.
var errInvalidCredentials = fmt.Errorf("%w: invalid email or password", apperror.ErrUnauthorized)

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
		return fmt.Errorf("%w: could not start transaction", apperror.ErrInternal)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(gen.New(tx)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		s.log.Error("failed to commit transaction", logger.Error(err))
		return fmt.Errorf("%w: could not commit transaction", apperror.ErrInternal)
	}
	return nil
}

type Tokens struct {
	AccessToken  string
	RefreshToken string
}

// Claims is the JWT access token payload. It carries the session ID so
// the middleware can verify the session is still alive (not revoked /
// expired) on every request.
type Claims struct {
	UserID      string `json:"user_id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	SessionID   int64  `json:"session_id"`
	jwt.RegisteredClaims
}

// Register creates a new user account.
func (s *AuthService) Register(ctx context.Context, req payload.RegisterRequest, deviceType, deviceName, ipAddress, country, city, userAgent string) (*payload.AuthResponse, error) {
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
		return nil, apperror.ErrConflict
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
	tokens, err := s.GenerateTokens(ctx, user.ID, displayUserID, user.Email, req.DisplayName, deviceType, deviceName, ipAddress, country, city, userAgent)
	if err != nil {
		return nil, err
	}

	return &payload.AuthResponse{
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		User: payload.UserResponse{
			ID:          displayUserID,
			Email:       user.Email,
			DisplayName: req.DisplayName,
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
	if blocked, err := s.checkLoginRateLimit(req.Email); err != nil {
		return nil, err
	} else if blocked {
		return nil, errors.New("too many failed login attempts, please try again after 30 minutes")
	}

	user, err := s.queries.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("login attempt with unknown email", logger.String("email", req.Email))
			return nil, errInvalidCredentials
		}
		s.log.Error("failed to get user by email", logger.Error(err), logger.String("email", req.Email))
		return nil, apperror.ErrInternal
	}

	// Verify password
	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password))
	if err != nil {

		// Record failed login attempt
		s.recordFailedLogin(req.Email)

		s.log.Warn("invalid password", logger.String("email", req.Email))
		return nil, errInvalidCredentials
	}

	// Clear failed login attempts on successful login
	s.clearFailedLogins(req.Email)

	displayUserID := ""
	if user.DisplayUserID.Valid {
		displayUserID = user.DisplayUserID.String
	}

	s.log.Info("user logged in", logger.Int64("userID", user.ID), logger.String("email", req.Email))

	// Determine revoke task: user-approved removal or automatic enforcement
	var revokeTask func()
	if req.RevokeSessionID != nil && *req.RevokeSessionID > 0 {
		revokeTask = s.revokeApprovedSession(ctx, user.ID, *req.RevokeSessionID)
	} else {
		var err error
		revokeTask, err = s.enforceMaxDevices(ctx, user.ID)
		if err != nil {
			return nil, err
		}
	}

	// Generate tokens and revoke old session in parallel
	var tokens *Tokens
	var tokensErr error
	done := make(chan struct{})

	go func() {
		defer close(done)
		tokens, tokensErr = s.GenerateTokens(ctx, user.ID, displayUserID, user.Email, user.DisplayUserName.String, deviceType, deviceName, ipAddress, country, city, userAgent)
	}()

	if revokeTask != nil {
		revokeTask()
	}

	<-done

	if tokensErr != nil {
		return nil, tokensErr
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
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		User: payload.UserResponse{
			ID:              displayUserID,
			Email:           user.Email,
			DisplayName:     user.DisplayUserName.String,
			PasswordAgeDays: passwordAgeDays,
			ChangeSuggested: changeSuggested,
		},
	}, nil
}

func (s *AuthService) GenerateTokens(ctx context.Context, userID int64, encodedUserID, email, displayName, deviceType, deviceName, ipAddress, country, city, userAgent string) (*Tokens, error) {

	refreshToken, err := s.generateRefreshToken()
	if err != nil {
		s.log.Error("failed to generate refresh token", logger.Error(err))
		return nil, apperror.ErrInternal
	}

	refreshTokenHash := s.hashToken(refreshToken)

	session, err := s.queries.CreateSession(ctx, gen.CreateSessionParams{
		UserID:           userID,
		RefreshTokenHash: refreshTokenHash,
		DeviceType:       utils.NullString(deviceType),
		DeviceName:       utils.NullString(deviceName),
		IpAddress:        utils.NullIP(ipAddress),
		UserAgent:        utils.NullString(userAgent),
		Country:          utils.NullString(country),
		City:             utils.NullString(city),
	})
	if err != nil {
		s.log.Error("failed to create session", logger.Error(err), logger.Int64("userID", userID))
		return nil, apperror.ErrInternal
	}

	// Regenerate access token with session ID embedded
	accessToken, err := s.generateAccessTokenWithSession(encodedUserID, email, displayName, session.ID)
	if err != nil {
		s.log.Error("failed to generate access token with session ID", logger.Error(err))
		return nil, apperror.ErrInternal
	}

	// Cache the new session so subsequent refresh calls skip the DB (single pipeline, best-effort)

	_ = s.cache.HSetFields(cacheKeyRefresh+refreshTokenHash, map[string]any{
		"id":             session.ID,
		"user_id":        session.UserID,
		"session_status": session.SessionStatus.Int16,
	})

	s.log.Info("tokens generated", logger.Int64("userID", userID), logger.Int64("sessionID", session.ID))

	return &Tokens{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenHash,
	}, nil
}

func (s *AuthService) generateAccessToken(encodedUserID, email, displayName string) (string, error) {
	return s.generateAccessTokenWithSession(encodedUserID, email, displayName, 0)
}

func (s *AuthService) generateAccessTokenWithSession(encodedUserID, email, displayName string, sessionID int64) (string, error) {
	claims := Claims{
		UserID:      encodedUserID,
		Email:       email,
		DisplayName: displayName,
		SessionID:   sessionID,
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

	return nil, errors.New("invalid token")
}

// ValidateSession checks whether a session is still alive (not revoked and
// not expired). It checks cache first, then falls back to the database.
func (s *AuthService) ValidateSession(ctx context.Context, sessionID int64) (bool, error) {
	sessionCacheKey := fmt.Sprintf("%s%d", cacheKeySession, sessionID)

	// Cache hit — check session_status field directly
	status, err := s.cache.HGet(sessionCacheKey, "session_status")
	if err == nil {
		return status == "1", nil
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

	// Populate cache for next time (single pipeline, best-effort)
	_ = s.cache.HSetFields(sessionCacheKey, map[string]any{
		"id":             session.ID,
		"user_id":        session.UserID,
		"session_status": session.SessionStatus.Int16,
	})

	return true, nil
}

// DecodeUserID decodes an HMAC-encoded display user ID (e.g. "USR_abc123")
// into the internal integer user ID.
func (s *AuthService) DecodeUserID(encodedUserID string) (int64, error) {
	return utils.DecodeID(encodedUserID, utils.UserIDPrefix, s.cfg.UserIDSecretKey)
}

func truncateKey(key string, maxLen int) string {
	if len(key) > maxLen {
		return key[:maxLen] + "..."
	}
	return key
}

// getSession looks up a session by refresh token hash, checking cache first.
func (s *AuthService) getSession(ctx context.Context, refreshTokenHash string) (gen.Session, error) {
	cacheKey := cacheKeyRefresh + refreshTokenHash

	// Cache hit — read individual hash fields
	idStr, err := s.cache.HGet(cacheKey, "id")
	if err == nil {
		uidStr, _ := s.cache.HGet(cacheKey, "user_id")
		statusStr, _ := s.cache.HGet(cacheKey, "session_status")

		id, _ := strconv.ParseInt(idStr, 10, 64)
		uid, _ := strconv.ParseInt(uidStr, 10, 64)
		status, _ := strconv.ParseInt(statusStr, 10, 16)

		s.log.Debug("session cache hit", logger.String("key", truncateKey(cacheKey, 16)))
		return gen.Session{
			ID:            id,
			UserID:        uid,
			SessionStatus: sql.NullInt16{Int16: int16(status), Valid: true},
		}, nil
	}

	s.log.Debug("session cache miss", logger.String("key", truncateKey(cacheKey, 16)))

	session, err := s.queries.GetSessionByRefreshTokenHash(ctx, refreshTokenHash)
	if err != nil {
		return gen.Session{}, err
	}

	// Populate cache (single pipeline, best-effort)
	_ = s.cache.HSetFields(cacheKey, map[string]any{
		"id":             session.ID,
		"user_id":        session.UserID,
		"session_status": session.SessionStatus.Int16,
	})

	return session, nil
}

// RefreshAccessToken validates the existing refresh token and issues a new
// access token. The refresh token itself is NOT rotated — it remains valid
// until its natural expiry or explicit revocation.
func (s *AuthService) RefreshAccessToken(ctx context.Context, refreshToken string) (*payload.RefreshTokenResponse, error) {

	refreshTokenHash := s.hashToken(refreshToken)
	session, err := s.getSession(ctx, refreshTokenHash)
	if err != nil {
		s.log.Warn("refresh failed: session lookup error", logger.Error(err))
		return nil, errors.New("invalid or expired refresh token")
	}

	if session.SessionStatus.Int16 != 1 {
		s.log.Warn("refresh failed: session revoked", logger.Int64("sessionID", session.ID))
		return nil, errors.New("session revoked")
	}

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

	// Generate only a new access token — no new session, no new refresh token
	accessToken, err := s.generateAccessTokenWithSession(encodedUserID, user.Email, displayName, session.ID)
	if err != nil {
		s.log.Error("failed to generate access token", logger.Error(err))
		return nil, apperror.ErrInternal
	}

	s.log.Info("token refreshed", logger.Int64("userID", session.UserID), logger.Int64("sessionID", session.ID))

	return &payload.RefreshTokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *AuthService) RevokeSession(ctx context.Context, sessionID, userID int64) error {
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
	}
	return resp
}

func (s *AuthService) buildActiveDevices(sessions []gen.Session) []apperror.ActiveDevice {
	devices := make([]apperror.ActiveDevice, len(sessions))
	for i, sess := range sessions {
		devices[i] = apperror.ActiveDevice{
			ID:           sess.ID,
			DeviceType:   sess.DeviceType.String,
			DeviceName:   sess.DeviceName.String,
			LoggedInAt:   sess.LoggedInAt.Time.Format(time.RFC3339),
			LastActiveAt: sess.LastActiveAt.Time.Format(time.RFC3339),
		}
		if sess.IpAddress.Valid {
			devices[i].IPAddress = sess.IpAddress.IPNet.IP.String()
		}
	}
	return devices
}

func (s *AuthService) RefreshToken(ctx context.Context, refreshToken string) (*payload.RefreshTokenResponse, error) {
	return s.RefreshAccessToken(ctx, refreshToken)
}

func (s *AuthService) Logout(ctx context.Context, refreshToken string) error {

	// Remove from cache
	_ = s.cache.HDel(cacheKeyRefresh+refreshToken, "id", "user_id", "session_status")

	session, err := s.queries.GetSessionByRefreshTokenHash(ctx, refreshToken)
	if err != nil {
		s.log.Warn("logout failed: session not found", logger.Error(err))
		return errors.New("invalid refresh token")
	}

	s.log.Info("user logged out", logger.Int64("userID", session.UserID), logger.Int64("sessionID", session.ID))

	err = s.queries.RevokeSession(ctx, gen.RevokeSessionParams{
		ID:     session.ID,
		UserID: session.UserID,
	})
	if err != nil {
		s.log.Error("failed to revoke session on logout", logger.Error(err), logger.Int64("sessionID", session.ID))
		return apperror.ErrInternal
	}
	return nil
}

// ForgotPassword validates the user's previous password and updates it.
// On success, all existing sessions for the user are revoked, forcing a
// re-login with the new password. Invalid credentials return a generic
// error to prevent account enumeration.
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
			return errInvalidCredentials
		}
		s.log.Error("forgot password: failed to get user", logger.Error(err), logger.String("email", req.Email))
		return apperror.ErrInternal
	}

	lastHash, err := s.queries.GetLastPasswordHistory(ctx, user.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.log.Warn("forgot password: no password history", logger.Int64("userID", user.ID))
			return errInvalidCredentials
		}
		s.log.Error("forgot password: failed to get password history", logger.Error(err), logger.Int64("userID", user.ID))
		return apperror.ErrInternal
	}

	if err := bcrypt.CompareHashAndPassword([]byte(lastHash), []byte(req.CurrentPassword)); err != nil {
		s.log.Warn("forgot password: invalid current password", logger.Int64("userID", user.ID))
		return errors.New("invalid current password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(lastHash), []byte(req.NewPassword)); err == nil {
		s.log.Warn("forgot password: new password same as current", logger.Int64("userID", user.ID))
		return errors.New("new password cannot be the same as current password")
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		s.log.Error("forgot password: failed to hash new password", logger.Error(err))
		return apperror.ErrInternal
	}
	newHashStr := string(newHash)

	if newHashStr == lastHash {
		s.log.Warn("forgot password: new hash equals old hash", logger.Int64("userID", user.ID))
		return fmt.Errorf("new password cannot be the same as current password")
	}

	err = s.withAuthTx(ctx, func(q gen.Querier) error {
		if _, txErr := q.UpdateUserPassword(ctx, gen.UpdateUserPasswordParams{
			ID:           user.ID,
			PasswordHash: newHashStr,
		}); txErr != nil {
			return txErr
		}
		return q.AddPasswordHistory(ctx, gen.AddPasswordHistoryParams{
			UserID:       user.ID,
			PasswordHash: newHashStr,
			IpAddress:    utils.NullIP(ipAddress),
			UserAgent:    utils.NullString(userAgent),
		})
	})
	if err != nil {
		s.log.Error("forgot password: transaction failed", logger.Error(err), logger.Int64("userID", user.ID))
		return apperror.ErrInternal
	}

	// Revoke all existing sessions — user must re-login with new password.
	sessions, listErr := s.queries.ListActiveSessionsByUser(ctx, user.ID)
	if listErr == nil {
		for _, sess := range sessions {
			_ = s.cache.HDel(cacheKeyRefresh+sess.RefreshTokenHash, "id", "user_id", "session_status")
			_ = s.queries.RevokeSession(ctx, gen.RevokeSessionParams{ID: sess.ID, UserID: user.ID})
		}
	}

	s.log.Info("forgot password: success", logger.Int64("userID", user.ID))
	return nil
}

const (
	maxDevices        = 2
	maxDeviceIdleTime = 6 * time.Hour
)

// enforceMaxDevices checks whether the user already has maxDevices active
// sessions. Behaviour when at capacity:
//   - If the oldest session has been idle for > maxDeviceIdleTime, a revoke
//     task is returned that can be run in parallel with the sign-in.
//   - Otherwise an *apperror.MaxDeviceError is returned carrying the active
//     session list so the client can prompt the user to choose one to remove.
//
// The returned revokeTask is always safe to call — it logs errors but never
// returns them, so it can be fire-and-forget in a goroutine.
func (s *AuthService) enforceMaxDevices(ctx context.Context, userID int64) (revokeTask func(), err error) {
	sessions, err := s.queries.ListActiveSessionsByUser(ctx, userID)
	if err != nil {
		s.log.Error("enforceMaxDevices: failed to list sessions", logger.Error(err), logger.Int64("userID", userID))
		return nil, apperror.ErrInternal
	}

	if len(sessions) < maxDevices {
		return nil, nil
	}

	// sessions are ordered ASC by last_active_at — oldest first
	oldest := sessions[0]

	if oldest.LastActiveAt.Valid && time.Since(oldest.LastActiveAt.Time) > maxDeviceIdleTime {
		s.log.Info("enforceMaxDevices: oldest session idle > 6h, will auto-revoke in parallel",
			logger.Int64("userID", userID),
			logger.Int64("sessionID", oldest.ID),
			logger.String("idle", time.Since(oldest.LastActiveAt.Time).Round(time.Second).String()),
		)

		revoke := func() {
			_ = s.cache.HDel(cacheKeyRefresh+oldest.RefreshTokenHash, "id", "user_id", "session_status")
			if err := s.queries.RevokeSession(ctx, gen.RevokeSessionParams{
				ID:     oldest.ID,
				UserID: oldest.UserID,
			}); err != nil {
				s.log.Error("enforceMaxDevices: failed to revoke oldest session",
					logger.Error(err),
					logger.Int64("sessionID", oldest.ID),
				)
			}
		}
		return revoke, nil
	}

	s.log.Warn("enforceMaxDevices: max devices reached",
		logger.Int64("userID", userID),
		logger.Int("activeSessions", len(sessions)),
	)
	return nil, &apperror.MaxDeviceError{Devices: s.buildActiveDevices(sessions)}
}

// revokeApprovedSession builds a fire-and-forget task that revokes the session
// the user explicitly approved removing. This is used when the client received
// a 409 MaxDeviceError, showed the device list, and the user picked one —
// allowing login to complete in a single request.
func (s *AuthService) revokeApprovedSession(ctx context.Context, userID, sessionID int64) func() {
	return func() {
		session, err := s.queries.GetSessionByID(ctx, sessionID)
		if err != nil {
			s.log.Warn("revokeApprovedSession: session not found",
				logger.Error(err),
				logger.Int64("sessionID", sessionID),
			)
			return
		}
		if session.UserID != userID {
			s.log.Warn("revokeApprovedSession: session not owned by user",
				logger.Int64("sessionID", sessionID),
				logger.Int64("userID", userID),
			)
			return
		}

		_ = s.cache.HDel(cacheKeyRefresh+session.RefreshTokenHash, "id", "user_id", "session_status")

		if err := s.queries.RevokeSession(ctx, gen.RevokeSessionParams{
			ID:     session.ID,
			UserID: session.UserID,
		}); err != nil {
			s.log.Error("revokeApprovedSession: failed to revoke",
				logger.Error(err),
				logger.Int64("sessionID", session.ID),
			)
		}
	}
}

// checkLoginRateLimit checks if the user has exceeded max failed login attempts.
// Returns (blocked, error) where blocked=true means user is locked out.
func (s *AuthService) checkLoginRateLimit(email string) (bool, error) {
	key := cacheKeyRateLimit + email

	lockedUntilStr, err := s.cache.HGet(key, "locked_until")
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
func (s *AuthService) recordFailedLogin(email string) {
	key := cacheKeyRateLimit + email

	attemptsStr, _ := s.cache.HGet(key, "attempts")
	attempts, _ := strconv.Atoi(attemptsStr)

	attempts++

	var lockedUntil int64
	if attempts >= 3 {
		lockedUntil = time.Now().Add(30 * time.Minute).Unix()
	}

	_ = s.cache.HSetWithTTL(key, "attempts", attempts, 30*time.Minute)
	_ = s.cache.HSetWithTTL(key, "locked_until", lockedUntil, 30*time.Minute)
}

// clearFailedLogins removes the failed login counter for an email.
func (s *AuthService) clearFailedLogins(email string) {
	key := cacheKeyRateLimit + email
	_ = s.cache.HDel(key, "attempts", "locked_until")
}

// --- Test helpers (exported for integration tests) ---

// TestConfig is an alias for config.Config to avoid importing internal/config
// from test packages. Only the fields needed for token generation are required.
type TestConfig = config.Config

// GenerateTestToken creates a signed JWT access token for testing. This is
// exported so integration tests can produce valid tokens without going through
// the full login flow.
func GenerateTestToken(cfg *config.Config, encodedUserID string, sessionID int64) (string, error) {
	svc := &AuthService{cfg: cfg}
	return svc.generateAccessTokenWithSession(encodedUserID, "", "", sessionID)
}
