package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/vicky/url-shortner/external/cache"
	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/payload"
)

const cacheKeySessionPrefix = "session:"

// AccountDeletionService handles self-service account deletion, cancellation,
// and background hard-deletion of expired accounts.
type AccountDeletionService struct {
	queries      gen.Querier
	db           *sql.DB
	adminService *AdminService
	UrlService   *URLService
	cache        *cache.RedisCache
	log          logger.Logger
}

// NewAccountDeletionService constructs an AccountDeletionService.
func NewAccountDeletionService(
	queries gen.Querier,
	db *sql.DB,
	adminService *AdminService,
	cache *cache.RedisCache,
	UrlService *URLService,
	log logger.Logger,
) *AccountDeletionService {
	return &AccountDeletionService{
		queries:      queries,
		db:           db,
		adminService: adminService,
		UrlService:   UrlService,
		cache:        cache,
		log:          log,
	}
}

// RequestDeletion validates the confirmation text, revokes all sessions,
// and marks the account as PENDING_DELETION with a 30-day grace period.
func (s *AccountDeletionService) RequestDeletion(ctx context.Context, userID int64) (*payload.AccountStatusResponse, error) {

	// Check user exists and is ACTIVE
	user, err := s.queries.GetUserStatusByID(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperror.ErrNotFound
		}
		return nil, apperror.ErrInternal
	}

	if user.Status != "ACTIVE" {
		return nil, fmt.Errorf("%w: account is already %s", apperror.ErrConflict, user.Status)
	}

	var sessions []gen.Session

	// Revoke all sessions and mark pending deletion in a transaction
	err = s.UrlService.withTx(ctx, func(q gen.Querier) error {
		// Fetch active sessions for cache cleanup
		sessions, err = q.ListActiveSessionsByUser(ctx, userID)
		if err != nil {
			s.log.Error("requestDeletion: failed to list sessions", logger.Error(err), logger.Int64("userID", userID))
			return apperror.ErrInternal
		}

		// Bulk revoke all sessions
		if rErr := q.RevokeAllSessionsByUser(ctx, userID); rErr != nil {
			s.log.Error("requestDeletion: failed to revoke sessions", logger.Error(rErr), logger.Int64("userID", userID))
			return apperror.ErrInternal
		}

		// Mark account as pending deletion
		if mErr := q.MarkPendingDeletion(ctx, userID); mErr != nil {
			s.log.Error("requestDeletion: failed to mark pending deletion", logger.Error(mErr), logger.Int64("userID", userID))
			return apperror.ErrInternal
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Evict session caches
	if s.cache != nil {
		for _, sess := range sessions {
			_ = s.cache.Del(ctx, fmt.Sprintf("%s%d", cacheKeySessionPrefix, sess.ID))
		}
	}

	// Audit log (user's own ID as admin_id)
	s.adminService.LogAction(ctx, userID, "ACCOUNT_DELETION_REQUESTED", "USER", userID, []byte(fmt.Sprintf(`{"scheduledAt": "%s"}`, time.Now().Add(30*24*time.Hour).Format(time.RFC3339)))...)

	s.log.Info("account deletion requested", logger.Int64("userID", userID))

	scheduledAt := time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339)
	return &payload.AccountStatusResponse{
		Status:              "PENDING_DELETION",
		DeletionScheduledAt: &scheduledAt,
	}, nil
}

// CancelDeletion restores a PENDING_DELETION account to ACTIVE.
func (s *AccountDeletionService) CancelDeletion(ctx context.Context, userID int64) error {

	user, err := s.queries.GetUserStatusByID(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return apperror.ErrNotFound
		}
		return apperror.ErrInternal
	}

	if user.Status != "PENDING_DELETION" {
		return fmt.Errorf("%w: account is not pending deletion", apperror.ErrConflict)
	}

	if err := s.queries.RestoreAccount(ctx, userID); err != nil {
		return apperror.ErrInternal
	}

	s.adminService.LogAction(ctx, userID, "ACCOUNT_RESTORED", "USER", userID, []byte(`{}`)...)
	s.log.Info("account deletion cancelled", logger.Int64("userID", userID))
	return nil
}

// GetStatus returns the current account status.
func (s *AccountDeletionService) GetStatus(ctx context.Context, userID int64) (*payload.AccountStatusResponse, error) {

	user, err := s.queries.GetUserStatusByID(ctx, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, apperror.ErrNotFound
		}
		return nil, apperror.ErrInternal
	}

	resp := &payload.AccountStatusResponse{Status: user.Status}
	return resp, nil
}

// ProcessDeletions hard-deletes accounts whose 30-day grace period has expired.
// Called by the retention worker.
func (s *AccountDeletionService) ProcessDeletions(ctx context.Context) error {

	accounts, err := s.queries.GetAccountsDueForDeletion(ctx)
	if err != nil {
		return err
	}

	for _, userID := range accounts {

		// HardDeleteUserByID cascades via ON DELETE CASCADE
		if err := s.queries.HardDeleteUserByID(ctx, userID); err != nil {
			s.log.Error("processDeletions: failed to hard delete account", logger.Int64("id", userID), logger.Error(err))
			continue
		}

		s.adminService.LogAction(ctx, 0, "ACCOUNT_DELETED", "USER", userID, []byte(`{}`)...)
		s.log.Info("account hard deleted", logger.Int64("id", userID))
	}

	return nil
}
