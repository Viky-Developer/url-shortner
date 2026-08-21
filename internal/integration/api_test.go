package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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

func testCfg() *config.Config {
	return &config.Config{
		UserIDSecretKey:    "test-secret-key",
		JWTSecretKey:       "test-jwt-secret-key-for-middleware-32c",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 7 * 24 * time.Hour,
	}
}

func newLog(t *testing.T) logger.Logger {
	t.Helper()
	l, err := logger.New(logger.WithLevel("error"))
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return l
}

func genToken(t *testing.T, encodedUserID string, sessionID int64) string {
	t.Helper()
	tok, err := service.GenerateTestToken(testCfg(), encodedUserID, sessionID)
	if err != nil {
		t.Fatalf("genToken: %v", err)
	}
	return tok
}

// mockAuthService implements handler.AuthService
type mockAuth struct {
	registerFn   func(context.Context, payload.RegisterRequest, string, string) (*payload.AuthResponse, error)
	loginFn      func(context.Context, payload.LoginRequest, string, string, string, string) (*payload.AuthResponse, error)
	forgotPassFn func(context.Context, payload.ForgotPasswordRequest, string, string) (*payload.AuthResponse, error)
	refreshFn    func(context.Context, string) (*payload.AuthResponse, error)
	logoutFn     func(context.Context, string) error
	listSessFn   func(context.Context, int64) ([]payload.SessionResponse, error)
	revokeFn     func(context.Context, int64, int64) error
	updatePassFn func(context.Context, int64, payload.UpdatePasswordRequest, string, string) (*payload.UpdatePasswordResponse, error)
}

func (m *mockAuth) Register(_ context.Context, req payload.RegisterRequest, _, _ string) (*payload.AuthResponse, error) {
	if m.registerFn != nil {
		return m.registerFn(context.Background(), req, "", "")
	}
	if req.Email == "dup@example.com" {
		return nil, fmt.Errorf("email already registered")
	}
	return &payload.AuthResponse{
		AccessToken: "mock-at", RefreshToken: "mock-rt",
		User: payload.UserResponse{ID: "USR_test", Email: req.Email, DisplayName: req.DisplayName},
	}, nil
}
func (m *mockAuth) Login(_ context.Context, req payload.LoginRequest, _, _, _, _ string) (*payload.AuthResponse, error) {
	if m.loginFn != nil {
		return m.loginFn(context.Background(), req, "", "", "", "")
	}
	if req.Email == "wrong@example.com" || req.Email == "x@x.com" {
		return nil, fmt.Errorf("invalid credentials")
	}
	return &payload.AuthResponse{
		AccessToken: "mock-at", RefreshToken: "mock-rt",
		User: payload.UserResponse{ID: "USR_test", Email: req.Email},
	}, nil
}
func (m *mockAuth) ForgotPassword(_ context.Context, req payload.ForgotPasswordRequest, _, _ string) (*payload.AuthResponse, error) {
	if m.forgotPassFn != nil {
		return m.forgotPassFn(context.Background(), req, "", "")
	}
	return &payload.AuthResponse{
		AccessToken: "mock-at", RefreshToken: "mock-rt",
		User: payload.UserResponse{ID: "USR_test", Email: req.Email},
	}, nil
}
func (m *mockAuth) RefreshToken(_ context.Context, rt string) (*payload.AuthResponse, error) {
	if m.refreshFn != nil {
		return m.refreshFn(context.Background(), rt)
	}
	if rt == "expired" || rt == "bad-token" || rt == "invalid" {
		return nil, fmt.Errorf("invalid or expired refresh token")
	}
	return &payload.AuthResponse{
		AccessToken: "new-at", RefreshToken: "new-rt",
		User: payload.UserResponse{ID: "USR_test", Email: "test@example.com"},
	}, nil
}
func (m *mockAuth) Logout(_ context.Context, rt string) error {
	if m.logoutFn != nil {
		return m.logoutFn(context.Background(), rt)
	}
	if rt == "bad-token" || rt == "invalid" {
		return fmt.Errorf("invalid refresh token")
	}
	return nil
}
func (m *mockAuth) ListSessions(_ context.Context, _ int64) ([]payload.SessionResponse, error) {
	if m.listSessFn != nil {
		return m.listSessFn(context.Background(), 0)
	}
	return []payload.SessionResponse{}, nil
}
func (m *mockAuth) RevokeSession(_ context.Context, _, _ int64) error {
	if m.revokeFn != nil {
		return m.revokeFn(context.Background(), 0, 0)
	}
	return nil
}

func (m *mockAuth) UpdatePassword(_ context.Context, _ int64, _ payload.UpdatePasswordRequest, _, _ string) (*payload.UpdatePasswordResponse, error) {
	if m.updatePassFn != nil {
		return m.updatePassFn(context.Background(), 0, payload.UpdatePasswordRequest{}, "", "")
	}
	return &payload.UpdatePasswordResponse{Message: "password updated successfully"}, nil
}

// mockURLService implements handler.URLService
type mockURL struct {
	createFn   func(context.Context, int64, payload.CreateURLRequest) (*payload.URLResponse, error)
	redirectFn func(context.Context, string, payload.ClickInfo) (*payload.URLResponse, error)
	byIDFn     func(context.Context, int64, int64) (*payload.URLResponse, error)
	listFn     func(context.Context, int64, int32, int32, int32) (*payload.URLListResponse, error)
	updateFn   func(context.Context, int64, int64, payload.UpdateURLRequest) (*payload.URLResponse, error)
	softFn     func(context.Context, int64, int64) (*payload.DeleteResponse, error)
	hardFn     func(context.Context, int64, int64) error
}

func (m *mockURL) Create(_ context.Context, _ int64, req payload.CreateURLRequest) (*payload.URLResponse, error) {
	if m.createFn != nil {
		return m.createFn(context.Background(), 0, req)
	}
	return &payload.URLResponse{
		ID: 1, UserID: "USR_test", ShortCode: "abc1234567",
		OriginalURL: req.OriginalURL, ShortURL: "http://localhost:8080/abc1234567",
		IsActive: true,
	}, nil
}
func (m *mockURL) Redirect(_ context.Context, _ string, _ payload.ClickInfo) (*payload.URLResponse, error) {
	if m.redirectFn != nil {
		return m.redirectFn(context.Background(), "", payload.ClickInfo{})
	}
	return &payload.URLResponse{OriginalURL: "https://example.com/target"}, nil
}
func (m *mockURL) GetByID(_ context.Context, _ int64, id int64) (*payload.URLResponse, error) {
	if m.byIDFn != nil {
		return m.byIDFn(context.Background(), 0, id)
	}
	return &payload.URLResponse{ID: id, UserID: "USR_test", ShortCode: "abc1234567", IsActive: true}, nil
}
func (m *mockURL) List(_ context.Context, _ int64, page, perPage, _ int32) (*payload.URLListResponse, error) {
	if m.listFn != nil {
		return m.listFn(context.Background(), 0, page, perPage, 0)
	}
	return &payload.URLListResponse{
		Items: []payload.URLResponse{{ID: 1, ShortCode: "abc1234567", OriginalURL: "https://example.com", IsActive: true}},
		Total: 1, Page: 1, PerPage: 10, TotalPages: 1,
	}, nil
}
func (m *mockURL) Update(_ context.Context, _ int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error) {
	if m.updateFn != nil {
		return m.updateFn(context.Background(), 0, id, req)
	}
	return &payload.URLResponse{ID: id, UserID: "USR_test", ShortCode: "abc1234567", IsActive: true}, nil
}
func (m *mockURL) SoftDelete(_ context.Context, _ int64, id int64) (*payload.DeleteResponse, error) {
	if m.softFn != nil {
		return m.softFn(context.Background(), 0, id)
	}
	return &payload.DeleteResponse{ID: id, ShortCode: "abc1234567", Message: "soft deleted"}, nil
}
func (m *mockURL) HardDelete(_ context.Context, _ int64, _ int64) error {
	if m.hardFn != nil {
		return m.hardFn(context.Background(), 0, 0)
	}
	return nil
}

func (m *mockURL) ListClickLogs(_ context.Context, _, _ int64, _, _ *time.Time, _, _, _ int32) (*payload.ClickLogsResponse, error) {
	return &payload.ClickLogsResponse{Items: []payload.ClickLogEntry{}, Total: 0}, nil
}

func (m *mockURL) GetAnalytics(_ context.Context, _, _ int64, _, _ *time.Time) (*payload.AnalyticsResponse, error) {
	return &payload.AnalyticsResponse{Stats: payload.ClickStats{}}, nil
}

// alwaysActiveCache returns "1" for session_status so ValidateSession
// always sees the session as active without hitting DB.
type alwaysActiveCache struct{}

func (alwaysActiveCache) HGet(_, field string) (string, error) {
	if field == "session_status" {
		return "1", nil
	}
	return "", fmt.Errorf("noop")
}
func (alwaysActiveCache) HSet(string, string, any) error                       { return nil }
func (alwaysActiveCache) HSetFields(string, map[string]any) error              { return nil }
func (alwaysActiveCache) HSetWithTTL(string, string, any, time.Duration) error { return nil }
func (alwaysActiveCache) HDel(string, ...string) error                         { return nil }

func buildMux(t *testing.T, auth handler.AuthService, url handler.URLService) http.Handler {
	t.Helper()
	l := newLog(t)
	ah := handler.NewAuthHandler(auth, l)
	uh := handler.NewURLHandler(url, l)
	realAuth := service.NewAuthService(nil, nil, testCfg(), alwaysActiveCache{}, l)
	return routes.New(uh, ah, realAuth)
}

type apiCase struct {
	name           string
	method         string
	path           string
	body           any
	rawBody        string // when rawBody is set it overrides body
	headers        map[string]string
	wantStatus     int
	wantContains   []string // substrings expected in response body
	wantNotContain []string // substrings NOT expected in response body
}

func runCases(t *testing.T, mux http.Handler, cases []apiCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			fmt.Printf("\n  %s %s — %s", tc.method, tc.path, tc.name)
			fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

			var reqBody *bytes.Buffer
			if tc.rawBody != "" {
				reqBody = bytes.NewBufferString(tc.rawBody)
				fmt.Printf("\n  >>> RAW BODY:\n  %s\n", tc.rawBody)
			} else if tc.body != nil {
				data, _ := json.Marshal(tc.body)
				reqBody = bytes.NewBuffer(data)
				fmt.Printf("\n  >>> REQUEST BODY:\n  %s\n", indented(data))
			} else {
				reqBody = bytes.NewBuffer(nil)
			}

			req := httptest.NewRequest(tc.method, tc.path, reqBody)
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			bodyStr := w.Body.String()
			fmt.Printf("\n  <<< STATUS: %d (expected %d)\n", w.Code, tc.wantStatus)
			if bodyStr != "" {
				fmt.Printf("  <<< RESPONSE BODY:\n  %s\n", indented([]byte(bodyStr)))
			}

			// Check status
			if w.Code != tc.wantStatus {
				t.Errorf("STATUS: got %d, want %d", w.Code, tc.wantStatus)
			}

			// Check contains
			for _, sub := range tc.wantContains {
				if !strings.Contains(bodyStr, sub) {
					t.Errorf("RESPONSE missing %q", sub)
				}
			}

			// Check not contains
			for _, sub := range tc.wantNotContain {
				if strings.Contains(bodyStr, sub) {
					t.Errorf("RESPONSE should not contain %q", sub)
				}
			}
		})
	}
}

func indented(data []byte) string {
	var buf bytes.Buffer
	if json.Indent(&buf, data, "  ", "  ") == nil {
		return buf.String()
	}
	return string(data)
}

// ════════════════════════════════════════════════════════════════════
//  API 1: POST /api/v1/auth/register
// ════════════════════════════════════════════════════════════════════

func TestAPI_Register(t *testing.T) {
	runCases(t, buildMux(t, &mockAuth{}, nil), []apiCase{
		{
			name:       "success — all fields provided",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			body:       map[string]string{"email": "alice@example.com", "password": "securePass1", "displayName": "Alice"},
			wantStatus: http.StatusCreated,
			wantContains: []string{
				`"statusCode":201`,
				`"message":"user registered"`,
				`"accessToken"`,
				`"refreshToken"`,
				`"id":"USR_test"`,
				`"email":"alice@example.com"`,
				`"displayName":"Alice"`,
			},
		},
		{
			name:       "success — displayName is optional",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			body:       map[string]string{"email": "bob@example.com", "password": "pass123"},
			wantStatus: http.StatusCreated,
			wantContains: []string{
				`"statusCode":201`,
				`"email":"bob@example.com"`,
			},
		},
		{
			name:       "fail — empty body",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			rawBody:    "",
			wantStatus: http.StatusBadRequest,
			wantContains: []string{
				`"statusCode":400`,
			},
		},
		{
			name:       "fail — invalid JSON",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			rawBody:    `{not json}`,
			wantStatus: http.StatusBadRequest,
			wantContains: []string{
				`"statusCode":400`,
			},
		},
		{
			name:       "fail — missing email",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			body:       map[string]string{"password": "pass123"},
			wantStatus: http.StatusBadRequest,
			wantContains: []string{
				`"statusCode":400`,
				`email and password are required`,
			},
		},
		{
			name:       "fail — missing password",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			body:       map[string]string{"email": "a@b.com"},
			wantStatus: http.StatusBadRequest,
			wantContains: []string{
				`"statusCode":400`,
				`email and password are required`,
			},
		},
		{
			name:       "fail — empty email string",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			body:       map[string]string{"email": "", "password": "pass123"},
			wantStatus: http.StatusBadRequest,
			wantContains: []string{
				`"statusCode":400`,
				`email and password are required`,
			},
		},
		{
			name:       "fail — extra fields ignored, missing required",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			body:       map[string]string{"phone": "+1234567890"},
			wantStatus: http.StatusBadRequest,
			wantContains: []string{
				`"statusCode":400`,
			},
		},
		{
			name:       "fail — wrong HTTP method GET",
			method:     http.MethodGet,
			path:       "/api/v1/auth/register",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "fail — wrong HTTP method PUT",
			method:     http.MethodPut,
			path:       "/api/v1/auth/register",
			rawBody:    `{"email":"a@b.com","password":"x"}`,
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "fail — service returns error (duplicate email)",
			method:     http.MethodPost,
			path:       "/api/v1/auth/register",
			body:       map[string]string{"email": "dup@example.com", "password": "pass123"},
			wantStatus: http.StatusInternalServerError,
			wantContains: []string{
				`"statusCode":500`,
			},
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 2: POST /api/v1/auth/login
// ════════════════════════════════════════════════════════════════════

func TestAPI_Login(t *testing.T) {
	runCases(t, buildMux(t, &mockAuth{}, nil), []apiCase{
		{
			name:       "success — valid credentials",
			method:     http.MethodPost,
			path:       "/api/v1/auth/login",
			body:       map[string]string{"email": "alice@example.com", "password": "securePass1"},
			wantStatus: http.StatusOK,
			wantContains: []string{
				`"statusCode":200`,
				`"message":"login successful"`,
				`"accessToken"`,
				`"refreshToken"`,
				`"id":"USR_test"`,
				`"email":"alice@example.com"`,
			},
		},
		{
			name:       "fail — empty body",
			method:     http.MethodPost,
			path:       "/api/v1/auth/login",
			rawBody:    "",
			wantStatus: http.StatusBadRequest,
			wantContains: []string{
				`"statusCode":400`,
			},
		},
		{
			name:         "fail — invalid JSON",
			method:       http.MethodPost,
			path:         "/api/v1/auth/login",
			rawBody:      `{"email": "a@b.com", }`,
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — missing email",
			method:       http.MethodPost,
			path:         "/api/v1/auth/login",
			body:         map[string]string{"password": "pass123"},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`, `email and password are required`},
		},
		{
			name:         "fail — missing password",
			method:       http.MethodPost,
			path:         "/api/v1/auth/login",
			body:         map[string]string{"email": "a@b.com"},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`, `email and password are required`},
		},
		{
			name:         "fail — both empty",
			method:       http.MethodPost,
			path:         "/api/v1/auth/login",
			body:         map[string]string{"email": "", "password": ""},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`, `email and password are required`},
		},
		{
			name:         "fail — service returns invalid credentials",
			method:       http.MethodPost,
			path:         "/api/v1/auth/login",
			body:         map[string]string{"email": "wrong@example.com", "password": "wrong"},
			wantStatus:   http.StatusInternalServerError,
			wantContains: []string{`"statusCode":500`},
		},
		{
			name:       "fail — wrong method GET",
			method:     http.MethodGet,
			path:       "/api/v1/auth/login",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:         "fail — extra unknown fields, email present",
			method:       http.MethodPost,
			path:         "/api/v1/auth/login",
			body:         map[string]string{"email": "a@b.com", "password": "pass", "token": "extra", "foo": "bar"},
			wantStatus:   http.StatusOK,
			wantContains: []string{`"statusCode":200`},
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 3: POST /api/v1/auth/refresh
// ════════════════════════════════════════════════════════════════════

func TestAPI_RefreshToken(t *testing.T) {
	token := genToken(t, "USR_test", 1)

	runCases(t, buildMux(t, &mockAuth{}, nil), []apiCase{
		{
			name:       "success — valid refresh token",
			method:     http.MethodPost,
			path:       "/api/v1/auth/refresh",
			body:       map[string]string{"refreshToken": "valid-refresh-token-abc123"},
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusOK,
			wantContains: []string{
				`"statusCode":200`,
				`"message":"token refreshed"`,
				`"accessToken":"new-at"`,
				`"refreshToken":"new-rt"`,
			},
		},
		{
			name:         "fail — no auth header",
			method:       http.MethodPost,
			path:         "/api/v1/auth/refresh",
			body:         map[string]string{"refreshToken": "valid-refresh-token-abc123"},
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`, `authorization header required`},
		},
		{
			name:         "fail — empty body",
			method:       http.MethodPost,
			path:         "/api/v1/auth/refresh",
			rawBody:      "",
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — invalid JSON",
			method:       http.MethodPost,
			path:         "/api/v1/auth/refresh",
			rawBody:      `{bad json`,
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — missing refreshToken field",
			method:       http.MethodPost,
			path:         "/api/v1/auth/refresh",
			body:         map[string]string{},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`, `refresh token is required`},
		},
		{
			name:         "fail — refreshToken is empty string",
			method:       http.MethodPost,
			path:         "/api/v1/auth/refresh",
			body:         map[string]string{"refreshToken": ""},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`, `refresh token is required`},
		},
		{
			name:         "fail — wrong field name",
			method:       http.MethodPost,
			path:         "/api/v1/auth/refresh",
			body:         map[string]string{"token": "some-token"},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`, `refresh token is required`},
		},
		{
			name:       "fail — wrong method GET",
			method:     http.MethodGet,
			path:       "/api/v1/auth/refresh",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:         "fail — service error (expired token)",
			method:       http.MethodPost,
			path:         "/api/v1/auth/refresh",
			body:         map[string]string{"refreshToken": "expired"},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusInternalServerError,
			wantContains: []string{`"statusCode":500`},
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 4: POST /api/v1/auth/logout
// ════════════════════════════════════════════════════════════════════

func TestAPI_Logout(t *testing.T) {
	token := genToken(t, "USR_test", 1)

	runCases(t, buildMux(t, &mockAuth{}, nil), []apiCase{
		{
			name:       "success — valid refresh token",
			method:     http.MethodPost,
			path:       "/api/v1/auth/logout",
			body:       map[string]string{"refreshToken": "valid-refresh-token"},
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusOK,
			wantContains: []string{
				`"statusCode":200`,
				`"message":"logged out"`,
			},
		},
		{
			name:         "fail — no auth header",
			method:       http.MethodPost,
			path:         "/api/v1/auth/logout",
			body:         map[string]string{"refreshToken": "valid-refresh-token"},
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`, `authorization header required`},
		},
		{
			name:         "fail — empty body",
			method:       http.MethodPost,
			path:         "/api/v1/auth/logout",
			rawBody:      "",
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — invalid JSON",
			method:       http.MethodPost,
			path:         "/api/v1/auth/logout",
			rawBody:      `{not valid`,
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — missing refreshToken",
			method:       http.MethodPost,
			path:         "/api/v1/auth/logout",
			body:         map[string]string{},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`, `refresh token is required`},
		},
		{
			name:         "fail — empty refreshToken",
			method:       http.MethodPost,
			path:         "/api/v1/auth/logout",
			body:         map[string]string{"refreshToken": ""},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`, `refresh token is required`},
		},
		{
			name:       "fail — wrong method DELETE",
			method:     http.MethodDelete,
			path:       "/api/v1/auth/logout",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:         "fail — service error",
			method:       http.MethodPost,
			path:         "/api/v1/auth/logout",
			body:         map[string]string{"refreshToken": "bad-token"},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusInternalServerError,
			wantContains: []string{`"statusCode":500`},
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 5: GET /api/v1/auth/sessions (protected)
// ════════════════════════════════════════════════════════════════════

func TestAPI_ListSessions(t *testing.T) {
	token := genToken(t, "USR_test", 1)

	runCases(t, buildMux(t, &mockAuth{}, nil), []apiCase{
		{
			name:       "success — returns session list",
			method:     http.MethodGet,
			path:       "/api/v1/auth/sessions",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusOK,
			wantContains: []string{
				`"statusCode":200`,
				`"message":"sessions listed"`,
			},
		},
		{
			name:         "fail — no Authorization header",
			method:       http.MethodGet,
			path:         "/api/v1/auth/sessions",
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`, `authorization header required`},
		},
		{
			name:         "fail — invalid token",
			method:       http.MethodGet,
			path:         "/api/v1/auth/sessions",
			headers:      map[string]string{"Authorization": "Bearer garbage"},
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`, `invalid or expired token`},
		},
		{
			name:         "fail — wrong format Basic auth",
			method:       http.MethodGet,
			path:         "/api/v1/auth/sessions",
			headers:      map[string]string{"Authorization": "Basic dXNlcjpwYXNz"},
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`, `invalid authorization header format`},
		},
		{
			name:       "fail — wrong method POST",
			method:     http.MethodPost,
			path:       "/api/v1/auth/sessions",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:         "fail — empty Bearer value",
			method:       http.MethodGet,
			path:         "/api/v1/auth/sessions",
			headers:      map[string]string{"Authorization": "Bearer "},
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`},
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 6: DELETE /api/v1/auth/sessions/{id} (protected)
// ════════════════════════════════════════════════════════════════════

func TestAPI_RevokeSession(t *testing.T) {
	token := genToken(t, "USR_test", 1)

	runCases(t, buildMux(t, &mockAuth{}, nil), []apiCase{
		{
			name:       "success — revoke session",
			method:     http.MethodDelete,
			path:       "/api/v1/auth/sessions/1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusOK,
			wantContains: []string{
				`"statusCode":200`,
				`"message":"session revoked"`,
			},
		},
		{
			name:         "fail — no auth",
			method:       http.MethodDelete,
			path:         "/api/v1/auth/sessions/1",
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`},
		},
		{
			name:         "fail — invalid session id",
			method:       http.MethodDelete,
			path:         "/api/v1/auth/sessions/abc",
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 7: POST /api/v1/shorten (protected)
// ════════════════════════════════════════════════════════════════════

func TestAPI_CreateShortURL(t *testing.T) {
	token := genToken(t, "USR_test", 1)

	runCases(t, buildMux(t, &mockAuth{}, &mockURL{}), []apiCase{
		{
			name:       "success — minimal required field",
			method:     http.MethodPost,
			path:       "/api/v1/shorten",
			body:       map[string]string{"originalURL": "https://example.com"},
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusCreated,
			wantContains: []string{
				`"statusCode":201`,
				`"message":"url created"`,
				`"originalURL":"https://example.com"`,
				`"shortCode":"abc1234567"`,
				`"shortURL":"http://localhost:8080/abc1234567"`,
				`"isActive":true`,
				`"clickCount":0`,
			},
		},
		{
			name:       "success — all optional fields",
			method:     http.MethodPost,
			path:       "/api/v1/shorten",
			body:       map[string]string{"originalURL": "https://example.com", "customCode": "mycode", "title": "My Link", "description": "A useful link"},
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusCreated,
			wantContains: []string{
				`"statusCode":201`,
				`"shortCode":"abc1234567"`,
			},
		},
		{
			name:         "success — only originalURL, no customCode",
			method:       http.MethodPost,
			path:         "/api/v1/shorten",
			body:         map[string]string{"originalURL": "https://go.dev/doc"},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusCreated,
			wantContains: []string{`"statusCode":201`},
		},
		{
			name:         "success — extra unknown fields ignored",
			method:       http.MethodPost,
			path:         "/api/v1/shorten",
			body:         map[string]string{"originalURL": "https://example.com", "randomField": "ignored", "another": "123"},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusCreated,
			wantContains: []string{`"statusCode":201`},
		},
		{
			name:         "fail — no auth token",
			method:       http.MethodPost,
			path:         "/api/v1/shorten",
			body:         map[string]string{"originalURL": "https://example.com"},
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`, `authorization header required`},
		},
		{
			name:         "fail — invalid token",
			method:       http.MethodPost,
			path:         "/api/v1/shorten",
			body:         map[string]string{"originalURL": "https://example.com"},
			headers:      map[string]string{"Authorization": "Bearer fake-token"},
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`, `invalid or expired token`},
		},
		{
			name:         "fail — empty body",
			method:       http.MethodPost,
			path:         "/api/v1/shorten",
			rawBody:      "",
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — invalid JSON",
			method:       http.MethodPost,
			path:         "/api/v1/shorten",
			rawBody:      `{"originalURL": }`,
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — empty JSON object",
			method:       http.MethodPost,
			path:         "/api/v1/shorten",
			body:         map[string]string{},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — missing originalURL",
			method:       http.MethodPost,
			path:         "/api/v1/shorten",
			body:         map[string]string{"title": "Only Title"},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — originalURL is empty string",
			method:       http.MethodPost,
			path:         "/api/v1/shorten",
			body:         map[string]string{"originalURL": ""},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:       "fail — wrong method GET (redirects as short code)",
			method:     http.MethodGet,
			path:       "/api/v1/shorten",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusFound, // 302 - treats "shorten" as short code
			wantContains: []string{
				`href="https://example.com/target"`,
			},
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 8: GET /api/v1/urls (protected)
// ════════════════════════════════════════════════════════════════════

func TestAPI_ListURLs(t *testing.T) {
	token := genToken(t, "USR_test", 1)

	runCases(t, buildMux(t, &mockAuth{}, &mockURL{}), []apiCase{
		{
			name:       "success — default pagination",
			method:     http.MethodGet,
			path:       "/api/v1/urls",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusOK,
			wantContains: []string{
				`"statusCode":200`,
				`"total":1`,
				`"page":1`,
				`"perPage":10`,
				`"data"`,
				`"shortCode":"abc1234567"`,
			},
		},
		{
			name:         "success — custom page params",
			method:       http.MethodGet,
			path:         "/api/v1/urls?page=2&perPage=5",
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusOK,
			wantContains: []string{`"statusCode":200`},
		},
		{
			name:         "fail — no auth",
			method:       http.MethodGet,
			path:         "/api/v1/urls",
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`},
		},
		{
			name:       "fail — wrong method POST",
			method:     http.MethodPost,
			path:       "/api/v1/urls",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusMethodNotAllowed,
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 9: GET /api/v1/urls/{id} (protected)
// ════════════════════════════════════════════════════════════════════

func TestAPI_GetURLByID(t *testing.T) {
	token := genToken(t, "USR_test", 1)

	runCases(t, buildMux(t, &mockAuth{}, &mockURL{}), []apiCase{
		{
			name:       "success — valid numeric id",
			method:     http.MethodGet,
			path:       "/api/v1/urls/1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusOK,
			wantContains: []string{
				`"statusCode":200`,
				`"id":1`,
				`"shortCode":"abc1234567"`,
			},
		},
		{
			name:         "fail — non-numeric id",
			method:       http.MethodGet,
			path:         "/api/v1/urls/abc",
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:       "fail — empty id segment (404 no route)",
			method:     http.MethodGet,
			path:       "/api/v1/urls/",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusNotFound,
			wantContains: []string{
				`404 page not found`,
			},
		},
		{
			name:         "fail — no auth",
			method:       http.MethodGet,
			path:         "/api/v1/urls/1",
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`},
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 10: PATCH /api/v1/urls/{id} (protected)
// ════════════════════════════════════════════════════════════════════

func TestAPI_UpdateURL(t *testing.T) {
	token := genToken(t, "USR_test", 1)

	runCases(t, buildMux(t, &mockAuth{}, &mockURL{}), []apiCase{
		{
			name:       "success — update originalURL",
			method:     http.MethodPatch,
			path:       "/api/v1/urls/1",
			body:       map[string]string{"originalURL": "https://updated.com"},
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusOK,
			wantContains: []string{
				`"statusCode":200`,
				`"message":"url updated"`,
			},
		},
		{
			name:         "success — update only title",
			method:       http.MethodPatch,
			path:         "/api/v1/urls/1",
			body:         map[string]string{"title": "New Title"},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusOK,
			wantContains: []string{`"statusCode":200`},
		},
		{
			name:         "success — update only description",
			method:       http.MethodPatch,
			path:         "/api/v1/urls/1",
			body:         map[string]string{"description": "New desc"},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusOK,
			wantContains: []string{`"statusCode":200`},
		},
		{
			name:         "success — update all fields",
			method:       http.MethodPatch,
			path:         "/api/v1/urls/1",
			body:         map[string]any{"originalURL": "https://new.com", "title": "T", "description": "D", "status": 1},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusOK,
			wantContains: []string{`"statusCode":200`},
		},
		{
			name:         "success — empty body (all optional)",
			method:       http.MethodPatch,
			path:         "/api/v1/urls/1",
			body:         map[string]string{},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusOK,
			wantContains: []string{`"statusCode":200`},
		},
		{
			name:         "fail — invalid JSON",
			method:       http.MethodPatch,
			path:         "/api/v1/urls/1",
			rawBody:      `{broken`,
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — non-numeric id",
			method:       http.MethodPatch,
			path:         "/api/v1/urls/abc",
			body:         map[string]string{"title": "X"},
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — no auth",
			method:       http.MethodPatch,
			path:         "/api/v1/urls/1",
			body:         map[string]string{"title": "X"},
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`},
		},
		{
			name:       "fail — wrong method PUT",
			method:     http.MethodPut,
			path:       "/api/v1/urls/1",
			body:       map[string]string{"title": "X"},
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusMethodNotAllowed,
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 11: DELETE /api/v1/urls/{id} (protected — soft delete)
// ════════════════════════════════════════════════════════════════════

func TestAPI_DeleteURL(t *testing.T) {
	token := genToken(t, "USR_test", 1)

	runCases(t, buildMux(t, &mockAuth{}, &mockURL{}), []apiCase{
		{
			name:       "success — soft delete",
			method:     http.MethodDelete,
			path:       "/api/v1/urls/1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusOK,
			wantContains: []string{
				`"statusCode":200`,
				`"message":"url soft deleted"`,
				`"id":1`,
				`"shortCode":"abc1234567"`,
			},
		},
		{
			name:         "fail — non-numeric id",
			method:       http.MethodDelete,
			path:         "/api/v1/urls/abc",
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — no auth",
			method:       http.MethodDelete,
			path:         "/api/v1/urls/1",
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`},
		},
		{
			name:       "fail — wrong method POST",
			method:     http.MethodPost,
			path:       "/api/v1/urls/1",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusMethodNotAllowed,
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 12: DELETE /api/v1/urls/{id}/approve (protected — hard delete)
// ════════════════════════════════════════════════════════════════════

func TestAPI_ApproveHardDelete(t *testing.T) {
	token := genToken(t, "USR_test", 1)

	runCases(t, buildMux(t, &mockAuth{}, &mockURL{}), []apiCase{
		{
			name:       "success — approve hard delete",
			method:     http.MethodDelete,
			path:       "/api/v1/urls/1/approve",
			headers:    map[string]string{"Authorization": "Bearer " + token},
			wantStatus: http.StatusOK,
			wantContains: []string{
				`"statusCode":200`,
				`"message":"url permanently deleted"`,
			},
		},
		{
			name:         "fail — non-numeric id",
			method:       http.MethodDelete,
			path:         "/api/v1/urls/abc/approve",
			headers:      map[string]string{"Authorization": "Bearer " + token},
			wantStatus:   http.StatusBadRequest,
			wantContains: []string{`"statusCode":400`},
		},
		{
			name:         "fail — no auth",
			method:       http.MethodDelete,
			path:         "/api/v1/urls/1/approve",
			wantStatus:   http.StatusUnauthorized,
			wantContains: []string{`"statusCode":401`},
		},
	})
}

// ════════════════════════════════════════════════════════════════════
//  API 13: GET /api/v1/{shortCode} (public redirect)
// ════════════════════════════════════════════════════════════════════

func TestAPI_Redirect(t *testing.T) {
	mux := buildMux(t, &mockAuth{}, &mockURL{})

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  GET /api/v1/abc — public redirect")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/abc", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	fmt.Printf("\n  <<< STATUS: %d\n", w.Code)
	fmt.Printf("  <<< LOCATION: %s\n", w.Header().Get("Location"))

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "https://example.com/target" {
		t.Errorf("expected redirect to https://example.com/target, got %q", loc)
	}
}

// ════════════════════════════════════════════════════════════════════
//  FULL FLOW: Register → Login → Create → List → Refresh → Logout
// ════════════════════════════════════════════════════════════════════

func TestAPI_FullFlow(t *testing.T) {
	token := genToken(t, "USR_test", 1)
	mux := buildMux(t, &mockAuth{}, &mockURL{})

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  FULL FLOW: Register → Login → Create → List")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	type step struct {
		name       string
		method     string
		path       string
		body       any
		headers    map[string]string
		wantStatus int
	}

	steps := []step{
		{"1. Register", http.MethodPost, "/api/v1/auth/register", map[string]string{"email": "flow@test.com", "password": "pass123", "displayName": "Flow Test"}, nil, http.StatusCreated},
		{"2. Login", http.MethodPost, "/api/v1/auth/login", map[string]string{"email": "flow@test.com", "password": "pass123"}, nil, http.StatusOK},
		{"3. Create Short URL", http.MethodPost, "/api/v1/shorten", map[string]string{"originalURL": "https://example.com/page"}, map[string]string{"Authorization": "Bearer " + token}, http.StatusCreated},
		{"4. List URLs", http.MethodGet, "/api/v1/urls", nil, map[string]string{"Authorization": "Bearer " + token}, http.StatusOK},
		{"5. Get URL by ID", http.MethodGet, "/api/v1/urls/1", nil, map[string]string{"Authorization": "Bearer " + token}, http.StatusOK},
		{"6. Update URL", http.MethodPatch, "/api/v1/urls/1", map[string]string{"title": "Updated"}, map[string]string{"Authorization": "Bearer " + token}, http.StatusOK},
		{"7. Refresh Token", http.MethodPost, "/api/v1/auth/refresh", map[string]string{"refreshToken": "valid-rt"}, map[string]string{"Authorization": "Bearer " + token}, http.StatusOK},
		{"8. List Sessions", http.MethodGet, "/api/v1/auth/sessions", nil, map[string]string{"Authorization": "Bearer " + token}, http.StatusOK},
		{"9. Soft Delete URL", http.MethodDelete, "/api/v1/urls/1", nil, map[string]string{"Authorization": "Bearer " + token}, http.StatusOK},
		{"10. Hard Delete URL", http.MethodDelete, "/api/v1/urls/1/approve", nil, map[string]string{"Authorization": "Bearer " + token}, http.StatusOK},
		{"11. Logout", http.MethodPost, "/api/v1/auth/logout", map[string]string{"refreshToken": "valid-rt"}, map[string]string{"Authorization": "Bearer " + token}, http.StatusOK},
	}

	for _, s := range steps {
		fmt.Printf("\n  ── %s ──\n", s.name)

		var reqBody *bytes.Buffer
		if s.body != nil {
			data, _ := json.Marshal(s.body)
			reqBody = bytes.NewBuffer(data)
			fmt.Printf("  >>> %s %s\n  BODY: %s\n", s.method, s.path, string(data))
		} else {
			reqBody = bytes.NewBuffer(nil)
			fmt.Printf("  >>> %s %s\n  BODY: (none)\n", s.method, s.path)
		}

		req := httptest.NewRequest(s.method, s.path, reqBody)
		req.Header.Set("Content-Type", "application/json")
		for k, v := range s.headers {
			req.Header.Set(k, v)
			fmt.Printf("  HEADER: %s: %s\n", k, v)
		}

		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)

		fmt.Printf("  <<< STATUS: %d (expected %d)\n", w.Code, s.wantStatus)
		if w.Body.Len() > 0 {
			fmt.Printf("  <<< BODY: %s\n", indented(w.Body.Bytes()))
		}

		if w.Code != s.wantStatus {
			t.Errorf("%s: got %d, want %d", s.name, w.Code, s.wantStatus)
		}
	}
}
