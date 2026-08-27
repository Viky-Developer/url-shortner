package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/vicky/url-shortner/external/cache"
	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/config"
	"github.com/vicky/url-shortner/internal/db"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/graceful"
	"github.com/vicky/url-shortner/internal/handler"
	"github.com/vicky/url-shortner/internal/middleware"
	"github.com/vicky/url-shortner/internal/routes"
	"github.com/vicky/url-shortner/internal/service"
	"github.com/vicky/url-shortner/internal/utils"
	"golang.org/x/crypto/bcrypt"
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

	queries := gen.New(database)
	sessionCache, err := cache.NewRedisCache(cache.RedisConfig{
		Addr:       cfg.RedisHost + ":" + cfg.RedisPort,
		UserName:   cfg.RedisUserName,
		Password:   cfg.RedisPassword,
		DB:         cfg.RedisDB,
		MaxRetries: cfg.RedisMaxRetries,
	})
	if err != nil {
		log.Warn("redis unavailable, falling back to no cache", logger.Error(err))
	} else {
		log.Info("redis connected", logger.String("addr", cfg.RedisHost+":"+cfg.RedisPort))
		defer func() { _ = sessionCache.Close() }()
	}

	authService := service.NewAuthService(queries, database, cfg, sessionCache, log)
	urlHandler := buildURLHandler(cfg, database, log)
	authHandler := handler.NewAuthHandler(authService, log)
	adminService := service.NewAdminService(queries)
	adminHandler := handler.NewAdminHandler(adminService, log)
	app := middleware.Chain(routes.New(urlHandler, authHandler, adminHandler, authService),
		middleware.Recovery(log),
		middleware.Logger(log),
		middleware.ContentTypeJSON,
	)

	// Start the background retention worker for session/password history cleanup.
	retentionWorker := service.NewRetentionWorker(adminService, queries, cfg, log)
	retentionCtx, retentionCancel := context.WithCancel(context.Background())
	go retentionWorker.Start(retentionCtx)

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
	retentionCancel() // stop the background retention worker
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

	if err := ensureDefaultUser(database, cfg, log); err != nil {
		return nil, err
	}

	return database, nil
}

// ensureDefaultUser creates a default user from .env credentials if one does
// not already exist, and ensures the default user always has the ADMIN role.
func ensureDefaultUser(database *sql.DB, cfg *config.Config, log logger.Logger) error {
	q := gen.New(database)
	user, err := q.GetUserByEmail(context.Background(), cfg.DefaultUserEmail)
	if err == nil {
		log.Info("default user already exists", logger.String("email", cfg.DefaultUserEmail))
		// Ensure default user always has ADMIN role
		if user.Role != "ADMIN" {
			if err := q.UpdateUserRole(context.Background(), gen.UpdateUserRoleParams{
				ID:   user.ID,
				Role: "ADMIN",
			}); err != nil {
				return fmt.Errorf("promote default user to admin: %w", err)
			}
			log.Info("default user promoted to ADMIN", logger.String("email", cfg.DefaultUserEmail))
		}
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check default user: %w", err)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(cfg.DefaultUserPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash default user password: %w", err)
	}

	row, err := q.CreateUser(context.Background(), gen.CreateUserParams{
		Email:         cfg.DefaultUserEmail,
		PasswordHash:  string(hashedPassword),
		DisplayUserID: sql.NullString{}, // NULL — computed and stored after insert
	})
	if err != nil {
		return fmt.Errorf("create default user: %w", err)
	}

	// Set default user role to ADMIN
	if err := q.UpdateUserRole(context.Background(), gen.UpdateUserRoleParams{
		ID:   row.ID,
		Role: "ADMIN",
	}); err != nil {
		return fmt.Errorf("set default user role to admin: %w", err)
	}

	// Seed the initial password history entry — forgot-password validates
	// against the last history row, so it must exist from day one.
	if err := q.AddPasswordHistory(context.Background(), gen.AddPasswordHistoryParams{
		UserID:       row.ID,
		PasswordHash: string(hashedPassword),
		IpAddress:    utils.NullIP(""),
		UserAgent:    utils.NullString(""),
	}); err != nil {
		return fmt.Errorf("seed default user password history: %w", err)
	}

	// The id is generated by the DB sequence, so the display_user_id is encoded
	// from it afterwards and stored in our format (e.g. "USR_8dUQqQrLwel").
	_, err = q.UpdateUserDisplayID(context.Background(), gen.UpdateUserDisplayIDParams{
		ID: row.ID,
		DisplayUserID: sql.NullString{
			String: utils.EncodeID(row.ID, utils.UserIDPrefix, cfg.UserIDSecretKey),
			Valid:  true,
		},
	})
	if err != nil {
		return fmt.Errorf("update default user display id: %w", err)
	}

	log.Info("default user created",
		logger.Int64("id", row.ID),
		logger.String("email", row.Email),
		logger.String("userId", utils.EncodeID(row.ID, utils.UserIDPrefix, cfg.UserIDSecretKey)),
	)
	return nil
}

// buildURLHandler constructs the query layer, service, and HTTP handler for
// the URL endpoints.
func buildURLHandler(cfg *config.Config, database *sql.DB, log logger.Logger) *handler.URLHandler {
	queries := gen.New(database)
	urlService := service.NewURLService(queries, database, cfg.ServerBaseURL, cfg.UserIDSecretKey, log)
	return handler.NewURLHandler(urlService, log)
}
