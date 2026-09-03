package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/vicky/url-shortner/external/logger"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/enum"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/utils"
)

type mockQuerier struct {
	createFn               func(context.Context, gen.CreateURLParams) (gen.Url, error)
	createUserFn           func(context.Context, gen.CreateUserParams) (gen.CreateUserRow, error)
	byCodeFn               func(context.Context, string) (gen.GetURLByShortCodeRow, error)
	byCodeForUpdateFn      func(context.Context, string) (gen.GetURLByShortCodeForUpdateRow, error)
	byIDFn                 func(context.Context, gen.GetURLByIDParams) (gen.GetURLByIDRow, error)
	listFn                 func(context.Context, gen.ListURLsParams) ([]gen.ListURLsRow, error)
	countFn                func(context.Context, int64) (int64, error)
	emailFn                func(context.Context, string) (gen.GetUserByEmailRow, error)
	updateUserFn           func(context.Context, gen.UpdateUserDisplayIDParams) (gen.UpdateUserDisplayIDRow, error)
	updateFn               func(context.Context, gen.UpdateURLParams) (gen.Url, error)
	softFn                 func(context.Context, gen.SoftDeleteURLParams) (gen.Url, error)
	hardFn                 func(context.Context, gen.HardDeleteURLParams) error
	createDestFn           func(context.Context, gen.CreateDestinationParams) (gen.CreateDestinationRow, error)
	destByHashFn           func(context.Context, string) (gen.GetDestinationByHashRow, error)
	destByIDFn             func(context.Context, int64) (gen.GetDestinationByIDRow, error)
	blockedFn              func(context.Context, string) (gen.GetBlockedDomainRow, error)
	createVerFn            func(context.Context, gen.CreateURLVersionParams) error
	latestVerFn            func(context.Context, int64) (int32, error)
	createClickFn          func(context.Context, gen.CreateClickLogParams) (gen.ClickLog, error)
	incrementClickFn       func(context.Context, int64) error
	updateHealthFn         func(context.Context, gen.UpdateURLHealthStatusParams) (gen.Url, error)
	shortCodeExistsFn      func(context.Context, string) (bool, error)
	listBlockedIPFn        func(context.Context) ([]gen.BlockedIpRange, error)
	createSessionFn        func(context.Context, gen.CreateSessionParams) (gen.Session, error)
	getSessionByHashFn     func(context.Context, string) (gen.Session, error)
	getSessionByIDFn       func(context.Context, int64) (gen.Session, error)
	getUserByIDFn          func(context.Context, int64) (gen.GetUserByIDRow, error)
	listSessionsFn         func(context.Context, int64) ([]gen.Session, error)
	revokeSessionFn        func(context.Context, gen.RevokeSessionParams) error
	updateSessionFn        func(context.Context, int64) error
	addPasswordHistoryFn   func(context.Context, gen.AddPasswordHistoryParams) error
	updateUserPasswordFn   func(context.Context, gen.UpdateUserPasswordParams) (gen.UpdateUserPasswordRow, error)
	listActiveSessionsFn   func(context.Context, int64) ([]gen.Session, error)
	countClicksFn          func(context.Context, gen.CountClickLogsByURLParams) (int64, error)
	listClicksFn           func(context.Context, gen.ListClickLogsByURLParams) ([]gen.ListClickLogsByURLRow, error)
	clickStatsFn           func(context.Context, gen.ClickStatsByURLParams) (gen.ClickStatsByURLRow, error)
	topReferrersFn         func(context.Context, gen.TopReferrersByURLParams) ([]gen.TopReferrersByURLRow, error)
	clicksByDateRangeFn    func(context.Context, gen.ClicksByDateRangeParams) ([]gen.ClicksByDateRangeRow, error)
	revokeAllSessionsFn    func(context.Context, int64) error
	revokeOtherSessionsFn  func(context.Context, gen.RevokeOtherSessionsByUserParams) error
	revokeSessionsExceptFn func(context.Context, gen.RevokeSessionsByUserExceptParams) error
	expireSessionFn        func(context.Context, int64) error
	expireSessionsByUserFn func(context.Context, int64) error
	userStatusByIDFn       func(context.Context, int64) (gen.GetUserStatusByIDRow, error)
	markPendingDeletionFn  func(context.Context, int64) error
	restoreAccountFn       func(context.Context, int64) error
	accountsDueDeletionFn  func(context.Context) ([]int64, error)
	hardDeleteUserByIDFn   func(context.Context, int64) error
	countAuditLogsFn       func(context.Context, gen.CountAuditLogsParams) (int64, error)
	listAuditLogsFn        func(context.Context, gen.ListAuditLogsParams) ([]gen.AuditLog, error)
}

func (m *mockQuerier) ExecContext(_ context.Context, _ string, _ ...interface{}) (sql.Result, error) {
	return nil, nil
}

func (m *mockQuerier) PrepareContext(_ context.Context, _ string) (*sql.Stmt, error) {
	return nil, nil
}

func (m *mockQuerier) QueryContext(_ context.Context, _ string, _ ...interface{}) (*sql.Rows, error) {
	return nil, nil
}

func (m *mockQuerier) QueryRowContext(_ context.Context, _ string, _ ...interface{}) *sql.Row {
	return &sql.Row{}
}

func (m *mockQuerier) GetBlockedDomain(ctx context.Context, domain string) (gen.GetBlockedDomainRow, error) {
	return m.blockedFn(ctx, domain)
}

func (m *mockQuerier) GetDestinationByID(ctx context.Context, id int64) (gen.GetDestinationByIDRow, error) {
	return m.destByIDFn(ctx, id)
}

func (m *mockQuerier) CreateURLVersion(ctx context.Context, arg gen.CreateURLVersionParams) error {
	return m.createVerFn(ctx, arg)
}

func (m *mockQuerier) GetLatestURLVersion(ctx context.Context, urlID int64) (int32, error) {
	return m.latestVerFn(ctx, urlID)
}

func (m *mockQuerier) CreateURL(ctx context.Context, arg gen.CreateURLParams) (gen.Url, error) {
	return m.createFn(ctx, arg)
}

func (m *mockQuerier) CreateUser(ctx context.Context, arg gen.CreateUserParams) (gen.CreateUserRow, error) {
	return m.createUserFn(ctx, arg)
}

func (m *mockQuerier) GetUserByEmail(ctx context.Context, email string) (gen.GetUserByEmailRow, error) {
	return m.emailFn(ctx, email)
}

func (m *mockQuerier) UpdateUserDisplayID(ctx context.Context, arg gen.UpdateUserDisplayIDParams) (gen.UpdateUserDisplayIDRow, error) {
	return m.updateUserFn(ctx, arg)
}

func (m *mockQuerier) GetURLByShortCode(ctx context.Context, shortCode string) (gen.GetURLByShortCodeRow, error) {
	return m.byCodeFn(ctx, shortCode)
}

func (m *mockQuerier) GetURLByShortCodeForUpdate(ctx context.Context, shortCode string) (gen.GetURLByShortCodeForUpdateRow, error) {
	return m.byCodeForUpdateFn(ctx, shortCode)
}

func (m *mockQuerier) CreateClickLog(ctx context.Context, arg gen.CreateClickLogParams) (gen.ClickLog, error) {
	return m.createClickFn(ctx, arg)
}

func (m *mockQuerier) IncrementURLClick(ctx context.Context, id int64) error {
	return m.incrementClickFn(ctx, id)
}

func (m *mockQuerier) GetURLByID(ctx context.Context, arg gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
	return m.byIDFn(ctx, arg)
}

func (m *mockQuerier) ListURLs(ctx context.Context, arg gen.ListURLsParams) ([]gen.ListURLsRow, error) {
	return m.listFn(ctx, arg)
}

func (m *mockQuerier) CreateDestination(ctx context.Context, arg gen.CreateDestinationParams) (gen.CreateDestinationRow, error) {
	return m.createDestFn(ctx, arg)
}

func (m *mockQuerier) GetDestinationByHash(ctx context.Context, urlHash string) (gen.GetDestinationByHashRow, error) {
	return m.destByHashFn(ctx, urlHash)
}

func (m *mockQuerier) CountURLs(ctx context.Context, userID int64) (int64, error) {
	return m.countFn(ctx, userID)
}

func (m *mockQuerier) UpdateURL(ctx context.Context, arg gen.UpdateURLParams) (gen.Url, error) {
	return m.updateFn(ctx, arg)
}

func (m *mockQuerier) SoftDeleteURL(ctx context.Context, arg gen.SoftDeleteURLParams) (gen.Url, error) {
	return m.softFn(ctx, arg)
}

func (m *mockQuerier) HardDeleteURL(ctx context.Context, arg gen.HardDeleteURLParams) error {
	return m.hardFn(ctx, arg)
}

func (m *mockQuerier) UpdateURLHealthStatus(ctx context.Context, arg gen.UpdateURLHealthStatusParams) (gen.Url, error) {
	if m.updateHealthFn != nil {
		return m.updateHealthFn(ctx, arg)
	}
	return gen.Url{}, nil
}

func (m *mockQuerier) ShortCodeExists(ctx context.Context, shortCode string) (bool, error) {
	if m.shortCodeExistsFn != nil {
		return m.shortCodeExistsFn(ctx, shortCode)
	}
	return false, nil
}

func (m *mockQuerier) ListBlockedIPRanges(ctx context.Context) ([]gen.BlockedIpRange, error) {
	if m.listBlockedIPFn != nil {
		return m.listBlockedIPFn(ctx)
	}
	return nil, nil
}

func (m *mockQuerier) CreateSession(ctx context.Context, arg gen.CreateSessionParams) (gen.Session, error) {
	if m.createSessionFn != nil {
		return m.createSessionFn(ctx, arg)
	}
	return gen.Session{}, nil
}

func (m *mockQuerier) GetSessionByRefreshTokenHash(ctx context.Context, refreshTokenHash string) (gen.Session, error) {
	if m.getSessionByHashFn != nil {
		return m.getSessionByHashFn(ctx, refreshTokenHash)
	}
	return gen.Session{}, nil
}

func (m *mockQuerier) GetSessionByID(ctx context.Context, id int64) (gen.Session, error) {
	if m.getSessionByIDFn != nil {
		return m.getSessionByIDFn(ctx, id)
	}
	return gen.Session{}, nil
}

func (m *mockQuerier) GetUserByID(ctx context.Context, id int64) (gen.GetUserByIDRow, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(ctx, id)
	}
	return gen.GetUserByIDRow{}, nil
}

func (m *mockQuerier) ListSessionsByUser(ctx context.Context, userID int64) ([]gen.Session, error) {
	if m.listSessionsFn != nil {
		return m.listSessionsFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockQuerier) RevokeSession(ctx context.Context, arg gen.RevokeSessionParams) error {
	if m.revokeSessionFn != nil {
		return m.revokeSessionFn(ctx, arg)
	}
	return nil
}

func (m *mockQuerier) UpdateSessionLastActive(ctx context.Context, id int64) error {
	if m.updateSessionFn != nil {
		return m.updateSessionFn(ctx, id)
	}
	return nil
}

func (m *mockQuerier) AddPasswordHistory(ctx context.Context, arg gen.AddPasswordHistoryParams) error {
	if m.addPasswordHistoryFn != nil {
		return m.addPasswordHistoryFn(ctx, arg)
	}
	return nil
}

func (m *mockQuerier) UpdateUserPassword(ctx context.Context, arg gen.UpdateUserPasswordParams) (gen.UpdateUserPasswordRow, error) {
	if m.updateUserPasswordFn != nil {
		return m.updateUserPasswordFn(ctx, arg)
	}
	return gen.UpdateUserPasswordRow{}, nil
}

func (m *mockQuerier) GetLastPasswordHistory(_ context.Context, _ int64) (string, error) {
	return "", nil
}

func (m *mockQuerier) ListPasswordHistory(_ context.Context, _ gen.ListPasswordHistoryParams) ([]string, error) {
	return nil, nil
}

func (m *mockQuerier) DeletePasswordHistoryOver(_ context.Context, _ gen.DeletePasswordHistoryOverParams) error {
	return nil
}

func (m *mockQuerier) InsertAuditLog(_ context.Context, _ gen.InsertAuditLogParams) error {
	return nil
}

func (m *mockQuerier) ListActiveSessionsByUser(ctx context.Context, userID int64) ([]gen.Session, error) {
	if m.listActiveSessionsFn != nil {
		return m.listActiveSessionsFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockQuerier) CountClickLogsByURL(ctx context.Context, arg gen.CountClickLogsByURLParams) (int64, error) {
	if m.countClicksFn != nil {
		return m.countClicksFn(ctx, arg)
	}
	return 0, nil
}

func (m *mockQuerier) ListClickLogsByURL(ctx context.Context, arg gen.ListClickLogsByURLParams) ([]gen.ListClickLogsByURLRow, error) {
	if m.listClicksFn != nil {
		return m.listClicksFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockQuerier) ClickStatsByURL(ctx context.Context, arg gen.ClickStatsByURLParams) (gen.ClickStatsByURLRow, error) {
	if m.clickStatsFn != nil {
		return m.clickStatsFn(ctx, arg)
	}
	return gen.ClickStatsByURLRow{}, nil
}

func (m *mockQuerier) TopReferrersByURL(ctx context.Context, arg gen.TopReferrersByURLParams) ([]gen.TopReferrersByURLRow, error) {
	if m.topReferrersFn != nil {
		return m.topReferrersFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockQuerier) ClicksByDateRange(ctx context.Context, arg gen.ClicksByDateRangeParams) ([]gen.ClicksByDateRangeRow, error) {
	if m.clicksByDateRangeFn != nil {
		return m.clicksByDateRangeFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockQuerier) CreateBlockedDomain(_ context.Context, _ gen.CreateBlockedDomainParams) (gen.BlockedDomain, error) {
	return gen.BlockedDomain{}, nil
}

func (m *mockQuerier) ListBlockedDomains(_ context.Context) ([]gen.BlockedDomain, error) {
	return nil, nil
}

func (m *mockQuerier) DeleteBlockedDomain(_ context.Context, _ int32) error {
	return nil
}

func (m *mockQuerier) CreateBlockedIPRange(_ context.Context, _ gen.CreateBlockedIPRangeParams) (gen.BlockedIpRange, error) {
	return gen.BlockedIpRange{}, nil
}

func (m *mockQuerier) DeleteBlockedIPRange(_ context.Context, _ int64) error {
	return nil
}

func (m *mockQuerier) GetDailyStatsByURL(_ context.Context, _ gen.GetDailyStatsByURLParams) ([]gen.GetDailyStatsByURLRow, error) {
	return nil, nil
}

func (m *mockQuerier) UpsertDailyStats(_ context.Context, _ gen.UpsertDailyStatsParams) error {
	return nil
}

func (m *mockQuerier) RefreshDailyStats(_ context.Context, _ gen.RefreshDailyStatsParams) error {
	return nil
}

func (m *mockQuerier) TopBrowsersByURL(_ context.Context, _ gen.TopBrowsersByURLParams) ([]gen.TopBrowsersByURLRow, error) {
	return nil, nil
}

func (m *mockQuerier) TopDeviceTypesByURL(_ context.Context, _ gen.TopDeviceTypesByURLParams) ([]gen.TopDeviceTypesByURLRow, error) {
	return nil, nil
}

func (m *mockQuerier) PurgeOldRevokedSessions(_ context.Context, _ sql.NullTime) error {
	return nil
}

func (m *mockQuerier) PurgeInactiveSessions(_ context.Context, _ sql.NullTime) error {
	return nil
}

func (m *mockQuerier) CountPasswordHistory(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (m *mockQuerier) PurgeOldPasswordHistory(_ context.Context, _ sql.NullTime) error {
	return nil
}

func (m *mockQuerier) SoftDeleteUser(_ context.Context, _ int64) error {
	return nil
}

func (m *mockQuerier) HardDeleteUser(_ context.Context, _ int64) error {
	return nil
}

func (m *mockQuerier) CountRevokedSessions(_ context.Context) (int64, error) {
	return 0, nil
}

func (m *mockQuerier) RevokeAllSessionsByUser(ctx context.Context, userID int64) error {
	if m.revokeAllSessionsFn != nil {
		return m.revokeAllSessionsFn(ctx, userID)
	}
	return nil
}

func (m *mockQuerier) GetUserStatusByID(ctx context.Context, id int64) (gen.GetUserStatusByIDRow, error) {
	if m.userStatusByIDFn != nil {
		return m.userStatusByIDFn(ctx, id)
	}
	return gen.GetUserStatusByIDRow{ID: id, Status: "ACTIVE"}, nil
}

func (m *mockQuerier) MarkPendingDeletion(ctx context.Context, id int64) error {
	if m.markPendingDeletionFn != nil {
		return m.markPendingDeletionFn(ctx, id)
	}
	return nil
}

func (m *mockQuerier) RestoreAccount(ctx context.Context, id int64) error {
	if m.restoreAccountFn != nil {
		return m.restoreAccountFn(ctx, id)
	}
	return nil
}

func (m *mockQuerier) GetAccountsDueForDeletion(ctx context.Context) ([]int64, error) {
	if m.accountsDueDeletionFn != nil {
		return m.accountsDueDeletionFn(ctx)
	}
	return nil, nil
}

func (m *mockQuerier) HardDeleteUserByID(ctx context.Context, id int64) error {
	if m.hardDeleteUserByIDFn != nil {
		return m.hardDeleteUserByIDFn(ctx, id)
	}
	return nil
}

func (m *mockQuerier) CountAuditLogs(ctx context.Context, arg gen.CountAuditLogsParams) (int64, error) {
	if m.countAuditLogsFn != nil {
		return m.countAuditLogsFn(ctx, arg)
	}
	return 0, nil
}

func (m *mockQuerier) ListAuditLogs(ctx context.Context, arg gen.ListAuditLogsParams) ([]gen.AuditLog, error) {
	if m.listAuditLogsFn != nil {
		return m.listAuditLogsFn(ctx, arg)
	}
	return nil, nil
}

func (m *mockQuerier) RevokeOtherSessionsByUser(ctx context.Context, arg gen.RevokeOtherSessionsByUserParams) error {
	if m.revokeOtherSessionsFn != nil {
		return m.revokeOtherSessionsFn(ctx, arg)
	}
	return nil
}

func (m *mockQuerier) RevokeSessionsByUserExcept(ctx context.Context, arg gen.RevokeSessionsByUserExceptParams) error {
	if m.revokeSessionsExceptFn != nil {
		return m.revokeSessionsExceptFn(ctx, arg)
	}
	return nil
}

func (m *mockQuerier) ExpireSession(ctx context.Context, id int64) error {
	if m.expireSessionFn != nil {
		return m.expireSessionFn(ctx, id)
	}
	return nil
}

func (m *mockQuerier) ExpireSessionsByUser(ctx context.Context, userID int64) error {
	if m.expireSessionsByUserFn != nil {
		return m.expireSessionsByUserFn(ctx, userID)
	}
	return nil
}

func (m *mockQuerier) UpdateUserRole(_ context.Context, _ gen.UpdateUserRoleParams) error {
	return nil
}

func testLog(t *testing.T) logger.Logger {
	t.Helper()
	log, err := logger.New(logger.WithLevel("error"))
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

func testURL(code string) gen.Url {
	now := time.Now()
	return gen.Url{
		ID:        1,
		UserID:    1,
		ShortCode: code,
		IsCustom:  sql.NullBool{Bool: false, Valid: true},
		UrlStatus: sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
		CreatedAt: sql.NullTime{Time: now, Valid: true},
		UpdatedAt: sql.NullTime{Time: now, Valid: true},
	}
}

func testListRow(code string) gen.ListURLsRow {
	now := time.Now()
	return gen.ListURLsRow{
		ID:          1,
		UserID:      1,
		ShortCode:   code,
		OriginalUrl: "https://example.com/original",
		IsCustom:    sql.NullBool{Bool: false, Valid: true},
		UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
		CreatedAt:   sql.NullTime{Time: now, Valid: true},
		UpdatedAt:   sql.NullTime{Time: now, Valid: true},
	}
}

func testDestination() gen.GetDestinationByHashRow {
	return gen.GetDestinationByHashRow{
		ID:          1,
		OriginalUrl: "https://example.com/long-url",
	}
}

func TestCreateRejectsBlockedDomain(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{ID: 1, Domain: "blocked.example.com"}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://blocked.example.com/page",
	})
	if err == nil {
		t.Fatal("expected error for blocked domain")
	}
}

func TestCreateGeneratesShortCode(t *testing.T) {
	var captured gen.CreateURLParams
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return testDestination(), nil
		},
		createFn: func(_ context.Context, arg gen.CreateURLParams) (gen.Url, error) {
			captured = arg
			return testURL(arg.ShortCode), nil
		},
		createVerFn: func(_ context.Context, _ gen.CreateURLVersionParams) error {
			return nil
		},
		latestVerFn: func(_ context.Context, _ int64) (int32, error) {
			return 0, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com/long-url",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if len(captured.ShortCode) != 10 {
		t.Errorf("expected 10-char short code, got %q (len %d)", captured.ShortCode, len(captured.ShortCode))
	}
	if resp.ShortURL != "http://localhost:8080/"+captured.ShortCode {
		t.Errorf("unexpected shortURL %q", resp.ShortURL)
	}
}

func TestCreateUsesCustomCode(t *testing.T) {
	var captured gen.CreateURLParams
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return testDestination(), nil
		},
		createFn: func(_ context.Context, arg gen.CreateURLParams) (gen.Url, error) {
			captured = arg
			return testURL(arg.ShortCode), nil
		},
		createVerFn: func(_ context.Context, _ gen.CreateURLVersionParams) error {
			return nil
		},
		latestVerFn: func(_ context.Context, _ int64) (int32, error) {
			return 0, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com/long-url",
		CustomCode:  "my-link",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if captured.ShortCode != "my-link" {
		t.Errorf("expected custom code to be used, got %q", captured.ShortCode)
	}
	if !captured.IsCustom.Bool {
		t.Error("expected isCustom=true for custom code")
	}
	if resp.ShortCode != "my-link" {
		t.Errorf("expected response shortCode my-link, got %q", resp.ShortCode)
	}
}

func TestRedirectNotFound(t *testing.T) {
	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, _ string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{}, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Redirect(context.Background(), "missing", payload.ClickInfo{})
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestRedirectRecordsClickBeforeResponse(t *testing.T) {
	now := time.Now()
	var clickLogCreated, clickIncremented bool
	var capturedClick gen.CreateClickLogParams

	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, code string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{
				ID:          1,
				UserID:      1,
				ShortCode:   code,
				OriginalUrl: "https://example.com/target",
				ClickCount:  sql.NullInt64{Int64: 3, Valid: true},
				IsCustom:    sql.NullBool{Bool: false, Valid: true},
				UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:   sql.NullTime{Time: now, Valid: true},
				UpdatedAt:   sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		createClickFn: func(_ context.Context, arg gen.CreateClickLogParams) (gen.ClickLog, error) {
			capturedClick = arg
			clickLogCreated = true
			return gen.ClickLog{ID: 10, UrlID: arg.UrlID}, nil
		},
		incrementClickFn: func(_ context.Context, id int64) error {
			clickIncremented = true
			if id != 1 {
				t.Errorf("expected url id 1, got %d", id)
			}
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Redirect(context.Background(), "abc1234567", payload.ClickInfo{
		IP:        net.ParseIP("203.0.113.10"),
		UserAgent: "test-agent",
		Referrer:  "https://ref.example.com",
	})
	if err != nil {
		t.Fatalf("redirect: %v", err)
	}

	if resp.OriginalURL != "https://example.com/target" {
		t.Errorf("unexpected originalURL %q", resp.OriginalURL)
	}
	if resp.ClickCount != 3 {
		t.Errorf("expected clickCount 3, got %d", resp.ClickCount)
	}
	if !clickLogCreated {
		t.Error("expected click log to be created before response")
	}
	if !clickIncremented {
		t.Error("expected click count to be incremented before response")
	}
	if capturedClick.UrlID != 1 {
		t.Errorf("expected click log url_id 1, got %d", capturedClick.UrlID)
	}
	if !capturedClick.IpAddress.Valid || capturedClick.IpAddress.IPNet.IP.String() != "203.0.113.10" {
		t.Errorf("unexpected ip in click log: %+v", capturedClick.IpAddress)
	}
	if !capturedClick.UserAgent.Valid || capturedClick.UserAgent.String != "test-agent" {
		t.Errorf("unexpected user agent in click log: %+v", capturedClick.UserAgent)
	}
	if !capturedClick.Referrer.Valid || capturedClick.Referrer.String != "https://ref.example.com" {
		t.Errorf("unexpected referrer in click log: %+v", capturedClick.Referrer)
	}
}

func TestRedirectFailsWhenClickLogFails(t *testing.T) {
	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, code string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{
				ID:        1,
				UserID:    1,
				ShortCode: code,
				UrlStatus: sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
			}, nil
		},
		createClickFn: func(_ context.Context, _ gen.CreateClickLogParams) (gen.ClickLog, error) {
			return gen.ClickLog{}, fmt.Errorf("insert failed")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Redirect(context.Background(), "abc1234567", payload.ClickInfo{})
	if err == nil {
		t.Fatal("expected error when click log insert fails")
	}
}

func TestListPagination(t *testing.T) {
	mock := &mockQuerier{
		listFn: func(_ context.Context, _ gen.ListURLsParams) ([]gen.ListURLsRow, error) {
			return []gen.ListURLsRow{testListRow("abc123"), testListRow("def456")}, nil
		},
		countFn: func(_ context.Context, _ int64) (int64, error) {
			return 25, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, total, err := svc.List(context.Background(), 1, 3, 10, 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if total != 25 {
		t.Errorf("expected total 25, got %d", total)
	}

	if len(resp) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp))
	}
}

func TestSoftDeleteSetsDeletedAt(t *testing.T) {
	deleted := testURL("abc123")
	deleted.DeletedAt = sql.NullTime{Time: time.Now(), Valid: true}

	mock := &mockQuerier{
		softFn: func(_ context.Context, _ gen.SoftDeleteURLParams) (gen.Url, error) {
			return deleted, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.SoftDelete(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if resp.Message == "" {
		t.Error("expected approval message")
	}
	if resp.DeletedAt == "" {
		t.Error("expected deletedAt timestamp")
	}
}

func TestUpdatePersistsUrlStatus(t *testing.T) {
	now := time.Now()
	disabled := int16(enum.URLStatusDisabled)

	var captured gen.UpdateURLParams
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:            1,
				UserID:        1,
				ShortCode:     "abc1234567",
				DestinationID: 1,
				UrlStatus:     sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:     sql.NullTime{Time: now, Valid: true},
				UpdatedAt:     sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		destByIDFn: func(_ context.Context, _ int64) (gen.GetDestinationByIDRow, error) {
			return gen.GetDestinationByIDRow{
				ID:          1,
				OriginalUrl: "https://example.com/original",
			}, nil
		},
		updateFn: func(_ context.Context, arg gen.UpdateURLParams) (gen.Url, error) {
			captured = arg
			return testURL("abc1234567"), nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{
		Status: &disabled,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if !captured.UrlStatus.Valid {
		t.Error("expected urlStatus to be provided to the query")
	}
	if captured.UrlStatus.Int16 != int16(enum.URLStatusDisabled) {
		t.Error("expected status=disabled to be passed through")
	}
	if resp.ID != 1 {
		t.Errorf("expected response id 1, got %d", resp.ID)
	}
}

func TestUpdateLeavesUrlStatusNilWhenOmitted(t *testing.T) {
	var captured gen.UpdateURLParams
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:            1,
				UserID:        1,
				ShortCode:     "abc1234567",
				DestinationID: 1,
				UrlStatus:     sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
			}, nil
		},
		destByIDFn: func(_ context.Context, _ int64) (gen.GetDestinationByIDRow, error) {
			return gen.GetDestinationByIDRow{
				ID:          1,
				OriginalUrl: "https://example.com/original",
			}, nil
		},
		updateFn: func(_ context.Context, arg gen.UpdateURLParams) (gen.Url, error) {
			captured = arg
			url := testURL("abc1234567")
			// Simulate DB returning the preserved url_status = active via COALESCE
			url.UrlStatus = sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true}
			return url, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if captured.UrlStatus.Valid {
		t.Error("expected urlStatus param to be NULL so COALESCE preserves the existing value")
	}
	if !resp.IsActive {
		t.Error("expected isActive to remain true after update without explicit isActive in the request")
	}
}

// ═══════════════════════════════════════════════════════════════
//  CREATE — service layer
// ═══════════════════════════════════════════════════════════════

func TestCreateCustomCodeAlreadyExists(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		shortCodeExistsFn: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
		CustomCode:  "taken",
	})
	if err == nil {
		t.Fatal("expected error for existing custom code")
	}
}

func TestCreateCustomCodeExistsCheckFails(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		shortCodeExistsFn: func(_ context.Context, _ string) (bool, error) {
			return false, fmt.Errorf("db error")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
		CustomCode:  "code",
	})
	if err == nil {
		t.Fatal("expected error when ShortCodeExists fails")
	}
}

func TestCreateDuplicateKeyOnInsert(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{ID: 1, OriginalUrl: "https://example.com"}, nil
		},
		createFn: func(_ context.Context, _ gen.CreateURLParams) (gen.Url, error) {
			return gen.Url{}, &pqError{code: "23505"}
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
	})
	if err == nil {
		t.Fatal("expected conflict error for duplicate key")
	}
}

type pqError struct{ code string }

func (e *pqError) Error() string { return "duplicate key" }

func TestCreateDestinationLookupError(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{}, fmt.Errorf("db error")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error when destination lookup fails")
	}
}

func TestCreateDestinationCreateFails(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{}, sql.ErrNoRows
		},
		createDestFn: func(_ context.Context, _ gen.CreateDestinationParams) (gen.CreateDestinationRow, error) {
			return gen.CreateDestinationRow{}, fmt.Errorf("insert failed")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error when destination create fails")
	}
}

func TestCreateInsertFails(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{ID: 1, OriginalUrl: "https://example.com"}, nil
		},
		createFn: func(_ context.Context, _ gen.CreateURLParams) (gen.Url, error) {
			return gen.Url{}, fmt.Errorf("insert failed")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error when URL insert fails")
	}
}

func TestCreateVersionInsertFails(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{ID: 1, OriginalUrl: "https://example.com"}, nil
		},
		createFn: func(_ context.Context, arg gen.CreateURLParams) (gen.Url, error) {
			return testURL(arg.ShortCode), nil
		},
		createVerFn: func(_ context.Context, _ gen.CreateURLVersionParams) error {
			return fmt.Errorf("version insert failed")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
	})
	if err == nil {
		t.Fatal("expected error when version insert fails")
	}
}

func TestCreateTitleDescriptionExpiresPassed(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	var capturedURL gen.CreateURLParams
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{ID: 1, OriginalUrl: "https://example.com"}, nil
		},
		createFn: func(_ context.Context, arg gen.CreateURLParams) (gen.Url, error) {
			capturedURL = arg
			return testURL(arg.ShortCode), nil
		},
		createVerFn: func(_ context.Context, _ gen.CreateURLVersionParams) error {
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
		Title:       "My Title",
		Description: "My Desc",
		ExpiresAt:   utils.UnixMilliTime{Time: future, Valid: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedURL.Title.String != "My Title" {
		t.Errorf("title = %q, want 'My Title'", capturedURL.Title.String)
	}
	if capturedURL.Description.String != "My Desc" {
		t.Errorf("description = %q, want 'My Desc'", capturedURL.Description.String)
	}
	if !capturedURL.ExpiresAt.Valid {
		t.Error("expiresAt should be valid")
	}
}

func TestCreateResponseHasCorrectFields(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{ID: 1, OriginalUrl: "https://example.com"}, nil
		},
		createFn: func(_ context.Context, _ gen.CreateURLParams) (gen.Url, error) {
			return gen.Url{
				ID:          100,
				UserID:      1,
				ShortCode:   "abc1234567",
				Title:       sql.NullString{String: "Test Title", Valid: true},
				Description: sql.NullString{String: "Test Desc", Valid: true},
				IsCustom:    sql.NullBool{Bool: true, Valid: true},
				UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:   sql.NullTime{Time: now, Valid: true},
				UpdatedAt:   sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		createVerFn: func(_ context.Context, _ gen.CreateURLVersionParams) error {
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
		CustomCode:  "abc1234567",
		Title:       "Test Title",
		Description: "Test Desc",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ShortCode != "abc1234567" {
		t.Errorf("shortCode = %q", resp.ShortCode)
	}
	if resp.OriginalURL != "https://example.com" {
		t.Errorf("originalURL = %q", resp.OriginalURL)
	}
	if resp.Title != "Test Title" {
		t.Errorf("title = %q", resp.Title)
	}
	if resp.Description != "Test Desc" {
		t.Errorf("description = %q", resp.Description)
	}
	if resp.IsActive != true {
		t.Error("isActive should be true")
	}
	if resp.IsCustom == nil || *resp.IsCustom != true {
		t.Error("isCustom should be true")
	}
	if resp.ShortURL != "http://localhost:8080/abc1234567" {
		t.Errorf("shortURL = %q", resp.ShortURL)
	}
}

func TestCreateHealthStatusPassedToURL(t *testing.T) {
	var capturedURL gen.CreateURLParams
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{ID: 1, OriginalUrl: "https://example.com"}, nil
		},
		createFn: func(_ context.Context, arg gen.CreateURLParams) (gen.Url, error) {
			capturedURL = arg
			return testURL(arg.ShortCode), nil
		},
		createVerFn: func(_ context.Context, _ gen.CreateURLVersionParams) error {
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Health check should populate status on the URL row
	if !capturedURL.DestinationHealthStatus.Valid {
		t.Error("destinationHealthStatus should be set")
	}
}

func TestCreateCodeLengthAndFormat(t *testing.T) {
	var capturedURL gen.CreateURLParams
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{ID: 1, OriginalUrl: "https://example.com"}, nil
		},
		createFn: func(_ context.Context, arg gen.CreateURLParams) (gen.Url, error) {
			capturedURL = arg
			return testURL(arg.ShortCode), nil
		},
		createVerFn: func(_ context.Context, _ gen.CreateURLVersionParams) error {
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Create(context.Background(), 1, payload.CreateURLRequest{
		OriginalURL: "https://example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(capturedURL.ShortCode) != 10 {
		t.Errorf("shortCode length = %d, want 10", len(capturedURL.ShortCode))
	}
	if capturedURL.IsCustom.Bool {
		t.Error("isCustom should be false for generated code")
	}
}

// ═══════════════════════════════════════════════════════════════
//  REDIRECT — service layer
// ═══════════════════════════════════════════════════════════════

func TestRedirectURLExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, code string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{
				ID:          1,
				ShortCode:   code,
				ExpiresAt:   sql.NullTime{Time: past, Valid: true},
				UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
				UpdatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
				OriginalUrl: "https://example.com",
			}, nil
		},
		createClickFn: func(_ context.Context, _ gen.CreateClickLogParams) (gen.ClickLog, error) {
			return gen.ClickLog{}, nil
		},
		incrementClickFn: func(_ context.Context, _ int64) error {
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Redirect(context.Background(), "abc", payload.ClickInfo{
		IP:        net.ParseIP("203.0.113.10"),
		UserAgent: "test-agent",
	})
	if err == nil {
		t.Fatal("expected expired error")
	}
}

func TestRedirectRecordsClickWithCorrectIP(t *testing.T) {
	var capturedIP net.IP
	var capturedUA string
	var capturedRef string

	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, code string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{
				ID:          1,
				ShortCode:   code,
				OriginalUrl: "https://example.com",
				ClickCount:  sql.NullInt64{Int64: 5, Valid: true},
				UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
				UpdatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
			}, nil
		},
		createClickFn: func(_ context.Context, arg gen.CreateClickLogParams) (gen.ClickLog, error) {
			capturedIP = arg.IpAddress.IPNet.IP
			capturedUA = arg.UserAgent.String
			capturedRef = arg.Referrer.String
			return gen.ClickLog{ID: 99, UrlID: arg.UrlID}, nil
		},
		incrementClickFn: func(_ context.Context, id int64) error {
			if id != 1 {
				t.Errorf("expected url id 1, got %d", id)
			}
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Redirect(context.Background(), "abc123", payload.ClickInfo{
		IP:        net.ParseIP("10.20.30.40"),
		UserAgent: "Mozilla/5.0",
		Referrer:  "https://google.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedIP == nil || capturedIP.String() != "10.20.30.40" {
		t.Errorf("click IP = %v, want 10.20.30.40", capturedIP)
	}
	if capturedUA != "Mozilla/5.0" {
		t.Errorf("user agent = %q, want 'Mozilla/5.0'", capturedUA)
	}
	if capturedRef != "https://google.com" {
		t.Errorf("referrer = %q, want 'https://google.com'", capturedRef)
	}
	if resp.ClickCount != 5 {
		t.Errorf("clickCount = %d, want 5", resp.ClickCount)
	}
}

func TestRedirectIncrementClickFails(t *testing.T) {
	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, code string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{
				ID:          1,
				ShortCode:   code,
				OriginalUrl: "https://example.com",
				UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
				UpdatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
			}, nil
		},
		createClickFn: func(_ context.Context, _ gen.CreateClickLogParams) (gen.ClickLog, error) {
			return gen.ClickLog{}, nil
		},
		incrementClickFn: func(_ context.Context, _ int64) error {
			return fmt.Errorf("increment failed")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Redirect(context.Background(), "abc", payload.ClickInfo{
		IP:        net.ParseIP("127.0.0.1"),
		UserAgent: "test",
	})
	if err == nil {
		t.Fatal("expected error when increment fails")
	}
}

func TestRedirectResponseFields(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, code string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{
				ID:                      1,
				UserID:                  1,
				ShortCode:               code,
				DestinationID:           100,
				OriginalUrl:             "https://example.com",
				ClickCount:              sql.NullInt64{Int64: 42, Valid: true},
				IsCustom:                sql.NullBool{Bool: true, Valid: true},
				UrlStatus:               sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				DestinationHealthStatus: sql.NullInt16{Int16: int16(enum.DestinationStatusHealthy), Valid: true},
				LastHealthCheck:         sql.NullTime{Time: now, Valid: true},
				CreatedAt:               sql.NullTime{Time: now, Valid: true},
				UpdatedAt:               sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		createClickFn: func(_ context.Context, _ gen.CreateClickLogParams) (gen.ClickLog, error) {
			return gen.ClickLog{}, nil
		},
		incrementClickFn: func(_ context.Context, _ int64) error {
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Redirect(context.Background(), "abc123", payload.ClickInfo{
		IP:        net.ParseIP("127.0.0.1"),
		UserAgent: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.OriginalURL != "https://example.com" {
		t.Errorf("originalURL = %q", resp.OriginalURL)
	}
	if resp.ClickCount != 42 {
		t.Errorf("clickCount = %d, want 42", resp.ClickCount)
	}
	if resp.ShortURL != "http://localhost:8080/abc123" {
		t.Errorf("shortURL = %q", resp.ShortURL)
	}
	if resp.DestinationStatusString != "Healthy" {
		t.Errorf("healthStatus = %q, want 'Healthy'", resp.DestinationStatusString)
	}
	if resp.DestinationHttpCode != "" {
		t.Errorf("httpCode = %q, want empty (field not in query)", resp.DestinationHttpCode)
	}
}

func TestRedirectNullClickCount(t *testing.T) {
	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, code string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{
				ID:          1,
				ShortCode:   code,
				OriginalUrl: "https://example.com",
				UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
				UpdatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
			}, nil
		},
		createClickFn: func(_ context.Context, _ gen.CreateClickLogParams) (gen.ClickLog, error) {
			return gen.ClickLog{}, nil
		},
		incrementClickFn: func(_ context.Context, _ int64) error {
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Redirect(context.Background(), "abc", payload.ClickInfo{
		IP:        net.ParseIP("127.0.0.1"),
		UserAgent: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ClickCount != 0 {
		t.Errorf("clickCount = %d, want 0 for null", resp.ClickCount)
	}
}

func TestRedirectClickLogFails(t *testing.T) {
	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, code string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{
				ID:          1,
				ShortCode:   code,
				OriginalUrl: "https://example.com",
				UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
				UpdatedAt:   sql.NullTime{Time: time.Now(), Valid: true},
			}, nil
		},
		createClickFn: func(_ context.Context, _ gen.CreateClickLogParams) (gen.ClickLog, error) {
			return gen.ClickLog{}, fmt.Errorf("click log insert failed")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Redirect(context.Background(), "abc", payload.ClickInfo{
		IP:        net.ParseIP("127.0.0.1"),
		UserAgent: "test",
	})
	if err == nil {
		t.Fatal("expected error when click log fails")
	}
}

// ═══════════════════════════════════════════════════════════════
//  REDIRECT CACHE — service layer
// ═══════════════════════════════════════════════════════════════

// redirectCacheAdapter adapts *mockCache (which has Set with variadic options)
// to the simpler URLRedirectCache interface (Set without options).
type redirectCacheAdapter struct {
	mc *mockCache
}

func (a *redirectCacheAdapter) Get(ctx context.Context, key string) (string, error) {
	return a.mc.Get(ctx, key)
}
func (a *redirectCacheAdapter) Set(ctx context.Context, key, value string) error {
	return a.mc.Set(ctx, key, value)
}
func (a *redirectCacheAdapter) Del(ctx context.Context, key string) error {
	return a.mc.Del(ctx, key)
}

func TestRedirectCacheHitDoesNotHitDB(t *testing.T) {
	mc := &redirectCacheAdapter{mc: newMockCache()}
	data := `{"id":1,"original_url":"https://example.com/cached-target","url_status":1}`
	_ = mc.Set(context.Background(), cacheKeyRedirectPrefix+"abc1234567", data)

	dbHit := false
	var clickLogged bool
	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, _ string) (gen.GetURLByShortCodeForUpdateRow, error) {
			dbHit = true
			return gen.GetURLByShortCodeForUpdateRow{}, nil
		},
		createClickFn: func(_ context.Context, _ gen.CreateClickLogParams) (gen.ClickLog, error) {
			clickLogged = true
			return gen.ClickLog{}, nil
		},
		incrementClickFn: func(_ context.Context, _ int64) error {
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t), WithRedirectCache(mc))

	resp, err := svc.Redirect(context.Background(), "abc1234567", payload.ClickInfo{
		IP:        net.ParseIP("203.0.113.10"),
		UserAgent: "agent",
	})
	if err != nil {
		t.Fatalf("redirect: %v", err)
	}
	if dbHit {
		t.Error("expected DB not to be hit on cache hit")
	}
	if !clickLogged {
		t.Error("expected click to be logged on cache hit")
	}
	if resp.OriginalURL != "https://example.com/cached-target" {
		t.Errorf("originalURL = %q, want cached target", resp.OriginalURL)
	}
	if resp.ShortCode != "abc1234567" {
		t.Errorf("shortCode = %q, want abc1234567", resp.ShortCode)
	}
}

func TestRedirectCacheMissFallsBackToDBAndPopulates(t *testing.T) {
	mc := &redirectCacheAdapter{mc: newMockCache()}
	now := time.Now()

	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, code string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{
				ID:          1,
				UserID:      1,
				ShortCode:   code,
				OriginalUrl: "https://example.com/db-target",
				UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:   sql.NullTime{Time: now, Valid: true},
				UpdatedAt:   sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		createClickFn: func(_ context.Context, _ gen.CreateClickLogParams) (gen.ClickLog, error) {
			return gen.ClickLog{}, nil
		},
		incrementClickFn: func(_ context.Context, _ int64) error {
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t), WithRedirectCache(mc))

	_, err := svc.Redirect(context.Background(), "abc1234567", payload.ClickInfo{})
	if err != nil {
		t.Fatalf("redirect: %v", err)
	}

	// Cache should now be populated.
	raw, cErr := mc.Get(context.Background(), cacheKeyRedirectPrefix+"abc1234567")
	if cErr != nil {
		t.Fatalf("expected cache to be populated, got error: %v", cErr)
	}
	var data redirectCacheData
	if json.Unmarshal([]byte(raw), &data) != nil {
		t.Fatalf("expected valid JSON in cache, got: %s", raw)
	}
	if data.OriginalURL != "https://example.com/db-target" {
		t.Errorf("cached originalURL = %q, want db-target", data.OriginalURL)
	}
	if data.ID != 1 {
		t.Errorf("cached id = %d, want 1", data.ID)
	}
}

func TestRedirectCacheExpiredInvalidates(t *testing.T) {
	mc := &redirectCacheAdapter{mc: newMockCache()}
	data := fmt.Sprintf(`{"id":1,"original_url":"https://example.com/old","expires_at":"%s","url_status":1}`,
		time.Now().Add(-time.Hour).Format(time.RFC3339))
	_ = mc.Set(context.Background(), cacheKeyRedirectPrefix+"expired", data)

	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret-key", testLog(t), WithRedirectCache(mc))

	_, err := svc.Redirect(context.Background(), "expired", payload.ClickInfo{})
	if err == nil {
		t.Fatal("expected expired error")
	}

	if _, cErr := mc.Get(context.Background(), cacheKeyRedirectPrefix+"expired"); cErr == nil {
		t.Error("expected expired cache entry to be invalidated")
	}
}

func TestRedirectCacheInactiveInvalidates(t *testing.T) {
	mc := &redirectCacheAdapter{mc: newMockCache()}
	data := `{"id":1,"original_url":"https://example.com/old","url_status":0}`
	_ = mc.Set(context.Background(), cacheKeyRedirectPrefix+"inactive", data)

	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret-key", testLog(t), WithRedirectCache(mc))

	_, err := svc.Redirect(context.Background(), "inactive", payload.ClickInfo{})
	if err == nil {
		t.Fatal("expected not found error for inactive cached url")
	}

	if _, cErr := mc.Get(context.Background(), cacheKeyRedirectPrefix+"inactive"); cErr == nil {
		t.Error("expected inactive cache entry to be invalidated")
	}
}

func TestRedirectCacheNotAvailableFallsBackToDB(t *testing.T) {
	mc := &redirectCacheAdapter{mc: newMockCache()}
	now := time.Now()

	mock := &mockQuerier{
		byCodeForUpdateFn: func(_ context.Context, code string) (gen.GetURLByShortCodeForUpdateRow, error) {
			return gen.GetURLByShortCodeForUpdateRow{
				ID:          1,
				UserID:      1,
				ShortCode:   code,
				OriginalUrl: "https://example.com/db-target",
				UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:   sql.NullTime{Time: now, Valid: true},
				UpdatedAt:   sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		createClickFn: func(_ context.Context, _ gen.CreateClickLogParams) (gen.ClickLog, error) {
			return gen.ClickLog{}, nil
		},
		incrementClickFn: func(_ context.Context, _ int64) error {
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t), WithRedirectCache(mc))

	resp, err := svc.Redirect(context.Background(), "abc1234567", payload.ClickInfo{})
	if err != nil {
		t.Fatalf("redirect: %v", err)
	}
	if resp.OriginalURL != "https://example.com/db-target" {
		t.Errorf("originalURL = %q, want db-target", resp.OriginalURL)
	}
}

// ═══════════════════════════════════════════════════════════════
//  GET BY ID — service layer
// ═══════════════════════════════════════════════════════════════

func TestGetByIDNotFound(t *testing.T) {
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{}, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.GetByID(context.Background(), 1, 999)
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestGetByIDQueryError(t *testing.T) {
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{}, fmt.Errorf("db error")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.GetByID(context.Background(), 1, 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetByIDResponseFields(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:                      1,
				UserID:                  1,
				ShortCode:               "abc123",
				DestinationID:           100,
				OriginalUrl:             "https://example.com",
				Title:                   sql.NullString{String: "Title", Valid: true},
				Description:             sql.NullString{String: "Desc", Valid: true},
				IsCustom:                sql.NullBool{Bool: true, Valid: true},
				ClickCount:              sql.NullInt64{Int64: 10, Valid: true},
				UrlStatus:               sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				DestinationHealthStatus: sql.NullInt16{Int16: int16(enum.DestinationStatusHealthy), Valid: true},
				LastHealthCheck:         sql.NullTime{Time: now, Valid: true},
				CreatedAt:               sql.NullTime{Time: now, Valid: true},
				UpdatedAt:               sql.NullTime{Time: now, Valid: true},
			}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.GetByID(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.ShortCode != "abc123" {
		t.Errorf("shortCode = %q", resp.ShortCode)
	}
	if resp.Title != "Title" {
		t.Errorf("title = %q", resp.Title)
	}
	if resp.Description != "Desc" {
		t.Errorf("description = %q", resp.Description)
	}
	if resp.ClickCount != 10 {
		t.Errorf("clickCount = %d", resp.ClickCount)
	}
	if resp.OriginalURL != "https://example.com" {
		t.Errorf("originalURL = %q", resp.OriginalURL)
	}
	if resp.DestinationStatusString != "Healthy" {
		t.Errorf("healthStatus = %q, want 'Healthy'", resp.DestinationStatusString)
	}
	if resp.DestinationHttpCode != "" {
		t.Errorf("httpCode = %q, want empty (field not in query)", resp.DestinationHttpCode)
	}
}

func TestGetByIDNullFields(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:          1,
				UserID:      1,
				ShortCode:   "abc",
				OriginalUrl: "https://example.com",
				UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:   sql.NullTime{Time: now, Valid: true},
				UpdatedAt:   sql.NullTime{Time: now, Valid: true},
			}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.GetByID(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Title != "" {
		t.Errorf("title = %q, want empty", resp.Title)
	}
	if resp.Description != "" {
		t.Errorf("description = %q, want empty", resp.Description)
	}
	if resp.ClickCount != 0 {
		t.Errorf("clickCount = %d, want 0", resp.ClickCount)
	}
	if resp.HasBeenAccessed {
		t.Error("hasBeenAccessed should be false")
	}
}

// ═══════════════════════════════════════════════════════════════
//  LIST — service layer
// ═══════════════════════════════════════════════════════════════

func TestListEmpty(t *testing.T) {
	mock := &mockQuerier{
		listFn: func(_ context.Context, _ gen.ListURLsParams) ([]gen.ListURLsRow, error) {
			return []gen.ListURLsRow{}, nil
		},
		countFn: func(_ context.Context, _ int64) (int64, error) {
			return 0, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	items, total, err := svc.List(context.Background(), 1, 1, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
}

func TestListCountFails(t *testing.T) {
	mock := &mockQuerier{
		listFn: func(_ context.Context, _ gen.ListURLsParams) ([]gen.ListURLsRow, error) {
			return []gen.ListURLsRow{}, nil
		},
		countFn: func(_ context.Context, _ int64) (int64, error) {
			return 0, fmt.Errorf("count error")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, _, err := svc.List(context.Background(), 1, 1, 10, 0)
	if err == nil {
		t.Fatal("expected error when count fails")
	}
}

func TestListQueryFails(t *testing.T) {
	mock := &mockQuerier{
		listFn: func(_ context.Context, _ gen.ListURLsParams) ([]gen.ListURLsRow, error) {
			return nil, fmt.Errorf("list error")
		},
		countFn: func(_ context.Context, _ int64) (int64, error) {
			return 0, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, _, err := svc.List(context.Background(), 1, 1, 10, 0)
	if err == nil {
		t.Fatal("expected error when list fails")
	}
}

func TestListPaginationMath(t *testing.T) {
	mock := &mockQuerier{
		listFn: func(_ context.Context, _ gen.ListURLsParams) ([]gen.ListURLsRow, error) {
			return []gen.ListURLsRow{
				testListRow("abc"),
				testListRow("def"),
			}, nil
		},
		countFn: func(_ context.Context, _ int64) (int64, error) {
			return 25, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	items, total, err := svc.List(context.Background(), 1, 3, 10, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if total != 25 {
		t.Errorf("total = %d, want 25", total)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items, got %d", len(items))
	}
}

func TestListExactMultiple(t *testing.T) {
	mock := &mockQuerier{
		listFn: func(_ context.Context, _ gen.ListURLsParams) ([]gen.ListURLsRow, error) {
			return []gen.ListURLsRow{testListRow("a")}, nil
		},
		countFn: func(_ context.Context, _ int64) (int64, error) {
			return 20, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	items, total, err := svc.List(context.Background(), 1, 1, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 20 {
		t.Errorf("total = %d, want 20", total)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item, got %d", len(items))
	}
}

func TestListAllNullFields(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		listFn: func(_ context.Context, _ gen.ListURLsParams) ([]gen.ListURLsRow, error) {
			return []gen.ListURLsRow{
				{
					ID:          1,
					UserID:      1,
					ShortCode:   "abc",
					OriginalUrl: "https://example.com",
					UrlStatus:   sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
					CreatedAt:   sql.NullTime{Time: now, Valid: true},
					UpdatedAt:   sql.NullTime{Time: now, Valid: true},
				},
			}, nil
		},
		countFn: func(_ context.Context, _ int64) (int64, error) {
			return 1, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	items, total, err := svc.List(context.Background(), 1, 1, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(items) != 1 {
		t.Fatal("expected 1 item")
	}
	item, ok := items[0].(payload.URLResponse)
	if !ok {
		t.Fatalf("expected payload.URLResponse, got %T", items[0])
	}
	if item.ClickCount != 0 {
		t.Errorf("clickCount = %d, want 0", item.ClickCount)
	}
	if item.DestinationStatusString != "Unknown / Not Checked" {
		t.Errorf("healthStatus = %q, want 'Unknown / Not Checked'", item.DestinationStatusString)
	}
	if item.DestinationHttpCode != "" {
		t.Errorf("httpCode = %q, want empty", item.DestinationHttpCode)
	}
	if item.IsCustom == nil || *item.IsCustom {
		t.Error("isCustom should be false")
	}
}

func TestListMultipleItemsWithHealthStatus(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		listFn: func(_ context.Context, _ gen.ListURLsParams) ([]gen.ListURLsRow, error) {
			return []gen.ListURLsRow{
				{
					ID:                      1,
					UserID:                  1,
					ShortCode:               "healthy",
					OriginalUrl:             "https://healthy.com",
					DestinationHealthStatus: sql.NullInt16{Int16: int16(enum.DestinationStatusHealthy), Valid: true},
					ClickCount:              sql.NullInt64{Int64: 5, Valid: true},
					UrlStatus:               sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
					CreatedAt:               sql.NullTime{Time: now, Valid: true},
					UpdatedAt:               sql.NullTime{Time: now, Valid: true},
				},
				{
					ID:                      2,
					UserID:                  1,
					ShortCode:               "unhealthy",
					OriginalUrl:             "https://unhealthy.com",
					DestinationHealthStatus: sql.NullInt16{Int16: int16(enum.DestinationStatusUnhealthy), Valid: true},
					UrlStatus:               sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
					CreatedAt:               sql.NullTime{Time: now, Valid: true},
					UpdatedAt:               sql.NullTime{Time: now, Valid: true},
				},
			}, nil
		},
		countFn: func(_ context.Context, _ int64) (int64, error) {
			return 2, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	items, _, err := svc.List(context.Background(), 1, 1, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	first, ok := items[0].(payload.URLResponse)
	if !ok {
		t.Fatalf("expected payload.URLResponse, got %T", items[0])
	}
	second, ok := items[1].(payload.URLResponse)
	if !ok {
		t.Fatalf("expected payload.URLResponse, got %T", items[1])
	}
	if first.DestinationStatusString != "Healthy" {
		t.Errorf("healthStatus[0] = %q", first.DestinationStatusString)
	}
	if second.DestinationStatusString != "Unhealthy" {
		t.Errorf("healthStatus[1] = %q", second.DestinationStatusString)
	}
}

// ═══════════════════════════════════════════════════════════════
//  UPDATE — service layer
// ═══════════════════════════════════════════════════════════════

func TestUpdateOriginalURLChange(t *testing.T) {
	now := time.Now()
	var capturedVer gen.CreateURLVersionParams
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:            1,
				UserID:        1,
				ShortCode:     "abc",
				DestinationID: 100,
				OriginalUrl:   "https://old.com",
				UrlStatus:     sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:     sql.NullTime{Time: now, Valid: true},
				UpdatedAt:     sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{}, sql.ErrNoRows
		},
		createDestFn: func(_ context.Context, _ gen.CreateDestinationParams) (gen.CreateDestinationRow, error) {
			return gen.CreateDestinationRow{ID: 200}, nil
		},
		updateFn: func(_ context.Context, arg gen.UpdateURLParams) (gen.Url, error) {
			return testURL("abc"), nil
		},
		latestVerFn: func(_ context.Context, _ int64) (int32, error) {
			return 0, sql.ErrNoRows
		},
		createVerFn: func(_ context.Context, arg gen.CreateURLVersionParams) error {
			capturedVer = arg
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{
		OriginalURL: "https://new.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedVer.UrlID != resp.ID {
		t.Errorf("version urlId = %d, want %d", capturedVer.UrlID, resp.ID)
	}
	if capturedVer.OriginalUrl != "https://new.com" {
		t.Errorf("version originalUrl = %q, want 'https://new.com'", capturedVer.OriginalUrl)
	}
}

func TestUpdateNotFound(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{}, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Update(context.Background(), 1, 999, payload.UpdateURLRequest{
		Title: "new title",
	})
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestUpdateGetByIDFails(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{}, fmt.Errorf("db error")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{
		Title: "new",
	})
	if err == nil {
		t.Fatal("expected error when GetByID fails")
	}
}

func TestUpdateURLNotFoundAfterFetch(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:            1,
				UserID:        1,
				ShortCode:     "abc",
				DestinationID: 100,
				OriginalUrl:   "https://same.com",
				UrlStatus:     sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:     sql.NullTime{Time: now, Valid: true},
				UpdatedAt:     sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		destByIDFn: func(_ context.Context, _ int64) (gen.GetDestinationByIDRow, error) {
			return gen.GetDestinationByIDRow{ID: 100, OriginalUrl: "https://same.com"}, nil
		},
		updateFn: func(_ context.Context, _ gen.UpdateURLParams) (gen.Url, error) {
			return gen.Url{}, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{
		Title: "new",
	})
	if err == nil {
		t.Fatal("expected error when update returns no rows")
	}
}

func TestUpdateDestinationNotFound(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:            1,
				UserID:        1,
				ShortCode:     "abc",
				DestinationID: 100,
				OriginalUrl:   "https://same.com",
				UrlStatus:     sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:     sql.NullTime{Time: now, Valid: true},
				UpdatedAt:     sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{}, sql.ErrNoRows
		},
		createDestFn: func(_ context.Context, _ gen.CreateDestinationParams) (gen.CreateDestinationRow, error) {
			return gen.CreateDestinationRow{}, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{
		OriginalURL: "https://new.com",
	})
	if err == nil {
		t.Fatal("expected error when destination creation fails")
	}
}

func TestUpdateVersionInsertFails(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:            1,
				UserID:        1,
				ShortCode:     "abc",
				DestinationID: 100,
				OriginalUrl:   "https://old.com",
				UrlStatus:     sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:     sql.NullTime{Time: now, Valid: true},
				UpdatedAt:     sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{}, sql.ErrNoRows
		},
		createDestFn: func(_ context.Context, _ gen.CreateDestinationParams) (gen.CreateDestinationRow, error) {
			return gen.CreateDestinationRow{ID: 200}, nil
		},
		updateFn: func(_ context.Context, _ gen.UpdateURLParams) (gen.Url, error) {
			return testURL("abc"), nil
		},
		latestVerFn: func(_ context.Context, _ int64) (int32, error) {
			return 0, sql.ErrNoRows
		},
		createVerFn: func(_ context.Context, _ gen.CreateURLVersionParams) error {
			return fmt.Errorf("version insert failed")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{
		OriginalURL: "https://new.com",
	})
	if err == nil {
		t.Fatal("expected error when version insert fails")
	}
}

func TestUpdateStatusChange(t *testing.T) {
	now := time.Now()
	var captured gen.UpdateURLParams
	disabled := int16(enum.URLStatusDisabled)
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:            1,
				UserID:        1,
				ShortCode:     "abc",
				DestinationID: 100,
				OriginalUrl:   "https://same.com",
				UrlStatus:     sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:     sql.NullTime{Time: now, Valid: true},
				UpdatedAt:     sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		destByIDFn: func(_ context.Context, _ int64) (gen.GetDestinationByIDRow, error) {
			return gen.GetDestinationByIDRow{ID: 100, OriginalUrl: "https://same.com"}, nil
		},
		updateFn: func(_ context.Context, arg gen.UpdateURLParams) (gen.Url, error) {
			captured = arg
			url := testURL("abc")
			url.UrlStatus = sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true}
			return url, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{
		Status: &disabled,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured.UrlStatus.Int16 != int16(enum.URLStatusDisabled) {
		t.Errorf("urlStatus = %d, want Disabled", captured.UrlStatus.Int16)
	}
	if !captured.UrlStatus.Valid {
		t.Error("urlStatus should be valid")
	}
	if resp.ID != 1 {
		t.Errorf("ID = %d, want 1", resp.ID)
	}
}

func TestUpdateExpiresAt(t *testing.T) {
	now := time.Now()
	future := time.Now().Add(24 * time.Hour)
	var captured gen.UpdateURLParams
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, sql.ErrNoRows
		},
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:            1,
				UserID:        1,
				ShortCode:     "abc",
				DestinationID: 100,
				OriginalUrl:   "https://same.com",
				UrlStatus:     sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
				CreatedAt:     sql.NullTime{Time: now, Valid: true},
				UpdatedAt:     sql.NullTime{Time: now, Valid: true},
			}, nil
		},
		destByIDFn: func(_ context.Context, _ int64) (gen.GetDestinationByIDRow, error) {
			return gen.GetDestinationByIDRow{ID: 100, OriginalUrl: "https://same.com"}, nil
		},
		updateFn: func(_ context.Context, arg gen.UpdateURLParams) (gen.Url, error) {
			captured = arg
			return testURL("abc"), nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{
		ExpiresAt: utils.UnixMilliTime{Time: future, Valid: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !captured.ExpiresAt.Valid {
		t.Error("expiresAt should be valid")
	}
}

func TestUpdateBlockedDomain(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{ID: 1, Domain: "evil.com"}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{
		OriginalURL: "https://evil.com/path",
	})
	if err == nil {
		t.Fatal("expected blocked domain error")
	}
}

// ═══════════════════════════════════════════════════════════════
//  SOFT DELETE — service layer
// ═══════════════════════════════════════════════════════════════

func TestSoftDeleteNotFound(t *testing.T) {
	mock := &mockQuerier{
		softFn: func(_ context.Context, _ gen.SoftDeleteURLParams) (gen.Url, error) {
			return gen.Url{}, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.SoftDelete(context.Background(), 1, 999)
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestSoftDeleteQueryError(t *testing.T) {
	mock := &mockQuerier{
		softFn: func(_ context.Context, _ gen.SoftDeleteURLParams) (gen.Url, error) {
			return gen.Url{}, fmt.Errorf("db error")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.SoftDelete(context.Background(), 1, 1)
	if err == nil {
		t.Fatal("expected error when query fails")
	}
}

func TestSoftDeleteResponse(t *testing.T) {
	deletedAt := time.Now().Add(7 * 24 * time.Hour)
	mock := &mockQuerier{
		softFn: func(_ context.Context, _ gen.SoftDeleteURLParams) (gen.Url, error) {
			return gen.Url{
				ID:        1,
				ShortCode: "abc123",
				DeletedAt: sql.NullTime{Time: deletedAt, Valid: true},
			}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.SoftDelete(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != 1 {
		t.Errorf("ID = %d, want 1", resp.ID)
	}
	if resp.ShortCode != "abc123" {
		t.Errorf("shortCode = %q, want 'abc123'", resp.ShortCode)
	}
	if resp.Message == "" {
		t.Error("message should not be empty")
	}
	if resp.DeletedAt == "" {
		t.Error("deletedAt should not be empty")
	}
}

func TestSoftDeleteInvalidatesCache(t *testing.T) {
	inner := newMockCache()
	mc := &redirectCacheAdapter{mc: inner}
	_ = inner.Set(context.Background(), cacheKeyRedirectPrefix+"abc123",
		`{"id":1,"original_url":"https://example.com/target","url_status":1}`)

	mock := &mockQuerier{
		softFn: func(_ context.Context, _ gen.SoftDeleteURLParams) (gen.Url, error) {
			return gen.Url{ID: 1, ShortCode: "abc123"}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t), WithRedirectCache(mc))

	if _, err := svc.SoftDelete(context.Background(), 1, 1); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	if _, cErr := mc.Get(context.Background(), cacheKeyRedirectPrefix+"abc123"); cErr == nil {
		t.Error("expected cache entry to be invalidated after soft delete")
	}
}

// ═══════════════════════════════════════════════════════════════
//  HARD DELETE — service layer
// ═══════════════════════════════════════════════════════════════

func TestHardDeleteSuccess(t *testing.T) {
	var capturedParams gen.HardDeleteURLParams
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:        42,
				UserID:    1,
				ShortCode: "abc123",
			}, nil
		},
		hardFn: func(_ context.Context, arg gen.HardDeleteURLParams) error {
			capturedParams = arg
			return nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	err := svc.HardDelete(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedParams.ID != 42 {
		t.Errorf("ID = %d, want 42", capturedParams.ID)
	}
	if capturedParams.UserID != 1 {
		t.Errorf("userID = %d, want 1", capturedParams.UserID)
	}
}

func TestHardDeleteQueryError(t *testing.T) {
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{ID: 1, UserID: 1, ShortCode: "abc123"}, nil
		},
		hardFn: func(_ context.Context, _ gen.HardDeleteURLParams) error {
			return fmt.Errorf("db error")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	err := svc.HardDelete(context.Background(), 1, 1)
	if err == nil {
		t.Fatal("expected error when query fails")
	}
}

func TestHardDeleteNotSoftDeleted(t *testing.T) {
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{ID: 1, UserID: 1, ShortCode: "abc123"}, nil
		},
		hardFn: func(_ context.Context, _ gen.HardDeleteURLParams) error {
			return sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	err := svc.HardDelete(context.Background(), 1, 1)
	if err == nil {
		t.Fatal("expected error when URL not soft-deleted")
	}
}

func TestHardDeleteFetchError(t *testing.T) {
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{}, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	err := svc.HardDelete(context.Background(), 1, 999)
	if err == nil {
		t.Fatal("expected not found error when URL does not exist")
	}
}

// ═══════════════════════════════════════════════════════════════
//  GENERATE SHORT CODE
// ═══════════════════════════════════════════════════════════════

func TestGenerateShortCodeLengthAndHex(t *testing.T) {
	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret-key", testLog(t))
	for i := 0; i < 100; i++ {
		code, err := svc.generateShortCode()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(code) != 10 {
			t.Fatalf("code length = %d, want 10", len(code))
		}
		for _, c := range code {
			if c < '0' || (c > '9' && c < 'a') || c > 'f' {
				t.Fatalf("code %q contains non-hex char %c", code, c)
			}
		}
	}
}

func TestGenerateShortCodeUniqueness(t *testing.T) {
	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret-key", testLog(t))
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code, err := svc.generateShortCode()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen[code] {
			t.Fatalf("duplicate code generated: %s", code)
		}
		seen[code] = true
	}
}

// ═══════════════════════════════════════════════════════════════
//  RESOLVE USER ID
// ═══════════════════════════════════════════════════════════════

func TestResolveUserIDSuccess(t *testing.T) {
	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret", testLog(t))
	encoded := utils.EncodeID(42, utils.UserIDPrefix, "test-secret")

	id, err := svc.ResolveUserID(context.Background(), encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Errorf("id = %d, want 42", id)
	}
}

func TestResolveUserIDInvalid(t *testing.T) {
	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret", testLog(t))

	_, err := svc.ResolveUserID(context.Background(), "invalid-token")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestResolveUserIDWrongSecret(t *testing.T) {
	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "secret-A", testLog(t))
	encoded := utils.EncodeID(42, utils.UserIDPrefix, "secret-B")

	id, err := svc.ResolveUserID(context.Background(), encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id == 42 {
		t.Fatal("wrong secret should produce different ID")
	}
}

// ═══════════════════════════════════════════════════════════════
//  CHECK BLOCKED DOMAIN
// ═══════════════════════════════════════════════════════════════

func TestCheckBlockedDomainQueryError(t *testing.T) {
	mock := &mockQuerier{
		blockedFn: func(_ context.Context, _ string) (gen.GetBlockedDomainRow, error) {
			return gen.GetBlockedDomainRow{}, fmt.Errorf("db error")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret", testLog(t))

	err := svc.checkBlockedDomain(context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected error when query fails")
	}
}

func TestCheckBlockedDomainInvalidURL(t *testing.T) {
	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret", testLog(t))

	err := svc.checkBlockedDomain(context.Background(), "not-a-url")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

// ═══════════════════════════════════════════════════════════════
//  FIND OR CREATE DESTINATION
// ═══════════════════════════════════════════════════════════════

func TestFindOrCreateCacheHit(t *testing.T) {
	var createCalled bool
	mock := &mockQuerier{
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{ID: 42, OriginalUrl: "https://example.com"}, nil
		},
		createDestFn: func(_ context.Context, _ gen.CreateDestinationParams) (gen.CreateDestinationRow, error) {
			createCalled = true
			return gen.CreateDestinationRow{}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret", testLog(t))

	id, err := svc.findOrCreateDestination(mock, context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Errorf("id = %d, want 42", id)
	}
	if createCalled {
		t.Error("should not call create when destination exists")
	}
}

func TestFindOrCreateCacheMiss(t *testing.T) {
	mock := &mockQuerier{
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{}, sql.ErrNoRows
		},
		createDestFn: func(_ context.Context, _ gen.CreateDestinationParams) (gen.CreateDestinationRow, error) {
			return gen.CreateDestinationRow{ID: 99}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret", testLog(t))

	id, err := svc.findOrCreateDestination(mock, context.Background(), "https://new.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 99 {
		t.Errorf("id = %d, want 99", id)
	}
}

func TestFindOrCreateLookupDBError(t *testing.T) {
	mock := &mockQuerier{
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{}, fmt.Errorf("db error")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret", testLog(t))

	_, err := svc.findOrCreateDestination(mock, context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected error when lookup fails")
	}
}

func TestFindOrCreateCreateDBError(t *testing.T) {
	mock := &mockQuerier{
		destByHashFn: func(_ context.Context, _ string) (gen.GetDestinationByHashRow, error) {
			return gen.GetDestinationByHashRow{}, sql.ErrNoRows
		},
		createDestFn: func(_ context.Context, _ gen.CreateDestinationParams) (gen.CreateDestinationRow, error) {
			return gen.CreateDestinationRow{}, fmt.Errorf("create failed")
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret", testLog(t))

	_, err := svc.findOrCreateDestination(mock, context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected error when create fails")
	}
}

// ═══════════════════════════════════════════════════════════════
//  TO RESPONSE — helper
// ═══════════════════════════════════════════════════════════════

func TestToResponseAllFields(t *testing.T) {
	now := time.Now()
	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret", testLog(t))

	u := gen.Url{
		ID:                        1,
		UserID:                    1,
		ShortCode:                 "abc",
		DestinationID:             100,
		Title:                     sql.NullString{String: "Title", Valid: true},
		Description:               sql.NullString{String: "Desc", Valid: true},
		IsCustom:                  sql.NullBool{Bool: true, Valid: true},
		ClickCount:                sql.NullInt64{Int64: 42, Valid: true},
		ExpiresAt:                 sql.NullTime{Time: now.Add(time.Hour), Valid: true},
		UrlStatus:                 sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
		LastAccessedAt:            sql.NullTime{Time: now.Add(-time.Minute), Valid: true},
		DestinationHealthStatus:   sql.NullInt16{Int16: int16(enum.DestinationStatusHealthy), Valid: true},
		DestinationLastHttpStatus: sql.NullInt32{Int32: 200, Valid: true},
		LastHealthCheck:           sql.NullTime{Time: now.Add(-time.Hour), Valid: true},
		CreatedAt:                 sql.NullTime{Time: now, Valid: true},
		UpdatedAt:                 sql.NullTime{Time: now, Valid: true},
	}

	resp := svc.toResponse(u, "https://example.com")

	if resp.ID != 1 {
		t.Errorf("ID = %d", resp.ID)
	}
	if resp.ShortCode != "abc" {
		t.Errorf("shortCode = %q", resp.ShortCode)
	}
	if resp.OriginalURL != "https://example.com" {
		t.Errorf("originalURL = %q", resp.OriginalURL)
	}
	if resp.Title != "Title" {
		t.Errorf("title = %q", resp.Title)
	}
	if resp.Description != "Desc" {
		t.Errorf("description = %q", resp.Description)
	}
	if resp.ClickCount != 42 {
		t.Errorf("clickCount = %d", resp.ClickCount)
	}
	if resp.IsActive != true {
		t.Error("isActive should be true")
	}
	if resp.IsCustom == nil || *resp.IsCustom != true {
		t.Error("isCustom should be true")
	}
	if !resp.HasBeenAccessed {
		t.Error("hasBeenAccessed should be true")
	}
	if !resp.HealthChecked {
		t.Error("healthChecked should be true")
	}
	if resp.DestinationStatusString != "Healthy" {
		t.Errorf("healthStatus = %q", resp.DestinationStatusString)
	}
	if resp.DestinationHttpCode != "200" {
		t.Errorf("httpCode = %q, want '200'", resp.DestinationHttpCode)
	}
	if resp.ExpiresAt == "" {
		t.Error("expiresAt should not be empty")
	}
	if resp.ShortURL != "http://localhost:8080/abc" {
		t.Errorf("shortURL = %q", resp.ShortURL)
	}
}

func TestToResponseAllNullFields(t *testing.T) {
	now := time.Now()
	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret", testLog(t))

	u := gen.Url{
		ID:        1,
		UserID:    1,
		ShortCode: "abc",
		UrlStatus: sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
		CreatedAt: sql.NullTime{Time: now, Valid: true},
		UpdatedAt: sql.NullTime{Time: now, Valid: true},
	}

	resp := svc.toResponse(u, "https://example.com")

	if resp.Title != "" {
		t.Errorf("title = %q, want empty", resp.Title)
	}
	if resp.Description != "" {
		t.Errorf("description = %q, want empty", resp.Description)
	}
	if resp.ClickCount != 0 {
		t.Errorf("clickCount = %d, want 0", resp.ClickCount)
	}
	if resp.ExpiresAt != "" {
		t.Errorf("expiresAt = %q, want empty", resp.ExpiresAt)
	}
	if resp.HasBeenAccessed {
		t.Error("hasBeenAccessed should be false")
	}
	if resp.HealthChecked {
		t.Error("healthChecked should be false")
	}
	if resp.DestinationStatusString != "Unknown / Not Checked" {
		t.Errorf("healthStatus = %q", resp.DestinationStatusString)
	}
	if resp.DestinationHttpCode != "" {
		t.Errorf("httpCode = %q, want empty", resp.DestinationHttpCode)
	}
	if resp.IsCustom == nil || *resp.IsCustom {
		t.Error("isCustom should be false")
	}
}

func TestToResponseUnhealthyStatus(t *testing.T) {
	now := time.Now()
	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret", testLog(t))

	u := gen.Url{
		ID:                        1,
		UserID:                    1,
		ShortCode:                 "abc",
		DestinationHealthStatus:   sql.NullInt16{Int16: int16(enum.DestinationStatusUnhealthy), Valid: true},
		DestinationLastHttpStatus: sql.NullInt32{Int32: 503, Valid: true},
		UrlStatus:                 sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
		CreatedAt:                 sql.NullTime{Time: now, Valid: true},
		UpdatedAt:                 sql.NullTime{Time: now, Valid: true},
	}

	resp := svc.toResponse(u, "https://example.com")
	if resp.DestinationStatusString != "Unhealthy" {
		t.Errorf("healthStatus = %q, want 'Unhealthy'", resp.DestinationStatusString)
	}
	if resp.DestinationHttpCode != "503" {
		t.Errorf("httpCode = %q, want '503'", resp.DestinationHttpCode)
	}
}

func TestToResponseInactiveStatus(t *testing.T) {
	now := time.Now()
	svc := NewURLService(&mockQuerier{}, nil, "http://localhost:8080", "test-secret", testLog(t))

	u := gen.Url{
		ID:        1,
		UserID:    1,
		ShortCode: "abc",
		UrlStatus: sql.NullInt16{Int16: int16(enum.URLStatusDisabled), Valid: true},
		CreatedAt: sql.NullTime{Time: now, Valid: true},
		UpdatedAt: sql.NullTime{Time: now, Valid: true},
	}

	resp := svc.toResponse(u, "https://example.com")
	if resp.IsActive {
		t.Error("isActive should be false for Disabled status")
	}
}

func TestToResponseBaseURL(t *testing.T) {
	now := time.Now()
	svc := NewURLService(&mockQuerier{}, nil, "https://myapp.com/", "test-secret", testLog(t))

	u := gen.Url{
		ID:        1,
		UserID:    1,
		ShortCode: "abc123",
		UrlStatus: sql.NullInt16{Int16: int16(enum.URLStatusActive), Valid: true},
		CreatedAt: sql.NullTime{Time: now, Valid: true},
		UpdatedAt: sql.NullTime{Time: now, Valid: true},
	}

	resp := svc.toResponse(u, "https://example.com")
	if resp.ShortURL != "https://myapp.com/abc123" {
		t.Errorf("shortURL = %q", resp.ShortURL)
	}
}

// ═══════════════════════════════════════════════════════════════
//  IS DUPLICATE KEY
// ═══════════════════════════════════════════════════════════════

func TestIsDuplicateKey(t *testing.T) {
	if !isDuplicateKey(&pgconn.PgError{Code: pgerrcode.UniqueViolation}) {
		t.Error("expected true for unique violation")
	}
	if isDuplicateKey(fmt.Errorf("other error")) {
		t.Error("expected false for non-unique error")
	}
}

// ═══════════════════════════════════════════════════════════════
//  CLICK LOGS — analytics
// ═══════════════════════════════════════════════════════════════

func TestListClickLogsSuccess(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:        1,
				UserID:    1,
				ShortCode: "abc",
			}, nil
		},
		countClicksFn: func(_ context.Context, _ gen.CountClickLogsByURLParams) (int64, error) {
			return 2, nil
		},
		listClicksFn: func(_ context.Context, _ gen.ListClickLogsByURLParams) ([]gen.ListClickLogsByURLRow, error) {
			return []gen.ListClickLogsByURLRow{
				{ID: 1, ClickedAt: sql.NullTime{Time: now, Valid: true}, IpAddress: inet(net.ParseIP("1.2.3.4"))},
				{ID: 2, ClickedAt: sql.NullTime{Time: now.Add(-time.Hour), Valid: true}, IpAddress: inet(net.ParseIP("5.6.7.8"))},
			}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	items, total, err := svc.ListClickLogs(context.Background(), 1, 1, nil, nil, 1, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(items) != 2 {
		t.Errorf("items = %d, want 2", len(items))
	}
	first, ok := items[0].(payload.ClickLogEntry)
	if !ok {
		t.Fatalf("expected payload.ClickLogEntry, got %T", items[0])
	}
	if first.IPAddress != "1.2.3.4" {
		t.Errorf("ip = %q, want 1.2.3.4", first.IPAddress)
	}
}

func TestListClickLogsOwnershipDenied(t *testing.T) {
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{}, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, _, err := svc.ListClickLogs(context.Background(), 1, 999, nil, nil, 1, 10, 0)
	if err == nil {
		t.Fatal("expected error for wrong ownership")
	}
}

func TestGetAnalyticsSuccess(t *testing.T) {
	now := time.Now()
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:        1,
				UserID:    1,
				ShortCode: "abc",
			}, nil
		},
		clickStatsFn: func(_ context.Context, _ gen.ClickStatsByURLParams) (gen.ClickStatsByURLRow, error) {
			return gen.ClickStatsByURLRow{
				TotalClicks:    100,
				UniqueVisitors: 50,
				FirstClickedAt: now.Add(-24 * time.Hour),
				LastClickedAt:  now,
			}, nil
		},
		topReferrersFn: func(_ context.Context, _ gen.TopReferrersByURLParams) ([]gen.TopReferrersByURLRow, error) {
			return []gen.TopReferrersByURLRow{
				{Referrer: "https://google.com", Count: 30},
				{Referrer: "https://twitter.com", Count: 20},
			}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.GetAnalytics(context.Background(), 1, 1, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Stats.TotalClicks != 100 {
		t.Errorf("totalClicks = %d, want 100", resp.Stats.TotalClicks)
	}
	if resp.Stats.UniqueVisitors != 50 {
		t.Errorf("uniqueVisitors = %d, want 50", resp.Stats.UniqueVisitors)
	}
	if len(resp.Referrers) != 2 {
		t.Errorf("referrers = %d, want 2", len(resp.Referrers))
	}
}

func TestGetAnalyticsOwnershipDenied(t *testing.T) {
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{}, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.GetAnalytics(context.Background(), 1, 999, nil, nil)
	if err == nil {
		t.Fatal("expected error for wrong ownership")
	}
}

func TestGetAnalyticsWithDailyStats(t *testing.T) {
	now := time.Now()
	from := now.AddDate(0, 0, -7)
	to := now
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:        1,
				UserID:    1,
				ShortCode: "abc",
			}, nil
		},
		clickStatsFn: func(_ context.Context, _ gen.ClickStatsByURLParams) (gen.ClickStatsByURLRow, error) {
			return gen.ClickStatsByURLRow{
				TotalClicks:    50,
				UniqueVisitors: 25,
			}, nil
		},
		topReferrersFn: func(_ context.Context, _ gen.TopReferrersByURLParams) ([]gen.TopReferrersByURLRow, error) {
			return []gen.TopReferrersByURLRow{}, nil
		},
		clicksByDateRangeFn: func(_ context.Context, _ gen.ClicksByDateRangeParams) ([]gen.ClicksByDateRangeRow, error) {
			return []gen.ClicksByDateRangeRow{
				{Date: now.AddDate(0, 0, -2).Truncate(24 * time.Hour), Clicks: 10},
				{Date: now.Truncate(24 * time.Hour), Clicks: 15},
			}, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.GetAnalytics(context.Background(), 1, 1, &from, &to)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.DailyStats) != 2 {
		t.Errorf("dailyStats = %d, want 2", len(resp.DailyStats))
	}
}

func TestInetHelper(t *testing.T) {
	ip := net.ParseIP("192.168.1.1")
	result := inet(ip)
	if !result.Valid {
		t.Error("expected valid inet")
	}
	if result.IPNet.IP.String() != "192.168.1.1" {
		t.Errorf("ip = %q, want 192.168.1.1", result.IPNet.IP.String())
	}
}
