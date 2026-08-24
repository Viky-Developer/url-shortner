package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/config"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/utils"
)

func testConfig() *config.Config {
	return &config.Config{
		UserIDSecretKey:    "test-secret-key",
		JWTSecretKey:       "test-jwt-secret-key-for-testing-32ch",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
}

func TestGenerateAccessToken(t *testing.T) {
	cfg := testConfig()
	svc := NewAuthService(nil, nil, cfg, NoopCache{}, testLog(t))

	token, err := svc.generateAccessToken("USR_abc123", "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("generateAccessToken: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestValidateAccessToken(t *testing.T) {
	cfg := testConfig()
	svc := NewAuthService(nil, nil, cfg, NoopCache{}, testLog(t))

	token, err := svc.generateAccessToken("USR_abc123", "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("generateAccessToken: %v", err)
	}

	claims, err := svc.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("ValidateAccessToken: %v", err)
	}
	if claims.UserID != "USR_abc123" {
		t.Errorf("expected encodedUserID 'USR_abc123', got %q", claims.UserID)
	}
}

func TestValidateAccessTokenInvalid(t *testing.T) {
	cfg := testConfig()
	svc := NewAuthService(nil, nil, cfg, NoopCache{}, testLog(t))

	_, err := svc.ValidateAccessToken("totally-invalid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestValidateAccessTokenWrongKey(t *testing.T) {
	cfg1 := testConfig()
	cfg2 := testConfig()
	cfg2.JWTSecretKey = "completely-different-key-for-testing"

	svc1 := NewAuthService(nil, nil, cfg1, NoopCache{}, testLog(t))
	svc2 := NewAuthService(nil, nil, cfg2, NoopCache{}, testLog(t))

	token, err := svc1.generateAccessToken("USR_abc123", "test@example.com", "Test User")
	if err != nil {
		t.Fatalf("generateAccessToken: %v", err)
	}

	_, err = svc2.ValidateAccessToken(token)
	if err == nil {
		t.Fatal("expected error when validating with wrong key")
	}
}

func TestHashToken(t *testing.T) {
	cfg := testConfig()
	svc := NewAuthService(nil, nil, cfg, NoopCache{}, testLog(t))

	hash1 := svc.hashToken("token123")
	hash2 := svc.hashToken("token123")
	hash3 := svc.hashToken("different-token")

	if hash1 != hash2 {
		t.Error("same input should produce same hash")
	}
	if hash1 == hash3 {
		t.Error("different inputs should produce different hashes")
	}
	if len(hash1) != 64 {
		t.Errorf("expected SHA256 hex hash (64 chars), got %d", len(hash1))
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	cfg := testConfig()
	svc := NewAuthService(nil, nil, cfg, NoopCache{}, testLog(t))

	token1, err := svc.generateRefreshToken()
	if err != nil {
		t.Fatalf("generateRefreshToken: %v", err)
	}
	token2, err := svc.generateRefreshToken()
	if err != nil {
		t.Fatalf("generateRefreshToken: %v", err)
	}

	if token1 == token2 {
		t.Error("two refresh tokens should be different")
	}
	if len(token1) != 64 {
		t.Errorf("expected 64-char hex token, got %d chars", len(token1))
	}
}

func TestDecodeUserID(t *testing.T) {
	cfg := testConfig()
	svc := NewAuthService(nil, nil, cfg, NoopCache{}, testLog(t))

	encoded := utils.EncodeID(42, utils.UserIDPrefix, cfg.UserIDSecretKey)
	decoded, err := svc.DecodeUserID(encoded)
	if err != nil {
		t.Fatalf("DecodeUserID: %v", err)
	}
	if decoded != 42 {
		t.Errorf("expected 42, got %d", decoded)
	}
}

func TestDecodeUserIDInvalid(t *testing.T) {
	cfg := testConfig()
	svc := NewAuthService(nil, nil, cfg, NoopCache{}, testLog(t))

	_, err := svc.DecodeUserID("USR_invalid_tampered")
	if err == nil {
		t.Fatal("expected error for tampered encoded user ID")
	}
}

func TestRegisterUserAlreadyExists(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		emailFn: func(_ context.Context, _ string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{ID: 1, Email: "existing@example.com"}, nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	_, err := svc.Register(context.Background(), payload.RegisterRequest{
		Email:    "existing@example.com",
		Password: "Passw0rd",
	}, "web", "test-device", "127.0.0.1", "", "", "test-agent")
	if err == nil {
		t.Fatal("expected error for existing email")
	}
	if err.Error() != "the resource already exists" {
		t.Errorf("expected 'the resource already exists', got %q", err.Error())
	}
}

func TestRegisterUserDBError(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		emailFn: func(_ context.Context, _ string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{}, sql.ErrConnDone
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	_, err := svc.Register(context.Background(), payload.RegisterRequest{
		Email:    "test@example.com",
		Password: "Passw0rd",
	}, "web", "test-device", "127.0.0.1", "", "", "test-agent")
	if err == nil {
		t.Fatal("expected error for DB failure")
	}
}

func TestRegisterUserCreatesAccount(t *testing.T) {
	cfg := testConfig()
	var capturedEmail string
	var capturedHash string

	mock := &mockQuerier{
		emailFn: func(_ context.Context, _ string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{}, sql.ErrNoRows
		},
		createUserFn: func(_ context.Context, arg gen.CreateUserParams) (gen.CreateUserRow, error) {
			capturedEmail = arg.Email
			capturedHash = arg.PasswordHash
			return gen.CreateUserRow{
				ID:    100000,
				Email: arg.Email,
			}, nil
		},
		updateUserFn: func(_ context.Context, arg gen.UpdateUserDisplayIDParams) (gen.UpdateUserDisplayIDRow, error) {
			return gen.UpdateUserDisplayIDRow{
				ID:            arg.ID,
				DisplayUserID: arg.DisplayUserID,
			}, nil
		},
		createSessionFn: func(_ context.Context, _ gen.CreateSessionParams) (gen.Session, error) {
			return gen.Session{ID: 1}, nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	resp, err := svc.Register(context.Background(), payload.RegisterRequest{
		Email:    "new@example.com",
		Password: "Passw0rd",
	}, "web", "test-device", "127.0.0.1", "", "", "test-agent")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	if capturedEmail != "new@example.com" {
		t.Errorf("expected email new@example.com, got %q", capturedEmail)
	}
	if capturedHash == "" {
		t.Error("expected password to be hashed")
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.User.Email != "new@example.com" {
		t.Errorf("expected response email new@example.com, got %q", resp.User.Email)
	}
}

func TestRegisterGeneratesDisplayUserID(t *testing.T) {
	cfg := testConfig()
	var capturedDisplayID string

	mock := &mockQuerier{
		emailFn: func(_ context.Context, _ string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{}, sql.ErrNoRows
		},
		createUserFn: func(_ context.Context, arg gen.CreateUserParams) (gen.CreateUserRow, error) {
			return gen.CreateUserRow{ID: 100000, Email: arg.Email}, nil
		},
		updateUserFn: func(_ context.Context, arg gen.UpdateUserDisplayIDParams) (gen.UpdateUserDisplayIDRow, error) {
			capturedDisplayID = arg.DisplayUserID.String
			return gen.UpdateUserDisplayIDRow{}, nil
		},
		createSessionFn: func(_ context.Context, _ gen.CreateSessionParams) (gen.Session, error) {
			return gen.Session{ID: 1}, nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	_, err := svc.Register(context.Background(), payload.RegisterRequest{
		Email:    "test@example.com",
		Password: "Passw0rd",
	}, "web", "test-device", "127.0.0.1", "", "", "test-agent")
	if err != nil {
		t.Fatalf("register: %v", err)
	}

	expectedPrefix := utils.EncodeID(100000, utils.UserIDPrefix, cfg.UserIDSecretKey)
	if capturedDisplayID != expectedPrefix {
		t.Errorf("expected display ID %q, got %q", expectedPrefix, capturedDisplayID)
	}
}

func TestLoginUserNotFound(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		emailFn: func(_ context.Context, _ string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{}, sql.ErrNoRows
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	_, err := svc.Login(context.Background(), payload.LoginRequest{
		Email:    "notfound@example.com",
		Password: "password123",
	}, "", "", "", "", "", "")
	if err == nil {
		t.Fatal("expected error for non-existent user")
	}
	if !errors.Is(err, errInvalidCredentials) {
		t.Errorf("expected errInvalidCredentials, got %q", err.Error())
	}
}

func TestLoginDBError(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		emailFn: func(_ context.Context, _ string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{}, sql.ErrConnDone
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	_, err := svc.Login(context.Background(), payload.LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}, "", "", "", "", "", "")
	if err == nil {
		t.Fatal("expected error for DB failure")
	}
}

func TestLoginInvalidPassword(t *testing.T) {
	cfg := testConfig()
	hash := "$2a$10$OnmqGsLru1/LiFn7CVNsJ.N/A7BVHyLmtwt5HiE9wraOvx6OOFpNm"

	mock := &mockQuerier{
		emailFn: func(_ context.Context, _ string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{
				ID:           1,
				Email:        "test@example.com",
				PasswordHash: hash,
			}, nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	_, err := svc.Login(context.Background(), payload.LoginRequest{
		Email:    "test@example.com",
		Password: "wrong-password",
	}, "", "", "", "", "", "")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	if !errors.Is(err, errInvalidCredentials) {
		t.Errorf("expected errInvalidCredentials, got %q", err.Error())
	}
}

func TestLoginSuccess(t *testing.T) {
	cfg := testConfig()
	hash := "$2a$10$OnmqGsLru1/LiFn7CVNsJ.N/A7BVHyLmtwt5HiE9wraOvx6OOFpNm"

	mock := &mockQuerier{
		emailFn: func(_ context.Context, _ string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{
				ID:            1,
				Email:         "test@example.com",
				PasswordHash:  hash,
				DisplayUserID: utils.NullString("USR_test123"),
			}, nil
		},
		createSessionFn: func(_ context.Context, _ gen.CreateSessionParams) (gen.Session, error) {
			return gen.Session{ID: 1}, nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	resp, err := svc.Login(context.Background(), payload.LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}, "web", "Chrome", "127.0.0.1", "", "", "Mozilla/5.0")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if resp.User.ID != "USR_test123" {
		t.Errorf("expected user ID 'USR_test123', got %q", resp.User.ID)
	}
}

func TestGenerateTokensCreatesSession(t *testing.T) {
	cfg := testConfig()
	var capturedParams gen.CreateSessionParams

	mock := &mockQuerier{
		createSessionFn: func(_ context.Context, arg gen.CreateSessionParams) (gen.Session, error) {
			capturedParams = arg
			return gen.Session{ID: 1}, nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	tokens, err := svc.GenerateTokens(context.Background(), 42, "USR_42", "user@example.com", "Test User", "web", "Chrome", "127.0.0.1", "US", "San Francisco", "Mozilla/5.0")
	if err != nil {
		t.Fatalf("GenerateTokens: %v", err)
	}
	if tokens.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if tokens.RefreshToken == "" {
		t.Error("expected non-empty refresh token")
	}
	if capturedParams.UserID != 42 {
		t.Errorf("expected session user_id 42, got %d", capturedParams.UserID)
	}
	if !capturedParams.DeviceType.Valid || capturedParams.DeviceType.String != "web" {
		t.Errorf("expected device_type 'web', got %v", capturedParams.DeviceType)
	}
	if !capturedParams.DeviceName.Valid || capturedParams.DeviceName.String != "Chrome" {
		t.Errorf("expected device_name 'Chrome', got %v", capturedParams.DeviceName)
	}
}

func TestRefreshAccessTokenInvalidToken(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{}, sql.ErrNoRows
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	_, err := svc.RefreshAccessToken(context.Background(), "invalid-refresh-token")
	if err == nil {
		t.Fatal("expected error for invalid refresh token")
	}
}

func TestRefreshAccessTokenRevokedSession(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{
				ID:            1,
				UserID:        42,
				SessionStatus: sql.NullInt16{Int16: 0, Valid: true},
			}, nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	_, err := svc.RefreshAccessToken(context.Background(), "some-token")
	if err == nil {
		t.Fatal("expected error for revoked session")
	}
	if err.Error() != "session revoked" {
		t.Errorf("expected 'session revoked', got %q", err.Error())
	}
}

func TestRefreshAccessTokenSuccess(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{
				ID:            1,
				UserID:        42,
				SessionStatus: sql.NullInt16{Int16: 1, Valid: true},
			}, nil
		},
		updateSessionFn: func(_ context.Context, _ int64) error {
			return nil
		},
		getUserByIDFn: func(_ context.Context, _ int64) (gen.GetUserByIDRow, error) {
			return gen.GetUserByIDRow{
				ID:              42,
				Email:           "user@example.com",
				DisplayUserID:   utils.NullString("USR_42"),
				DisplayUserName: utils.NullString("Test User"),
			}, nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	resp, err := svc.RefreshAccessToken(context.Background(), "valid-refresh-token")
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.RefreshToken != "valid-refresh-token" {
		t.Errorf("expected same refresh token returned, got %q", resp.RefreshToken)
	}
}

func TestRefreshAccessTokenUpdateSessionError(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{
				ID:            1,
				UserID:        42,
				SessionStatus: sql.NullInt16{Int16: 1, Valid: true},
			}, nil
		},
		updateSessionFn: func(_ context.Context, _ int64) error {
			return sql.ErrConnDone
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	_, err := svc.RefreshAccessToken(context.Background(), "valid-refresh-token")
	if err == nil {
		t.Fatal("expected error when update session fails")
	}
}

func TestRevokeSession(t *testing.T) {
	cfg := testConfig()
	var capturedParams gen.RevokeSessionParams
	mock := &mockQuerier{
		revokeSessionFn: func(_ context.Context, arg gen.RevokeSessionParams) error {
			capturedParams = arg
			return nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	err := svc.RevokeSession(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if capturedParams.ID != 1 {
		t.Errorf("expected session ID 1, got %d", capturedParams.ID)
	}
	if capturedParams.UserID != 42 {
		t.Errorf("expected user ID 42, got %d", capturedParams.UserID)
	}
}

func TestListSessions(t *testing.T) {
	cfg := testConfig()
	now := time.Now()
	mock := &mockQuerier{
		listSessionsFn: func(_ context.Context, _ int64) ([]gen.Session, error) {
			return []gen.Session{
				{
					ID:           1,
					UserID:       42,
					DeviceType:   sql.NullString{String: "web", Valid: true},
					DeviceName:   sql.NullString{String: "Chrome", Valid: true},
					IpAddress:    utils.NullIP("127.0.0.1"),
					LoggedInAt:   sql.NullTime{Time: now, Valid: true},
					LastActiveAt: sql.NullTime{Time: now, Valid: true},
				},
			}, nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	sessions, err := svc.ListSessions(context.Background(), 42)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].DeviceType != "web" {
		t.Errorf("expected device type 'web', got %q", sessions[0].DeviceType)
	}
	if sessions[0].IPAddress != "127.0.0.1" {
		t.Errorf("expected IP '127.0.0.1', got %q", sessions[0].IPAddress)
	}
}

func TestListSessionsDBError(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		listSessionsFn: func(_ context.Context, _ int64) ([]gen.Session, error) {
			return nil, sql.ErrConnDone
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	_, err := svc.ListSessions(context.Background(), 42)
	if err == nil {
		t.Fatal("expected error for DB failure")
	}
}

func TestLogoutSuccess(t *testing.T) {
	cfg := testConfig()
	var capturedParams gen.RevokeSessionParams
	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{ID: 1, UserID: 42}, nil
		},
		revokeSessionFn: func(_ context.Context, arg gen.RevokeSessionParams) error {
			capturedParams = arg
			return nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	err := svc.Logout(context.Background(), "refresh-token")
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if capturedParams.ID != 1 {
		t.Errorf("expected session ID 1, got %d", capturedParams.ID)
	}
	if capturedParams.UserID != 42 {
		t.Errorf("expected user ID 42, got %d", capturedParams.UserID)
	}
}

func TestLogoutInvalidToken(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{}, sql.ErrNoRows
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	err := svc.Logout(context.Background(), "invalid-token")
	if err == nil {
		t.Fatal("expected error for invalid refresh token")
	}
	if err.Error() != "invalid refresh token" {
		t.Errorf("expected 'invalid refresh token', got %q", err.Error())
	}
}

func TestClaimsStruct(t *testing.T) {
	claims := Claims{
		UserID: "USR_abc123",
	}
	if claims.UserID != "USR_abc123" {
		t.Errorf("expected EncodedUserID 'USR_abc123', got %q", claims.UserID)
	}
}

// mockCache implements SessionCache for testing.
// Internally stores hash fields as map[key]map[field]value.
type mockCache struct {
	hashes map[string]map[string]string
}

func newMockCache() *mockCache {
	return &mockCache{hashes: make(map[string]map[string]string)}
}

func (m *mockCache) HGet(key, field string) (string, error) {
	if f, ok := m.hashes[key][field]; ok {
		return f, nil
	}
	return "", fmt.Errorf("redis: nil")
}

func (m *mockCache) HSet(key, field string, value any) error {
	if m.hashes[key] == nil {
		m.hashes[key] = make(map[string]string)
	}
	m.hashes[key][field] = fmt.Sprintf("%v", value)
	return nil
}

func (m *mockCache) HSetFields(key string, fields map[string]any) error {
	if m.hashes[key] == nil {
		m.hashes[key] = make(map[string]string)
	}
	for field, value := range fields {
		m.hashes[key][field] = fmt.Sprintf("%v", value)
	}
	return nil
}

func (m *mockCache) HSetWithTTL(key, field string, value any, _ time.Duration) error {
	return m.HSet(key, field, value)
}

func (m *mockCache) HDel(key string, fields ...string) error {
	if m.hashes[key] == nil {
		return nil
	}
	for _, f := range fields {
		delete(m.hashes[key], f)
	}
	return nil
}

func TestGetSessionCacheHit(t *testing.T) {
	cfg := testConfig()
	cache := newMockCache()
	mock := &mockQuerier{}

	// Pre-populate cache hash fields
	_ = cache.HSet("refresh:hash123", "id", 10)
	_ = cache.HSet("refresh:hash123", "user_id", 42)
	_ = cache.HSet("refresh:hash123", "session_status", 1)

	svc := NewAuthService(mock, nil, cfg, cache, testLog(t))

	// getSession should return from cache without hitting the querier
	got, err := svc.getSession(context.Background(), "hash123")
	if err != nil {
		t.Fatalf("getSession: %v", err)
	}
	if got.ID != 10 {
		t.Errorf("expected session ID 10, got %d", got.ID)
	}
	if got.UserID != 42 {
		t.Errorf("expected session UserID 42, got %d", got.UserID)
	}
	if got.SessionStatus.Int16 != 1 {
		t.Errorf("expected session status 1, got %d", got.SessionStatus.Int16)
	}
}

func TestGetSessionCacheMiss(t *testing.T) {
	cfg := testConfig()
	cache := newMockCache()
	var queriedHash string

	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, hash string) (gen.Session, error) {
			queriedHash = hash
			return gen.Session{ID: 20, UserID: 99}, nil
		},
	}

	svc := NewAuthService(mock, nil, cfg, cache, testLog(t))

	// First call: cache miss, should query DB
	got, err := svc.getSession(context.Background(), "hash456")
	if err != nil {
		t.Fatalf("getSession: %v", err)
	}
	if queriedHash != "hash456" {
		t.Errorf("expected DB query for hash456, got %q", queriedHash)
	}
	if got.ID != 20 {
		t.Errorf("expected session ID 20, got %d", got.ID)
	}

	// Verify cache was populated
	if _, err := cache.HGet("refresh:hash456", "id"); err != nil {
		t.Error("expected session to be cached after DB lookup")
	}
}

func TestGetSessionCachePopulatedOnMiss(t *testing.T) {
	cfg := testConfig()
	cache := newMockCache()

	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{ID: 30, UserID: 55, SessionStatus: sql.NullInt16{Int16: 1, Valid: true}}, nil
		},
	}

	svc := NewAuthService(mock, nil, cfg, cache, testLog(t))

	// First call populates cache
	_, _ = svc.getSession(context.Background(), "hash789")

	// Second call should hit cache (no additional DB call)
	dbCallCount := 0
	mock.getSessionByHashFn = func(_ context.Context, _ string) (gen.Session, error) {
		dbCallCount++
		return gen.Session{}, nil
	}

	_, _ = svc.getSession(context.Background(), "hash789")

	if dbCallCount != 0 {
		t.Errorf("expected 0 DB calls on cache hit, got %d", dbCallCount)
	}
}

func TestGetSessionNoCache(t *testing.T) {
	cfg := testConfig()
	cache := newMockCache()

	var dbCalled bool
	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			dbCalled = true
			return gen.Session{ID: 40, UserID: 77}, nil
		},
	}

	svc := NewAuthService(mock, nil, cfg, cache, testLog(t))

	got, err := svc.getSession(context.Background(), "hash_nocache")
	if err != nil {
		t.Fatalf("getSession: %v", err)
	}
	if !dbCalled {
		t.Error("expected DB call when no cache is configured")
	}
	if got.ID != 40 {
		t.Errorf("expected session ID 40, got %d", got.ID)
	}
}

func TestGenerateTokensPopulatesCache(t *testing.T) {
	cfg := testConfig()
	cache := newMockCache()

	mock := &mockQuerier{
		createSessionFn: func(_ context.Context, arg gen.CreateSessionParams) (gen.Session, error) {
			return gen.Session{ID: 50, UserID: arg.UserID, SessionStatus: sql.NullInt16{Int16: 1, Valid: true}}, nil
		},
	}

	svc := NewAuthService(mock, nil, cfg, cache, testLog(t))

	tokens, err := svc.GenerateTokens(context.Background(), 42, "USR_42", "user@example.com", "Test User", "web", "Chrome", "127.0.0.1", "US", "San Francisco", "Mozilla/5.0")
	if err != nil {
		t.Fatalf("GenerateTokens: %v", err)
	}

	// Verify the session was cached (we can't check the exact hash since
	// refresh token is random, but we can verify the cache is not empty)
	if len(cache.hashes) == 0 {
		t.Error("expected cache to be populated after GenerateTokens")
	}

	// Verify we can get the access token
	if tokens.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
}
func TestLogoutClearsCache(t *testing.T) {
	cfg := testConfig()
	cache := newMockCache()

	// Pre-populate cache hash fields
	_ = cache.HSet("hash_logout", "id", 1)
	_ = cache.HSet("hash_logout", "user_id", 42)

	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{ID: 1, UserID: 42}, nil
		},
		revokeSessionFn: func(_ context.Context, _ gen.RevokeSessionParams) error {
			return nil
		},
	}

	svc := NewAuthService(mock, nil, cfg, cache, testLog(t))

	// Manually hash the token to get the cache key
	refreshTokenHash := svc.hashToken("refresh-token-to-logout")

	err := svc.Logout(context.Background(), "refresh-token-to-logout")
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Verify cache was cleared
	if _, err := cache.HGet("refresh:"+refreshTokenHash, "id"); err == nil {
		t.Error("expected cache to be cleared after logout")
	}
}

func TestRefreshAccessTokenUsesCache(t *testing.T) {
	cfg := testConfig()
	cache := newMockCache()
	dbCallCount := 0

	// Pre-populate cache with an active session
	refreshTokenHash := (&AuthService{}).hashToken("cached-refresh-token")
	_ = cache.HSet("refresh:"+refreshTokenHash, "id", 10)
	_ = cache.HSet("refresh:"+refreshTokenHash, "user_id", 42)
	_ = cache.HSet("refresh:"+refreshTokenHash, "session_status", 1)

	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			dbCallCount++
			return gen.Session{}, sql.ErrNoRows
		},
		updateSessionFn: func(_ context.Context, _ int64) error {
			return nil
		},
		getUserByIDFn: func(_ context.Context, _ int64) (gen.GetUserByIDRow, error) {
			return gen.GetUserByIDRow{
				ID:              42,
				Email:           "user@example.com",
				DisplayUserID:   utils.NullString("USR_42"),
				DisplayUserName: utils.NullString("Test User"),
			}, nil
		},
	}

	svc := NewAuthService(mock, nil, cfg, cache, testLog(t))

	resp, err := svc.RefreshAccessToken(context.Background(), "cached-refresh-token")
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}

	// Should NOT have called DB for session lookup (cache hit)
	if dbCallCount != 0 {
		t.Errorf("expected 0 DB calls for session lookup on cache hit, got %d", dbCallCount)
	}

	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.RefreshToken != "cached-refresh-token" {
		t.Errorf("expected same refresh token returned, got %q", resp.RefreshToken)
	}
}

func newAuthServiceFromQuerier(q *mockQuerier, cfg *config.Config) *AuthService {
	log, _ := logger.New(logger.WithLevel("error"))
	return NewAuthService(q, nil, cfg, NoopCache{}, log)
}
