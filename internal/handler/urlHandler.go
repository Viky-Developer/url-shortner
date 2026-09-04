// Package handler contains the HTTP handlers for the URL shortener API.
package handler

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/enum"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/response"
	"github.com/vicky/url-shortner/internal/utils"
	"github.com/vicky/url-shortner/internal/validation"
)

// lookupIP resolves a host to its IP addresses. It is a variable so tests can
// stub DNS resolution.
var lookupIP = net.LookupIP

// URLService is the contract the handlers depend on for URL business logic.
// It is satisfied by *service.URLService and can be mocked in tests.
type URLService interface {
	Create(ctx context.Context, userID int64, req payload.CreateURLRequest) (*payload.URLResponse, error)
	Redirect(ctx context.Context, shortCode string, click payload.ClickInfo) (*payload.URLResponse, error)
	GetByID(ctx context.Context, userID int64, id int64) (*payload.URLResponse, error)
	List(ctx context.Context, userID int64, page, perPage, offset int32, status *int16) ([]any, int64, error)
	CountByStatus(ctx context.Context, userID int64) (*payload.URLStatusCounts, error)
	Update(ctx context.Context, userID int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error)
	SoftDelete(ctx context.Context, userID int64, id int64) (*payload.DeleteResponse, error)
	HardDelete(ctx context.Context, userID int64, id int64) error
	ListClickLogs(ctx context.Context, userID, urlID int64, from, to *time.Time, page, perPage, offset int32) ([]any, int64, error)
	GetAnalytics(ctx context.Context, userID, urlID int64, from, to *time.Time) (*payload.AnalyticsResponse, error)
}

// URLHandler holds the dependencies required by the URL HTTP handlers.
type URLHandler struct {
	urlService URLService
	log        logger.Logger
}

// NewURLHandler constructs a URLHandler with the given service and logger.
func NewURLHandler(urlService URLService, log logger.Logger) *URLHandler {
	return &URLHandler{urlService: urlService, log: log}
}

// CreateShortURL handles POST /shorten and creates a new short URL.
func (h *URLHandler) CreateShortURL(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	req, ok := validation.BindAndValidate[payload.CreateURLRequest](r, w)
	if !ok {
		return
	}

	if err := utils.ValidateURL(req.OriginalURL, lookupIP); err != nil {
		h.log.Error("invalid url", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %s", apperror.ErrInvalidURL, err))
		return
	}

	created, err := h.urlService.Create(r.Context(), userID, *req)
	if err != nil {
		h.log.Error("failed to create url", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("url created", logger.Int64("id", created.ID), logger.String("shortCode", utils.SanitizeLog(created.ShortCode)))

	response.Success(w, http.StatusCreated, "url created", []any{created})
}

// RedirectShortURL handles GET /api/v1/{shortCode}, records a click, and
// issues an HTTP 302 redirect to the destination URL. The browser reads the
// Location header and follows the redirect automatically.
func (h *URLHandler) RedirectShortURL(w http.ResponseWriter, r *http.Request) {

	u, err := h.urlService.Redirect(r.Context(), r.PathValue("shortCode"), payload.ClickInfo{
		IP:        utils.ClientIP(r),
		UserAgent: r.UserAgent(),
		Referrer:  r.Referer(),
	})
	if err != nil {
		h.log.Error("redirect failed", logger.Error(err), logger.String("shortCode", utils.SanitizeLog(r.PathValue("shortCode"))))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	http.Redirect(w, r, u.OriginalURL, http.StatusFound)
}

// GetURLByID handles GET /urls/{id} and returns the URL details.
func (h *URLHandler) GetURLByID(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		h.log.Error("invalid url id", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: the provided URL id is not valid", apperror.ErrInvalidPayload))
		return
	}

	u, err := h.urlService.GetByID(r.Context(), userID, id)
	if err != nil {
		h.log.Error("get url failed", logger.Error(err), logger.Int64("id", id))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "url retrieved", []any{u})
}

// ListURLs handles GET /urls and returns a paginated list of URLs, optionally
// filtered by status (active, expired, deleted). When no status is provided,
// all non-deleted URLs are returned.
func (h *URLHandler) ListURLs(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	page, perPage, offset := utils.ParsePagination(
		r.URL.Query().Get("page"),
		r.URL.Query().Get("perPage"),
	)

	var status *int16
	if s := r.URL.Query().Get("status"); s != "" {
		parsed, err := enum.ParseURLStatus(strings.ToUpper(s))
		if err != nil {
			response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %s", apperror.ErrInvalidPayload, err))
			return
		}
		v := int16(parsed)
		status = &v
	}

	list, total, err := h.urlService.List(r.Context(), userID, page, perPage, offset, status)
	if err != nil {
		h.log.Error("list urls failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	totalPages := int(total) / int(perPage)

	if int(total)%int(perPage) > 0 {
		totalPages++
	}

	if len(list) == 0 {
		response.Success(w, http.StatusOK, "urls listed", list, &payload.Pagination{
			Total:      total,
			Page:       page,
			PerPage:    perPage,
			TotalPages: totalPages,
		})

		return
	}

	response.Success(w, http.StatusOK, "urls listed", list, &payload.Pagination{
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	})
}

// GetURLStatusCounts handles GET /urls/status-counts and returns the count of
// URLs per status for the authenticated user.
func (h *URLHandler) GetURLStatusCounts(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	counts, err := h.urlService.CountByStatus(r.Context(), userID)
	if err != nil {
		h.log.Error("failed to get url status counts", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "url status counts retrieved", []any{counts})
}

// UpdateURL handles PATCH /urls/{id} and updates the URL details.
func (h *URLHandler) UpdateURL(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		h.log.Error("invalid url id", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: the provided URL id is not valid", apperror.ErrInvalidPayload))
		return
	}

	var req payload.UpdateURLRequest

	if err := utils.DecodeBody(r, &req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	if req.OriginalURL != "" {
		if err := utils.ValidateURL(req.OriginalURL, lookupIP); err != nil {
			h.log.Error("invalid url", logger.Error(err))
			response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %s", apperror.ErrInvalidURL, err))
			return
		}
	}

	updated, err := h.urlService.Update(r.Context(), userID, id, req)
	if err != nil {
		h.log.Error("update url failed", logger.Error(err), logger.Int64("id", id))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("url updated", logger.Int64("id", updated.ID))

	response.Success(w, http.StatusOK, "url updated", []any{updated})
}

// DeleteURL handles DELETE /urls/{id} and soft deletes the URL, leaving a
// hard-delete pending approval.
func (h *URLHandler) DeleteURL(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		h.log.Error("invalid url id", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: the provided URL id is not valid", apperror.ErrInvalidPayload))
		return
	}

	deleted, err := h.urlService.SoftDelete(r.Context(), userID, id)
	if err != nil {
		h.log.Error("soft delete failed", logger.Error(err), logger.Int64("id", id))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("url soft deleted", logger.Int64("id", deleted.ID), logger.String("shortCode", utils.SanitizeLog(deleted.ShortCode)))

	response.Success(w, http.StatusOK, "url soft deleted", []any{deleted})
}

// ApproveHardDelete handles DELETE /urls/{id}/approve and permanently removes
// a previously soft-deleted URL.
func (h *URLHandler) ApproveHardDelete(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		h.log.Error("invalid url id", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: the provided URL id is not valid", apperror.ErrInvalidPayload))
		return
	}

	if err := h.urlService.HardDelete(r.Context(), userID, id); err != nil {
		h.log.Error("hard delete failed", logger.Error(err), logger.Int64("id", id))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("url hard deleted", logger.Int64("id", id))

	response.Success(w, http.StatusOK, "url permanently deleted", []any{})
}

// ListClickLogs handles GET /urls/{id}/clicks and returns paginated click logs
// for a specific URL, optionally filtered by from/to query parameters.
func (h *URLHandler) ListClickLogs(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		h.log.Error("invalid url id", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: the provided URL id is not valid", apperror.ErrInvalidPayload))
		return
	}

	from, to := utils.ParseTimeRange(r)
	page, perPage, offset := utils.ParsePagination(r.URL.Query().Get("page"), r.URL.Query().Get("perPage"))

	clicks, total, err := h.urlService.ListClickLogs(r.Context(), userID, id, from, to, page, perPage, offset)
	if err != nil {
		h.log.Error("failed to list click logs", logger.Error(err), logger.Int64("id", id))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	totalPages := int(total) / int(perPage)

	if int(total)%int(perPage) > 0 {
		totalPages++
	}

	if len(clicks) == 0 {
		response.Success(w, http.StatusOK, "click logs retrieved", clicks, &payload.Pagination{
			Total:      total,
			Page:       page,
			PerPage:    perPage,
			TotalPages: totalPages,
		})

		return
	}

	response.Success(w, http.StatusOK, "click logs retrieved", clicks, &payload.Pagination{
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	})
}

// GetAnalytics handles GET /urls/{id}/analytics and returns aggregate click
// analytics for a specific URL, optionally filtered by from/to query parameters.
func (h *URLHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) {

	userID, ok := utils.GetUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		h.log.Error("invalid url id", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: the provided URL id is not valid", apperror.ErrInvalidPayload))
		return
	}

	from, to := utils.ParseTimeRange(r)

	analytics, err := h.urlService.GetAnalytics(r.Context(), userID, id, from, to)
	if err != nil {
		h.log.Error("failed to get analytics", logger.Error(err), logger.Int64("id", id))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "analytics retrieved", []any{analytics})
}
