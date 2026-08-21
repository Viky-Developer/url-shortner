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
	"github.com/vicky/url-shortner/internal/contextutil"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/response"
	"github.com/vicky/url-shortner/internal/utils"
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
	List(ctx context.Context, userID int64, page, perPage, offset int32) (*payload.URLListResponse, error)
	Update(ctx context.Context, userID int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error)
	SoftDelete(ctx context.Context, userID int64, id int64) (*payload.DeleteResponse, error)
	HardDelete(ctx context.Context, userID int64, id int64) error
	ListClickLogs(ctx context.Context, userID, urlID int64, from, to *time.Time, page, perPage, offset int32) (*payload.ClickLogsResponse, error)
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

// getUserIDFromContext extracts the authenticated user ID from the request
// context. This is set by the JWT auth middleware after decoding the token.
func (h *URLHandler) getUserIDFromContext(r *http.Request) (int64, bool) {
	userID, ok := r.Context().Value(contextutil.UserIDKey).(int64)
	return userID, ok
}

// getUserIDOrError is getUserIDFromContext plus HTTP error mapping. It reports
// false when the response has already been written.
func (h *URLHandler) getUserIDOrError(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := h.getUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: user not authenticated", apperror.ErrUnauthorized))
		return 0, false
	}
	return userID, true
}

// clientIP extracts the client IP from the X-Forwarded-For header or the
// request's remote address. Returns a fallback loopback address when the
// source IP cannot be determined.
func clientIP(r *http.Request) net.IP {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if ip := net.ParseIP(strings.TrimSpace(strings.Split(xff, ",")[0])); ip != nil {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return net.IPv4(127, 0, 0, 1)
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip
	}
	return net.IPv4(127, 0, 0, 1)
}

// CreateShortURL handles POST /shorten and creates a new short URL.
func (h *URLHandler) CreateShortURL(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.getUserIDOrError(w, r)
	if !ok {
		return
	}

	var req payload.CreateURLRequest
	if err := utils.DecodeBody(r, &req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	if req.OriginalURL == "" && req.CustomCode == "" && req.Title == "" &&
		req.Description == "" && !req.ExpiresAt.Valid {
		err := fmt.Errorf("%w: request body is required", apperror.ErrInvalidPayload)
		h.log.Error("invalid payload", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	if err := utils.ValidateURL(req.OriginalURL, lookupIP); err != nil {
		h.log.Error("invalid url", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: %s", apperror.ErrInvalidURL, err))
		return
	}

	created, err := h.urlService.Create(r.Context(), userID, req)
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
		IP:        clientIP(r),
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

	userID, ok := h.getUserIDOrError(w, r)
	if !ok {
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

// ListURLs handles GET /urls and returns a paginated list of active URLs.
func (h *URLHandler) ListURLs(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.getUserIDOrError(w, r)
	if !ok {
		return
	}

	page, perPage, offset := utils.ParsePagination(
		r.URL.Query().Get("page"),
		r.URL.Query().Get("perPage"),
	)

	list, err := h.urlService.List(r.Context(), userID, page, perPage, offset)
	if err != nil {
		h.log.Error("list urls failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	items := make([]any, len(list.Items))
	for i, item := range list.Items {
		items[i] = item
	}

	response.Success(w, http.StatusOK, "urls listed", items, &payload.Pagination{
		Total:      list.Total,
		Page:       list.Page,
		PerPage:    list.PerPage,
		TotalPages: list.TotalPages,
	})
}

// UpdateURL handles PATCH /urls/{id} and updates the URL details.
func (h *URLHandler) UpdateURL(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.getUserIDOrError(w, r)
	if !ok {
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

	userID, ok := h.getUserIDOrError(w, r)
	if !ok {
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

	userID, ok := h.getUserIDOrError(w, r)
	if !ok {
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
	userID, ok := h.getUserIDOrError(w, r)
	if !ok {
		return
	}

	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		h.log.Error("invalid url id", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: the provided URL id is not valid", apperror.ErrInvalidPayload))
		return
	}

	from, to := parseTimeRange(r)
	page, perPage, offset := utils.ParsePagination(r.URL.Query().Get("page"), r.URL.Query().Get("perPage"))

	clicks, err := h.urlService.ListClickLogs(r.Context(), userID, id, from, to, page, perPage, offset)
	if err != nil {
		h.log.Error("failed to list click logs", logger.Error(err), logger.Int64("id", id))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "click logs retrieved", []any{clicks})
}

// GetAnalytics handles GET /urls/{id}/analytics and returns aggregate click
// analytics for a specific URL, optionally filtered by from/to query parameters.
func (h *URLHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.getUserIDOrError(w, r)
	if !ok {
		return
	}

	id, err := utils.ParseID(r.PathValue("id"))
	if err != nil {
		h.log.Error("invalid url id", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: the provided URL id is not valid", apperror.ErrInvalidPayload))
		return
	}

	from, to := parseTimeRange(r)

	analytics, err := h.urlService.GetAnalytics(r.Context(), userID, id, from, to)
	if err != nil {
		h.log.Error("failed to get analytics", logger.Error(err), logger.Int64("id", id))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "analytics retrieved", []any{analytics})
}

// parseTimeRange extracts optional "from" and "to" RFC3339 query parameters.
func parseTimeRange(r *http.Request) (from, to *time.Time) {
	if s := r.URL.Query().Get("from"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			from = &t
		}
	}
	if s := r.URL.Query().Get("to"); s != "" {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			to = &t
		}
	}
	return
}
