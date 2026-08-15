package service

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/payload"
)

type mockQuerier struct {
	createFn     func(context.Context, gen.CreateURLParams) (gen.Url, error)
	createUserFn func(context.Context, gen.CreateUserParams) (gen.CreateUserRow, error)
	byCodeFn     func(context.Context, gen.GetURLByShortCodeParams) (gen.Url, error)
	byIDFn       func(context.Context, gen.GetURLByIDParams) (gen.Url, error)
	listFn       func(context.Context, gen.ListURLsParams) ([]gen.Url, error)
	countFn      func(context.Context, int64) (int64, error)
	emailFn      func(context.Context, string) (gen.GetUserByEmailRow, error)
	updateUserFn func(context.Context, gen.UpdateUserDisplayIDParams) (gen.UpdateUserDisplayIDRow, error)
	updateFn     func(context.Context, gen.UpdateURLParams) (gen.Url, error)
	softFn       func(context.Context, gen.SoftDeleteURLParams) (gen.Url, error)
	hardFn       func(context.Context, gen.HardDeleteURLParams) error
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

func (m *mockQuerier) GetURLByShortCode(ctx context.Context, arg gen.GetURLByShortCodeParams) (gen.Url, error) {
	return m.byCodeFn(ctx, arg)
}

func (m *mockQuerier) GetURLByID(ctx context.Context, arg gen.GetURLByIDParams) (gen.Url, error) {
	return m.byIDFn(ctx, arg)
}

func (m *mockQuerier) ListURLs(ctx context.Context, arg gen.ListURLsParams) ([]gen.Url, error) {
	return m.listFn(ctx, arg)
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

func TestCreateGeneratesShortCode(t *testing.T) {
	var captured gen.CreateURLParams
	mock := &mockQuerier{
		createFn: func(_ context.Context, arg gen.CreateURLParams) (gen.Url, error) {
			captured = arg
			return testURL(arg.ShortCode), nil
		},
	}
	svc := NewURLService(mock, "http://localhost:8080", "test-secret-key", testLog(t))

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
		createFn: func(_ context.Context, arg gen.CreateURLParams) (gen.Url, error) {
			captured = arg
			return testURL(arg.ShortCode), nil
		},
	}
	svc := NewURLService(mock, "http://localhost:8080", "test-secret-key", testLog(t))

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

func TestGetByShortCodeNotFound(t *testing.T) {
	mock := &mockQuerier{
		byCodeFn: func(_ context.Context, _ gen.GetURLByShortCodeParams) (gen.Url, error) {
			return gen.Url{}, sql.ErrNoRows
		},
	}
	svc := NewURLService(mock, "http://localhost:8080", "test-secret-key", testLog(t))

	_, err := svc.GetByShortCode(context.Background(), 1, "missing")
	if err == nil {
		t.Fatal("expected not found error")
	}
}

func TestListPagination(t *testing.T) {
	mock := &mockQuerier{
		listFn: func(_ context.Context, _ gen.ListURLsParams) ([]gen.Url, error) {
			return []gen.Url{testURL("abc123"), testURL("def456")}, nil
		},
		countFn: func(_ context.Context, _ int64) (int64, error) {
			return 25, nil
		},
	}
	svc := NewURLService(mock, "http://localhost:8080", "test-secret-key", testLog(t))

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
	svc := NewURLService(mock, "http://localhost:8080", "test-secret-key", testLog(t))

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
