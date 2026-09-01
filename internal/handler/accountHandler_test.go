package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/payload"
)

type mockAccountDeletionService struct {
	requestFn   func(context.Context, int64) (*payload.AccountStatusResponse, error)
	cancelFn    func(context.Context, int64) error
	getStatusFn func(context.Context, int64) (*payload.AccountStatusResponse, error)
}

func (m *mockAccountDeletionService) RequestDeletion(ctx context.Context, userID int64) (*payload.AccountStatusResponse, error) {
	if m.requestFn != nil {
		return m.requestFn(ctx, userID)
	}
	return &payload.AccountStatusResponse{Status: "PENDING_DELETION"}, nil
}

func (m *mockAccountDeletionService) CancelDeletion(ctx context.Context, userID int64) error {
	if m.cancelFn != nil {
		return m.cancelFn(ctx, userID)
	}
	return nil
}

func (m *mockAccountDeletionService) GetStatus(ctx context.Context, userID int64) (*payload.AccountStatusResponse, error) {
	if m.getStatusFn != nil {
		return m.getStatusFn(ctx, userID)
	}
	return &payload.AccountStatusResponse{Status: "ACTIVE"}, nil
}

func TestDeleteAccount(t *testing.T) {
	mock := &mockAccountDeletionService{
		requestFn: func(_ context.Context, _ int64) (*payload.AccountStatusResponse, error) {
			return &payload.AccountStatusResponse{Status: "PENDING_DELETION"}, nil
		},
	}
	h := NewAccountHandler(mock, testLog(t))

	body := payload.DeleteAccountRequest{Confirmation: "DELETE"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/account", bytes.NewReader(b))
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.DeleteAccount(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestDeleteAccountWrongConfirmation(t *testing.T) {
	mock := &mockAccountDeletionService{
		requestFn: func(_ context.Context, _ int64) (*payload.AccountStatusResponse, error) {
			return nil, apperror.ErrInvalidPayload
		},
	}
	h := NewAccountHandler(mock, testLog(t))

	body := payload.DeleteAccountRequest{Confirmation: "delete"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/account", bytes.NewReader(b))
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.DeleteAccount(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestDeleteAccountUnauthorized(t *testing.T) {
	mock := &mockAccountDeletionService{}
	h := NewAccountHandler(mock, testLog(t))

	body := payload.DeleteAccountRequest{Confirmation: "DELETE"}
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/account", bytes.NewReader(b))
	// No userID in context
	w := httptest.NewRecorder()

	h.DeleteAccount(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestCancelDeletion(t *testing.T) {
	mock := &mockAccountDeletionService{
		cancelFn: func(_ context.Context, _ int64) error {
			return nil
		},
	}
	h := NewAccountHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/cancel-deletion", nil)
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.CancelDeletion(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestCancelDeletionNotPending(t *testing.T) {
	mock := &mockAccountDeletionService{
		cancelFn: func(_ context.Context, _ int64) error {
			return apperror.ErrConflict
		},
	}
	h := NewAccountHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/account/cancel-deletion", nil)
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.CancelDeletion(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", w.Code)
	}
}

func TestGetAccountStatus(t *testing.T) {
	mock := &mockAccountDeletionService{
		getStatusFn: func(_ context.Context, _ int64) (*payload.AccountStatusResponse, error) {
			return &payload.AccountStatusResponse{Status: "PENDING_DELETION"}, nil
		},
	}
	h := NewAccountHandler(mock, testLog(t))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/account/status", nil)
	req = withUserID(req, 1)
	w := httptest.NewRecorder()

	h.GetAccountStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
