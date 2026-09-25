// Package config loads application configuration from environment variables
// and provides database connectivity helpers.
package config

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
)

// Config holds all runtime configuration values for the application.
type Config struct {
	DBHost                   string        // Postgres host.
	DBPort                   string        // Postgres port.
	DBUser                   string        // Postgres user.
	DBPassword               string        // Postgres password.
	DBName                   string        // Postgres database name.
	SSLMode                  string        // Postgres sslmode.
	LogLevel                 string        // Minimum log level (debug, info, warn, error).
	LogColor                 bool          // Whether console logs should use ANSI colors.
	DBMaxOpen                int           // Maximum number of open connections.
	DBMaxIdle                int           // Maximum number of idle connections.
	DBMaxLife                time.Duration // Maximum connection lifetime.
	ServerHost               string        // HTTP server bind host.
	ServerPort               string        // HTTP server bind port.
	ServerBaseURL            string        // Public base URL used to build short URLs.
	DefaultUserEmail         string        // Email of the default user created at startup.
	DefaultUserPassword      string        // Password of the default user created at startup.
	UserIDSecretKey          string        // Secret key used to encode/decode display user ids.
	JWTSecretKey             string        // Secret key for signing JWT tokens.
	AccessTokenExpiry        time.Duration // Access token expiry duration.
	RefreshTokenExpiry       time.Duration // Refresh token expiry duration.
	RedisHost                string        // Redis host.
	RedisPort                string        // Redis port.
	RedisUserName            string        // Redis username.
	RedisPassword            string        // Redis password.
	RedisDB                  int           // Redis database number.
	RedisMaxRetries          int           // Redis max retries.
	SessionRetention         time.Duration // Retention period for revoked/expired sessions before hard delete.
	PasswordRetention        time.Duration // Retention period for old password hashes before hard delete.
	PasswordReuseLimit       int           // Number of recent password hashes to keep (reuse check window).
	RetentionRunInterval     time.Duration // Interval between scheduled retention cleanup runs.
	EnableRetentionWorker    bool          // Whether the background retention worker runs.
	RabbitMQURL              string        // Full amqp(s):// connection URL (e.g. from CloudAMQP).
	RabbitMQExchangeClicks   string        // RabbitMQ exchange name for click events.
	RabbitMQRoutingKeyClicks string        // RabbitMQ routing key for click events.
	RabbitMQQueueClicks      string        // RabbitMQ queue name for click events.
	EnableRabbitMQ           bool          // Whether RabbitMQ async queuing is enabled.
	RedisTLS                 bool          // Whether to use TLS for Redis (required by Upstash).
	GoogleClientID           string        // Google OAuth web client ID.
	GoogleClientSecret       string        // Google OAuth web client secret.
	GoogleRedirectURL        string        // Google OAuth callback URL.
	GoogleAuthURL            string        // Google OAuth authorization endpoint.
	GoogleTokenURL           string        // Google OAuth token endpoint.
	GoogleUserInfoURL        string        // Google OpenID Connect user-info endpoint.
	FrontendURL              string        // Frontend URL used after browser authentication.
}

// Load reads configuration from the .env file (if present) and the process
// environment. It strictly enforces that all required environment variables
// are present and valid, returning an error if any variables are missing or unparsable.
func Load() (*Config, error) {
	_ = godotenv.Load()
	l := &envLoader{}

	// Server Port: on cloud hosting platforms (like Render), PORT is dynamically assigned;
	// otherwise SERVER_PORT is used.
	serverPort := os.Getenv("PORT")
	if serverPort == "" {
		serverPort = l.require("SERVER_PORT")
	}

	cfg := &Config{
		DBHost:                   l.require("DB_HOST"),
		DBPort:                   l.require("DB_PORT"),
		DBUser:                   l.require("DB_USER"),
		DBPassword:               l.require("DB_PASSWORD"),
		DBName:                   l.require("DB_NAME"),
		SSLMode:                  l.require("DB_SSLMODE"),
		LogLevel:                 l.require("LOG_LEVEL"),
		LogColor:                 l.requireBool("LOG_COLOR"),
		DBMaxOpen:                l.requireInt("DB_MAX_OPEN_CONNS"),
		DBMaxIdle:                l.requireInt("DB_MAX_IDLE_CONNS"),
		DBMaxLife:                l.requireDurationMinutes("DB_MAX_LIFETIME"),
		ServerHost:               l.require("SERVER_HOST"),
		ServerPort:               serverPort,
		ServerBaseURL:            l.require("SERVER_BASE_URL"),
		DefaultUserEmail:         l.require("DEFAULT_USER_EMAIL"),
		DefaultUserPassword:      l.require("DEFAULT_USER_PASSWORD"),
		UserIDSecretKey:          l.require("USER_ID_SECRET_KEY"),
		JWTSecretKey:             l.require("JWT_SECRET_KEY"),
		AccessTokenExpiry:        l.requireDurationMinutes("ACCESS_TOKEN_EXPIRY"),
		RefreshTokenExpiry:       l.requireDurationDays("REFRESH_TOKEN_EXPIRY"),
		RedisHost:                l.require("REDIS_HOST"),
		RedisPort:                l.require("REDIS_PORT"),
		RedisUserName:            l.optional("REDIS_USERNAME"),
		RedisPassword:            l.optional("REDIS_PASSWORD"),
		RedisDB:                  l.requireInt("REDIS_DB"),
		RedisMaxRetries:          l.requireInt("REDIS_MAX_RETRIES"),
		SessionRetention:         l.requireDuration("SESSION_RETENTION"),
		PasswordRetention:        l.requireDuration("PASSWORD_RETENTION"),
		PasswordReuseLimit:       l.requireInt("PASSWORD_REUSE_LIMIT"),
		RetentionRunInterval:     l.requireDuration("RETENTION_RUN_INTERVAL"),
		EnableRetentionWorker:    l.requireBool("ENABLE_RETENTION_WORKER"),
		RabbitMQURL:              l.require("RABBITMQ_URL"),
		RabbitMQExchangeClicks:   l.require("RABBITMQ_EXCHANGE_CLICKS"),
		RabbitMQRoutingKeyClicks: l.require("RABBITMQ_ROUTING_KEY_CLICKS"),
		RabbitMQQueueClicks:      l.require("RABBITMQ_QUEUE_CLICKS"),
		EnableRabbitMQ:           l.requireBool("ENABLE_RABBITMQ"),
		RedisTLS:                 l.optionalBool("REDIS_TLS"),
		GoogleClientID:           l.require("GOOGLE_CLIENT_ID"),
		GoogleClientSecret:       l.require("GOOGLE_CLIENT_SECRET"),
		GoogleRedirectURL:        l.require("GOOGLE_REDIRECT_URL"),
		GoogleAuthURL:            l.require("GOOGLE_AUTH_URL"),
		GoogleTokenURL:           l.require("GOOGLE_TOKEN_URL"),
		GoogleUserInfoURL:        l.require("GOOGLE_USER_INFO_URL"),
		FrontendURL:              l.require("FRONTEND_URL"),
	}

	if len(l.errors) > 0 {
		return nil, fmt.Errorf("configuration errors:\n  - %s", strings.Join(l.errors, "\n  - "))
	}

	return cfg, nil
}

// DSN returns the Postgres connection string built from the config values,
// or returns DATABASE_URL directly if set (standard in cloud environments like Render).
func (c *Config) DSN() string {
	if dbURL := os.Getenv("DATABASE_URL"); dbURL != "" {
		return dbURL
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.SSLMode,
	)
}

// Connect opens a Postgres connection pool configured from the config values
// and verifies it with a ping.
func (c *Config) Connect() (*sql.DB, error) {
	db, err := sql.Open("pgx", c.DSN())
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(c.DBMaxOpen)
	db.SetMaxIdleConns(c.DBMaxIdle)
	db.SetConnMaxLifetime(c.DBMaxLife)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

type envLoader struct {
	errors []string
}

func (l *envLoader) require(key string) string {
	v, ok := os.LookupEnv(key)
	if !ok || strings.TrimSpace(v) == "" {
		l.errors = append(l.errors, fmt.Sprintf("%s is required but not set", key))
		return ""
	}
	return v
}

func (l *envLoader) optional(key string) string {
	return os.Getenv(key)
}

// optionalBool returns false if key is not set; returns parsed bool if set.
func (l *envLoader) optionalBool(key string) bool {
	v := os.Getenv(key)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.errors = append(l.errors, fmt.Sprintf("%s must be a boolean (true/false), got %q: %v", key, v, err))
		return false
	}
	return b
}

func (l *envLoader) requireInt(key string) int {
	v := l.require(key)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.errors = append(l.errors, fmt.Sprintf("%s must be a valid integer, got %q: %v", key, v, err))
		return 0
	}
	return n
}

func (l *envLoader) requireBool(key string) bool {
	v := l.require(key)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		l.errors = append(l.errors, fmt.Sprintf("%s must be a boolean (true/false), got %q: %v", key, v, err))
		return false
	}
	return b
}

func (l *envLoader) requireDurationMinutes(key string) time.Duration {
	v := l.require(key)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Minute
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errors = append(l.errors, fmt.Sprintf("%s must be a valid duration or minutes integer, got %q: %v", key, v, err))
		return 0
	}
	return d
}

func (l *envLoader) requireDurationDays(key string) time.Duration {
	v := l.require(key)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * 24 * time.Hour
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errors = append(l.errors, fmt.Sprintf("%s must be a valid duration or days integer, got %q: %v", key, v, err))
		return 0
	}
	return d
}

func (l *envLoader) requireDuration(key string) time.Duration {
	v := l.require(key)
	if v == "" {
		return 0
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.errors = append(l.errors, fmt.Sprintf("%s must be a valid duration, got %q: %v", key, v, err))
		return 0
	}
	return d
}
