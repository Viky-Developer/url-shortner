package config

import (
	"strings"
	"testing"
	"time"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")
	t.Setenv("DB_USER", "urlshortner")
	t.Setenv("DB_PASSWORD", "urlshortner123")
	t.Setenv("DB_NAME", "urlshortner")
	t.Setenv("DB_SSLMODE", "disable")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("LOG_COLOR", "true")
	t.Setenv("DB_MAX_OPEN_CONNS", "25")
	t.Setenv("DB_MAX_IDLE_CONNS", "25")
	t.Setenv("DB_MAX_LIFETIME", "5")
	t.Setenv("SERVER_HOST", "0.0.0.0")
	t.Setenv("SERVER_PORT", "8085")
	t.Setenv("SERVER_BASE_URL", "http://localhost:8085/api/v1")
	t.Setenv("DEFAULT_USER_EMAIL", "default@urlshortner.local")
	t.Setenv("DEFAULT_USER_PASSWORD", "default123")
	t.Setenv("USER_ID_SECRET_KEY", "secret-key-12345")
	t.Setenv("JWT_SECRET_KEY", "jwt-secret-key-67890")
	t.Setenv("ACCESS_TOKEN_EXPIRY", "15")
	t.Setenv("REFRESH_TOKEN_EXPIRY", "7")
	t.Setenv("REDIS_HOST", "localhost")
	t.Setenv("REDIS_PORT", "6379")
	t.Setenv("REDIS_DB", "0")
	t.Setenv("REDIS_MAX_RETRIES", "3")
	t.Setenv("SESSION_RETENTION", "2160h")
	t.Setenv("PASSWORD_RETENTION", "8760h")
	t.Setenv("PASSWORD_REUSE_LIMIT", "5")
	t.Setenv("RETENTION_RUN_INTERVAL", "24h")
	t.Setenv("ENABLE_RETENTION_WORKER", "true")
	t.Setenv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")
	t.Setenv("RABBITMQ_EXCHANGE_CLICKS", "url.clicks.direct")
	t.Setenv("RABBITMQ_ROUTING_KEY_CLICKS", "url.clicks.route")
	t.Setenv("RABBITMQ_QUEUE_CLICKS", "url.clicks")
	t.Setenv("ENABLE_RABBITMQ", "true")
	t.Setenv("GOOGLE_CLIENT_ID", "test-client.apps.googleusercontent.com")
	t.Setenv("GOOGLE_CLIENT_SECRET", "test-client-secret")
	t.Setenv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/v1/auth/google/callback")
	t.Setenv("GOOGLE_AUTH_URL", "https://provider.test/auth")
	t.Setenv("GOOGLE_TOKEN_URL", "https://provider.test/token")
	t.Setenv("GOOGLE_USER_INFO_URL", "https://provider.test/userinfo")
	t.Setenv("FRONTEND_URL", "http://localhost:5173")
}

func TestLoad(t *testing.T) {
	t.Run("succeeds when all env variables are set", func(t *testing.T) {
		setValidEnv(t)

		cfg, err := Load()
		if err != nil {
			t.Fatalf("expected Load() to succeed, got error: %v", err)
		}
		if cfg.DBHost != "localhost" {
			t.Fatalf("expected DBHost localhost, got %s", cfg.DBHost)
		}
		if cfg.ServerPort != "8085" {
			t.Fatalf("expected ServerPort 8085, got %s", cfg.ServerPort)
		}
		if cfg.DBMaxOpen != 25 {
			t.Fatalf("expected DBMaxOpen 25, got %d", cfg.DBMaxOpen)
		}
		if !cfg.LogColor {
			t.Fatal("expected LogColor true")
		}
		if cfg.AccessTokenExpiry != 15*time.Minute {
			t.Fatalf("expected AccessTokenExpiry 15m, got %v", cfg.AccessTokenExpiry)
		}
		if cfg.RefreshTokenExpiry != 7*24*time.Hour {
			t.Fatalf("expected RefreshTokenExpiry 7d, got %v", cfg.RefreshTokenExpiry)
		}
		if cfg.RabbitMQURL != "amqp://guest:guest@localhost:5672/" {
			t.Fatalf("expected RabbitMQURL amqp://guest:guest@localhost:5672/, got %s", cfg.RabbitMQURL)
		}
		if cfg.RedisTLS {
			t.Fatal("expected RedisTLS false by default")
		}
		if cfg.GoogleRedirectURL != "http://localhost:8080/api/v1/auth/google/callback" {
			t.Fatalf("unexpected GoogleRedirectURL: %s", cfg.GoogleRedirectURL)
		}
		if cfg.GoogleAuthURL == "" || cfg.GoogleTokenURL == "" || cfg.GoogleUserInfoURL == "" {
			t.Fatal("expected all Google OAuth endpoint URLs to be loaded")
		}
	})

	t.Run("returns error when required env variable is missing", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("JWT_SECRET_KEY", "")

		cfg, err := Load()
		if err == nil {
			t.Fatal("expected Load() to fail when JWT_SECRET_KEY is empty, but got nil error")
		}
		if cfg != nil {
			t.Fatal("expected cfg to be nil on error")
		}
		if !strings.Contains(err.Error(), "JWT_SECRET_KEY is required but not set") {
			t.Fatalf("expected error message to mention JWT_SECRET_KEY, got: %v", err)
		}
	})

	t.Run("returns error when a Google endpoint URL is missing", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("GOOGLE_TOKEN_URL", "")
		_, err := Load()
		if err == nil || !strings.Contains(err.Error(), "GOOGLE_TOKEN_URL is required but not set") {
			t.Fatalf("expected missing Google token URL error, got %v", err)
		}
	})

	t.Run("returns error for invalid integer", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("DB_MAX_OPEN_CONNS", "notanumber")

		_, err := Load()
		if err == nil {
			t.Fatal("expected Load() to fail for invalid integer")
		}
		if !strings.Contains(err.Error(), "DB_MAX_OPEN_CONNS must be a valid integer") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("returns error for invalid boolean", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("ENABLE_RABBITMQ", "notabool")

		_, err := Load()
		if err == nil {
			t.Fatal("expected Load() to fail for invalid boolean")
		}
		if !strings.Contains(err.Error(), "ENABLE_RABBITMQ must be a boolean") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})

	t.Run("returns error for invalid duration", func(t *testing.T) {
		setValidEnv(t)
		t.Setenv("SESSION_RETENTION", "invalid-duration")

		_, err := Load()
		if err == nil {
			t.Fatal("expected Load() to fail for invalid duration")
		}
		if !strings.Contains(err.Error(), "SESSION_RETENTION must be a valid duration") {
			t.Fatalf("unexpected error message: %v", err)
		}
	})
}

func TestDSN(t *testing.T) {
	t.Run("constructs standard DSN", func(t *testing.T) {
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
	})

	t.Run("DATABASE_URL overrides standard DSN", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://custom-user:custom-pass@cloud-host:5432/cloud-db")
		cfg := &Config{
			DBHost: "myhost",
		}
		if cfg.DSN() != "postgres://custom-user:custom-pass@cloud-host:5432/cloud-db" {
			t.Fatalf("expected DATABASE_URL to override DSN, got %q", cfg.DSN())
		}
	})
}
