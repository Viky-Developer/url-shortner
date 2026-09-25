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
	goose.SetTableName(goose.DefaultTablename)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}
	if err := goose.Up(db, dir, goose.WithAllowMissing()); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	return nil
}

// Seed applies all pending goose seed migrations from the given directory to
// the database using a dedicated table goose_seed_version.
func Seed(db *sql.DB, dir string) error {
	goose.SetBaseFS(nil)
	goose.SetTableName("goose_seed_version")
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}
	if err := goose.Up(db, dir, goose.WithAllowMissing()); err != nil {
		return fmt.Errorf("failed to run seeds: %w", err)
	}
	goose.SetTableName(goose.DefaultTablename)
	return nil
}
