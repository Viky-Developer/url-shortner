// Package config loads application configuration from environment variables
// and provides database connectivity helpers.
package config

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

// Config holds all runtime configuration values for the application.
type Config struct {
	DBHost        string        // Postgres host.
	DBPort        string        // Postgres port.
	DBUser        string        // Postgres user.
	DBPassword    string        // Postgres password.
	DBName        string        // Postgres database name.
	SSLMode       string        // Postgres sslmode.
	LogLevel      string        // Minimum log level (debug, info, warn, error).
	DBMaxOpen     int           // Maximum number of open connections.
	DBMaxIdle     int           // Maximum number of idle connections.
	DBMaxLife     time.Duration // Maximum connection lifetime.
	ServerHost    string        // HTTP server bind host.
	ServerPort    string        // HTTP server bind port.
	ServerBaseURL string        // Public base URL used to build short URLs.
}

// Load reads configuration from the .env file (if present) and the process
// environment, applying sensible defaults for any missing values.
func Load() *Config {
	_ = godotenv.Load()
	return &Config{
		DBHost:        getEnv("DB_HOST", "localhost"),
		DBPort:        getEnv("DB_PORT", "5432"),
		DBUser:        getEnv("DB_USER", "urlshortner"),
		DBPassword:    getEnv("DB_PASSWORD", "urlshortner123"),
		DBName:        getEnv("DB_NAME", "urlshortner"),
		SSLMode:       getEnv("DB_SSLMODE", "disable"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
		DBMaxOpen:     getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdle:     getEnvInt("DB_MAX_IDLE_CONNS", 25),
		DBMaxLife:     time.Duration(getEnvInt("DB_MAX_LIFETIME", 5)) * time.Minute,
		ServerHost:    getEnv("SERVER_HOST", "0.0.0.0"),
		ServerPort:    getEnv("SERVER_PORT", "8080"),
		ServerBaseURL: getEnv("SERVER_BASE_URL", "http://localhost:8080"),
	}
}

// DSN returns the Postgres connection string built from the config values.
func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.SSLMode,
	)
}

// Connect opens a Postgres connection pool configured from the config values
// and verifies it with a ping.
func (c *Config) Connect() (*sql.DB, error) {
	db, err := sql.Open("postgres", c.DSN())
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

// getEnv returns the value of the environment variable key, or fallback when
// the variable is empty or unset.
func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// getEnvInt returns the integer value of the environment variable key, or
// fallback when the variable is empty or unparsable.
func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	var n int
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return fallback
	}
	return n
}
