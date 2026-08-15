package handler

import (
	"bytes"
	"context"
	"database/sql"
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
	byCodeFn     func(context.Context, int64, string) (*payload.URLResponse, error)
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

func (m *mockService) GetByShortCode(ctx context.Context, userID int64, code string) (*payload.URLResponse, error) {
	return m.byCodeFn(ctx, userID, code)
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

func TestRedirectShortURL(t *testing.T) {
	mock := &mockService{
		resolveFn: mockResolveUserID,
		byCodeFn: func(_ context.Context, _ int64, _ string) (*payload.URLResponse, error) {
			return &payload.URLResponse{OriginalURL: "https://example.com/target"}, nil
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/USR_test123/abc1234567", nil)
	req.SetPathValue("userId", "USR_test123")
	req.SetPathValue("shortCode", "abc1234567")
	w := httptest.NewRecorder()

	h.RedirectShortURL(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "https://example.com/target") {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestRedirectShortURLNotFound(t *testing.T) {
	mock := &mockService{
		resolveFn: mockResolveUserID,
		byCodeFn: func(_ context.Context, _ int64, _ string) (*payload.URLResponse, error) {
			return nil, sql.ErrNoRows
		},
	}
	h := NewURLHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/USR_test123/missing", nil)
	req.SetPathValue("userId", "USR_test123")
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
