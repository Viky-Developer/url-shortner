package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/vicky/url-shortner/internal/apperror"
	gen "github.com/vicky/url-shortner/internal/db/gen"
)

func TestAccountDeletionRequestDeletion(t *testing.T) {
	mock := &mockQuerier{
		userStatusByIDFn: func(_ context.Context, id int64) (gen.GetUserStatusByIDRow, error) {
			return gen.GetUserStatusByIDRow{ID: id, Status: "ACTIVE"}, nil
		},
		listActiveSessionsFn: func(_ context.Context, userID int64) ([]gen.Session, error) {
			return []gen.Session{{ID: 1}, {ID: 2}}, nil
		},
		revokeAllSessionsFn: func(_ context.Context, userID int64) error {
			return nil
		},
		markPendingDeletionFn: func(_ context.Context, id int64) error {
			return nil
		},
	}
	urlSvc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))
	svc := NewAccountDeletionService(mock, nil, NewAdminService(mock), nil, urlSvc, testLog(t))

	resp, err := svc.RequestDeletion(context.Background(), int64(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "PENDING_DELETION" {
		t.Errorf("status = %s, want PENDING_DELETION", resp.Status)
	}
	if resp.DeletionScheduledAt == nil {
		t.Error("expected DeletionScheduledAt to be set")
	}
}

func TestAccountDeletionRequestDeletionNotFound(t *testing.T) {
	mock := &mockQuerier{
		userStatusByIDFn: func(_ context.Context, id int64) (gen.GetUserStatusByIDRow, error) {
			return gen.GetUserStatusByIDRow{}, sql.ErrNoRows
		},
	}
	svc := NewAccountDeletionService(mock, nil, nil, nil, nil, testLog(t))

	_, err := svc.RequestDeletion(context.Background(), int64(999))
	if err == nil || err != apperror.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAccountDeletionRequestDeletionAlreadyPending(t *testing.T) {
	mock := &mockQuerier{
		userStatusByIDFn: func(_ context.Context, id int64) (gen.GetUserStatusByIDRow, error) {
			return gen.GetUserStatusByIDRow{ID: id, Status: "PENDING_DELETION"}, nil
		},
	}
	svc := NewAccountDeletionService(mock, nil, nil, nil, nil, testLog(t))

	_, err := svc.RequestDeletion(context.Background(), int64(1))
	if !errors.Is(err, apperror.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestAccountDeletionCancelDeletion(t *testing.T) {
	mock := &mockQuerier{
		userStatusByIDFn: func(_ context.Context, id int64) (gen.GetUserStatusByIDRow, error) {
			return gen.GetUserStatusByIDRow{ID: id, Status: "PENDING_DELETION"}, nil
		},
		restoreAccountFn: func(_ context.Context, id int64) error {
			return nil
		},
	}
	svc := NewAccountDeletionService(mock, nil, NewAdminService(mock), nil, nil, testLog(t))

	err := svc.CancelDeletion(context.Background(), int64(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAccountDeletionCancelDeletionNotPending(t *testing.T) {
	mock := &mockQuerier{
		userStatusByIDFn: func(_ context.Context, id int64) (gen.GetUserStatusByIDRow, error) {
			return gen.GetUserStatusByIDRow{ID: id, Status: "ACTIVE"}, nil
		},
	}
	svc := NewAccountDeletionService(mock, nil, nil, nil, nil, testLog(t))

	err := svc.CancelDeletion(context.Background(), int64(1))
	if !errors.Is(err, apperror.ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestAccountDeletionGetStatus(t *testing.T) {
	mock := &mockQuerier{
		userStatusByIDFn: func(_ context.Context, id int64) (gen.GetUserStatusByIDRow, error) {
			return gen.GetUserStatusByIDRow{ID: id, Status: "PENDING_DELETION"}, nil
		},
	}
	svc := NewAccountDeletionService(mock, nil, nil, nil, nil, testLog(t))

	resp, err := svc.GetStatus(context.Background(), int64(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "PENDING_DELETION" {
		t.Errorf("status = %s, want PENDING_DELETION", resp.Status)
	}
}

func TestAccountDeletionProcessDeletions(t *testing.T) {
	mock := &mockQuerier{
		accountsDueDeletionFn: func(_ context.Context) ([]int64, error) {
			return []int64{1, 2}, nil
		},
		hardDeleteUserByIDFn: func(_ context.Context, id int64) error {
			return nil
		},
	}
	svc := NewAccountDeletionService(mock, nil, NewAdminService(mock), nil, nil, testLog(t))

	err := svc.ProcessDeletions(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAccountDeletionRequestDeletionWithTransaction(t *testing.T) {
	mock := &mockQuerier{
		userStatusByIDFn: func(_ context.Context, id int64) (gen.GetUserStatusByIDRow, error) {
			return gen.GetUserStatusByIDRow{ID: id, Status: "ACTIVE"}, nil
		},
		listActiveSessionsFn: func(_ context.Context, userID int64) ([]gen.Session, error) {
			return []gen.Session{{ID: 1}}, nil
		},
		revokeAllSessionsFn: func(_ context.Context, userID int64) error {
			return nil
		},
		markPendingDeletionFn: func(_ context.Context, id int64) error {
			return nil
		},
	}
	urlSvc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))
	svc := NewAccountDeletionService(mock, nil, NewAdminService(mock), nil, urlSvc, testLog(t))

	resp, err := svc.RequestDeletion(context.Background(), int64(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Status != "PENDING_DELETION" {
		t.Errorf("status = %s, want PENDING_DELETION", resp.Status)
	}
}

func TestAccountDeletionRequestDeletionTxRollback(t *testing.T) {
	mock := &mockQuerier{
		userStatusByIDFn: func(_ context.Context, id int64) (gen.GetUserStatusByIDRow, error) {
			return gen.GetUserStatusByIDRow{ID: id, Status: "ACTIVE"}, nil
		},
		listActiveSessionsFn: func(_ context.Context, userID int64) ([]gen.Session, error) {
			return []gen.Session{{ID: 1}}, nil
		},
		revokeAllSessionsFn: func(_ context.Context, userID int64) error {
			return nil
		},
		markPendingDeletionFn: func(_ context.Context, id int64) error {
			return fmt.Errorf("db error")
		},
	}
	urlSvc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))
	svc := NewAccountDeletionService(mock, nil, nil, nil, urlSvc, testLog(t))

	_, err := svc.RequestDeletion(context.Background(), int64(1))
	if err == nil {
		t.Fatal("expected error when markPendingDeletion fails")
	}
}
