// Package service provides the business logic layer for the URL shortener.
package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/config"
	gen "github.com/vicky/url-shortner/internal/db/gen"
)

// RetentionWorker runs periodic cleanup of expired sessions, old password
// history records, and expired account deletions. It uses AdminService so
// both the cron and manual API share the same purge logic.
type RetentionWorker struct {
	adminService           *AdminService
	accountDeletionService *AccountDeletionService
	queries                gen.Querier
	cfg                    *config.Config
	log                    logger.Logger
}

// NewRetentionWorker creates a new RetentionWorker.
func NewRetentionWorker(adminService *AdminService, accountDeletionService *AccountDeletionService, queries gen.Querier, cfg *config.Config, log logger.Logger) *RetentionWorker {
	return &RetentionWorker{
		adminService:           adminService,
		accountDeletionService: accountDeletionService,
		queries:                queries,
		cfg:                    cfg,
		log:                    log,
	}
}

// Start begins the retention cleanup loop. It runs the first purge immediately,
// then waits for the configured interval between subsequent runs. The loop
// stops when ctx is cancelled (typically on server shutdown).
func (w *RetentionWorker) Start(ctx context.Context) {
	if !w.cfg.EnableRetentionWorker {
		w.log.Info("retention worker disabled")
		return
	}

	w.log.Info("retention worker started",
		logger.String("interval", w.cfg.RetentionRunInterval.String()),
		logger.String("sessionRetention", w.cfg.SessionRetention.String()),
	)

	// Run immediately on startup.
	w.run(ctx)

	ticker := time.NewTicker(w.cfg.RetentionRunInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("retention worker stopped")
			return
		case <-ticker.C:
			w.run(ctx)
		}
	}
}

// run executes the two retention purge tasks through AdminService so the
// same logic is shared with the manual admin API.
func (w *RetentionWorker) run(ctx context.Context) {
	w.log.Info("retention worker: starting purge cycle")

	// 1. Purge revoked and expired sessions older than retention period.
	if err := w.adminService.PurgeOldRevokedSessions(ctx, w.cfg.SessionRetention); err != nil {
		w.log.Error("retention worker: failed to purge sessions", logger.Error(err))
	} else {
		w.log.Info("retention worker: sessions purged")
	}

	// 2. Purge password history older than retention period.
	if err := w.adminService.PurgeOldPasswordHistory(ctx, w.cfg.PasswordRetention); err != nil {
		w.log.Error("retention worker: failed to purge password history", logger.Error(err))
	} else {
		w.log.Info("retention worker: password history purged by age")
	}

	// 3. Hard-delete accounts whose deletion grace period has expired.
	if w.accountDeletionService != nil {
		if err := w.accountDeletionService.ProcessDeletions(ctx); err != nil {
			w.log.Error("retention worker: failed to process account deletions", logger.Error(err))
		} else {
			w.log.Info("retention worker: account deletions processed")
		}
	}

	// 3. Log the worker's own action for audit trail.
	details, _ := json.Marshal(map[string]string{
		"sessionRetention":  w.cfg.SessionRetention.String(),
		"passwordRetention": w.cfg.PasswordRetention.String(),
	})
	w.adminService.LogAction(ctx, 0, "RETENTION_PURGE", "SYSTEM", 0, details...)

	w.log.Info("retention worker: purge cycle complete")
}
