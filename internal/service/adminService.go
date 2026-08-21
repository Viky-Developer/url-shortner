// Package service contains the business logic for the URL shortener.
package service

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"time"

	"github.com/sqlc-dev/pqtype"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/payload"
)

// AdminService handles admin-only operations like managing blocked domains,
// IP ranges, and running maintenance purges.
type AdminService struct {
	queries gen.Querier
}

// NewAdminService creates a new AdminService.
func NewAdminService(q gen.Querier) *AdminService {
	return &AdminService{queries: q}
}

// ═══════════════════════════════════════════════════════════════
//  BLOCKED DOMAINS
// ═══════════════════════════════════════════════════════════════

// ListBlockedDomains returns all blocked domains.
func (a *AdminService) ListBlockedDomains(ctx context.Context) ([]payload.BlockedDomainResponse, error) {
	rows, err := a.queries.ListBlockedDomains(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: could not list blocked domains", payload.ErrInternal)
	}

	items := make([]payload.BlockedDomainResponse, len(rows))
	for i, r := range rows {
		items[i] = payload.BlockedDomainResponse{
			ID:        int64(r.ID),
			Domain:    r.Domain,
			Reason:    r.Reason.String,
			CreatedAt: formatNullTime(r.CreatedAt),
		}
	}
	return items, nil
}

// CreateBlockedDomain adds a new domain to the block list.
func (a *AdminService) CreateBlockedDomain(ctx context.Context, req payload.CreateBlockedDomainRequest) (*payload.BlockedDomainResponse, error) {
	if req.Domain == "" {
		return nil, fmt.Errorf("%w: domain is required", payload.ErrInvalidPayload)
	}

	row, err := a.queries.CreateBlockedDomain(ctx, gen.CreateBlockedDomainParams{
		Domain: req.Domain,
		Reason: toNullString(req.Reason),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: could not create blocked domain", payload.ErrInternal)
	}

	return &payload.BlockedDomainResponse{
		ID:        int64(row.ID),
		Domain:    row.Domain,
		Reason:    row.Reason.String,
		CreatedAt: formatNullTime(row.CreatedAt),
	}, nil
}

// DeleteBlockedDomain removes a domain from the block list.
func (a *AdminService) DeleteBlockedDomain(ctx context.Context, id int32) error {
	if err := a.queries.DeleteBlockedDomain(ctx, id); err != nil {
		return fmt.Errorf("%w: could not delete blocked domain", payload.ErrInternal)
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════
//  BLOCKED IP RANGES
// ═══════════════════════════════════════════════════════════════

// ListBlockedIPRanges returns all blocked IP ranges.
func (a *AdminService) ListBlockedIPRanges(ctx context.Context) ([]payload.BlockedIPRangeResponse, error) {
	rows, err := a.queries.ListBlockedIPRanges(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: could not list blocked IP ranges", payload.ErrInternal)
	}

	items := make([]payload.BlockedIPRangeResponse, len(rows))
	for i, r := range rows {
		items[i] = payload.BlockedIPRangeResponse{
			ID:          r.ID,
			CIDR:        r.Cidr.IPNet.String(),
			Description: r.Description,
		}
	}
	return items, nil
}

// CreateBlockedIPRange adds a new IP range to the block list.
func (a *AdminService) CreateBlockedIPRange(ctx context.Context, req payload.CreateBlockedIPRangeRequest) (*payload.BlockedIPRangeResponse, error) {
	if req.CIDR == "" {
		return nil, fmt.Errorf("%w: cidr is required", payload.ErrInvalidPayload)
	}

	_, cidr, err := net.ParseCIDR(req.CIDR)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid CIDR format", payload.ErrInvalidPayload)
	}

	row, err := a.queries.CreateBlockedIPRange(ctx, gen.CreateBlockedIPRangeParams{
		Cidr:        pqtype.CIDR{IPNet: *cidr, Valid: true},
		Description: req.Description,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: could not create blocked IP range", payload.ErrInternal)
	}

	return &payload.BlockedIPRangeResponse{
		ID:          row.ID,
		CIDR:        row.Cidr.IPNet.String(),
		Description: row.Description,
	}, nil
}

// DeleteBlockedIPRange removes an IP range from the block list.
func (a *AdminService) DeleteBlockedIPRange(ctx context.Context, id int64) error {
	if err := a.queries.DeleteBlockedIPRange(ctx, id); err != nil {
		return fmt.Errorf("%w: could not delete blocked IP range", payload.ErrInternal)
	}
	return nil
}

// ═══════════════════════════════════════════════════════════════
//  MAINTENANCE PURGES
// ═══════════════════════════════════════════════════════════════

// PurgeOldRevokedSessions deletes revoked sessions older than the given duration.
func (a *AdminService) PurgeOldRevokedSessions(ctx context.Context, olderThan time.Duration) error {
	before := time.Now().Add(-olderThan)
	if err := a.queries.PurgeOldRevokedSessions(ctx, sql.NullTime{Time: before, Valid: true}); err != nil {
		return fmt.Errorf("%w: could not purge revoked sessions", payload.ErrInternal)
	}
	return nil
}

// PurgeOldPasswordHistory deletes password history records older than the given duration.
func (a *AdminService) PurgeOldPasswordHistory(ctx context.Context, olderThan time.Duration) error {
	before := time.Now().Add(-olderThan)
	if err := a.queries.PurgeOldPasswordHistory(ctx, sql.NullTime{Time: before, Valid: true}); err != nil {
		return fmt.Errorf("%w: could not purge password history", payload.ErrInternal)
	}
	return nil
}

// SoftDeleteUser marks a user as deleted.
func (a *AdminService) SoftDeleteUser(ctx context.Context, userID int64) error {
	if err := a.queries.SoftDeleteUser(ctx, userID); err != nil {
		return fmt.Errorf("%w: could not soft delete user", payload.ErrInternal)
	}
	return nil
}

// HardDeleteUser permanently removes a soft-deleted user.
func (a *AdminService) HardDeleteUser(ctx context.Context, userID int64) error {
	if err := a.queries.HardDeleteUser(ctx, userID); err != nil {
		return fmt.Errorf("%w: could not hard delete user", payload.ErrInternal)
	}
	return nil
}

func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}
