// Package handler contains the HTTP handlers for the URL shortener API.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/contextutil"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/response"
	"github.com/vicky/url-shortner/internal/utils"
)

// AdminService is the contract the admin handler depends on.
type AdminService interface {
	ListBlockedDomains(ctx context.Context) ([]any, error)
	CreateBlockedDomain(ctx context.Context, req payload.CreateBlockedDomainRequest) (*payload.BlockedDomainResponse, error)
	DeleteBlockedDomain(ctx context.Context, id int32) error
	ListBlockedIPRanges(ctx context.Context) ([]payload.BlockedIPRangeResponse, error)
	CreateBlockedIPRange(ctx context.Context, req payload.CreateBlockedIPRangeRequest) (*payload.BlockedIPRangeResponse, error)
	DeleteBlockedIPRange(ctx context.Context, id int64) error
	PurgeOldRevokedSessions(ctx context.Context, olderThan time.Duration) error
	PurgeOldPasswordHistory(ctx context.Context, olderThan time.Duration) error
	SoftDeleteUser(ctx context.Context, userID int64) error
	HardDeleteUser(ctx context.Context, userID int64) error
	LogAction(ctx context.Context, adminID int64, action, targetType string, targetID int64)
}

// AdminHandler handles admin-only HTTP endpoints.
type AdminHandler struct {
	adminService AdminService
	log          logger.Logger
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(svc AdminService, log logger.Logger) *AdminHandler {
	return &AdminHandler{adminService: svc, log: log}
}

// ───────────────────────────────────────────────────────────────
//  BLOCKED DOMAINS
// ───────────────────────────────────────────────────────────────

// ListBlockedDomains handles GET /admin/blocked-domains.
func (h *AdminHandler) ListBlockedDomains(w http.ResponseWriter, r *http.Request) {
	domains, err := h.adminService.ListBlockedDomains(r.Context())
	if err != nil {
		h.log.Error("failed to list blocked domains", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "blocked domains retrieved", domains)
}

// CreateBlockedDomain handles POST /admin/blocked-domains.
func (h *AdminHandler) CreateBlockedDomain(w http.ResponseWriter, r *http.Request) {
	var req payload.CreateBlockedDomainRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: invalid request body", apperror.ErrInvalidPayload))
		return
	}

	domain, err := h.adminService.CreateBlockedDomain(r.Context(), req)
	if err != nil {
		h.log.Error("failed to create blocked domain", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}
	h.adminService.LogAction(r.Context(), getAdminID(r.Context()), "create_blocked_domain", "blocked_domain", int64(domain.ID))
	response.Success(w, http.StatusCreated, "domain blocked", []any{domain})
}

// DeleteBlockedDomain handles DELETE /admin/blocked-domains/{id}.
func (h *AdminHandler) DeleteBlockedDomain(w http.ResponseWriter, r *http.Request) {
	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: invalid id", apperror.ErrInvalidPayload))
		return
	}
	if id < 0 || id > math.MaxInt32 {
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: id out of range", apperror.ErrInvalidPayload))
		return
	}

	if err := h.adminService.DeleteBlockedDomain(r.Context(), int32(id)); err != nil {
		h.log.Error("failed to delete blocked domain", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}
	h.adminService.LogAction(r.Context(), getAdminID(r.Context()), "delete_blocked_domain", "blocked_domain", id)
	response.Success(w, http.StatusOK, "domain unblocked", []any{})
}

// ───────────────────────────────────────────────────────────────
//  BLOCKED IP RANGES
// ───────────────────────────────────────────────────────────────

// ListBlockedIPRanges handles GET /admin/blocked-ip-ranges.
func (h *AdminHandler) ListBlockedIPRanges(w http.ResponseWriter, r *http.Request) {
	ranges, err := h.adminService.ListBlockedIPRanges(r.Context())
	if err != nil {
		h.log.Error("failed to list blocked IP ranges", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}
	response.Success(w, http.StatusOK, "blocked IP ranges retrieved", []any{ranges})
}

// CreateBlockedIPRange handles POST /admin/blocked-ip-ranges.
func (h *AdminHandler) CreateBlockedIPRange(w http.ResponseWriter, r *http.Request) {
	var req payload.CreateBlockedIPRangeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: invalid request body", apperror.ErrInvalidPayload))
		return
	}

	ipRange, err := h.adminService.CreateBlockedIPRange(r.Context(), req)
	if err != nil {
		h.log.Error("failed to create blocked IP range", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}
	h.adminService.LogAction(r.Context(), getAdminID(r.Context()), "create_blocked_ip_range", "blocked_ip_range", ipRange.ID)
	response.Success(w, http.StatusCreated, "IP range blocked", []any{ipRange})
}

// DeleteBlockedIPRange handles DELETE /admin/blocked-ip-ranges/{id}.
func (h *AdminHandler) DeleteBlockedIPRange(w http.ResponseWriter, r *http.Request) {
	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: invalid id", apperror.ErrInvalidPayload))
		return
	}

	if err := h.adminService.DeleteBlockedIPRange(r.Context(), id); err != nil {
		h.log.Error("failed to delete blocked IP range", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}
	h.adminService.LogAction(r.Context(), getAdminID(r.Context()), "delete_blocked_ip_range", "blocked_ip_range", id)
	response.Success(w, http.StatusOK, "IP range unblocked", []any{})
}

// ───────────────────────────────────────────────────────────────
//  USER MANAGEMENT
// ───────────────────────────────────────────────────────────────

// SoftDeleteUser handles DELETE /admin/users/{id}/soft-delete.
func (h *AdminHandler) SoftDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: invalid id", apperror.ErrInvalidPayload))
		return
	}

	if err := h.adminService.SoftDeleteUser(r.Context(), id); err != nil {
		h.log.Error("failed to soft delete user", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}
	h.adminService.LogAction(r.Context(), getAdminID(r.Context()), "soft_delete_user", "user", id)
	response.Success(w, http.StatusOK, "user soft deleted", []any{})
}

// HardDeleteUser handles DELETE /admin/users/{id}/hard-delete.
func (h *AdminHandler) HardDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: invalid id", apperror.ErrInvalidPayload))
		return
	}

	if err := h.adminService.HardDeleteUser(r.Context(), id); err != nil {
		h.log.Error("failed to hard delete user", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}
	h.adminService.LogAction(r.Context(), getAdminID(r.Context()), "hard_delete_user", "user", id)
	response.Success(w, http.StatusOK, "user permanently deleted", []any{})
}

// ───────────────────────────────────────────────────────────────
//  MAINTENANCE
// ───────────────────────────────────────────────────────────────

// PurgeSessions handles POST /admin/maintenance/purge-sessions.
func (h *AdminHandler) PurgeSessions(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r, 30)
	if err := h.adminService.PurgeOldRevokedSessions(r.Context(), days); err != nil {
		h.log.Error("failed to purge sessions", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}
	h.adminService.LogAction(r.Context(), getAdminID(r.Context()), "purge_sessions", "session", 0)
	response.Success(w, http.StatusOK, "old sessions purged", []any{})
}

// PurgePasswordHistory handles POST /admin/maintenance/purge-password-history.
func (h *AdminHandler) PurgePasswordHistory(w http.ResponseWriter, r *http.Request) {
	days := parseDaysParam(r, 90)
	if err := h.adminService.PurgeOldPasswordHistory(r.Context(), days); err != nil {
		h.log.Error("failed to purge password history", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}
	h.adminService.LogAction(r.Context(), getAdminID(r.Context()), "purge_password_history", "password_history", 0)
	response.Success(w, http.StatusOK, "old password history purged", []any{})
}

func parseDaysParam(r *http.Request, defaultDays int32) time.Duration {
	if s := r.URL.Query().Get("days"); s != "" {
		n := utils.ParsePositiveInt(s, defaultDays)
		if n > 0 {
			return time.Duration(n) * 24 * time.Hour
		}
	}
	return time.Duration(defaultDays) * 24 * time.Hour
}

// getAdminID extracts the authenticated admin's user ID from context.
// Returns 0 if not found (should not happen behind auth middleware).
func getAdminID(ctx context.Context) int64 {
	id, _ := ctx.Value(contextutil.UserIDKey).(int64)
	return id
}
