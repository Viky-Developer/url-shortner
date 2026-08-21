package handler

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/contextutil"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/utils"
)

type mockService struct {
	createFn     func(context.Context, int64, payload.CreateURLRequest) (*payload.URLResponse, error)
	redirectFn   func(context.Context, string, payload.ClickInfo) (*payload.URLResponse, error)
	byIDFn       func(context.Context, int64, int64) (*payload.URLResponse, error)
	listFn       func(context.Context, int64, int32, int32, int32) (*payload.URLListResponse, error)
	updateFn     func(context.Context, int64, int64, payload.UpdateURLRequest) (*payload.URLResponse, error)
	softDeleteFn func(context.Context, int64, int64) (*payload.DeleteResponse, error)
	hardDeleteFn func(context.Context, int64, int64) error
}

func (m *mockService) Create(ctx context.Context, userID int64, req payload.CreateURLRequest) (*payload.URLResponse, error) {
	return m.createFn(ctx, userID, req)
}

func (m *mockService) Redirect(ctx context.Context, code string, click payload.ClickInfo) (*payload.URLResponse, error) {
	return m.redirectFn(ctx, code, click)
}

func (m *mockService) GetByID(ctx context.Context, userID int64, id int64) (*payload.URLResponse, error) {
	return m.byIDFn(ctx, userID, id)
}

func (m *mockService) List(ctx context.Context, userID int64, page, perPage, offset int32) (*payload.URLListResponse, error) {
	return m.listFn(ctx, userID, page, perPage, offset)
}

func (m *mockService) Update(ctx context.Context, userID int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error) {
	return m.updateFn(ctx, userID, id, req)
}

func (m *mockService) SoftDelete(ctx context.Context, userID int64, id int64) (*payload.DeleteResponse, error) {
	return m.softDeleteFn(ctx, userID, id)
}

func (m *mockService) HardDelete(ctx context.Context, userID int64, id int64) error {
	return m.hardDeleteFn(ctx, userID, id)
}

func testLog(t *testing.T) logger.Logger {
	t.Helper()
	log, err := logger.New(logger.WithLevel("error"))
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

// withUserID injects the authenticated user ID into the request context,
// simulating what the auth middleware does.
func withUserID(r *http.Request, userID int64) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), contextutil.UserIDKey, userID))
}

// stubLookupIP replaces DNS resolution with a fixed public IP so tests do not
// hit the real network, and restores it after the test.
func stubLookupIP(t *testing.T) {
	t.Helper()
	orig := lookupIP
	lookupIP = func(_ string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	t.Cleanup(func() { lookupIP = orig })
}

func sampleResponse() *payload.URLResponse {
	now := time.Now()
	return &payload.URLResponse{
		ID:          1,
		UserID:      "USR_test123",
		ShortCode:   "abc1234567",
		OriginalURL: "https://example.com/original",
		ShortURL:    "http://localhost:8080/abc1234567",
		IsActive:    true,
		CreatedAt:   now.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   now.Format("2006-01-02T15:04:05Z"),
	}
}

func TestCreateShortURLHandler(t *testing.T) {
	stubLookupIP(t)
	mock := &mockService{
		createFn: func(_ context.Context, _ int64, _ payload.CreateURLRequest) (*payload.URLResponse, error) {
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL": "https://example.com/long-url"}`
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(body))
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"shortCode":"abc1234567"`) {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestCreateShortURLInvalidJSON(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString("{invalid"))
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateShortURLEmptyBody(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(""))
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "request body is required") {
		t.Errorf("expected invalid payload error, got %s", w.Body.String())
	}
}

func TestCreateShortURLValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing originalURL", body: `{}`},
		{name: "invalid format", body: `{"originalURL": "not-a-url"}`},
		{name: "missing scheme", body: `{"originalURL": "example.com/foo"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockService{}
			h := NewURLHandler(mock, testLog(t))

			req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(tt.body))
			req = withUserID(req, 1)
			w := httptest.NewRecorder()

			h.CreateShortURL(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", w.Code)
			}
			if !strings.Contains(w.Body.String(), `"statusCode":400`) {
				t.Errorf("expected validation error, got %s", w.Body.String())
			}
		})
	}
}

func TestCreateShortURLUnauthorized(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL": "https://example.com/long-url"}`
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestValidateURL(t *testing.T) {
	stubLookupIP(t)

	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{name: "valid https", url: "https://example.com/foo", wantErr: false},
		{name: "http scheme rejected", url: "http://example.com/foo", wantErr: true},
		{name: "missing url", url: "", wantErr: true},
		{name: "not a url", url: "not-a-url", wantErr: true},
		{name: "missing scheme", url: "example.com/foo", wantErr: true},
		{name: "local hostname", url: "https://localhost:8080/x", wantErr: true},
		{name: "local suffix", url: "https://api.internal/x", wantErr: true},
		{name: "lan suffix", url: "https://printer.lan/x", wantErr: true},
		{name: "home suffix", url: "https://nas.home/x", wantErr: true},
		{name: "loopback ip", url: "https://127.0.0.1/x", wantErr: true},
		{name: "ipv6 loopback", url: "https://[::1]/x", wantErr: true},
		{name: "private ip", url: "https://192.168.1.10/x", wantErr: true},
		{name: "link-local ip", url: "https://169.254.169.254/x", wantErr: true},
	}

	orig := lookupIP
	lookupIP = func(host string) ([]net.IP, error) {
		if host == "internal.local" || host == "10.0.0.5" {
			return []net.IP{net.ParseIP("10.0.0.5")}, nil
		}
		return []net.IP{net.ParseIP("93.184.216.34")}, nil
	}
	t.Cleanup(func() { lookupIP = orig })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.ValidateURL(tt.url, lookupIP)
			if tt.wantErr && err == nil {
				t.Errorf("expected error for %q, got nil", tt.url)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error for %q, got %v", tt.url, err)
			}
		})
	}
}

func TestValidateURLRejectsPrivateIPViaDNS(t *testing.T) {
	orig := lookupIP
	lookupIP = func(_ string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	}
	t.Cleanup(func() { lookupIP = orig })

	if err := utils.ValidateURL("https://somehost.example.com/x", lookupIP); err == nil {
		t.Fatal("expected error for host resolving to private IP")
	}
}

func TestValidateURLRejectsUnresolvableHost(t *testing.T) {
	orig := lookupIP
	lookupIP = func(_ string) ([]net.IP, error) {
		return nil, fmt.Errorf("no such host")
	}
	t.Cleanup(func() { lookupIP = orig })

	if err := utils.ValidateURL("https://nonexistent.example.com/x", lookupIP); err == nil {
		t.Fatal("expected error for unresolvable host")
	}
}

func TestRedirectShortURL(t *testing.T) {
	mock := &mockService{
		redirectFn: func(_ context.Context, _ string, _ payload.ClickInfo) (*payload.URLResponse, error) {
			return &payload.URLResponse{OriginalURL: "https://example.com/target"}, nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/abc1234567", nil)
	req.SetPathValue("shortCode", "abc1234567")
	w := httptest.NewRecorder()

	h.RedirectShortURL(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if w.Header().Get("Location") != "https://example.com/target" {
		t.Errorf("unexpected Location header: %s", w.Header().Get("Location"))
	}
}

func TestRedirectShortURLNotFound(t *testing.T) {
	mock := &mockService{
		redirectFn: func(_ context.Context, _ string, _ payload.ClickInfo) (*payload.URLResponse, error) {
			return nil, fmt.Errorf("%w: missing", apperror.ErrNotFound)
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.SetPathValue("shortCode", "missing")
	w := httptest.NewRecorder()

	h.RedirectShortURL(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetURLByID(t *testing.T) {
	mock := &mockService{
		byIDFn: func(_ context.Context, _ int64, _ int64) (*payload.URLResponse, error) {
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls/1", nil)
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.GetURLByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetURLByIDInvalid(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls/abc", nil)
	req = withUserID(req, 1)
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()

	h.GetURLByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestGetURLByIDUnauthorized(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.GetURLByID(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestListURLsDefaults(t *testing.T) {
	mock := &mockService{
		listFn: func(_ context.Context, _ int64, page, perPage, offset int32) (*payload.URLListResponse, error) {
			if page != 1 {
				t.Errorf("expected default page 1, got %d", page)
			}
			if perPage != 10 {
				t.Errorf("expected default perPage 10, got %d", perPage)
			}
			if offset != 0 {
				t.Errorf("expected default offset 0, got %d", offset)
			}
			return &payload.URLListResponse{Items: []payload.URLResponse{*sampleResponse()}, Total: 1, Page: 1, PerPage: 10, TotalPages: 1}, nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls", nil)
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.ListURLs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestUpdateURL(t *testing.T) {
	stubLookupIP(t)
	mock := &mockService{
		updateFn: func(_ context.Context, _ int64, _ int64, _ payload.UpdateURLRequest) (*payload.URLResponse, error) {
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL": "https://example.com/updated"}`
	req := httptest.NewRequest(http.MethodPatch, "/urls/1", bytes.NewBufferString(body))
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.UpdateURL(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteURLSoftDelete(t *testing.T) {
	mock := &mockService{
		softDeleteFn: func(_ context.Context, _ int64, _ int64) (*payload.DeleteResponse, error) {
			return &payload.DeleteResponse{ID: 1, ShortCode: "abc1234567", Message: "soft deleted"}, nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/1", nil)
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.DeleteURL(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "soft deleted") {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestApproveHardDelete(t *testing.T) {
	mock := &mockService{
		hardDeleteFn: func(_ context.Context, _ int64, _ int64) error {
			return nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/1/approve", nil)
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.ApproveHardDelete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ═══════════════════════════════════════════════════════════════
//  CREATE — additional edge cases
// ═══════════════════════════════════════════════════════════════

func TestCreateServiceConflict(t *testing.T) {
	mock := &mockService{
		createFn: func(_ context.Context, _ int64, _ payload.CreateURLRequest) (*payload.URLResponse, error) {
			return nil, apperror.ErrConflict
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL":"https://example.com","customCode":"taken"}`
	req := httptest.NewRequest(http.MethodPost, "/urls", strings.NewReader(body))
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestCreateServiceInternalError(t *testing.T) {
	mock := &mockService{
		createFn: func(_ context.Context, _ int64, _ payload.CreateURLRequest) (*payload.URLResponse, error) {
			return nil, apperror.ErrInternal
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL":"https://example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/urls", strings.NewReader(body))
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestCreateBlockedDomainError(t *testing.T) {
	mock := &mockService{
		createFn: func(_ context.Context, _ int64, _ payload.CreateURLRequest) (*payload.URLResponse, error) {
			return nil, apperror.ErrBlockedDomain
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL":"https://blocked.example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/urls", strings.NewReader(body))
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreatePassesAllFields(t *testing.T) {
	var captured payload.CreateURLRequest
	mock := &mockService{
		createFn: func(_ context.Context, _ int64, req payload.CreateURLRequest) (*payload.URLResponse, error) {
			captured = req
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL":"https://example.com","customCode":"abc","title":"My URL","description":"desc","expiresAt":"2099-01-01T00:00:00Z"}`
	req := httptest.NewRequest(http.MethodPost, "/urls", strings.NewReader(body))
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if captured.OriginalURL != "https://example.com" {
		t.Errorf("originalURL = %q", captured.OriginalURL)
	}
	if captured.CustomCode != "abc" {
		t.Errorf("customCode = %q", captured.CustomCode)
	}
	if captured.Title != "My URL" {
		t.Errorf("title = %q", captured.Title)
	}
	if captured.Description != "desc" {
		t.Errorf("description = %q", captured.Description)
	}
	if !captured.ExpiresAt.Valid {
		t.Error("expiresAt should be valid")
	}
}

func TestCreateSuccessReturns201(t *testing.T) {
	mock := &mockService{
		createFn: func(_ context.Context, _ int64, _ payload.CreateURLRequest) (*payload.URLResponse, error) {
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL":"https://example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/urls", strings.NewReader(body))
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
}

// ═══════════════════════════════════════════════════════════════
//  REDIRECT — click info and error paths
// ═══════════════════════════════════════════════════════════════

func TestRedirectPassesClickInfo(t *testing.T) {
	var capturedClick payload.ClickInfo
	mock := &mockService{
		redirectFn: func(_ context.Context, code string, click payload.ClickInfo) (*payload.URLResponse, error) {
			capturedClick = click
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	req.SetPathValue("shortCode", "abc")
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.Header.Set("Referer", "https://google.com")
	w := httptest.NewRecorder()

	h.RedirectShortURL(w, req)

	if capturedClick.UserAgent != "TestAgent/1.0" {
		t.Errorf("userAgent = %q, want %q", capturedClick.UserAgent, "TestAgent/1.0")
	}
	if capturedClick.Referrer != "https://google.com" {
		t.Errorf("referrer = %q, want %q", capturedClick.Referrer, "https://google.com")
	}
}

func TestRedirectServiceExpired(t *testing.T) {
	mock := &mockService{
		redirectFn: func(_ context.Context, _ string, _ payload.ClickInfo) (*payload.URLResponse, error) {
			return nil, apperror.ErrURLExpired
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	req.SetPathValue("shortCode", "abc")
	w := httptest.NewRecorder()

	h.RedirectShortURL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRedirectServiceNotFound(t *testing.T) {
	mock := &mockService{
		redirectFn: func(_ context.Context, _ string, _ payload.ClickInfo) (*payload.URLResponse, error) {
			return nil, apperror.ErrNotFound
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/xyz", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	req.SetPathValue("shortCode", "xyz")
	w := httptest.NewRecorder()

	h.RedirectShortURL(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestRedirectServiceInternalError(t *testing.T) {
	mock := &mockService{
		redirectFn: func(_ context.Context, _ string, _ payload.ClickInfo) (*payload.URLResponse, error) {
			return nil, apperror.ErrInternal
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	req.SetPathValue("shortCode", "abc")
	w := httptest.NewRecorder()

	h.RedirectShortURL(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestRedirectUsesXForwardedFor(t *testing.T) {
	var capturedIP net.IP
	mock := &mockService{
		redirectFn: func(_ context.Context, _ string, click payload.ClickInfo) (*payload.URLResponse, error) {
			capturedIP = click.IP
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	req.SetPathValue("shortCode", "abc")
	req.Header.Set("X-Forwarded-For", "10.20.30.40")
	w := httptest.NewRecorder()

	h.RedirectShortURL(w, req)

	if capturedIP == nil {
		t.Fatal("expected IP to be set")
	}
	if capturedIP.String() != "10.20.30.40" {
		t.Errorf("IP = %q, want %q", capturedIP.String(), "10.20.30.40")
	}
}

// ═══════════════════════════════════════════════════════════════
//  GET BY ID — error paths
// ═══════════════════════════════════════════════════════════════

func TestGetByIDNotFound(t *testing.T) {
	mock := &mockService{
		byIDFn: func(_ context.Context, _ int64, _ int64) (*payload.URLResponse, error) {
			return nil, apperror.ErrNotFound
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls/999", nil)
	req = withUserID(req, 1)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	h.GetURLByID(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetByIDInternalError(t *testing.T) {
	mock := &mockService{
		byIDFn: func(_ context.Context, _ int64, _ int64) (*payload.URLResponse, error) {
			return nil, apperror.ErrInternal
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls/1", nil)
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.GetURLByID(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestGetByIDPassesArgs(t *testing.T) {
	var capturedID int64
	var capturedUserID int64
	mock := &mockService{
		byIDFn: func(_ context.Context, userID int64, id int64) (*payload.URLResponse, error) {
			capturedUserID = userID
			capturedID = id
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls/42", nil)
	req = withUserID(req, 10)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	h.GetURLByID(w, req)

	if capturedUserID != 10 {
		t.Errorf("userID = %d, want 10", capturedUserID)
	}
	if capturedID != 42 {
		t.Errorf("id = %d, want 42", capturedID)
	}
}

// ═══════════════════════════════════════════════════════════════
//  LIST — pagination and error paths
// ═══════════════════════════════════════════════════════════════

func TestListCustomPagination(t *testing.T) {
	var capturedPage, capturedPerPage, capturedOffset int32
	mock := &mockService{
		listFn: func(_ context.Context, _ int64, page, perPage, offset int32) (*payload.URLListResponse, error) {
			capturedPage = page
			capturedPerPage = perPage
			capturedOffset = offset
			return &payload.URLListResponse{Items: []payload.URLResponse{}}, nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls?page=2&perPage=15", nil)
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.ListURLs(w, req)

	if capturedPage != 2 {
		t.Errorf("page = %d, want 2", capturedPage)
	}
	if capturedPerPage != 15 {
		t.Errorf("perPage = %d, want 15", capturedPerPage)
	}
	if capturedOffset != 15 {
		t.Errorf("offset = %d, want 15", capturedOffset)
	}
}

func TestListPerPageClamped(t *testing.T) {
	var capturedPerPage int32
	mock := &mockService{
		listFn: func(_ context.Context, _ int64, _, perPage, _ int32) (*payload.URLListResponse, error) {
			capturedPerPage = perPage
			return &payload.URLListResponse{Items: []payload.URLResponse{}}, nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls?perPage=999", nil)
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.ListURLs(w, req)

	if capturedPerPage != 100 {
		t.Errorf("perPage = %d, want 100 (clamped)", capturedPerPage)
	}
}

func TestListServiceError(t *testing.T) {
	mock := &mockService{
		listFn: func(_ context.Context, _ int64, _, _, _ int32) (*payload.URLListResponse, error) {
			return nil, apperror.ErrInternal
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls", nil)
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.ListURLs(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestListDefaultsWhenNoQueryParams(t *testing.T) {
	var capturedPage, capturedPerPage, capturedOffset int32
	mock := &mockService{
		listFn: func(_ context.Context, _ int64, page, perPage, offset int32) (*payload.URLListResponse, error) {
			capturedPage = page
			capturedPerPage = perPage
			capturedOffset = offset
			return &payload.URLListResponse{
				Items: []payload.URLResponse{},
				Total: 0,
			}, nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls", nil)
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.ListURLs(w, req)

	if capturedPage != 1 {
		t.Errorf("page = %d, want 1", capturedPage)
	}
	if capturedPerPage != 10 {
		t.Errorf("perPage = %d, want 10", capturedPerPage)
	}
	if capturedOffset != 0 {
		t.Errorf("offset = %d, want 0", capturedOffset)
	}
}

func TestListResponseBody(t *testing.T) {
	mock := &mockService{
		listFn: func(_ context.Context, _ int64, _, _, _ int32) (*payload.URLListResponse, error) {
			return &payload.URLListResponse{
				Items: []payload.URLResponse{*sampleResponse()},
				Total: 1,
			}, nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls", nil)
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.ListURLs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"total":1`) {
		t.Errorf("expected total=1 in body, got %s", w.Body.String())
	}
}

// ═══════════════════════════════════════════════════════════════
//  UPDATE — error paths and field validation
// ═══════════════════════════════════════════════════════════════

func TestUpdateInvalidJSON(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPatch, "/urls/1", strings.NewReader(`{bad`))
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.UpdateURL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpdateNotFound(t *testing.T) {
	mock := &mockService{
		updateFn: func(_ context.Context, _ int64, _ int64, _ payload.UpdateURLRequest) (*payload.URLResponse, error) {
			return nil, apperror.ErrNotFound
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"title":"new"}`
	req := httptest.NewRequest(http.MethodPatch, "/urls/1", strings.NewReader(body))
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.UpdateURL(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdateInternalError(t *testing.T) {
	mock := &mockService{
		updateFn: func(_ context.Context, _ int64, _ int64, _ payload.UpdateURLRequest) (*payload.URLResponse, error) {
			return nil, apperror.ErrInternal
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"title":"new"}`
	req := httptest.NewRequest(http.MethodPatch, "/urls/1", strings.NewReader(body))
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.UpdateURL(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestUpdateConflict(t *testing.T) {
	mock := &mockService{
		updateFn: func(_ context.Context, _ int64, _ int64, _ payload.UpdateURLRequest) (*payload.URLResponse, error) {
			return nil, apperror.ErrConflict
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL":"https://example.com","customCode":"taken"}`
	req := httptest.NewRequest(http.MethodPatch, "/urls/1", strings.NewReader(body))
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.UpdateURL(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestUpdatePassesAllFields(t *testing.T) {
	var capturedID int64
	var capturedReq payload.UpdateURLRequest
	mock := &mockService{
		updateFn: func(_ context.Context, _ int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error) {
			capturedID = id
			capturedReq = req
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL":"https://new.com","title":"New","description":"Desc","status":2}`
	req := httptest.NewRequest(http.MethodPatch, "/urls/42", strings.NewReader(body))
	req = withUserID(req, 1)
	req.SetPathValue("id", "42")
	w := httptest.NewRecorder()

	h.UpdateURL(w, req)

	if capturedID != 42 {
		t.Errorf("id = %d, want 42", capturedID)
	}
	if capturedReq.OriginalURL != "https://new.com" {
		t.Errorf("originalURL = %q", capturedReq.OriginalURL)
	}
	if capturedReq.Title != "New" {
		t.Errorf("title = %q", capturedReq.Title)
	}
	if capturedReq.Description != "Desc" {
		t.Errorf("description = %q", capturedReq.Description)
	}
	if capturedReq.Status == nil || *capturedReq.Status != 2 {
		t.Error("status should be 2")
	}
}

func TestUpdateEmptyBodySuccess(t *testing.T) {
	mock := &mockService{
		updateFn: func(_ context.Context, _ int64, _ int64, _ payload.UpdateURLRequest) (*payload.URLResponse, error) {
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPatch, "/urls/1", strings.NewReader(`{}`))
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.UpdateURL(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ═══════════════════════════════════════════════════════════════
//  DELETE (SOFT) — error paths
// ═══════════════════════════════════════════════════════════════

func TestDeleteNotFound(t *testing.T) {
	mock := &mockService{
		softDeleteFn: func(_ context.Context, _ int64, _ int64) (*payload.DeleteResponse, error) {
			return nil, apperror.ErrNotFound
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/999", nil)
	req = withUserID(req, 1)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	h.DeleteURL(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteInternalError(t *testing.T) {
	mock := &mockService{
		softDeleteFn: func(_ context.Context, _ int64, _ int64) (*payload.DeleteResponse, error) {
			return nil, apperror.ErrInternal
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/1", nil)
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.DeleteURL(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestDeletePassesArgs(t *testing.T) {
	var capturedUserID, capturedID int64
	mock := &mockService{
		softDeleteFn: func(_ context.Context, userID int64, id int64) (*payload.DeleteResponse, error) {
			capturedUserID = userID
			capturedID = id
			return &payload.DeleteResponse{Message: "soft deleted", DeletedAt: time.Now().Format(time.RFC3339)}, nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/5", nil)
	req = withUserID(req, 7)
	req.SetPathValue("id", "5")
	w := httptest.NewRecorder()

	h.DeleteURL(w, req)

	if capturedUserID != 7 {
		t.Errorf("userID = %d, want 7", capturedUserID)
	}
	if capturedID != 5 {
		t.Errorf("id = %d, want 5", capturedID)
	}
}

func TestDeleteUnauthorized(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.DeleteURL(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// ═══════════════════════════════════════════════════════════════
//  HARD DELETE — error paths
// ═══════════════════════════════════════════════════════════════

func TestHardDeleteInternalError(t *testing.T) {
	mock := &mockService{
		hardDeleteFn: func(_ context.Context, _ int64, _ int64) error {
			return apperror.ErrInternal
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/1/approve", nil)
	req = withUserID(req, 1)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.ApproveHardDelete(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

func TestHardDeleteNotFound(t *testing.T) {
	mock := &mockService{
		hardDeleteFn: func(_ context.Context, _ int64, _ int64) error {
			return apperror.ErrNotFound
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/999/approve", nil)
	req = withUserID(req, 1)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()

	h.ApproveHardDelete(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHardDeletePassesArgs(t *testing.T) {
	var capturedUserID, capturedID int64
	mock := &mockService{
		hardDeleteFn: func(_ context.Context, userID int64, id int64) error {
			capturedUserID = userID
			capturedID = id
			return nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/8/approve", nil)
	req = withUserID(req, 3)
	req.SetPathValue("id", "8")
	w := httptest.NewRecorder()

	h.ApproveHardDelete(w, req)

	if capturedUserID != 3 {
		t.Errorf("userID = %d, want 3", capturedUserID)
	}
	if capturedID != 8 {
		t.Errorf("id = %d, want 8", capturedID)
	}
}

func TestHardDeleteUnauthorized(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/1/approve", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.ApproveHardDelete(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

// ═══════════════════════════════════════════════════════════════
//  UNIVERSAL — no auth, invalid ID, wrong methods
// ═══════════════════════════════════════════════════════════════

func TestGetByIDUnauthorized(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls/1", nil)
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.GetURLByID(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestListUnauthorized(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls", nil)
	w := httptest.NewRecorder()

	h.ListURLs(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestUpdateUnauthorized(t *testing.T) {
	mock := &mockService{}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPatch, "/urls/1", strings.NewReader(`{}`))
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.UpdateURL(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestRedirectXForwardedForMultiple(t *testing.T) {
	var capturedIP net.IP
	mock := &mockService{
		redirectFn: func(_ context.Context, _ string, click payload.ClickInfo) (*payload.URLResponse, error) {
			capturedIP = click.IP
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/abc", nil)
	req = req.WithContext(context.WithValue(req.Context(), contextutil.UserIDKey, int64(1)))
	req.SetPathValue("shortCode", "abc")
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")
	w := httptest.NewRecorder()

	h.RedirectShortURL(w, req)

	if capturedIP == nil {
		t.Fatal("expected IP to be set")
	}
	if capturedIP.String() != "1.2.3.4" {
		t.Errorf("IP = %q, want first IP from X-Forwarded-For", capturedIP.String())
	}
}
