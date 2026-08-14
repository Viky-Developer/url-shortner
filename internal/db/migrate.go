package db

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
)

// Migrate applies all pending goose migrations from the given directory to
// the database, using the postgres dialect.
func Migrate(db *sql.DB, dir string) error {
	goose.SetBaseFS(nil)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}
	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}
