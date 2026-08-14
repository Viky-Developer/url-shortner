package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/config"
	"github.com/vicky/url-shortner/internal/db"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/graceful"
	"github.com/vicky/url-shortner/internal/handler"
	"github.com/vicky/url-shortner/internal/middleware"
	"github.com/vicky/url-shortner/internal/routes"
	"github.com/vicky/url-shortner/internal/service"
)

// main is the application entry point. It recovers from panics and exits with
// a non-zero status when run fails.
func main() {
	defer logger.Recover()

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

// run wires configuration, logging, database, handlers, routes, and the HTTP
// server together, then blocks until the server shuts down gracefully.
func run() error {
	cfg := config.Load()

	log, err := logger.New(logger.WithLevel(cfg.LogLevel))
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	defer func() { _ = log.Sync() }()

	database, err := connectDatabase(cfg, log)
	if err != nil {
		return err
	}

	defer func() {
		if err := database.Close(); err != nil {
			log.Error("failed to close database", logger.Error(err))
		}
	}()

	urlHandler := buildURLHandler(cfg, database, log)
	app := middleware.Chain(routes.New(urlHandler),
		middleware.Recovery(log),
		middleware.Logger(log),
		middleware.ContentTypeJSON,
	)

	server := &http.Server{
		Addr:         cfg.ServerHost + ":" + cfg.ServerPort,
		Handler:      app,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("server starting", logger.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("server failed", logger.Error(err))
		}
	}()

	graceful.WaitForSignal()
	graceful.Shutdown(server, log, 10*time.Second)

	return nil
}

// connectDatabase opens a Postgres connection and applies pending migrations,
// logging each step along the way.
func connectDatabase(cfg *config.Config, log logger.Logger) (*sql.DB, error) {
	database, err := cfg.Connect()
	if err != nil {
		return nil, err
	}

	log.Info("connected to postgres",
		logger.String("host", cfg.DBHost),
		logger.String("port", cfg.DBPort),
		logger.String("database", cfg.DBName),
	)

	if err := db.Migrate(database, "internal/db/migrations"); err != nil {
		return nil, err
	}

	log.Info("migrations applied successfully")
	return database, nil
}

// buildURLHandler constructs the query layer, service, and HTTP handler for
// the URL endpoints.
func buildURLHandler(cfg *config.Config, database *sql.DB, log logger.Logger) *handler.URLHandler {
	queries := gen.New(database)
	urlService := service.NewURLService(queries, cfg.ServerBaseURL, log)
	return handler.NewURLHandler(urlService, log)
}
