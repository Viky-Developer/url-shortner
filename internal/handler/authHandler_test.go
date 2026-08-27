package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/contextutil"
	"github.com/vicky/url-shortner/internal/payload"
)

type mockAuthService struct {
	registerFn     func(context.Context, *payload.RegisterRequest, string, string, string, string, string, string) (*payload.AuthResponse, error)
	loginFn        func(context.Context, payload.LoginRequest, string, string, string, string, string, string) (*payload.AuthResponse, error)
	forgotPassFn   func(context.Context, payload.ForgotPasswordRequest, string, string) error
	ChangePassFn   func(context.Context, int64, payload.ChangePasswordRequest, int64, string, string) error
	refreshFn      func(context.Context, string, int64) (*payload.RefreshTokenResponse, error)
	logoutFn       func(context.Context, string, int64, int64) error
	listSessionsFn func(context.Context, int64) ([]payload.SessionResponse, error)
	revokeFn       func(context.Context, int64, int64) error
	revokeOtherFn  func(context.Context, int64, int64) error
	revokeAllFn    func(context.Context, int64) error
}

func (m *mockAuthService) Register(ctx context.Context, req *payload.RegisterRequest, deviceType, deviceName, ipAddress, country, city, userAgent string) (*payload.AuthResponse, error) {
	return m.registerFn(ctx, req, deviceType, deviceName, ipAddress, country, city, userAgent)
}

func (m *mockAuthService) Login(ctx context.Context, req payload.LoginRequest, deviceType, deviceName, ipAddress, country, city, userAgent string) (*payload.AuthResponse, error) {
	return m.loginFn(ctx, req, deviceType, deviceName, ipAddress, country, city, userAgent)
}

func (m *mockAuthService) ForgotPassword(ctx context.Context, req payload.ForgotPasswordRequest, ipAddress, userAgent string) error {
	if m.forgotPassFn != nil {
		return m.forgotPassFn(ctx, req, ipAddress, userAgent)
	}
	return nil
}

func (m *mockAuthService) ChangePassword(ctx context.Context, userID int64, req payload.ChangePasswordRequest, sessionID int64, ipAddress, userAgent string) error {
	if m.ChangePassFn != nil {
		return m.ChangePassFn(ctx, userID, req, sessionID, ipAddress, userAgent)
	}
	return nil
}

func (m *mockAuthService) RefreshToken(ctx context.Context, refreshToken string, sessionID int64) (*payload.RefreshTokenResponse, error) {
	return m.refreshFn(ctx, refreshToken, sessionID)
}

func (m *mockAuthService) Logout(ctx context.Context, refreshToken string, userID, sessionID int64) error {
	return m.logoutFn(ctx, refreshToken, userID, sessionID)
}

func (m *mockAuthService) ListSessions(ctx context.Context, userID int64) ([]payload.SessionResponse, error) {
	return m.listSessionsFn(ctx, userID)
}

func (m *mockAuthService) RevokeSession(ctx context.Context, sessionID, userID int64) error {
	return m.revokeFn(ctx, sessionID, userID)
}

func (m *mockAuthService) RevokeOtherDevices(ctx context.Context, userID, currentSessionID int64) error {
	if m.revokeOtherFn != nil {
		return m.revokeOtherFn(ctx, userID, currentSessionID)
	}
	return nil
}

func (m *mockAuthService) RevokeAllSessions(ctx context.Context, userID int64) error {
	if m.revokeAllFn != nil {
		return m.revokeAllFn(ctx, userID)
	}
	return nil
}

func sampleAuthResponse() *payload.AuthResponse {
	return &payload.AuthResponse{
		AccessToken:  "access-token",
		RefreshToken: "refresh-token",
		User: payload.UserResponse{
			ID:          "USR_test123",
			Email:       "test@example.com",
			DisplayName: "Test User",
		},
	}
}

func TestRegisterHandler(t *testing.T) {
	mock := &mockAuthService{
		registerFn: func(_ context.Context, _ *payload.RegisterRequest, _, _, _, _, _, _ string) (*payload.AuthResponse, error) {
			return sampleAuthResponse(), nil
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"test@example.com","password":"secret123","displayName":"Test User"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"accessToken":"access-token"`) {
		t.Errorf("expected accessToken in body, got %s", w.Body.String())
	}
}

func TestRegisterHandlerInvalidJSON(t *testing.T) {
	mock := &mockAuthService{}
	h := NewAuthHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString("{invalid"))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRegisterHandlerMissingFields(t *testing.T) {
	mock := &mockAuthService{}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Password is required") {
		t.Errorf("expected missing fields error, got %s", w.Body.String())
	}
}

func TestRegisterHandlerServiceError(t *testing.T) {
	mock := &mockAuthService{
		registerFn: func(_ context.Context, _ *payload.RegisterRequest, _, _, _, _, _, _ string) (*payload.AuthResponse, error) {
			return nil, apperror.ErrConflict
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"existing@example.com","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLoginHandler(t *testing.T) {
	mock := &mockAuthService{
		loginFn: func(_ context.Context, _ payload.LoginRequest, _, _, _, _, _, _ string) (*payload.AuthResponse, error) {
			return sampleAuthResponse(), nil
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"test@example.com","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLoginHandlerInvalidJSON(t *testing.T) {
	mock := &mockAuthService{}
	h := NewAuthHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString("{bad"))
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestLoginHandlerMissingFields(t *testing.T) {
	mock := &mockAuthService{}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestLoginHandlerServiceError(t *testing.T) {
	mock := &mockAuthService{
		loginFn: func(_ context.Context, _ payload.LoginRequest, _, _, _, _, _, _ string) (*payload.AuthResponse, error) {
			return nil, apperror.ErrNotFound
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"notfound@example.com","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.Login(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRefreshTokenHandler(t *testing.T) {
	mock := &mockAuthService{
		refreshFn: func(_ context.Context, _ string, _ int64) (*payload.RefreshTokenResponse, error) {
			return &payload.RefreshTokenResponse{
				AccessToken:  "new-access-token",
				RefreshToken: "same-refresh-token",
			}, nil
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"refreshToken":"some-token"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.RefreshToken(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRefreshTokenHandlerMissingToken(t *testing.T) {
	mock := &mockAuthService{}
	h := NewAuthHandler(mock, testLog(t))

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.RefreshToken(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRefreshTokenHandlerServiceError(t *testing.T) {
	mock := &mockAuthService{
		refreshFn: func(_ context.Context, _ string, _ int64) (*payload.RefreshTokenResponse, error) {
			return nil, apperror.ErrUnauthorized
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"refreshToken":"expired-token"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.RefreshToken(w, req)

	// ErrUnauthorized maps to 401 in StatusCodeFromError
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogoutHandler(t *testing.T) {
	mock := &mockAuthService{
		logoutFn: func(_ context.Context, _ string, _, _ int64) error {
			return nil
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"refreshToken":"some-token"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	req = req.WithContext(context.WithValue(req.Context(), contextutil.SessionIDKey, int64(10)))
	w := httptest.NewRecorder()

	h.Logout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLogoutHandlerMissingToken(t *testing.T) {
	mock := &mockAuthService{}
	h := NewAuthHandler(mock, testLog(t))

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.Logout(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestLogoutHandlerServiceError(t *testing.T) {
	mock := &mockAuthService{
		logoutFn: func(_ context.Context, _ string, _, _ int64) error {
			return apperror.ErrNotFound
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"refreshToken":"invalid-token"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/logout", bytes.NewBufferString(body))
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	req = req.WithContext(context.WithValue(req.Context(), contextutil.SessionIDKey, int64(10)))
	w := httptest.NewRecorder()

	h.Logout(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListSessionsHandler(t *testing.T) {
	mock := &mockAuthService{
		listSessionsFn: func(_ context.Context, _ int64) ([]payload.SessionResponse, error) {
			return []payload.SessionResponse{
				{ID: 1, DeviceType: "web", DeviceName: "Chrome", IPAddress: "127.0.0.1"},
			}, nil
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/auth/sessions", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	w := httptest.NewRecorder()

	h.ListSessions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListSessionsHandlerUnauthorized(t *testing.T) {
	mock := &mockAuthService{}
	h := NewAuthHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/auth/sessions", nil)
	w := httptest.NewRecorder()

	h.ListSessions(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestListSessionsHandlerServiceError(t *testing.T) {
	mock := &mockAuthService{
		listSessionsFn: func(_ context.Context, _ int64) ([]payload.SessionResponse, error) {
			return nil, apperror.ErrInternal
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/auth/sessions", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	w := httptest.NewRecorder()

	h.ListSessions(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRevokeSessionHandler(t *testing.T) {
	mock := &mockAuthService{
		revokeFn: func(_ context.Context, _, _ int64) error {
			return nil
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/auth/sessions/1", nil)
	req.SetPathValue("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	w := httptest.NewRecorder()

	h.RevokeSession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRevokeSessionHandlerUnauthorized(t *testing.T) {
	mock := &mockAuthService{}
	h := NewAuthHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/auth/sessions/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.RevokeSession(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRevokeSessionHandlerInvalidID(t *testing.T) {
	mock := &mockAuthService{
		revokeFn: func(_ context.Context, _, _ int64) error {
			return nil
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/auth/sessions/abc", nil)
	req.SetPathValue("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	w := httptest.NewRecorder()

	h.RevokeSession(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRevokeSessionHandlerServiceError(t *testing.T) {
	mock := &mockAuthService{
		revokeFn: func(_ context.Context, _, _ int64) error {
			return apperror.ErrInternal
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/auth/sessions/1", nil)
	req.SetPathValue("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	w := httptest.NewRecorder()

	h.RevokeSession(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegisterHandlerJSONResponse(t *testing.T) {
	mock := &mockAuthService{
		registerFn: func(_ context.Context, _ *payload.RegisterRequest, _, _, _, _, _, _ string) (*payload.AuthResponse, error) {
			return sampleAuthResponse(), nil
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"test@example.com","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.Register(w, req)

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok || len(data) == 0 {
		t.Fatalf("expected data array in response, got %s", w.Body.String())
	}
}

func TestLoginHandlerPassesDeviceInfo(t *testing.T) {
	var capturedDeviceType, capturedDeviceName, capturedIP, capturedUA string
	mock := &mockAuthService{
		loginFn: func(_ context.Context, _ payload.LoginRequest, deviceType, deviceName, ipAddress, _, _, userAgent string) (*payload.AuthResponse, error) {
			capturedDeviceType = deviceType
			capturedDeviceName = deviceName
			capturedIP = ipAddress
			capturedUA = userAgent
			return sampleAuthResponse(), nil
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"test@example.com","password":"secret123"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewBufferString(body))
	req.Header.Set("X-Device-Type", "web")
	req.Header.Set("X-Device-Name", "Chrome")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	w := httptest.NewRecorder()

	h.Login(w, req)

	if capturedDeviceType != "web" {
		t.Errorf("deviceType = %q, want %q", capturedDeviceType, "web")
	}
	if capturedDeviceName != "Chrome" {
		t.Errorf("deviceName = %q, want %q", capturedDeviceName, "Chrome")
	}
	if capturedUA != "Mozilla/5.0" {
		t.Errorf("userAgent = %q, want %q", capturedUA, "Mozilla/5.0")
	}
	_ = capturedIP
}

func TestForgotPasswordHandler(t *testing.T) {
	mock := &mockAuthService{}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"test@example.com","currentPassword":"oldPass1","newPassword":"newPass1"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.ForgotPassword(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "password updated successfully") {
		t.Errorf("expected success message, got %s", w.Body.String())
	}
}

func TestForgotPasswordHandlerMissingFields(t *testing.T) {
	mock := &mockAuthService{}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"test@example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.ForgotPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestForgotPasswordHandlerServiceError(t *testing.T) {
	mock := &mockAuthService{
		forgotPassFn: func(_ context.Context, _ payload.ForgotPasswordRequest, _, _ string) error {
			return apperror.ErrInternal
		},
	}
	h := NewAuthHandler(mock, testLog(t))

	body := `{"email":"test@example.com","currentPassword":"oldPass1","newPassword":"newPass1"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/forgot-password", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.ForgotPassword(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}
