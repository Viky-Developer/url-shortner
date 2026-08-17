package service

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/payload"
)

type mockQuerier struct {
	createFn          func(context.Context, gen.CreateURLParams) (gen.Url, error)
	createUserFn      func(context.Context, gen.CreateUserParams) (gen.CreateUserRow, error)
	byCodeFn          func(context.Context, string) (gen.GetURLByShortCodeRow, error)
	byCodeForUpdateFn func(context.Context, string) (gen.GetURLByShortCodeForUpdateRow, error)
	byIDFn            func(context.Context, gen.GetURLByIDParams) (gen.GetURLByIDRow, error)
	listFn            func(context.Context, gen.ListURLsParams) ([]gen.ListURLsRow, error)
	countFn           func(context.Context, int64) (int64, error)
	emailFn           func(context.Context, string) (gen.GetUserByEmailRow, error)
	updateUserFn      func(context.Context, gen.UpdateUserDisplayIDParams) (gen.UpdateUserDisplayIDRow, error)
	updateFn          func(context.Context, gen.UpdateURLParams) (gen.Url, error)
	softFn            func(context.Context, gen.SoftDeleteURLParams) (gen.Url, error)
	hardFn            func(context.Context, gen.HardDeleteURLParams) error
	createDestFn      func(context.Context, gen.CreateDestinationParams) (gen.CreateDestinationRow, error)
	destByHashFn      func(context.Context, string) (gen.GetDestinationByHashRow, error)
	destByIDFn        func(context.Context, int64) (gen.GetDestinationByIDRow, error)
	blockedFn         func(context.Context, string) (gen.GetBlockedDomainRow, error)
	createVerFn       func(context.Context, gen.CreateURLVersionParams) error
	latestVerFn       func(context.Context, int64) (int32, error)
	createClickFn     func(context.Context, gen.CreateClickLogParams) (gen.ClickLog, error)
	incrementClickFn  func(context.Context, int64) error
	updateHealthFn    func(context.Context, gen.UpdateURLHealthStatusParams) (gen.Url, error)
	shortCodeExistsFn func(context.Context, string) (bool, error)
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
		IsActive:  sql.NullBool{Bool: true, Valid: true},
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
		IsActive:    sql.NullBool{Bool: true, Valid: true},
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
				IsActive:    sql.NullBool{Bool: true, Valid: true},
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
				IsActive:  sql.NullBool{Bool: true, Valid: true},
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

	resp, err := svc.List(context.Background(), 1, 3, 10, 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if resp.Total != 25 {
		t.Errorf("expected total 25, got %d", resp.Total)
	}
	if resp.Page != 3 {
		t.Errorf("expected page 3, got %d", resp.Page)
	}
	if resp.TotalPages != 3 {
		t.Errorf("expected totalPages 3, got %d", resp.TotalPages)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
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

func TestUpdatePersistsIsActive(t *testing.T) {
	now := time.Now()
	isActiveFalse := false

	var captured gen.UpdateURLParams
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:            1,
				UserID:        1,
				ShortCode:     "abc1234567",
				DestinationID: 1,
				IsActive:      sql.NullBool{Bool: true, Valid: true},
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
		IsActive: &isActiveFalse,
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if !captured.IsActive.Valid {
		t.Error("expected isActive to be provided to the query")
	}
	if captured.IsActive.Bool {
		t.Error("expected isActive=false to be passed through")
	}
	if resp.ID != 1 {
		t.Errorf("expected response id 1, got %d", resp.ID)
	}
}

func TestUpdateLeavesIsActiveNilWhenOmitted(t *testing.T) {
	var captured gen.UpdateURLParams
	mock := &mockQuerier{
		byIDFn: func(_ context.Context, _ gen.GetURLByIDParams) (gen.GetURLByIDRow, error) {
			return gen.GetURLByIDRow{
				ID:            1,
				UserID:        1,
				ShortCode:     "abc1234567",
				DestinationID: 1,
				IsActive:      sql.NullBool{Bool: true, Valid: true},
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
			// Simulate DB returning the preserved is_active = true via COALESCE
			url.IsActive = sql.NullBool{Bool: true, Valid: true}
			return url, nil
		},
	}
	svc := NewURLService(mock, nil, "http://localhost:8080", "test-secret-key", testLog(t))

	resp, err := svc.Update(context.Background(), 1, 1, payload.UpdateURLRequest{})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	if captured.IsActive.Valid {
		t.Error("expected isActive param to be NULL so COALESCE preserves the existing value")
	}
	if !resp.IsActive {
		t.Error("expected isActive to remain true after update without explicit isActive in the request")
	}
}
