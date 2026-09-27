package service

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/sqlc-dev/pqtype"
	"github.com/vicky/url-shortner/internal/apperror"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/payload"
)

type adminQuerierStub struct {
	gen.Querier
	insertAuditLogFn          func(context.Context, gen.InsertAuditLogParams) error
	listBlockedDomainsFn      func(context.Context) ([]gen.BlockedDomain, error)
	createBlockedDomainFn     func(context.Context, gen.CreateBlockedDomainParams) (gen.BlockedDomain, error)
	deleteBlockedDomainFn     func(context.Context, int32) error
	listBlockedIPRangesFn     func(context.Context) ([]gen.BlockedIpRange, error)
	createBlockedIPRangeFn    func(context.Context, gen.CreateBlockedIPRangeParams) (gen.BlockedIpRange, error)
	deleteBlockedIPRangeFn    func(context.Context, int64) error
	purgeOldRevokedSessionsFn func(context.Context, sql.NullTime) error
	purgeOldPasswordHistoryFn func(context.Context, sql.NullTime) error
	softDeleteUserFn          func(context.Context, int64) error
	hardDeleteUserFn          func(context.Context, int64) error
	accountsDueDeletionFn     func(context.Context) ([]int64, error)
	hardDeleteUserByIDFn      func(context.Context, int64) error
}

func (s *adminQuerierStub) InsertAuditLog(ctx context.Context, arg gen.InsertAuditLogParams) error {
	return s.insertAuditLogFn(ctx, arg)
}

func (s *adminQuerierStub) ListBlockedDomains(ctx context.Context) ([]gen.BlockedDomain, error) {
	return s.listBlockedDomainsFn(ctx)
}

func (s *adminQuerierStub) CreateBlockedDomain(ctx context.Context, arg gen.CreateBlockedDomainParams) (gen.BlockedDomain, error) {
	return s.createBlockedDomainFn(ctx, arg)
}

func (s *adminQuerierStub) DeleteBlockedDomain(ctx context.Context, id int32) error {
	return s.deleteBlockedDomainFn(ctx, id)
}

func (s *adminQuerierStub) ListBlockedIPRanges(ctx context.Context) ([]gen.BlockedIpRange, error) {
	return s.listBlockedIPRangesFn(ctx)
}

func (s *adminQuerierStub) CreateBlockedIPRange(ctx context.Context, arg gen.CreateBlockedIPRangeParams) (gen.BlockedIpRange, error) {
	return s.createBlockedIPRangeFn(ctx, arg)
}

func (s *adminQuerierStub) DeleteBlockedIPRange(ctx context.Context, id int64) error {
	return s.deleteBlockedIPRangeFn(ctx, id)
}

func (s *adminQuerierStub) PurgeOldRevokedSessions(ctx context.Context, before sql.NullTime) error {
	return s.purgeOldRevokedSessionsFn(ctx, before)
}

func (s *adminQuerierStub) PurgeOldPasswordHistory(ctx context.Context, before sql.NullTime) error {
	return s.purgeOldPasswordHistoryFn(ctx, before)
}

func (s *adminQuerierStub) SoftDeleteUser(ctx context.Context, id int64) error {
	return s.softDeleteUserFn(ctx, id)
}

func (s *adminQuerierStub) HardDeleteUser(ctx context.Context, id int64) error {
	return s.hardDeleteUserFn(ctx, id)
}

func (s *adminQuerierStub) GetAccountsDueForDeletion(ctx context.Context) ([]int64, error) {
	return s.accountsDueDeletionFn(ctx)
}

func (s *adminQuerierStub) HardDeleteUserByID(ctx context.Context, id int64) error {
	return s.hardDeleteUserByIDFn(ctx, id)
}

func TestAdminServiceLogAction(t *testing.T) {
	var calls []gen.InsertAuditLogParams
	q := &adminQuerierStub{insertAuditLogFn: func(_ context.Context, arg gen.InsertAuditLogParams) error {
		calls = append(calls, arg)
		return errors.New("audit unavailable")
	}}
	svc := NewAdminService(q)

	svc.LogAction(context.Background(), 0, "SYSTEM_ACTION", "", 0)
	svc.LogAction(context.Background(), 42, "DELETE", "URL", 7, []byte(`{"source":"test"}`)...)

	if len(calls) != 2 {
		t.Fatalf("InsertAuditLog calls = %d, want 2", len(calls))
	}
	if calls[0].ActorUserID.Valid || calls[0].EntityType.Valid || calls[0].EntityID.Valid {
		t.Fatalf("system audit nullable fields = %+v, want invalid", calls[0])
	}
	if got := string(calls[0].Metadata.RawMessage); got != `{}` {
		t.Errorf("default metadata = %q, want {}", got)
	}
	if calls[1].ActorUserID.Int64 != 42 || !calls[1].ActorUserID.Valid || calls[1].EntityType.String != "URL" || calls[1].EntityID.Int64 != 7 {
		t.Errorf("custom audit fields = %+v", calls[1])
	}
	if got := string(calls[1].Metadata.RawMessage); got != `{"source":"test"}` {
		t.Errorf("custom metadata = %q", got)
	}
}

func TestAdminServiceBlockedDomains(t *testing.T) {
	createdAt := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	q := &adminQuerierStub{
		listBlockedDomainsFn: func(context.Context) ([]gen.BlockedDomain, error) {
			return []gen.BlockedDomain{{ID: 3, Domain: "example.com", Reason: sql.NullString{String: "abuse", Valid: true}, CreatedAt: sql.NullTime{Time: createdAt, Valid: true}}}, nil
		},
		createBlockedDomainFn: func(_ context.Context, arg gen.CreateBlockedDomainParams) (gen.BlockedDomain, error) {
			if arg.Domain != "bad.example" || arg.Reason.String != "malware" || !arg.Reason.Valid {
				t.Fatalf("CreateBlockedDomain params = %+v", arg)
			}
			return gen.BlockedDomain{ID: 4, Domain: arg.Domain, Reason: arg.Reason, CreatedAt: sql.NullTime{Time: createdAt, Valid: true}}, nil
		},
		deleteBlockedDomainFn: func(_ context.Context, id int32) error {
			if id != 4 {
				t.Fatalf("DeleteBlockedDomain id = %d, want 4", id)
			}
			return nil
		},
	}
	svc := NewAdminService(q)

	items, err := svc.ListBlockedDomains(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("ListBlockedDomains() = %#v, %v", items, err)
	}
	item, ok := items[0].(payload.BlockedDomainResponse)
	if !ok || item.ID != 3 || item.Domain != "example.com" || item.Reason != "abuse" || item.CreatedAt == "" {
		t.Errorf("blocked domain response = %#v", items[0])
	}

	if _, err := svc.CreateBlockedDomain(context.Background(), payload.CreateBlockedDomainRequest{}); !errors.Is(err, apperror.ErrInvalidPayload) {
		t.Errorf("empty domain error = %v, want ErrInvalidPayload", err)
	}
	created, err := svc.CreateBlockedDomain(context.Background(), payload.CreateBlockedDomainRequest{Domain: "bad.example", Reason: "malware"})
	if err != nil || created.ID != 4 || created.Domain != "bad.example" || created.Reason != "malware" {
		t.Fatalf("CreateBlockedDomain() = %#v, %v", created, err)
	}
	if err := svc.DeleteBlockedDomain(context.Background(), 4); err != nil {
		t.Fatalf("DeleteBlockedDomain() error = %v", err)
	}
}

func TestAdminServiceBlockedDomainDatabaseErrors(t *testing.T) {
	dbErr := errors.New("database unavailable")
	q := &adminQuerierStub{
		listBlockedDomainsFn: func(context.Context) ([]gen.BlockedDomain, error) { return nil, dbErr },
		createBlockedDomainFn: func(context.Context, gen.CreateBlockedDomainParams) (gen.BlockedDomain, error) {
			return gen.BlockedDomain{}, dbErr
		},
		deleteBlockedDomainFn: func(context.Context, int32) error { return dbErr },
	}
	svc := NewAdminService(q)

	if _, err := svc.ListBlockedDomains(context.Background()); !errors.Is(err, apperror.ErrInternal) {
		t.Errorf("ListBlockedDomains error = %v", err)
	}
	if _, err := svc.CreateBlockedDomain(context.Background(), payload.CreateBlockedDomainRequest{Domain: "bad.example"}); !errors.Is(err, apperror.ErrInternal) {
		t.Errorf("CreateBlockedDomain error = %v", err)
	}
	if err := svc.DeleteBlockedDomain(context.Background(), 1); !errors.Is(err, apperror.ErrInternal) {
		t.Errorf("DeleteBlockedDomain error = %v", err)
	}
}

func TestAdminServiceBlockedIPRanges(t *testing.T) {
	_, network, err := net.ParseCIDR("10.0.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	q := &adminQuerierStub{
		listBlockedIPRangesFn: func(context.Context) ([]gen.BlockedIpRange, error) {
			return []gen.BlockedIpRange{{ID: 8, Cidr: pqtype.CIDR{IPNet: *network, Valid: true}, Description: "private range"}}, nil
		},
		createBlockedIPRangeFn: func(_ context.Context, arg gen.CreateBlockedIPRangeParams) (gen.BlockedIpRange, error) {
			if arg.Cidr.IPNet.String() != "10.0.0.0/24" || arg.Description != "private range" {
				t.Fatalf("CreateBlockedIPRange params = %+v", arg)
			}
			return gen.BlockedIpRange{ID: 8, Cidr: arg.Cidr, Description: arg.Description}, nil
		},
		deleteBlockedIPRangeFn: func(_ context.Context, id int64) error {
			if id != 8 {
				t.Fatalf("DeleteBlockedIPRange id = %d, want 8", id)
			}
			return nil
		},
	}
	svc := NewAdminService(q)

	items, err := svc.ListBlockedIPRanges(context.Background())
	if err != nil || len(items) != 1 {
		t.Fatalf("ListBlockedIPRanges() = %#v, %v", items, err)
	}
	item, ok := items[0].(payload.BlockedIPRangeResponse)
	if !ok || item.ID != 8 || item.CIDR != "10.0.0.0/24" || item.Description != "private range" {
		t.Errorf("blocked IP response = %#v", items[0])
	}

	for _, cidr := range []string{"", "not-a-cidr"} {
		if _, err := svc.CreateBlockedIPRange(context.Background(), payload.CreateBlockedIPRangeRequest{CIDR: cidr}); !errors.Is(err, apperror.ErrInvalidPayload) {
			t.Errorf("CIDR %q error = %v, want ErrInvalidPayload", cidr, err)
		}
	}
	created, err := svc.CreateBlockedIPRange(context.Background(), payload.CreateBlockedIPRangeRequest{CIDR: "10.0.0.0/24", Description: "private range"})
	if err != nil || created.ID != 8 || created.CIDR != "10.0.0.0/24" {
		t.Fatalf("CreateBlockedIPRange() = %#v, %v", created, err)
	}
	if err := svc.DeleteBlockedIPRange(context.Background(), 8); err != nil {
		t.Fatalf("DeleteBlockedIPRange() error = %v", err)
	}
}

func TestAdminServiceBlockedIPRangeDatabaseErrors(t *testing.T) {
	dbErr := errors.New("database unavailable")
	q := &adminQuerierStub{
		listBlockedIPRangesFn: func(context.Context) ([]gen.BlockedIpRange, error) { return nil, dbErr },
		createBlockedIPRangeFn: func(context.Context, gen.CreateBlockedIPRangeParams) (gen.BlockedIpRange, error) {
			return gen.BlockedIpRange{}, dbErr
		},
		deleteBlockedIPRangeFn: func(context.Context, int64) error { return dbErr },
	}
	svc := NewAdminService(q)

	if _, err := svc.ListBlockedIPRanges(context.Background()); !errors.Is(err, apperror.ErrInternal) {
		t.Errorf("ListBlockedIPRanges error = %v", err)
	}
	if _, err := svc.CreateBlockedIPRange(context.Background(), payload.CreateBlockedIPRangeRequest{CIDR: "10.0.0.0/24"}); !errors.Is(err, apperror.ErrInternal) {
		t.Errorf("CreateBlockedIPRange error = %v", err)
	}
	if err := svc.DeleteBlockedIPRange(context.Background(), 1); !errors.Is(err, apperror.ErrInternal) {
		t.Errorf("DeleteBlockedIPRange error = %v", err)
	}
}

func TestAdminServiceMaintenanceAndUserDeletion(t *testing.T) {
	const retention = 24 * time.Hour
	start := time.Now().Add(-retention)
	var sessionsBefore, passwordsBefore sql.NullTime
	q := &adminQuerierStub{
		purgeOldRevokedSessionsFn: func(_ context.Context, before sql.NullTime) error { sessionsBefore = before; return nil },
		purgeOldPasswordHistoryFn: func(_ context.Context, before sql.NullTime) error { passwordsBefore = before; return nil },
		softDeleteUserFn: func(_ context.Context, id int64) error {
			if id != 12 {
				t.Fatalf("SoftDeleteUser id = %d", id)
			}
			return nil
		},
		hardDeleteUserFn: func(_ context.Context, id int64) error {
			if id != 12 {
				t.Fatalf("HardDeleteUser id = %d", id)
			}
			return nil
		},
	}
	svc := NewAdminService(q)

	if err := svc.PurgeOldRevokedSessions(context.Background(), retention); err != nil {
		t.Fatal(err)
	}
	if err := svc.PurgeOldPasswordHistory(context.Background(), retention); err != nil {
		t.Fatal(err)
	}
	if !sessionsBefore.Valid || sessionsBefore.Time.Before(start) || sessionsBefore.Time.After(time.Now().Add(-retention)) {
		t.Errorf("session purge cutoff = %v, outside expected window", sessionsBefore)
	}
	if !passwordsBefore.Valid || passwordsBefore.Time.Before(start) || passwordsBefore.Time.After(time.Now().Add(-retention)) {
		t.Errorf("password purge cutoff = %v, outside expected window", passwordsBefore)
	}
	if err := svc.SoftDeleteUser(context.Background(), 12); err != nil {
		t.Fatal(err)
	}
	if err := svc.HardDeleteUser(context.Background(), 12); err != nil {
		t.Fatal(err)
	}
}

func TestAdminServiceMaintenanceDatabaseErrors(t *testing.T) {
	dbErr := errors.New("database unavailable")
	q := &adminQuerierStub{
		purgeOldRevokedSessionsFn: func(context.Context, sql.NullTime) error { return dbErr },
		purgeOldPasswordHistoryFn: func(context.Context, sql.NullTime) error { return dbErr },
		softDeleteUserFn:          func(context.Context, int64) error { return dbErr },
		hardDeleteUserFn:          func(context.Context, int64) error { return dbErr },
	}
	svc := NewAdminService(q)

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "sessions", run: func() error { return svc.PurgeOldRevokedSessions(context.Background(), time.Hour) }},
		{name: "passwords", run: func() error { return svc.PurgeOldPasswordHistory(context.Background(), time.Hour) }},
		{name: "soft delete", run: func() error { return svc.SoftDeleteUser(context.Background(), 1) }},
		{name: "hard delete", run: func() error { return svc.HardDeleteUser(context.Background(), 1) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, apperror.ErrInternal) {
				t.Errorf("error = %v, want ErrInternal", err)
			}
		})
	}
}

func TestToNullString(t *testing.T) {
	if got := toNullString(""); got.Valid {
		t.Errorf("toNullString(empty) = %+v, want invalid", got)
	}
	if got := toNullString("value"); !got.Valid || got.String != "value" {
		t.Errorf("toNullString(value) = %+v", got)
	}
}
