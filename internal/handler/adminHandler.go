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
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/response"
	"github.com/vicky/url-shortner/internal/utils"
)

// AdminService is the contract the admin handler depends on.
type AdminService interface {
	ListBlockedDomains(ctx context.Context) ([]any, error)
	CreateBlockedDomain(ctx context.Context, req payload.CreateBlockedDomainRequest) (*payload.BlockedDomainResponse, error)
	DeleteBlockedDomain(ctx context.Context, id int32) error
	ListBlockedIPRanges(ctx context.Context) ([]any, error)
	CreateBlockedIPRange(ctx context.Context, req payload.CreateBlockedIPRangeRequest) (*payload.BlockedIPRangeResponse, error)
	DeleteBlockedIPRange(ctx context.Context, id int64) error
	PurgeOldRevokedSessions(ctx context.Context, olderThan time.Duration) error
	PurgeOldPasswordHistory(ctx context.Context, olderThan time.Duration) error
	SoftDeleteUser(ctx context.Context, userID int64) error
	HardDeleteUser(ctx context.Context, userID int64) error
	LogAction(ctx context.Context, adminID int64, action, targetType string, targetID int64, metaData ...byte)
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

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	domain, err := h.adminService.CreateBlockedDomain(r.Context(), req)
	if err != nil {
		h.log.Error("failed to create blocked domain", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.adminService.LogAction(r.Context(), userID, "CREATE_BLOCKED_DOMAIN", "BLOCKED_DOMAIN", int64(domain.ID))

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

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	if err := h.adminService.DeleteBlockedDomain(r.Context(), int32(id)); err != nil {
		h.log.Error("failed to delete blocked domain", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.adminService.LogAction(r.Context(), userID, "DELETE_BLOCKED_DOMAIN", "BLOCKED_DOMAIN", id)

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

	response.Success(w, http.StatusOK, "blocked IP ranges retrieved", ranges)
}

// CreateBlockedIPRange handles POST /admin/blocked-ip-ranges.
func (h *AdminHandler) CreateBlockedIPRange(w http.ResponseWriter, r *http.Request) {

	var req payload.CreateBlockedIPRangeRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: invalid request body", apperror.ErrInvalidPayload))
		return
	}

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	ipRange, err := h.adminService.CreateBlockedIPRange(r.Context(), req)
	if err != nil {
		h.log.Error("failed to create blocked IP range", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.adminService.LogAction(r.Context(), userID, "CREATE_BLOCKED_IP_RANGE", "BLOCKED_IP_RANGE", ipRange.ID)

	response.Success(w, http.StatusCreated, "IP range blocked", []any{ipRange})
}

// DeleteBlockedIPRange handles DELETE /admin/blocked-ip-ranges/{id}.
func (h *AdminHandler) DeleteBlockedIPRange(w http.ResponseWriter, r *http.Request) {

	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: invalid id", apperror.ErrInvalidPayload))
		return
	}

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	if err := h.adminService.DeleteBlockedIPRange(r.Context(), id); err != nil {
		h.log.Error("failed to delete blocked IP range", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.adminService.LogAction(r.Context(), userID, "DELETE_BLOCKED_IP_RANGE", "BLOCKED_IP_RANGE", id)

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

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	if err := h.adminService.SoftDeleteUser(r.Context(), id); err != nil {
		h.log.Error("failed to soft delete user", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.adminService.LogAction(r.Context(), userID, "SOFT_DELETE_USER", "USER", id)

	response.Success(w, http.StatusOK, "user soft deleted", []any{})
}

// HardDeleteUser handles DELETE /admin/users/{id}/hard-delete.
func (h *AdminHandler) HardDeleteUser(w http.ResponseWriter, r *http.Request) {

	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: invalid id", apperror.ErrInvalidPayload))
		return
	}

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	if err := h.adminService.HardDeleteUser(r.Context(), id); err != nil {
		h.log.Error("failed to hard delete user", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.adminService.LogAction(r.Context(), userID, "HARD_DELETE_USER", "USER", id)
	response.Success(w, http.StatusOK, "user permanently deleted", []any{})
}

// ───────────────────────────────────────────────────────────────
//  MAINTENANCE
// ───────────────────────────────────────────────────────────────

// PurgeSessions handles POST /admin/maintenance/purge-sessions.
func (h *AdminHandler) PurgeSessions(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	days := utils.ParseDaysParam(r, 30)

	if err := h.adminService.PurgeOldRevokedSessions(r.Context(), days); err != nil {
		h.log.Error("failed to purge sessions", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.adminService.LogAction(r.Context(), userID, "PURGE_SESSION", "SESSION", 0)

	response.Success(w, http.StatusOK, "old sessions purged", []any{})
}

// PurgePasswordHistory handles POST /admin/maintenance/purge-password-history.
func (h *AdminHandler) PurgePasswordHistory(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	days := utils.ParseDaysParam(r, 90)

	if err := h.adminService.PurgeOldPasswordHistory(r.Context(), days); err != nil {
		h.log.Error("failed to purge password history", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.adminService.LogAction(r.Context(), userID, "PURGE_PASSWORD_HISTORY", "PASSWORD_HISTORY", 0)

	response.Success(w, http.StatusOK, "old password history purged", []any{})
}
