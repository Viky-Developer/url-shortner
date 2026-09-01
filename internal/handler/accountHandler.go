package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/response"
	"github.com/vicky/url-shortner/internal/utils"
	"github.com/vicky/url-shortner/internal/validation"
)

// AccountDeletionService is the contract the handlers depend on for account
// deletion business logic. Implemented by *service.AccountDeletionService.
type AccountDeletionService interface {
	RequestDeletion(ctx context.Context, userID int64) (*payload.AccountStatusResponse, error)
	CancelDeletion(ctx context.Context, userID int64) error
	GetStatus(ctx context.Context, userID int64) (*payload.AccountStatusResponse, error)
}

// AccountHandler holds the dependencies required by the account HTTP handlers.
type AccountHandler struct {
	deletionService AccountDeletionService
	log             logger.Logger
}

// NewAccountHandler creates an AccountHandler.
func NewAccountHandler(deletionService AccountDeletionService, log logger.Logger) *AccountHandler {
	return &AccountHandler{
		deletionService: deletionService,
		log:             log,
	}
}

// DeleteAccount handles DELETE /api/v1/account.
func (h *AccountHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, apperror.ErrUnauthorized)
		return
	}

	body, ok := validation.BindAndValidate[payload.DeleteAccountRequest](r, w)
	if !ok {
		return
	}

	if strings.ToUpper(body.Confirmation) != "DELETE" {
		response.Error(w, response.StatusCodeFromError(apperror.ErrInvalidPayload),
			fmt.Errorf("%w: confirmation text must be exactly 'DELETE'", apperror.ErrInvalidPayload))
	}

	resp, err := h.deletionService.RequestDeletion(r.Context(), userID)
	if err != nil {
		h.log.Error("delete account failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "account scheduled for deletion in 30 days", []any{*resp})
}

// CancelDeletion handles POST /api/v1/account/cancel-deletion.
func (h *AccountHandler) CancelDeletion(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, apperror.ErrUnauthorized)
		return
	}

	if err := h.deletionService.CancelDeletion(r.Context(), userID); err != nil {
		h.log.Error("cancel deletion failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "account deletion cancelled", nil)
}

// GetAccountStatus handles GET /api/v1/account/status.
func (h *AccountHandler) GetAccountStatus(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, apperror.ErrUnauthorized)
		return
	}

	resp, err := h.deletionService.GetStatus(r.Context(), userID)
	if err != nil {
		h.log.Error("get account status failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "account status retrieved", []any{*resp})
}
