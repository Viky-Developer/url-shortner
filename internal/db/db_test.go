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
		ShortCode:   code,
		OriginalUrl: "https://example.com/ci-test",
		IsCustom:    sql.NullBool{Bool: false, Valid: true},
		ExpiresAt:   sql.NullTime{Valid: false},
	})
	if err != nil {
		t.Fatalf("create url: %v", err)
	}

	got, err := q.GetURLByShortCode(ctx, code)
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
