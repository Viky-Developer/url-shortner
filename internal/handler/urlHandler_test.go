package handler

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/payload"
)

type mockService struct {
	resolveFn    func(context.Context, string) (int64, error)
	createFn     func(context.Context, int64, payload.CreateURLRequest) (*payload.URLResponse, error)
	redirectFn   func(context.Context, string, payload.ClickInfo) (*payload.URLResponse, error)
	byIDFn       func(context.Context, int64, int64) (*payload.URLResponse, error)
	listFn       func(context.Context, int64, int32, int32, int32) (*payload.URLListResponse, error)
	updateFn     func(context.Context, int64, int64, payload.UpdateURLRequest) (*payload.URLResponse, error)
	softDeleteFn func(context.Context, int64, int64) (*payload.DeleteResponse, error)
	hardDeleteFn func(context.Context, int64, int64) error
}

func (m *mockService) ResolveUserID(ctx context.Context, encodedUserID string) (int64, error) {
	return m.resolveFn(ctx, encodedUserID)
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

func mockResolveUserID(_ context.Context, _ string) (int64, error) {
	return 1, nil
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
		resolveFn: mockResolveUserID,
		createFn: func(_ context.Context, _ int64, _ payload.CreateURLRequest) (*payload.URLResponse, error) {
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL": "https://example.com/long-url"}`
	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(body))
	req.SetPathValue("userId", "USR_test123")
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
	mock := &mockService{resolveFn: mockResolveUserID}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString("{invalid"))
	req.SetPathValue("userId", "USR_test123")
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateShortURLEmptyBody(t *testing.T) {
	mock := &mockService{resolveFn: mockResolveUserID}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(""))
	req.SetPathValue("userId", "USR_test123")
	w := httptest.NewRecorder()

	h.CreateShortURL(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid payload") {
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
			mock := &mockService{resolveFn: mockResolveUserID}
			h := NewURLHandler(mock, testLog(t))

			req := httptest.NewRequest(http.MethodPost, "/shorten", bytes.NewBufferString(tt.body))
			req.SetPathValue("userId", "USR_test123")
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
			err := validateURL(tt.url)
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

	if err := validateURL("https://somehost.example.com/x"); err == nil {
		t.Fatal("expected error for host resolving to private IP")
	}
}

func TestValidateURLRejectsUnresolvableHost(t *testing.T) {
	orig := lookupIP
	lookupIP = func(_ string) ([]net.IP, error) {
		return nil, fmt.Errorf("no such host")
	}
	t.Cleanup(func() { lookupIP = orig })

	if err := validateURL("https://nonexistent.example.com/x"); err == nil {
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
			return nil, sql.ErrNoRows
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
		resolveFn: mockResolveUserID,
		byIDFn: func(_ context.Context, _ int64, _ int64) (*payload.URLResponse, error) {
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls/1", nil)
	req.SetPathValue("userId", "USR_test123")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.GetURLByID(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetURLByIDInvalid(t *testing.T) {
	mock := &mockService{resolveFn: mockResolveUserID}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/urls/abc", nil)
	req.SetPathValue("userId", "USR_test123")
	req.SetPathValue("id", "abc")
	w := httptest.NewRecorder()

	h.GetURLByID(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestListURLsDefaults(t *testing.T) {
	mock := &mockService{
		resolveFn: mockResolveUserID,
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
	req.SetPathValue("userId", "USR_test123")
	w := httptest.NewRecorder()

	h.ListURLs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestUpdateURL(t *testing.T) {
	stubLookupIP(t)
	mock := &mockService{
		resolveFn: mockResolveUserID,
		updateFn: func(_ context.Context, _ int64, _ int64, _ payload.UpdateURLRequest) (*payload.URLResponse, error) {
			return sampleResponse(), nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	body := `{"originalURL": "https://example.com/updated"}`
	req := httptest.NewRequest(http.MethodPatch, "/urls/1", bytes.NewBufferString(body))
	req.SetPathValue("userId", "USR_test123")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.UpdateURL(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteURLSoftDelete(t *testing.T) {
	mock := &mockService{
		resolveFn: mockResolveUserID,
		softDeleteFn: func(_ context.Context, _ int64, _ int64) (*payload.DeleteResponse, error) {
			return &payload.DeleteResponse{ID: 1, ShortCode: "abc1234567", Message: "soft deleted"}, nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/1", nil)
	req.SetPathValue("userId", "USR_test123")
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
		resolveFn: mockResolveUserID,
		hardDeleteFn: func(_ context.Context, _ int64, _ int64) error {
			return nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodDelete, "/urls/1/approve", nil)
	req.SetPathValue("userId", "USR_test123")
	req.SetPathValue("id", "1")
	w := httptest.NewRecorder()

	h.ApproveHardDelete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
