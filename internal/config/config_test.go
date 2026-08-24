package config

import (
	"os"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Run("returns defaults when env is empty", func(t *testing.T) {
		cfg := Load()

		if cfg.DBHost != "localhost" {
			t.Fatalf("expected DBHost localhost, got %s", cfg.DBHost)
		}
		if cfg.DBPort != "5432" {
			t.Fatalf("expected DBPort 5432, got %s", cfg.DBPort)
		}
		if cfg.ServerPort != "8085" {
			t.Fatalf("expected ServerPort 8085, got %s", cfg.ServerPort)
		}
		if cfg.DBMaxOpen != 25 {
			t.Fatalf("expected DBMaxOpen 25, got %d", cfg.DBMaxOpen)
		}
	})

	t.Run("reads from environment", func(t *testing.T) {
		t.Setenv("DB_HOST", "remotehost")
		t.Setenv("DB_PORT", "5433")
		t.Setenv("SERVER_PORT", "9090")
		t.Setenv("DB_MAX_OPEN_CONNS", "50")

		cfg := Load()

		if cfg.DBHost != "remotehost" {
			t.Fatalf("expected DBHost remotehost, got %s", cfg.DBHost)
		}
		if cfg.DBPort != "5433" {
			t.Fatalf("expected DBPort 5433, got %s", cfg.DBPort)
		}
		if cfg.ServerPort != "9090" {
			t.Fatalf("expected ServerPort 9090, got %s", cfg.ServerPort)
		}
		if cfg.DBMaxOpen != 50 {
			t.Fatalf("expected DBMaxOpen 50, got %d", cfg.DBMaxOpen)
		}
	})
}

func TestDSN(t *testing.T) {
	cfg := &Config{
		DBHost:     "myhost",
		DBPort:     "5432",
		DBUser:     "myuser",
		DBPassword: "mypass",
		DBName:     "mydb",
		SSLMode:    "require",
	}

	dsn := cfg.DSN()
	expected := "host=myhost port=5432 user=myuser password=mypass dbname=mydb sslmode=require"
	if dsn != expected {
		t.Fatalf("expected DSN %q, got %q", expected, dsn)
	}
}

func TestGetEnv(t *testing.T) {
	t.Run("returns value when set", func(t *testing.T) {
		t.Setenv("TEST_GET_ENV_KEY", "hello")
		got := getEnv("TEST_GET_ENV_KEY", "fallback")
		if got != "hello" {
			t.Fatalf("expected hello, got %s", got)
		}
	})

	t.Run("returns fallback when empty", func(t *testing.T) {
		_ = os.Unsetenv("TEST_GET_ENV_KEY_EMPTY")
		got := getEnv("TEST_GET_ENV_KEY_EMPTY", "fallback")
		if got != "fallback" {
			t.Fatalf("expected fallback, got %s", got)
		}
	})
}

func TestGetEnvInt(t *testing.T) {
	t.Run("returns parsed int", func(t *testing.T) {
		t.Setenv("TEST_GET_ENV_INT_KEY", "42")
		got := getEnvInt("TEST_GET_ENV_INT_KEY", 0)
		if got != 42 {
			t.Fatalf("expected 42, got %d", got)
		}
	})

	t.Run("returns fallback for empty", func(t *testing.T) {
		_ = os.Unsetenv("TEST_GET_ENV_INT_EMPTY")
		got := getEnvInt("TEST_GET_ENV_INT_EMPTY", 99)
		if got != 99 {
			t.Fatalf("expected 99, got %d", got)
		}
	})

	t.Run("returns fallback for invalid", func(t *testing.T) {
		t.Setenv("TEST_GET_ENV_INT_BAD", "notanumber")
		got := getEnvInt("TEST_GET_ENV_INT_BAD", 77)
		if got != 77 {
			t.Fatalf("expected 77, got %d", got)
		}
	})
}
