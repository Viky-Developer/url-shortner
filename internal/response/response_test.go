package response

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/payload"
)

func TestJSON(t *testing.T) {
	t.Run("writes status and JSON body", func(t *testing.T) {
		w := httptest.NewRecorder()
		JSON(w, http.StatusCreated, map[string]string{"key": "value"})

		if w.Code != http.StatusCreated {
			t.Fatalf("expected status %d, got %d", http.StatusCreated, w.Code)
		}
		ct := w.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Fatalf("expected Content-Type application/json, got %s", ct)
		}
		var body map[string]string
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if body["key"] != "value" {
			t.Fatalf("expected key=value, got %s", body["key"])
		}
	})

	t.Run("handles nil body", func(t *testing.T) {
		w := httptest.NewRecorder()
		JSON(w, http.StatusOK, nil)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
		}
	})
}

func TestSuccess(t *testing.T) {
	t.Run("without pagination", func(t *testing.T) {
		w := httptest.NewRecorder()
		Success(w, http.StatusOK, "ok", []any{"a", "b"})

		if w.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, w.Code)
		}
		var resp payload.SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Message != "ok" {
			t.Fatalf("expected message ok, got %s", resp.Message)
		}
		if len(resp.Data) != 2 {
			t.Fatalf("expected 2 data items, got %d", len(resp.Data))
		}
		if resp.Pagination != nil {
			t.Fatalf("expected nil pagination, got %+v", resp.Pagination)
		}
	})

	t.Run("with pagination", func(t *testing.T) {
		w := httptest.NewRecorder()
		pag := &payload.Pagination{Total: 100, Page: 1, PerPage: 10, TotalPages: 10}
		Success(w, http.StatusOK, "list", nil, pag)

		var resp payload.SuccessResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Pagination == nil {
			t.Fatal("expected pagination, got nil")
		}
		if resp.Pagination.Total != 100 {
			t.Fatalf("expected total 100, got %d", resp.Pagination.Total)
		}
	})
}

func TestError(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, http.StatusBadRequest, errors.New("bad input"))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	var resp payload.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Message != "bad input" {
		t.Fatalf("expected message bad input, got %s", resp.Message)
	}
}

func TestStatusCodeFromError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected int
	}{
		{"ErrNotFound", apperror.ErrNotFound, http.StatusNotFound},
		{"ErrInvalidURL", apperror.ErrInvalidURL, http.StatusBadRequest},
		{"ErrInvalidPayload", apperror.ErrInvalidPayload, http.StatusBadRequest},
		{"ErrURLExpired", apperror.ErrURLExpired, http.StatusBadRequest},
		{"ErrURLInactive", apperror.ErrURLInactive, http.StatusBadRequest},
		{"ErrBlockedDomain", apperror.ErrBlockedDomain, http.StatusBadRequest},
		{"ErrConflict", apperror.ErrConflict, http.StatusConflict},
		{"ErrURLDeleted", apperror.ErrURLDeleted, http.StatusGone},
		{"wrapped ErrNotFound", fmt.Errorf("wrap: %w", apperror.ErrNotFound), http.StatusNotFound},
		{"unknown error", errors.New("something"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StatusCodeFromError(tt.err)
			if got != tt.expected {
				t.Fatalf("expected %d, got %d", tt.expected, got)
			}
		})
	}
}
