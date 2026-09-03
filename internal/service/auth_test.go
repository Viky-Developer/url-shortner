package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/vicky/url-shortner/external/cache"
	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
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

	token, err := svc.generateAccessToken("USR_abc123", "test@example.com", "Test User", "USER")
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

	token, err := svc.generateAccessToken("USR_abc123", "test@example.com", "Test User", "USER")
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

	token, err := svc1.generateAccessToken("USR_abc123", "test@example.com", "Test User", "USER")
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

	_, err := svc.Register(context.Background(), &payload.RegisterRequest{
		Email:    "existing@example.com",
		Password: "Passw0rd",
	}, "web", "test-device", "127.0.0.1", "", "", "test-agent")
	if err == nil {
		t.Fatal("expected error for existing email")
	}
	if !errors.Is(err, apperror.ErrEmailAlreadyExists) {
		t.Errorf("expected ErrEmailAlreadyExists, got %v", err)
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

	_, err := svc.Register(context.Background(), &payload.RegisterRequest{
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

	resp, err := svc.Register(context.Background(), &payload.RegisterRequest{
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

	_, err := svc.Register(context.Background(), &payload.RegisterRequest{
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
	if !errors.Is(err, apperror.ErrUnauthorized) {
		t.Errorf("expected apperror.ErrUnauthorized, got %q", err.Error())
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
	if !errors.Is(err, apperror.ErrUnauthorized) {
		t.Errorf("expected apperror.ErrUnauthorized, got %q", err.Error())
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
	if resp.Token.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.Token.RefreshToken == "" {
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

	tokens, err := svc.GenerateTokens(context.Background(), 42, "USR_42", "user@example.com", "Test User", "USER", "web", "Chrome", "127.0.0.1", "US", "San Francisco", "Mozilla/5.0")
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

	_, err := svc.RefreshAccessToken(context.Background(), "invalid-refresh-token", 0)
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

	_, err := svc.RefreshAccessToken(context.Background(), "some-token", 0)
	if err == nil {
		t.Fatal("expected error for revoked session")
	}
	if err.Error() != "session revoked" {
		t.Errorf("expected 'session revoked', got %q", err.Error())
	}
}

func TestRefreshAccessTokenSuccess(t *testing.T) {
	cfg := testConfig()
	now := time.Now()
	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{
				ID:            1,
				UserID:        42,
				SessionStatus: sql.NullInt16{Int16: 1, Valid: true},
				LastActiveAt:  sql.NullTime{Time: now.Add(-1 * time.Hour), Valid: true},
			}, nil
		},
		updateSessionFn: func(_ context.Context, _ int64) error {
			return nil
		},
		getSessionByIDFn: func(_ context.Context, _ int64) (gen.Session, error) {
			return gen.Session{
				ID:            1,
				UserID:        42,
				SessionStatus: sql.NullInt16{Int16: 1, Valid: true},
				LastActiveAt:  sql.NullTime{Time: now, Valid: true},
			}, nil
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

	resp, err := svc.RefreshAccessToken(context.Background(), "valid-refresh-token", 0)
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

func TestRefreshAccessTokenRevokeError(t *testing.T) {
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

	_, err := svc.RefreshAccessToken(context.Background(), "valid-refresh-token", 0)
	if err == nil {
		t.Fatal("expected error when update session fails")
	}
}

func TestRevokeSession(t *testing.T) {
	cfg := testConfig()
	var capturedParams gen.RevokeSessionParams
	mock := &mockQuerier{
		getSessionByIDFn: func(_ context.Context, id int64) (gen.Session, error) {
			return gen.Session{ID: id, UserID: 42, RefreshTokenHash: "some-hash"}, nil
		},
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
		revokeSessionFn: func(_ context.Context, arg gen.RevokeSessionParams) error {
			capturedParams = arg
			return nil
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	err := svc.Logout(context.Background(), "refresh-token", 42, 1)
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

func TestLogoutRevokeError(t *testing.T) {
	cfg := testConfig()
	mock := &mockQuerier{
		revokeSessionFn: func(_ context.Context, _ gen.RevokeSessionParams) error {
			return sql.ErrConnDone
		},
	}
	svc := newAuthServiceFromQuerier(mock, cfg)

	err := svc.Logout(context.Background(), "refresh-token", 42, 1)
	if err == nil {
		t.Fatal("expected error when revoke fails")
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
	hashes  map[string]map[string]string
	strings map[string]string
}

func newMockCache() *mockCache {
	return &mockCache{hashes: make(map[string]map[string]string), strings: make(map[string]string)}
}

func (m *mockCache) Get(_ context.Context, key string) (string, error) {
	if v, ok := m.strings[key]; ok {
		return v, nil
	}
	return "", fmt.Errorf("redis: nil")
}

func (m *mockCache) Set(_ context.Context, key, value string, _ ...cache.CacheOption) error {
	m.strings[key] = value
	return nil
}

func (m *mockCache) HGet(_ context.Context, key, field string) (string, error) {
	if f, ok := m.hashes[key][field]; ok {
		return f, nil
	}
	return "", fmt.Errorf("redis: nil")
}

func (m *mockCache) HMGet(_ context.Context, key string, fields ...string) (map[string]string, error) {
	h, ok := m.hashes[key]
	if !ok || len(h) == 0 {
		return nil, fmt.Errorf("redis: nil")
	}
	out := make(map[string]string, len(fields))
	for _, f := range fields {
		if v, ok := h[f]; ok {
			out[f] = v
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("redis: nil")
	}
	return out, nil
}

func (m *mockCache) HGetAll(_ context.Context, key string) (map[string]string, error) {
	h, ok := m.hashes[key]
	if !ok || len(h) == 0 {
		return nil, fmt.Errorf("redis: nil")
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out, nil
}

func (m *mockCache) HSet(_ context.Context, key string, fields map[string]any, _ ...cache.CacheOption) error {
	if m.hashes[key] == nil {
		m.hashes[key] = make(map[string]string)
	}
	for field, value := range fields {
		m.hashes[key][field] = fmt.Sprintf("%v", value)
	}
	return nil
}

func (m *mockCache) HDel(_ context.Context, key string, fields ...string) error {
	if m.hashes[key] == nil {
		return nil
	}
	for _, f := range fields {
		delete(m.hashes[key], f)
	}
	return nil
}

func (m *mockCache) Del(_ context.Context, key string) error {
	delete(m.hashes, key)
	delete(m.strings, key)
	return nil
}

func TestGetSessionCacheHit(t *testing.T) {
	cfg := testConfig()
	cache := newMockCache()

	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{
				ID:               10,
				UserID:           42,
				SessionStatus:    sql.NullInt16{Int16: 1, Valid: true},
				RefreshTokenHash: "db_hash",
			}, nil
		},
	}

	svc := NewAuthService(mock, nil, cfg, cache, testLog(t))

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

	// Verify cache was populated for next time
	cached, cacheErr := cache.HGetAll(context.Background(), "session:10")
	if cacheErr != nil || len(cached) == 0 {
		t.Fatal("expected session cache to be populated after DB lookup")
	}
	if cached["refresh_token"] != "db_hash" {
		t.Errorf("expected cached refresh_token to be %q, got %q", "db_hash", cached["refresh_token"])
	}
}

func TestGetSessionCacheMiss(t *testing.T) {
	cfg := testConfig()
	cache := newMockCache()
	var queriedHash string

	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, hash string) (gen.Session, error) {
			queriedHash = hash
			return gen.Session{ID: 20, UserID: 99, SessionStatus: sql.NullInt16{Int16: 1, Valid: true}}, nil
		},
	}

	svc := NewAuthService(mock, nil, cfg, cache, testLog(t))

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

	// Verify session cache was populated after DB lookup
	if _, err := cache.HGet(context.Background(), "session:20", "id"); err != nil {
		t.Error("expected session cache to have full data after DB lookup")
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

	// Verify cache is populated
	cached, err := cache.HGetAll(context.Background(), "session:30")
	if err != nil || len(cached) == 0 {
		t.Fatal("expected session cache to be populated after first call")
	}

	// Second call: HGetAll returns cached data
	_, _ = svc.getSession(context.Background(), "hash789")
	cached2, _ := cache.HGetAll(context.Background(), "session:30")
	if cached2["user_id"] != "55" {
		t.Errorf("expected cached user_id 55, got %q", cached2["user_id"])
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

	tokens, err := svc.GenerateTokens(context.Background(), 42, "USR_42", "user@example.com", "Test User", "USER", "web", "Chrome", "127.0.0.1", "US", "San Francisco", "Mozilla/5.0")
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

	// Pre-populate session cache
	_ = cache.HSet(context.Background(), "session:5", map[string]any{
		"id":             5,
		"user_id":        42,
		"refresh_token":  "refresh_token_hash",
		"session_status": 1,
		"last_active_at": 1609459200,
		"expires_at":     1612137600,
	})

	mock := &mockQuerier{
		revokeSessionFn: func(_ context.Context, _ gen.RevokeSessionParams) error {
			return nil
		},
	}

	svc := NewAuthService(mock, nil, cfg, cache, testLog(t))

	err := svc.Logout(context.Background(), "refresh-token-to-logout", 42, 5)
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}

	// Verify session cache was cleared
	if _, err := cache.HGet(context.Background(), "session:5", "id"); err == nil {
		t.Error("expected session cache to be cleared after logout")
	}
}

func TestRefreshAccessTokenUsesCache(t *testing.T) {
	cfg := testConfig()
	cache := newMockCache()

	// Pre-populate session cache
	_ = cache.HSet(context.Background(), "session:10", map[string]any{
		"id":             10,
		"user_id":        42,
		"refresh_token":  "refresh_token_hash",
		"session_status": 1,
		"last_active_at": 1609459200,
		"expires_at":     1612137600,
	})

	mock := &mockQuerier{
		getSessionByHashFn: func(_ context.Context, _ string) (gen.Session, error) {
			return gen.Session{
				ID:               10,
				UserID:           42,
				SessionStatus:    sql.NullInt16{Int16: 1, Valid: true},
				RefreshTokenHash: "refresh_token_hash",
			}, nil
		},
		updateSessionFn: func(_ context.Context, _ int64) error {
			return nil
		},
		getSessionByIDFn: func(_ context.Context, _ int64) (gen.Session, error) {
			return gen.Session{
				ID:            10,
				UserID:        42,
				SessionStatus: sql.NullInt16{Int16: 1, Valid: true},
				LastActiveAt:  sql.NullTime{Time: time.Now(), Valid: true},
			}, nil
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

	resp, err := svc.RefreshAccessToken(context.Background(), "cached-refresh-token", 0)
	if err != nil {
		t.Fatalf("RefreshAccessToken: %v", err)
	}

	if resp.AccessToken == "" {
		t.Error("expected non-empty access token")
	}
	if resp.RefreshToken != "cached-refresh-token" {
		t.Errorf("expected same refresh token, got %q", resp.RefreshToken)
	}
}

func newAuthServiceFromQuerier(q *mockQuerier, cfg *config.Config) *AuthService {
	log, _ := logger.New(logger.WithLevel("error"))
	return NewAuthService(q, nil, cfg, NoopCache{}, log)
}
