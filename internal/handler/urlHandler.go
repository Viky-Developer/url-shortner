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
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/utils"
)

// lookupIP resolves a host to its IP addresses. It is a variable so tests can
// stub DNS resolution.
var lookupIP = net.LookupIP

// URLService is the contract the handlers depend on for URL business logic.
// It is satisfied by *service.URLService and can be mocked in tests.
type URLService interface {
	ResolveUserID(ctx context.Context, encodedUserID string) (int64, error)
	Create(ctx context.Context, userID int64, req payload.CreateURLRequest) (*payload.URLResponse, error)
	Redirect(ctx context.Context, shortCode string, click payload.ClickInfo) (*payload.URLResponse, error)
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
// validateURL checks that the raw URL is a well-formed http(s) URL and does
// not point at a localhost, internal, private, or loopback destination.  DNS
// resolution is performed so a host resolving to a private IP is rejected.
func validateURL(rawURL string) error {

	if rawURL == "" {
		return fmt.Errorf("%w: originalURL is required", apperror.ErrInvalidURL)
	}
	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil || parsed.Scheme != "https" {
		return fmt.Errorf("%w: must start with https://", apperror.ErrInvalidURL)
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return fmt.Errorf("%w: invalid host", apperror.ErrInvalidURL)
	}

	if host == "localhost" {
		return fmt.Errorf("%w: localhost is not allowed", apperror.ErrInvalidURL)
	}

	// Internal TLD suffixes are never allowed.
	blockedSuffixes := []string{".local", ".internal", ".lan", ".corp", ".home"}
	for _, suffix := range blockedSuffixes {
		if strings.HasSuffix(host, suffix) {
			return fmt.Errorf("%w: internal domain not allowed", apperror.ErrInvalidURL)
		}
	}

	// If the host is an IP literal, validate it directly.
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("%w: private or loopback IP not allowed", apperror.ErrInvalidURL)
		}
		return nil
	}

	// Otherwise resolve the host and reject any private/loopback addresses.
	ips, err := lookupIP(host)
	if err != nil {
		logger.Error(err)
		return fmt.Errorf("%w: unable to resolve host", apperror.ErrInvalidURL)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("%w: host resolves to private or loopback IP", apperror.ErrInvalidURL)
		}
	}

	return nil
}

// validateExpiresAt ensures that when an expiration time is provided it is not
// in the past (i.e. it must be the current moment or a future time).
func validateExpiresAt(e utils.OptionalTime) error {
	if !e.Valid {
		return nil
	}
	if e.Time.Before(time.Now()) {
		return fmt.Errorf("%w: expiresAt must not be in the past", apperror.ErrInvalidPayload)
	}
	return nil
}

// isBlockedIP reports whether ip is loopback, private, link-local, multicast,
// or unspecified — any of which make an address unsafe as a redirect target.
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
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

	if req.OriginalURL == "" && req.CustomCode == "" && req.Title == "" &&
		req.Description == "" && !req.ExpiresAt.Valid {
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

	if err := validateExpiresAt(req.ExpiresAt); err != nil {
		h.log.Error("invalid expiresAt", logger.Error(err))
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
		if errors.Is(err, apperror.ErrURLExpired) {
			h.log.Warn("redirect expired", logger.Error(err))
			writeError(w, http.StatusGone, err)
			return
		}
		h.log.Error("redirect failed", logger.Error(err))
		writeError(w, http.StatusNotFound, err)
		return
	}

	http.Redirect(w, r, u.OriginalURL, http.StatusFound)
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

	if req.OriginalURL != "" {
		if err := validateURL(req.OriginalURL); err != nil {
			h.log.Error("invalid url", logger.Error(err))
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}

	if err := validateExpiresAt(req.ExpiresAt); err != nil {
		h.log.Error("invalid expiresAt", logger.Error(err))
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
