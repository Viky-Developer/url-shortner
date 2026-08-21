package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/config"
	"github.com/vicky/url-shortner/internal/handler"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/routes"
	"github.com/vicky/url-shortner/internal/service"
)

// ─── HELPERS ───────────────────────────────────────────────────

func testConfig() *config.Config {
	return &config.Config{
		UserIDSecretKey:    "test-secret-key",
		JWTSecretKey:       "test-jwt-secret-key-for-middleware-32c",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
}

func testLog(t *testing.T) logger.Logger {
	t.Helper()
	log, err := logger.New(logger.WithLevel("error"))
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

func generateTestToken(t *testing.T, cfg *config.Config, encodedUserID string, sessionID int64) string {
	t.Helper()
	token, err := service.GenerateTestToken(cfg, encodedUserID, sessionID)
	if err != nil {
		t.Fatalf("generate test token: %v", err)
	}
	return token
}

// mockAuthService implements handler.AuthService for testing.
type mockAuthService struct {
	registerFn     func(context.Context, payload.RegisterRequest, string, string) (*payload.AuthResponse, error)
	loginFn        func(context.Context, payload.LoginRequest, string, string, string, string) (*payload.AuthResponse, error)
	forgotPassFn   func(context.Context, payload.ForgotPasswordRequest, string, string) (*payload.AuthResponse, error)
	refreshFn      func(context.Context, string) (*payload.AuthResponse, error)
	logoutFn       func(context.Context, string) error
	listSessionsFn func(context.Context, int64) ([]payload.SessionResponse, error)
	revokeFn       func(context.Context, int64, int64) error
	updatePassFn   func(context.Context, int64, payload.UpdatePasswordRequest, string, string) (*payload.UpdatePasswordResponse, error)
}

func (m *mockAuthService) Register(ctx context.Context, req payload.RegisterRequest, _, _ string) (*payload.AuthResponse, error) {
	if m.registerFn != nil {
		return m.registerFn(ctx, req, "", "")
	}
	return &payload.AuthResponse{
		AccessToken:  "mock-access-token",
		RefreshToken: "mock-refresh-token",
		User:         payload.UserResponse{ID: "USR_mock123", Email: req.Email, DisplayName: req.DisplayName},
	}, nil
}

func (m *mockAuthService) Login(ctx context.Context, req payload.LoginRequest, deviceType, deviceName, ipAddress, userAgent string) (*payload.AuthResponse, error) {
	if m.loginFn != nil {
		return m.loginFn(ctx, req, deviceType, deviceName, ipAddress, userAgent)
	}
	return &payload.AuthResponse{
		AccessToken:  "mock-access-token",
		RefreshToken: "mock-refresh-token",
		User:         payload.UserResponse{ID: "USR_mock123", Email: req.Email},
	}, nil
}

func (m *mockAuthService) ForgotPassword(ctx context.Context, req payload.ForgotPasswordRequest, ipAddress, userAgent string) (*payload.AuthResponse, error) {
	if m.forgotPassFn != nil {
		return m.forgotPassFn(ctx, req, ipAddress, userAgent)
	}
	return &payload.AuthResponse{
		AccessToken:  "mock-access-token",
		RefreshToken: "mock-refresh-token",
		User:         payload.UserResponse{ID: "USR_mock123", Email: req.Email},
	}, nil
}

func (m *mockAuthService) RefreshToken(ctx context.Context, refreshToken string) (*payload.AuthResponse, error) {
	if m.refreshFn != nil {
		return m.refreshFn(ctx, refreshToken)
	}
	return &payload.AuthResponse{
		AccessToken:  "mock-new-access-token",
		RefreshToken: "mock-new-refresh-token",
		User:         payload.UserResponse{ID: "USR_mock123", Email: "test@example.com"},
	}, nil
}

func (m *mockAuthService) Logout(ctx context.Context, refreshToken string) error {
	if m.logoutFn != nil {
		return m.logoutFn(ctx, refreshToken)
	}
	return nil
}

func (m *mockAuthService) ListSessions(ctx context.Context, userID int64) ([]payload.SessionResponse, error) {
	if m.listSessionsFn != nil {
		return m.listSessionsFn(ctx, userID)
	}
	return []payload.SessionResponse{}, nil
}

func (m *mockAuthService) RevokeSession(ctx context.Context, sessionID, userID int64) error {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, sessionID, userID)
	}
	return nil
}

func (m *mockAuthService) UpdatePassword(ctx context.Context, userID int64, req payload.UpdatePasswordRequest, _, _ string) (*payload.UpdatePasswordResponse, error) {
	if m.updatePassFn != nil {
		return m.updatePassFn(ctx, userID, req, "", "")
	}
	return &payload.UpdatePasswordResponse{Message: "password updated successfully"}, nil
}

// mockURLService implements handler.URLService for testing.
type mockURLService struct {
	createFn     func(context.Context, int64, payload.CreateURLRequest) (*payload.URLResponse, error)
	redirectFn   func(context.Context, string, payload.ClickInfo) (*payload.URLResponse, error)
	byIDFn       func(context.Context, int64, int64) (*payload.URLResponse, error)
	listFn       func(context.Context, int64, int32, int32, int32) (*payload.URLListResponse, error)
	updateFn     func(context.Context, int64, int64, payload.UpdateURLRequest) (*payload.URLResponse, error)
	softDeleteFn func(context.Context, int64, int64) (*payload.DeleteResponse, error)
	hardDeleteFn func(context.Context, int64, int64) error
}

func (m *mockURLService) Create(ctx context.Context, userID int64, req payload.CreateURLRequest) (*payload.URLResponse, error) {
	if m.createFn != nil {
		return m.createFn(ctx, userID, req)
	}
	return &payload.URLResponse{
		ID: 1, UserID: "USR_mock123", ShortCode: "abc1234567",
		OriginalURL: req.OriginalURL, ShortURL: "http://localhost:8080/abc1234567",
		IsActive: true,
	}, nil
}

func (m *mockURLService) Redirect(ctx context.Context, code string, click payload.ClickInfo) (*payload.URLResponse, error) {
	if m.redirectFn != nil {
		return m.redirectFn(ctx, code, click)
	}
	return &payload.URLResponse{OriginalURL: "https://example.com/target"}, nil
}

func (m *mockURLService) GetByID(ctx context.Context, userID int64, id int64) (*payload.URLResponse, error) {
	if m.byIDFn != nil {
		return m.byIDFn(ctx, userID, id)
	}
	return &payload.URLResponse{ID: id, UserID: "USR_mock123", ShortCode: "abc1234567", IsActive: true}, nil
}

func (m *mockURLService) List(ctx context.Context, userID int64, page, perPage, offset int32) (*payload.URLListResponse, error) {
	if m.listFn != nil {
		return m.listFn(ctx, userID, page, perPage, offset)
	}
	return &payload.URLListResponse{Items: []payload.URLResponse{}, Total: 0, Page: 1, PerPage: 10, TotalPages: 0}, nil
}

func (m *mockURLService) Update(ctx context.Context, userID int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, userID, id, req)
	}
	return &payload.URLResponse{ID: id, UserID: "USR_mock123", ShortCode: "abc1234567", IsActive: true}, nil
}

func (m *mockURLService) SoftDelete(ctx context.Context, userID int64, id int64) (*payload.DeleteResponse, error) {
	if m.softDeleteFn != nil {
		return m.softDeleteFn(ctx, userID, id)
	}
	return &payload.DeleteResponse{ID: id, ShortCode: "abc1234567", Message: "soft deleted"}, nil
}

func (m *mockURLService) HardDelete(ctx context.Context, userID int64, id int64) error {
	if m.hardDeleteFn != nil {
		return m.hardDeleteFn(ctx, userID, id)
	}
	return nil
}

func (m *mockURLService) ListClickLogs(_ context.Context, _, _ int64, _, _ *time.Time, _, _, _ int32) (*payload.ClickLogsResponse, error) {
	return &payload.ClickLogsResponse{Items: []payload.ClickLogEntry{}, Total: 0}, nil
}

func (m *mockURLService) GetAnalytics(_ context.Context, _, _ int64, _, _ *time.Time) (*payload.AnalyticsResponse, error) {
	return &payload.AnalyticsResponse{Stats: payload.ClickStats{}}, nil
}

func newTestMux(t *testing.T, authSvc *mockAuthService, urlSvc *mockURLService) http.Handler {
	t.Helper()
	log := testLog(t)

	// Real auth service for middleware (needs ValidateAccessToken)
	cfg := testConfig()
	realAuthService := service.NewAuthService(nil, nil, cfg, alwaysActiveCache{}, log)

	var urlHandler *handler.URLHandler
	if urlSvc != nil {
		urlHandler = handler.NewURLHandler(urlSvc, log)
	} else {
		urlHandler = handler.NewURLHandler(nil, log)
	}
	authHandler := handler.NewAuthHandler(authSvc, log)
	adminHandler := handler.NewAdminHandler(nil, log)
	return routes.New(urlHandler, authHandler, adminHandler, realAuthService)
}

func doRequest(t *testing.T, mux http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	var reqBody *bytes.Buffer
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		reqBody = bytes.NewBuffer(data)
		fmt.Printf("\n>>> REQUEST BODY:\n%s\n", string(data))
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	fmt.Printf("<<< RESPONSE [%s %s] Status: %d\n", method, path, w.Code)

	var pretty bytes.Buffer
	if json.Indent(&pretty, w.Body.Bytes(), "", "  ") == nil {
		fmt.Printf("<<< RESPONSE BODY:\n%s\n", pretty.String())
	} else {
		fmt.Printf("<<< RESPONSE BODY:\n%s\n", w.Body.String())
	}
	fmt.Println("---")

	return w
}

// ─── AUTH: REGISTER ────────────────────────────────────────────

func TestRegisterSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/register (success)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email":       "test@example.com",
		"password":    "password123",
		"displayName": "Test User",
	}, nil)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestRegisterMissingEmail(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/register (missing email)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"password": "password123",
	}, nil)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegisterMissingPassword(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/register (missing password)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email": "test@example.com",
	}, nil)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegisterInvalidJSON(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/register (invalid JSON)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	fmt.Printf("<<< RESPONSE [POST /api/v1/auth/register] Status: %d\n", w.Code)
	fmt.Printf("<<< RESPONSE BODY:\n%s\n", w.Body.String())
	fmt.Println("---")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRegisterServiceError(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/register (service error)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{
		registerFn: func(_ context.Context, _ payload.RegisterRequest, _, _ string) (*payload.AuthResponse, error) {
			return nil, fmt.Errorf("email already registered")
		},
	}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/register", map[string]string{
		"email":    "existing@example.com",
		"password": "password123",
	}, nil)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ─── AUTH: LOGIN ───────────────────────────────────────────────

func TestLoginSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/login (success)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email":    "test@example.com",
		"password": "password123",
	}, nil)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLoginMissingFields(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/login (missing password)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "test@example.com",
	}, nil)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLoginInvalidCredentials(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/login (invalid credentials)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{
		loginFn: func(_ context.Context, _ payload.LoginRequest, _, _, _, _ string) (*payload.AuthResponse, error) {
			return nil, fmt.Errorf("invalid credentials")
		},
	}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email":    "wrong@example.com",
		"password": "wrongpass",
	}, nil)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ─── AUTH: REFRESH (authenticated) ───────────────────────────────

func TestRefreshSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/refresh (success)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/refresh", map[string]string{
		"refreshToken": "valid-refresh-token",
	}, map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRefreshNoAuth(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/refresh (no auth)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/refresh", map[string]string{
		"refreshToken": "valid-refresh-token",
	}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRefreshMissingToken(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/refresh (missing token)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/refresh", map[string]string{},
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRefreshServiceError(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/refresh (service error)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{
		refreshFn: func(_ context.Context, _ string) (*payload.AuthResponse, error) {
			return nil, fmt.Errorf("invalid or expired refresh token")
		},
	}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/refresh", map[string]string{
		"refreshToken": "expired-token",
	}, map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ─── AUTH: LOGOUT (authenticated) ────────────────────────────────

func TestLogoutSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/logout (success)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/logout", map[string]string{
		"refreshToken": "valid-refresh-token",
	}, map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestLogoutNoAuth(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/logout (no auth)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/logout", map[string]string{
		"refreshToken": "valid-refresh-token",
	}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestLogoutMissingToken(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/logout (missing token)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/logout", map[string]string{},
		map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestLogoutServiceError(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/auth/logout (service error)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{
		logoutFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("invalid refresh token")
		},
	}, nil)

	w := doRequest(t, mux, http.MethodPost, "/api/v1/auth/logout", map[string]string{
		"refreshToken": "bad-token",
	}, map[string]string{"Authorization": "Bearer " + token})

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ─── MIDDLEWARE: AUTH GUARD ────────────────────────────────────

func TestNoTokenRejects(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/shorten (no token)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodPost, "/api/v1/shorten", map[string]string{
		"originalURL": "https://example.com",
	}, nil)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestInvalidTokenRejects(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/shorten (invalid token)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodPost, "/api/v1/shorten", map[string]string{
		"originalURL": "https://example.com",
	}, map[string]string{
		"Authorization": "Bearer totally-invalid",
	})

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWrongFormatRejects(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: GET /api/v1/urls (Basic auth)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodGet, "/api/v1/urls", nil, map[string]string{
		"Authorization": "Basic dXNlcjpwYXNz",
	})

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestValidTokenPasses(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/shorten (valid token)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodPost, "/api/v1/shorten", map[string]string{
		"originalURL": "https://example.com",
	}, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

// ─── URL: CREATE ───────────────────────────────────────────────

func TestCreateShortURLSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/shorten (success)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodPost, "/api/v1/shorten", map[string]string{
		"originalURL": "https://example.com/long-url",
	}, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestCreateShortURLEmptyBody(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/shorten (empty body)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodPost, "/api/v1/shorten", map[string]string{}, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateShortURLInvalidURL(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: POST /api/v1/shorten (invalid URL)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodPost, "/api/v1/shorten", map[string]string{
		"originalURL": "not-a-url",
	}, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── URL: GET BY ID ────────────────────────────────────────────

func TestGetURLByIDSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: GET /api/v1/urls/1 (success)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodGet, "/api/v1/urls/1", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestGetURLByIDInvalidID(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: GET /api/v1/urls/abc (invalid id)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodGet, "/api/v1/urls/abc", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── URL: LIST ─────────────────────────────────────────────────

func TestListURLsSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: GET /api/v1/urls (success)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodGet, "/api/v1/urls", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ─── URL: UPDATE ───────────────────────────────────────────────

func TestUpdateURLSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: PATCH /api/v1/urls/1 (success)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodPatch, "/api/v1/urls/1", map[string]string{
		"originalURL": "https://example.com/updated",
	}, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUpdateURLInvalidBody(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: PATCH /api/v1/urls/1 (invalid JSON)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/urls/1", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	fmt.Printf("<<< RESPONSE [PATCH /api/v1/urls/1] Status: %d\n", w.Code)
	fmt.Printf("<<< RESPONSE BODY:\n%s\n", w.Body.String())
	fmt.Println("---")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── URL: DELETE ───────────────────────────────────────────────

func TestDeleteURLSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: DELETE /api/v1/urls/1 (success)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodDelete, "/api/v1/urls/1", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestDeleteURLInvalidID(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: DELETE /api/v1/urls/abc (invalid id)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodDelete, "/api/v1/urls/abc", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── URL: APPROVE HARD DELETE ──────────────────────────────────

func TestApproveHardDeleteSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: DELETE /api/v1/urls/1/approve (success)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodDelete, "/api/v1/urls/1/approve", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestApproveHardDeleteInvalidID(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: DELETE /api/v1/urls/abc/approve (invalid id)")
	fmt.Println("========================================")

	cfg := testConfig()
	token := generateTestToken(t, cfg, "USR_test123", 1)
	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	w := doRequest(t, mux, http.MethodDelete, "/api/v1/urls/abc/approve", nil, map[string]string{
		"Authorization": "Bearer " + token,
	})

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ─── URL: REDIRECT ─────────────────────────────────────────────

func TestRedirectSuccess(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: GET /api/v1/abc (public redirect)")
	fmt.Println("========================================")

	mux := newTestMux(t, &mockAuthService{}, &mockURLService{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	fmt.Printf("<<< RESPONSE [GET /api/v1/abc] Status: %d\n", w.Code)
	if w.Code == http.StatusFound {
		fmt.Printf("<<< LOCATION: %s\n", w.Header().Get("Location"))
	}
	fmt.Println("---")

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
}

// ─── ROUTE REGISTRATION ────────────────────────────────────────

func TestAllRoutesRegistered(t *testing.T) {
	fmt.Println("\n========================================")
	fmt.Println("TEST: Route Registration")
	fmt.Println("========================================")

	cfg := testConfig()
	realAuthService := service.NewAuthService(nil, nil, cfg, alwaysActiveCache{}, testLog(t))
	urlHandler := handler.NewURLHandler(nil, testLog(t))
	authHandler := handler.NewAuthHandler(nil, testLog(t))
	adminHandler := handler.NewAdminHandler(nil, testLog(t))
	mux := routes.New(urlHandler, authHandler, adminHandler, realAuthService)

	serveMux, ok := mux.(*http.ServeMux)
	if !ok {
		t.Fatal("expected *http.ServeMux")
	}

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/auth/register"},
		{http.MethodPost, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/forgot-password"},
		{http.MethodPost, "/api/v1/auth/refresh"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodGet, "/api/v1/auth/sessions"},
		{http.MethodDelete, "/api/v1/auth/sessions/1"},
		{http.MethodPatch, "/api/v1/auth/password"},
		{http.MethodPost, "/api/v1/shorten"},
		{http.MethodGet, "/api/v1/urls"},
		{http.MethodGet, "/api/v1/urls/1"},
		{http.MethodPatch, "/api/v1/urls/1"},
		{http.MethodDelete, "/api/v1/urls/1"},
		{http.MethodDelete, "/api/v1/urls/1/approve"},
		{http.MethodGet, "/api/v1/abc"},
	}

	for _, rt := range routes {
		fmt.Printf("  %-7s %-40s ", rt.method, rt.path)
		req, _ := http.NewRequest(rt.method, rt.path, nil)
		_, pattern := serveMux.Handler(req)
		if pattern == "" {
			fmt.Println("FAIL")
			t.Errorf("no route for %s %s", rt.method, rt.path)
		} else {
			fmt.Printf("OK (%s)\n", pattern)
		}
	}
}
