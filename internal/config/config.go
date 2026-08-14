package config

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	SSLMode    string
	LogLevel   string
	DBMaxOpen  int
	DBMaxIdle  int
	DBMaxLife  time.Duration
}

func Load() *Config {
	_ = godotenv.Load()
	return &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "urlshortner"),
		DBPassword: getEnv("DB_PASSWORD", "urlshortner123"),
		DBName:     getEnv("DB_NAME", "urlshortner"),
		SSLMode:    getEnv("DB_SSLMODE", "disable"),
		LogLevel:   getEnv("LOG_LEVEL", "info"),
		DBMaxOpen:  getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdle:  getEnvInt("DB_MAX_IDLE_CONNS", 25),
		DBMaxLife:  time.Duration(getEnvInt("DB_MAX_LIFETIME", 5)) * time.Minute,
	}
}

func (c *Config) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		c.DBHost, c.DBPort, c.DBUser, c.DBPassword, c.DBName, c.SSLMode,
	)
}

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

func getEnv(key, fallback string) string {

	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

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
