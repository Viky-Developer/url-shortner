package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/vicky/url-shortner/internal/config"
	gen "github.com/vicky/url-shortner/internal/db/gen"
)

func TestRetentionWorkerRunExecutesAllTasks(t *testing.T) {
	var sessionsPurged, passwordsPurged bool
	var deletedIDs []int64
	var audit gen.InsertAuditLogParams
	q := &adminQuerierStub{
		purgeOldRevokedSessionsFn: func(_ context.Context, before sql.NullTime) error {
			sessionsPurged = before.Valid
			return nil
		},
		purgeOldPasswordHistoryFn: func(_ context.Context, before sql.NullTime) error {
			passwordsPurged = before.Valid
			return nil
		},
		accountsDueDeletionFn: func(context.Context) ([]int64, error) { return []int64{5, 9}, nil },
		hardDeleteUserByIDFn: func(_ context.Context, id int64) error {
			deletedIDs = append(deletedIDs, id)
			return nil
		},
		insertAuditLogFn: func(_ context.Context, arg gen.InsertAuditLogParams) error {
			audit = arg
			return nil
		},
	}
	admin := NewAdminService(q)
	deletions := NewAccountDeletionService(q, nil, admin, nil, nil, testLog(t))
	cfg := &config.Config{SessionRetention: 48 * time.Hour, PasswordRetention: 72 * time.Hour}
	worker := NewRetentionWorker(admin, deletions, q, cfg, testLog(t))

	worker.run(context.Background())

	if !sessionsPurged || !passwordsPurged {
		t.Errorf("purges executed: sessions=%v passwords=%v", sessionsPurged, passwordsPurged)
	}
	if len(deletedIDs) != 2 || deletedIDs[0] != 5 || deletedIDs[1] != 9 {
		t.Errorf("deleted IDs = %v, want [5 9]", deletedIDs)
	}
	if audit.Action != "RETENTION_PURGE" || audit.EntityType.String != "SYSTEM" || audit.ActorUserID.Valid || audit.EntityID.Valid {
		t.Errorf("audit entry = %+v", audit)
	}
	metadata := string(audit.Metadata.RawMessage)
	if metadata == "" || metadata == `{}` {
		t.Errorf("audit metadata = %q, want retention details", metadata)
	}
}

func TestRetentionWorkerRunContinuesAfterPurgeFailures(t *testing.T) {
	var passwordCalled, auditCalled bool
	q := &adminQuerierStub{
		purgeOldRevokedSessionsFn: func(context.Context, sql.NullTime) error { return errors.New("sessions failed") },
		purgeOldPasswordHistoryFn: func(context.Context, sql.NullTime) error {
			passwordCalled = true
			return errors.New("passwords failed")
		},
		insertAuditLogFn: func(context.Context, gen.InsertAuditLogParams) error {
			auditCalled = true
			return errors.New("audit failed")
		},
	}
	admin := NewAdminService(q)
	worker := NewRetentionWorker(admin, nil, q, &config.Config{}, testLog(t))

	worker.run(context.Background())

	if !passwordCalled || !auditCalled {
		t.Errorf("worker stopped early: passwordCalled=%v auditCalled=%v", passwordCalled, auditCalled)
	}
}

func TestRetentionWorkerStartDisabled(t *testing.T) {
	called := false
	q := &adminQuerierStub{
		purgeOldRevokedSessionsFn: func(context.Context, sql.NullTime) error { called = true; return nil },
	}
	worker := NewRetentionWorker(NewAdminService(q), nil, q, &config.Config{EnableRetentionWorker: false}, testLog(t))

	worker.Start(context.Background())

	if called {
		t.Error("disabled worker executed a purge")
	}
}

func TestRetentionWorkerStartRunsImmediatelyAndStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var sessionsCalls, passwordsCalls, auditCalls int
	q := &adminQuerierStub{
		purgeOldRevokedSessionsFn: func(context.Context, sql.NullTime) error {
			sessionsCalls++
			cancel()
			return nil
		},
		purgeOldPasswordHistoryFn: func(context.Context, sql.NullTime) error { passwordsCalls++; return nil },
		insertAuditLogFn:          func(context.Context, gen.InsertAuditLogParams) error { auditCalls++; return nil },
	}
	cfg := &config.Config{
		EnableRetentionWorker: true,
		RetentionRunInterval:  time.Hour,
		SessionRetention:      24 * time.Hour,
		PasswordRetention:     48 * time.Hour,
	}
	worker := NewRetentionWorker(NewAdminService(q), nil, q, cfg, testLog(t))

	worker.Start(ctx)

	if sessionsCalls != 1 || passwordsCalls != 1 || auditCalls != 1 {
		t.Errorf("immediate cycle calls = sessions:%d passwords:%d audit:%d", sessionsCalls, passwordsCalls, auditCalls)
	}
}
