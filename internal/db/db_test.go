package db_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/vicky/url-shortner/internal/config"
	gen "github.com/vicky/url-shortner/internal/db/gen"
)

func hashURL(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

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

	expected := []string{
		"click_logs", "daily_url_stats", "url_versions", "urls",
		"destinations", "blocked_domains", "blocked_ip_ranges",
		"sessions", "users", "goose_db_version",
	}
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

func ensureUser(t *testing.T, q *gen.Queries) int64 {
	t.Helper()
	existing, err := q.GetUserByEmail(context.Background(), "default@urlshortner.local")
	if err == nil {
		return existing.ID
	}
	created, err := q.CreateUser(context.Background(), gen.CreateUserParams{
		Email:         "default@urlshortner.local",
		PasswordHash:  "test",
		DisplayUserID: sql.NullString{String: "USR_default", Valid: true},
	})
	if err != nil {
		t.Fatalf("create default user: %v", err)
	}
	return created.ID
}

func TestCreateAndGetURL(t *testing.T) {
	database := setup(t)

	q := gen.New(database)
	ctx := context.Background()
	userID := ensureUser(t, q)
	code := fmt.Sprintf("c%d", time.Now().UnixNano()%100000000)

	dest, err := q.CreateDestination(ctx, gen.CreateDestinationParams{
		OriginalUrl: "https://example.com/ci-test-" + code,
		UrlHash:     hashURL("https://example.com/ci-test-" + code),
	})
	if err != nil {
		t.Fatalf("create destination: %v", err)
	}

	created, err := q.CreateURL(ctx, gen.CreateURLParams{
		UserID:        userID,
		ShortCode:     code,
		DestinationID: dest.ID,
		IsCustom:      sql.NullBool{Bool: false, Valid: true},
		ExpiresAt:     sql.NullTime{Valid: false},
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
	expectedURL := "https://example.com/ci-test-" + code
	if got.OriginalUrl != expectedURL {
		t.Errorf("expected original_url %q, got %q", expectedURL, got.OriginalUrl)
	}
}

func TestListUpdateSoftDeleteHardDeleteURL(t *testing.T) {
	database := setup(t)

	q := gen.New(database)
	ctx := context.Background()
	userID := ensureUser(t, q)
	code := fmt.Sprintf("u%d", time.Now().UnixNano()%100000000)

	dest, err := q.CreateDestination(ctx, gen.CreateDestinationParams{
		OriginalUrl: "https://example.com/original-" + code,
		UrlHash:     hashURL("https://example.com/original-" + code),
	})
	if err != nil {
		t.Fatalf("create destination: %v", err)
	}

	created, err := q.CreateURL(ctx, gen.CreateURLParams{
		UserID:        userID,
		ShortCode:     code,
		DestinationID: dest.ID,
		IsCustom:      sql.NullBool{Bool: false, Valid: true},
		ExpiresAt:     sql.NullTime{Valid: false},
	})
	if err != nil {
		t.Fatalf("create url: %v", err)
	}

	t.Run("list", func(t *testing.T) {
		items, err := q.ListURLs(ctx, gen.ListURLsParams{UserID: userID, Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list urls: %v", err)
		}
		if len(items) == 0 {
			t.Fatal("expected at least one url")
		}
	})

	t.Run("update", func(t *testing.T) {
		updDest, err := q.CreateDestination(ctx, gen.CreateDestinationParams{
			OriginalUrl: "https://example.com/updated-" + code,
			UrlHash:     hashURL("https://example.com/updated-" + code),
		})
		if err != nil {
			t.Fatalf("create destination: %v", err)
		}

		updated, err := q.UpdateURL(ctx, gen.UpdateURLParams{
			ID:            created.ID,
			UserID:        userID,
			DestinationID: updDest.ID,
			ExpiresAt:     sql.NullTime{Valid: false},
		})
		if err != nil {
			t.Fatalf("update url: %v", err)
		}
		if updated.DestinationID != updDest.ID {
			t.Errorf("expected destination_id %d, got %d", updDest.ID, updated.DestinationID)
		}

		got, err := q.GetURLByShortCode(ctx, code)
		if err != nil {
			t.Fatalf("get url by short code: %v", err)
		}
		expectedUpdatedURL := "https://example.com/updated-" + code
		if got.OriginalUrl != expectedUpdatedURL {
			t.Errorf("expected updated original_url %q, got %q", expectedUpdatedURL, got.OriginalUrl)
		}
	})

	t.Run("soft delete", func(t *testing.T) {
		deleted, err := q.SoftDeleteURL(ctx, gen.SoftDeleteURLParams{ID: created.ID, UserID: userID})
		if err != nil {
			t.Fatalf("soft delete url: %v", err)
		}
		if !deleted.DeletedAt.Valid {
			t.Error("expected deleted_at to be set")
		}

		if _, err := q.GetURLByID(ctx, gen.GetURLByIDParams{ID: created.ID, UserID: userID}); err != sql.ErrNoRows {
			t.Errorf("expected ErrNoRows after soft delete, got %v", err)
		}
	})

	t.Run("hard delete", func(t *testing.T) {
		if err := q.HardDeleteURL(ctx, gen.HardDeleteURLParams{ID: created.ID, UserID: userID}); err != nil {
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
