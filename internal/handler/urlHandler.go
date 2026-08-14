// Package handler contains the HTTP handlers for the URL shortener API.
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	Create(ctx context.Context, req payload.CreateURLRequest) (*payload.URLResponse, error)
	GetByShortCode(ctx context.Context, shortCode string) (*payload.URLResponse, error)
	GetByID(ctx context.Context, id int64) (*payload.URLResponse, error)
	List(ctx context.Context, page, perPage int) (*payload.URLListResponse, error)
	Update(ctx context.Context, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error)
	SoftDelete(ctx context.Context, id int64) (*payload.DeleteResponse, error)
	HardDelete(ctx context.Context, id int64) error
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
// a message, and optional data.
func writeSuccess(w http.ResponseWriter, status int, message string, data any) {
	writeJSON(w, status, payload.SuccessResponse{
		StatusCode: status,
		Message:    message,
		Data:       data,
	})
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

// parsePagination reads and clamps the page and perPage query parameters,
// defaulting to page 1 and 10 per page (maximum 100).
func parsePagination(r *http.Request) (page, perPage int) {

	page = parsePositiveInt(r.URL.Query().Get("page"), 1)
	perPage = parsePositiveInt(r.URL.Query().Get("perPage"), 10)
	if perPage > 100 {
		perPage = 100
	}
	return page, perPage
}

// parsePositiveInt parses value as a positive integer, falling back when the
// value is empty or invalid.
func parsePositiveInt(value string, fallback int) int {

	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

// decodeBody decodes the request body into v.
func decodeBody(r *http.Request, v any) error {

	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

// CreateShortURL handles POST /shorten and creates a new short URL.
func (h *URLHandler) CreateShortURL(w http.ResponseWriter, r *http.Request) {

	var req payload.CreateURLRequest
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

	created, err := h.urlService.Create(r.Context(), req)
	if err != nil {
		h.log.Error("failed to create url", logger.Error(err))
		writeError(w, http.StatusBadRequest, err)
		return
	}

	h.log.Info("url created", logger.Int64("id", created.ID), logger.String("shortCode", created.ShortCode))

	writeSuccess(w, http.StatusCreated, "url created", created)
}

// RedirectShortURL handles GET /{shortCode} and redirects the client to the
// original URL.
func (h *URLHandler) RedirectShortURL(w http.ResponseWriter, r *http.Request) {

	u, err := h.urlService.GetByShortCode(r.Context(), r.PathValue("shortCode"))
	if err != nil {
		h.log.Error("redirect failed", logger.Error(err))
		writeError(w, http.StatusNotFound, err)
		return
	}

	http.Redirect(w, r, u.OriginalURL, http.StatusFound)
}

// GetURLByID handles GET /urls/{id} and returns the URL details.
func (h *URLHandler) GetURLByID(w http.ResponseWriter, r *http.Request) {

	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	u, err := h.urlService.GetByID(r.Context(), id)
	if err != nil {
		h.log.Error("get url failed", logger.Error(err))
		writeError(w, http.StatusNotFound, err)
		return
	}

	writeSuccess(w, http.StatusOK, "url retrieved", u)
}

// ListURLs handles GET /urls and returns a paginated list of active URLs.
func (h *URLHandler) ListURLs(w http.ResponseWriter, r *http.Request) {

	page, perPage := parsePagination(r)

	list, err := h.urlService.List(r.Context(), page, perPage)
	if err != nil {
		h.log.Error("list urls failed", logger.Error(err))
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeSuccess(w, http.StatusOK, "urls listed", list)
}

// UpdateURL handles PUT /urls/{id} and updates the URL details.
func (h *URLHandler) UpdateURL(w http.ResponseWriter, r *http.Request) {

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

	updated, err := h.urlService.Update(r.Context(), id, req)
	if err != nil {
		h.log.Error("update url failed", logger.Error(err))
		writeError(w, http.StatusBadRequest, err)
		return
	}

	h.log.Info("url updated", logger.Int64("id", updated.ID))

	writeSuccess(w, http.StatusOK, "url updated", updated)
}

// DeleteURL handles DELETE /urls/{id} and soft deletes the URL, leaving a
// hard-delete pending approval.
func (h *URLHandler) DeleteURL(w http.ResponseWriter, r *http.Request) {

	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	deleted, err := h.urlService.SoftDelete(r.Context(), id)
	if err != nil {
		h.log.Error("soft delete failed", logger.Error(err))
		writeError(w, http.StatusNotFound, err)
		return
	}

	h.log.Info("url soft deleted", logger.Int64("id", deleted.ID), logger.String("shortCode", deleted.ShortCode))

	writeSuccess(w, http.StatusOK, "url soft deleted", deleted)
}

// ApproveHardDelete handles DELETE /urls/{id}/approve and permanently removes
// a previously soft-deleted URL.
func (h *URLHandler) ApproveHardDelete(w http.ResponseWriter, r *http.Request) {

	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.urlService.HardDelete(r.Context(), id); err != nil {
		h.log.Error("hard delete failed", logger.Error(err))
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	h.log.Info("url hard deleted", logger.Int64("id", id))
	
	writeSuccess(w, http.StatusOK, "url permanently deleted", nil)
}
