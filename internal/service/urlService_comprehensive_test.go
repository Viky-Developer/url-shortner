package service

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/lib/pq"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/enum"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/utils"
)

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

	resp, err := svc.List(context.Background(), 1, 1, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(resp.Items))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
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

	_, err := svc.List(context.Background(), 1, 1, 10, 0)
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

	_, err := svc.List(context.Background(), 1, 1, 10, 0)
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

	resp, err := svc.List(context.Background(), 1, 3, 10, 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.Page != 3 {
		t.Errorf("page = %d, want 3", resp.Page)
	}
	if resp.PerPage != 10 {
		t.Errorf("perPage = %d, want 10", resp.PerPage)
	}
	if resp.Total != 25 {
		t.Errorf("total = %d, want 25", resp.Total)
	}
	if resp.TotalPages != 3 {
		t.Errorf("totalPages = %d, want 3", resp.TotalPages)
	}
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
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

	resp, err := svc.List(context.Background(), 1, 1, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TotalPages != 2 {
		t.Errorf("totalPages = %d, want 2 (20/10 exact)", resp.TotalPages)
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

	resp, err := svc.List(context.Background(), 1, 1, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Fatal("expected 1 item")
	}
	item := resp.Items[0]
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

	resp, err := svc.List(context.Background(), 1, 1, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Items[0].DestinationStatusString != "Healthy" {
		t.Errorf("healthStatus[0] = %q", resp.Items[0].DestinationStatusString)
	}
	if resp.Items[1].DestinationStatusString != "Unhealthy" {
		t.Errorf("healthStatus[1] = %q", resp.Items[1].DestinationStatusString)
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

// ═══════════════════════════════════════════════════════════════
//  HARD DELETE — service layer
// ═══════════════════════════════════════════════════════════════

func TestHardDeleteSuccess(t *testing.T) {
	var capturedParams gen.HardDeleteURLParams
	mock := &mockQuerier{
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
	if !isDuplicateKey(&pq.Error{Code: "23505"}) {
		t.Error("expected true for unique violation")
	}
	if isDuplicateKey(fmt.Errorf("other error")) {
		t.Error("expected false for non-unique error")
	}
}
