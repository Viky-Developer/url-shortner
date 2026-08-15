// Package handler contains the HTTP handlers for the URL shortener API.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/payload"
)

// URLService is the contract the handlers depend on for URL business logic.
// It is satisfied by *service.URLService and can be mocked in tests.
type URLService interface {
	ResolveUserID(ctx context.Context, encodedUserID string) (int64, error)
	Create(ctx context.Context, userID int64, req payload.CreateURLRequest) (*payload.URLResponse, error)
	GetByShortCode(ctx context.Context, userID int64, shortCode string) (*payload.URLResponse, error)
	GetByID(ctx context.Context, userID int64, id int64) (*payload.URLResponse, error)
	List(ctx context.Context, userID int64, page, perPage, offset int32) (*payload.URLListResponse, error)
	Update(ctx context.Context, userID int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error)
	SoftDelete(ctx context.Context, userID int64, id int64) (*payload.DeleteResponse, error)
	HardDelete(ctx context.Context, userID int64, id int64) error
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

// writeJSON serialises v as JSON and writes it to the response with the given
// HTTP status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("failed to encode response: %v", err)
	}
}

// writeSuccess writes a unified success envelope containing the status code,
// a message, optional data array, and optional pagination metadata.
func writeSuccess(w http.ResponseWriter, status int, message string, data []any, pagination ...*payload.Pagination) {
	resp := payload.SuccessResponse{
		StatusCode: status,
		Message:    message,
		Data:       data,
	}
	if len(pagination) > 0 {
		resp.Pagination = pagination[0]
	}
	writeJSON(w, status, resp)
}

// writeError writes a unified error envelope containing the status code and
// the error message.
func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, payload.ErrorResponse{
		StatusCode: status,
		Message:    err.Error(),
	})
}

// validateURL ensures the given URL is present and uses http or https.
func validateURL(rawURL string) error {

	if rawURL == "" {
		return fmt.Errorf("%w: originalURL is required", apperror.ErrInvalidURL)
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("%w: must start with http:// or https://", apperror.ErrInvalidURL)
	}
	return nil
}

// parseID extracts the integer id from the {id} path segment.
func parseID(r *http.Request) (int64, error) {

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, apperror.ErrInvalidURL
	}
	return id, nil
}

// resolveUserID decodes the HMAC-signed {userId} path segment into the
// internal integer user id via the service, so callers can scope URL queries.
func (h *URLHandler) resolveUserID(r *http.Request) (int64, error) {
	userID := r.PathValue("userId")
	if userID == "" {
		return 0, fmt.Errorf("%w: userId is required", apperror.ErrInvalidPayload)
	}
	return h.urlService.ResolveUserID(r.Context(), userID)
}

// resolveUserIDOrError is resolveUserID plus HTTP error mapping: unknown users
// yield 404 and malformed requests yield 400. It reports false when the
// response has already been written.
func (h *URLHandler) resolveUserIDOrError(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, err := h.resolveUserID(r)
	if err != nil {
		if errors.Is(err, apperror.ErrNotFound) {
			h.log.Warn("displayUserId not found", logger.Error(err))
			writeError(w, http.StatusNotFound, err)
		} else {
			h.log.Warn("invalid displayUserId", logger.Error(err))
			writeError(w, http.StatusBadRequest, err)
		}
		return 0, false
	}
	return userID, true
}

// parsePagination reads and clamps the page and perPage query parameters,
// defaulting to page 1 and 10 per page (maximum 100). The offset is computed
// in int64 and clamped to int32 so the LIMIT/OFFSET pair cannot overflow.
func parsePagination(r *http.Request) (page, perPage, offset int32) {

	page = max(parsePositiveInt(r.URL.Query().Get("page"), 1), 1)
	perPage = min(max(parsePositiveInt(r.URL.Query().Get("perPage"), 10), 1), 100)

	o := int64(page-1) * int64(perPage)
	offset = int32(min(max(o, 0), math.MaxInt32))

	return page, perPage, offset
}

// parsePositiveInt parses value as a positive int32, falling back when the
// value is empty or invalid.
func parsePositiveInt(value string, fallback int32) int32 {

	if value == "" {
		return fallback
	}
	n, err := strconv.ParseInt(value, 10, 32)
	if err != nil || n < 1 {
		return fallback
	}
	return int32(n)
}

// decodeBody decodes the request body into v.
func decodeBody(r *http.Request, v any) error {

	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("%w: request body is required", apperror.ErrInvalidPayload)
		}
		return fmt.Errorf("%w: %v", apperror.ErrInvalidPayload, err)
	}
	return nil
}

// CreateShortURL handles POST /shorten and creates a new short URL.
func (h *URLHandler) CreateShortURL(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.resolveUserIDOrError(w, r)
	if !ok {
		return
	}

	var req payload.CreateURLRequest
	if err := decodeBody(r, &req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.OriginalURL == "" && req.CustomCode == "" && !req.ExpiresAt.Valid {
		err := fmt.Errorf("%w: request body is required", apperror.ErrInvalidPayload)
		h.log.Error("invalid payload", logger.Error(err))
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := validateURL(req.OriginalURL); err != nil {
		h.log.Error("invalid url", logger.Error(err))
		writeError(w, http.StatusBadRequest, err)
		return
	}

	created, err := h.urlService.Create(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, apperror.ErrConflict) {
			h.log.Warn("short code already taken", logger.Error(err))
			writeError(w, http.StatusConflict, err)
			return
		}
		h.log.Error("failed to create url", logger.Error(err))
		writeError(w, http.StatusBadRequest, err)
		return
	}

	h.log.Info("url created", logger.Int64("id", created.ID), logger.String("shortCode", created.ShortCode))

	writeSuccess(w, http.StatusCreated, "url created", []any{created})
}

// RedirectShortURL handles GET /{userId}/{shortCode} and redirects the client
// to the original URL.
func (h *URLHandler) RedirectShortURL(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.resolveUserIDOrError(w, r)
	if !ok {
		return
	}

	u, err := h.urlService.GetByShortCode(r.Context(), userID, r.PathValue("shortCode"))
	if err != nil {
		h.log.Error("redirect failed", logger.Error(err))
		writeError(w, http.StatusNotFound, err)
		return
	}

	// Return the destination URL in the response body so the client performs
	// the redirect itself; this avoids server-side redirects dropping any
	// client cookies or browser context.
	writeSuccess(w, http.StatusOK, "redirect url", []any{map[string]string{"redirectURL": u.OriginalURL}})
}

// GetURLByID handles GET /urls/{id} and returns the URL details.
func (h *URLHandler) GetURLByID(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.resolveUserIDOrError(w, r)
	if !ok {
		return
	}

	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	u, err := h.urlService.GetByID(r.Context(), userID, id)
	if err != nil {
		h.log.Error("get url failed", logger.Error(err))
		writeError(w, http.StatusNotFound, err)
		return
	}

	writeSuccess(w, http.StatusOK, "url retrieved", []any{u})
}

// ListURLs handles GET /urls and returns a paginated list of active URLs.
func (h *URLHandler) ListURLs(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.resolveUserIDOrError(w, r)
	if !ok {
		return
	}

	page, perPage, offset := parsePagination(r)

	list, err := h.urlService.List(r.Context(), userID, page, perPage, offset)
	if err != nil {
		h.log.Error("list urls failed", logger.Error(err))
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	items := make([]any, len(list.Items))
	for i, item := range list.Items {
		items[i] = item
	}

	writeSuccess(w, http.StatusOK, "urls listed", items, &payload.Pagination{
		Total:      list.Total,
		Page:       list.Page,
		PerPage:    list.PerPage,
		TotalPages: list.TotalPages,
	})
}

// UpdateURL handles PATCH /urls/{id} and updates the URL details.
func (h *URLHandler) UpdateURL(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.resolveUserIDOrError(w, r)
	if !ok {
		return
	}

	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	var req payload.UpdateURLRequest
	if err := decodeBody(r, &req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := validateURL(req.OriginalURL); err != nil {
		h.log.Error("invalid url", logger.Error(err))
		writeError(w, http.StatusBadRequest, err)
		return
	}

	updated, err := h.urlService.Update(r.Context(), userID, id, req)
	if err != nil {
		h.log.Error("update url failed", logger.Error(err))
		writeError(w, http.StatusBadRequest, err)
		return
	}

	h.log.Info("url updated", logger.Int64("id", updated.ID))

	writeSuccess(w, http.StatusOK, "url updated", []any{updated})
}

// DeleteURL handles DELETE /urls/{id} and soft deletes the URL, leaving a
// hard-delete pending approval.
func (h *URLHandler) DeleteURL(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.resolveUserIDOrError(w, r)
	if !ok {
		return
	}

	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	deleted, err := h.urlService.SoftDelete(r.Context(), userID, id)
	if err != nil {
		h.log.Error("soft delete failed", logger.Error(err))
		writeError(w, http.StatusNotFound, err)
		return
	}

	h.log.Info("url soft deleted", logger.Int64("id", deleted.ID), logger.String("shortCode", deleted.ShortCode))

	writeSuccess(w, http.StatusOK, "url soft deleted", []any{deleted})
}

// ApproveHardDelete handles DELETE /urls/{id}/approve and permanently removes
// a previously soft-deleted URL.
func (h *URLHandler) ApproveHardDelete(w http.ResponseWriter, r *http.Request) {

	userID, ok := h.resolveUserIDOrError(w, r)
	if !ok {
		return
	}

	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.urlService.HardDelete(r.Context(), userID, id); err != nil {
		h.log.Error("hard delete failed", logger.Error(err))
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	h.log.Info("url hard deleted", logger.Int64("id", id))

	writeSuccess(w, http.StatusOK, "url permanently deleted", []any{})
}
