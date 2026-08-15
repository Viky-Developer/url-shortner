package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/vicky/url-shortner/internal/config"
	"github.com/vicky/url-shortner/internal/db"
	gen "github.com/vicky/url-shortner/internal/db/gen"
)

func setup(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("RUN_DB_TESTS") == "" {
		t.Skip("set RUN_DB_TESTS=1 to run database integration tests")
	}
	database, err := config.Load().Connect()
	if err != nil {
		t.Fatalf("connect to database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestMigrationsApplyAndCreateTables(t *testing.T) {
	database := setup(t)

	if err := db.Migrate(database, "migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	expected := []string{"click_logs", "goose_db_version", "sessions", "urls", "users"}
	for _, table := range expected {
		var exists bool
		err := database.QueryRow(
			`SELECT EXISTS (
				SELECT 1 FROM pg_tables WHERE schemaname = 'public' AND tablename = $1
			)`,
			table,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if !exists {
			t.Errorf("expected table %s to exist after migrations", table)
		}
	}
}

func TestCreateAndGetURL(t *testing.T) {
	database := setup(t)

	if err := db.Migrate(database, "migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	q := gen.New(database)
	ctx := context.Background()
	code := fmt.Sprintf("c%d", time.Now().UnixNano()%100000000)

	created, err := q.CreateURL(ctx, gen.CreateURLParams{
		UserID:      1,
		ShortCode:   code,
		OriginalUrl: "https://example.com/ci-test",
		IsCustom:    sql.NullBool{Bool: false, Valid: true},
		ExpiresAt:   sql.NullTime{Valid: false},
	})
	if err != nil {
		t.Fatalf("create url: %v", err)
	}

	got, err := q.GetURLByShortCode(ctx, gen.GetURLByShortCodeParams{UserID: 1, ShortCode: code})
	if err != nil {
		t.Fatalf("get url by short code: %v", err)
	}

	if got.ID != created.ID {
		t.Errorf("expected id %d, got %d", created.ID, got.ID)
	}
	if got.OriginalUrl != created.OriginalUrl {
		t.Errorf("expected original_url %q, got %q", created.OriginalUrl, got.OriginalUrl)
	}
}

func TestListUpdateSoftDeleteHardDeleteURL(t *testing.T) {
	database := setup(t)

	if err := db.Migrate(database, "migrations"); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	q := gen.New(database)
	ctx := context.Background()
	code := fmt.Sprintf("u%d", time.Now().UnixNano()%100000000)

	created, err := q.CreateURL(ctx, gen.CreateURLParams{
		UserID:      1,
		ShortCode:   code,
		OriginalUrl: "https://example.com/original",
		IsCustom:    sql.NullBool{Bool: false, Valid: true},
		ExpiresAt:   sql.NullTime{Valid: false},
	})
	if err != nil {
		t.Fatalf("create url: %v", err)
	}

	t.Run("list", func(t *testing.T) {
		items, err := q.ListURLs(ctx, gen.ListURLsParams{UserID: 1, Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list urls: %v", err)
		}
		if len(items) == 0 {
			t.Fatal("expected at least one url")
		}
	})

	t.Run("update", func(t *testing.T) {
		updated, err := q.UpdateURL(ctx, gen.UpdateURLParams{
			ID:          created.ID,
			UserID:      1,
			OriginalUrl: "https://example.com/updated",
			ExpiresAt:   sql.NullTime{Valid: false},
		})
		if err != nil {
			t.Fatalf("update url: %v", err)
		}
		if updated.OriginalUrl != "https://example.com/updated" {
			t.Errorf("expected updated original_url, got %q", updated.OriginalUrl)
		}
	})

	t.Run("soft delete", func(t *testing.T) {
		deleted, err := q.SoftDeleteURL(ctx, gen.SoftDeleteURLParams{ID: created.ID, UserID: 1})
		if err != nil {
			t.Fatalf("soft delete url: %v", err)
		}
		if !deleted.DeletedAt.Valid {
			t.Error("expected deleted_at to be set")
		}

		if _, err := q.GetURLByID(ctx, gen.GetURLByIDParams{ID: created.ID, UserID: 1}); err != sql.ErrNoRows {
			t.Errorf("expected ErrNoRows after soft delete, got %v", err)
		}
	})

	t.Run("hard delete", func(t *testing.T) {
		if err := q.HardDeleteURL(ctx, gen.HardDeleteURLParams{ID: created.ID, UserID: 1}); err != nil {
			t.Fatalf("hard delete url: %v", err)
		}

		var count int64
		err := database.QueryRow(
			`SELECT COUNT(*) FROM urls WHERE id = $1 AND deleted_at IS NOT NULL`,
			created.ID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("count after hard delete: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 rows after hard delete, got %d", count)
		}
	})
}
