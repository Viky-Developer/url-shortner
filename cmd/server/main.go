package main

import (
	"fmt"
	"os"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/config"
	"github.com/vicky/url-shortner/internal/db"
)

func main() {
	defer logger.Recover()

	cfg := config.Load()

	log, err := logger.New(logger.WithLevel(cfg.LogLevel))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = log.Sync() }()

	database, err := cfg.Connect()
	if err != nil {
		log.Fatal("failed to connect to database", logger.Error(err))
	}
	defer func() {
		if err := database.Close(); err != nil {
			log.Error("failed to close database", logger.Error(err))
		}
	}()

	log.Info("connected to postgres",
		logger.String("host", cfg.DBHost),
		logger.String("port", cfg.DBPort),
		logger.String("database", cfg.DBName),
	)

	if err := db.Migrate(database, "internal/db/migrations"); err != nil {
		log.Fatal("failed to run migrations", logger.Error(err))
	}

	log.Info("migrations applied successfully")
}
