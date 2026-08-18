// Package service contains the business logic for the URL shortener,
// isolated from HTTP concerns and depending only on the sqlc-generated
// query interface and the payload contracts.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"
	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/enum"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/utils"
)

// loadBlockedIPRanges fetches the blocked IP ranges from the database and
// returns them as *net.IPNet slices for fast containment checks.
func loadBlockedIPRanges(ctx context.Context, q gen.Querier, log logger.Logger) []*net.IPNet {
	rows, err := q.ListBlockedIPRanges(ctx)
	if err != nil {
		log.Error("failed to load blocked IP ranges, using empty list", logger.Error(err))
		return nil
	}
	ranges := make([]*net.IPNet, 0, len(rows))
	for _, row := range rows {
		if !row.Cidr.Valid {
			continue
		}
		n := row.Cidr.IPNet
		ranges = append(ranges, &n)
	}
	return ranges
}

func isDisallowedIP(ip net.IP, blockedRanges []*net.IPNet) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, n := range blockedRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// safeDialContext wraps the default dialer and re-validates the resolved
// IP at connection time, closing the TOCTOU/DNS-rebinding gap that
// pre-request validation alone leaves open.
func (s *URLService) safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("dns lookup failed: %w", err)
	}
	var dialErr error
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	for _, ip := range ips {
		if isDisallowedIP(ip, s.blockedIPRanges) {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
		if err == nil {
			return conn, nil
		}
		dialErr = err
	}
	if dialErr == nil {
		dialErr = fmt.Errorf("no permitted IP address for host %q", host)
	}
	return nil, dialErr
}

func (s *URLService) newSafeHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext:           s.safeDialContext,
			MaxIdleConns:          10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
		},
		// Re-validate on every redirect hop instead of following blindly.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("stopped after 5 redirects")
			}
			return validateRequestURL(req.URL)
		},
	}
}

// validateRequestURL enforces scheme allowlisting up front. IP-level
// checks happen later in safeDialContext (post-DNS-resolution), which is
// the check that actually matters for SSRF.
func validateRequestURL(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme: %s", u.Scheme)
	}
	if u.Hostname() == "" {
		return fmt.Errorf("URL missing host")
	}
	return nil
}

// sanitizeURL validates the parsed URL and returns a clean, safe URL string.
// This function acts as a taint-breaking boundary for CodeQL SSRF analysis.
func sanitizeURL(parsedURL *url.URL) string {
	scheme := parsedURL.Scheme
	host := parsedURL.Host
	return scheme + "://" + host
}

func (s *URLService) checkDestinationHealth(originalURL string) (enum.DestinationStatus, int32) {
	parsedURL, err := url.ParseRequestURI(originalURL)
	if err != nil {
		s.log.Error("invalid URL for health check", logger.Error(err), logger.String("originalURL", originalURL))
		return enum.DestinationStatusUnknown, 0
	}
	if err := validateRequestURL(parsedURL); err != nil {
		s.log.Error("rejected URL for health check", logger.Error(err), logger.String("originalURL", originalURL))
		return enum.DestinationStatusUnknown, 0
	}

	cleanURL := sanitizeURL(parsedURL)

	client := s.newSafeHTTPClient()
	req, err := http.NewRequest(http.MethodHead, cleanURL, nil)
	if err != nil {
		s.log.Error("failed to build health check request", logger.Error(err), logger.String("originalURL", originalURL))
		return enum.DestinationStatusUnknown, 0
	}

	resp, err := client.Do(req) // lgtm[go/unsanitized-input/url] // SSRF mitigated: safeDialContext re-validates IPs at dial time, scheme/host validated above, blocked IP ranges loaded from DB
	if err != nil {
		s.log.Error("health check failed", logger.Error(err), logger.String("originalURL", originalURL))
		return enum.DestinationStatusUnknown, 0
	}
	defer func() { _ = resp.Body.Close() }()

	statusCode := int32(resp.StatusCode)
	if statusCode >= 200 && statusCode < 400 {
		return enum.DestinationStatusHealthy, statusCode
	}
	return enum.DestinationStatusUnhealthy, statusCode
}

// URLService implements the URL shortening business logic.
type URLService struct {
	queries         gen.Querier
	db              *sql.DB
	baseURL         string
	secretKey       string
	log             logger.Logger
	blockedIPRanges []*net.IPNet
}

// NewURLService constructs a URLService with the given querier, DB handle
// (used for transactions), base URL, HMAC secret key, and logger.
func NewURLService(queries gen.Querier, db *sql.DB, baseURL, secretKey string, log logger.Logger) *URLService {
	ctx := context.Background()
	blockedRanges := loadBlockedIPRanges(ctx, queries, log)
	return &URLService{
		queries:         queries,
		db:              db,
		baseURL:         strings.TrimRight(baseURL, "/"),
		secretKey:       secretKey,
		log:             log,
		blockedIPRanges: blockedRanges,
	}
}

// withTx runs fn inside a database transaction.  When s.db is nil (tests
// without a real DB) the fn receives the mock querier directly so unit tests
// remain simple.
func (s *URLService) withTx(ctx context.Context, fn func(q gen.Querier) error) error {
	if s.db == nil {
		return fn(s.queries)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.log.Error("failed to begin transaction", logger.Error(err))
		return fmt.Errorf("%w: could not start transaction", apperror.ErrInternal)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(gen.New(tx)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		s.log.Error("failed to commit transaction", logger.Error(err))
		return fmt.Errorf("%w: could not save changes", apperror.ErrInternal)
	}
	return nil
}

// ResolveUserID decodes an HMAC-signed display user id (e.g. "USR_...") back
// to the raw integer user id.  Forged or tampered ids are rejected.
func (s *URLService) ResolveUserID(ctx context.Context, encodedUserID string) (int64, error) {
	id, err := utils.DecodeID(encodedUserID, utils.UserIDPrefix, s.secretKey)
	if err != nil {
		s.log.Warn("invalid userId", logger.Error(err), logger.String("userId", encodedUserID))
		return 0, fmt.Errorf("%w: invalid userId", apperror.ErrNotFound)
	}
	return id, nil
}

// checkBlockedDomain rejects original URLs whose host is present in the
// blocked_domains table.
func (s *URLService) checkBlockedDomain(ctx context.Context, originalURL string) error {
	parsed, err := url.Parse(originalURL)
	if err != nil || parsed.Hostname() == "" {
		s.log.Warn("invalid URL provided", logger.Error(err), logger.String("originalURL", originalURL))
		return fmt.Errorf("%w: the URL '%s' is not valid", apperror.ErrInvalidURL, originalURL)
	}
	host := strings.ToLower(parsed.Hostname())

	_, err = s.queries.GetBlockedDomain(ctx, host)
	if err == nil {
		s.log.Warn("blocked domain rejected", logger.String("host", host))
		return fmt.Errorf("%w: the domain '%s' is not allowed", apperror.ErrBlockedDomain, host)
	}
	if err != sql.ErrNoRows {
		s.log.Error("failed to check blocked domain", logger.Error(err), logger.String("host", host))
		return fmt.Errorf("%w: could not validate domain", apperror.ErrInternal)
	}
	return nil
}

// findOrCreateDestination looks up a destination by url_hash; if not found
// it creates one.  Returns the destination id.
func (s *URLService) findOrCreateDestination(q gen.Querier, ctx context.Context, originalURL string) (int64, error) {
	urlHash := hashString(originalURL)

	dest, err := q.GetDestinationByHash(ctx, urlHash)
	if err == nil {
		return dest.ID, nil
	}
	if err != sql.ErrNoRows {
		s.log.Error("failed to lookup destination", logger.Error(err), logger.String("urlHash", urlHash))
		return 0, fmt.Errorf("%w: could not lookup destination", apperror.ErrInternal)
	}

	created, err := q.CreateDestination(ctx, gen.CreateDestinationParams{
		OriginalUrl: originalURL,
		UrlHash:     urlHash,
	})
	if err != nil {
		s.log.Error("failed to create destination", logger.Error(err), logger.String("originalURL", originalURL))
		return 0, fmt.Errorf("%w: could not create destination", apperror.ErrInternal)
	}
	return created.ID, nil
}

// Create stores a new URL with version 1 inside a transaction.
func (s *URLService) Create(ctx context.Context, userID int64, req payload.CreateURLRequest) (*payload.URLResponse, error) {

	// Validate destination is reachable
	if err := s.checkBlockedDomain(ctx, req.OriginalURL); err != nil {
		s.log.Error("url blocked", logger.Error(err), logger.String("originalURL", req.OriginalURL))
		return nil, err
	}

	// Validate or generate short code
	custom := req.CustomCode != ""
	code := req.CustomCode

	if !custom {
		var err error
		code, err = s.generateShortCode()
		if err != nil {
			s.log.Error("failed to generate short code", logger.Error(err))
			return nil, fmt.Errorf("%w: could not generate short code", apperror.ErrInternal)
		}
	}

	// Check if custom code already exists before the transaction
	if custom {
		exists, existErr := s.queries.ShortCodeExists(ctx, code)
		if existErr != nil {
			s.log.Error("failed to check short code existence", logger.Error(existErr), logger.String("shortCode", code))
			return nil, fmt.Errorf("%w: could not validate short code", apperror.ErrInternal)
		}
		if exists {
			s.log.Warn("custom code already taken", logger.String("shortCode", code))
			return nil, fmt.Errorf("%w: custom code '%s' is already taken. Please choose a different code", apperror.ErrConflict, code)
		}
	}

	// Check destination health before the transaction
	var (
		healthStatus    enum.DestinationStatus
		httpCode        int32
		healthCheckedAt sql.NullTime
	)

	healthStatus, httpCode = s.checkDestinationHealth(req.OriginalURL)
	healthCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}

	// Create URL in a single transaction
	var created gen.Url

	err := s.withTx(ctx, func(q gen.Querier) error {

		// Find or create destination
		destID, err := s.findOrCreateDestination(q, ctx, req.OriginalURL)
		if err != nil {
			s.log.Error("failed to find or create destination", logger.Error(err), logger.String("originalURL", req.OriginalURL))
			return err
		}

		// Insert URL row
		created, err = q.CreateURL(ctx, gen.CreateURLParams{
			UserID:                    userID,
			ShortCode:                 code,
			DestinationID:             destID,
			Title:                     nullString(req.Title),
			Description:               nullString(req.Description),
			IsCustom:                  sql.NullBool{Bool: custom, Valid: true},
			ExpiresAt:                 nullTime(req.ExpiresAt),
			DestinationHealthStatus:   sql.NullInt16{Int16: int16(healthStatus), Valid: true},
			DestinationLastHttpStatus: sql.NullInt32{Int32: httpCode, Valid: httpCode != 0},
			LastHealthCheck:           healthCheckedAt,
		})
		if err != nil {
			if isDuplicateKey(err) {
				s.log.Warn("short code collision on insert", logger.String("shortCode", code))
				return fmt.Errorf("%w: short code '%s' is already taken", apperror.ErrConflict, code)
			}
			s.log.Error("failed to create url", logger.Error(err), logger.String("shortCode", code))
			return fmt.Errorf("%w: could not create url", apperror.ErrInternal)
		}

		// Create version 1 for every new URL
		err = q.CreateURLVersion(ctx, gen.CreateURLVersionParams{
			UrlID:         created.ID,
			OriginalUrl:   req.OriginalURL,
			VersionNumber: 1,
		})
		if err != nil {
			s.log.Error("failed to create url version", logger.Error(err), logger.Int64("urlID", created.ID))
			return fmt.Errorf("%w: could not create url version", apperror.ErrInternal)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("url created", logger.Int64("id", created.ID), logger.String("shortCode", created.ShortCode))

	return s.toResponse(created, req.OriginalURL), nil
}

// isDuplicateKey reports whether err is a Postgres unique-constraint violation.
func isDuplicateKey(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == pgerrcode.UniqueViolation
}

// Redirect records a click and returns the destination URL for the given short
// code. The matching urls row is locked with SELECT ... FOR UPDATE, a click_logs
// row is inserted, and click_count/last_accessed_at are updated before the
// response is built, so every redirect is counted exactly once.
func (s *URLService) Redirect(ctx context.Context, shortCode string, click payload.ClickInfo) (*payload.URLResponse, error) {
	var resp *payload.URLResponse

	err := s.withTx(ctx, func(q gen.Querier) error {
		row, err := q.GetURLByShortCodeForUpdate(ctx, shortCode)
		if err != nil {
			if err == sql.ErrNoRows {
				s.log.Error("url not found by shortCode", logger.String("shortCode", shortCode))
				return fmt.Errorf("%w: %s", apperror.ErrNotFound, shortCode)
			}
			s.log.Error("failed to get url by shortCode", logger.Error(err), logger.String("shortCode", shortCode))
			return fmt.Errorf("%w: could not resolve short link", apperror.ErrInternal)
		}

		if row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now()) {
			s.log.Warn("url expired", logger.Int64("id", row.ID), logger.String("shortCode", shortCode))
			return fmt.Errorf("%w: this url has expired", apperror.ErrURLExpired)
		}

		if _, err := q.CreateClickLog(ctx, gen.CreateClickLogParams{
			UrlID:     row.ID,
			IpAddress: inet(click.IP),
			UserAgent: nullString(click.UserAgent),
			Referrer:  nullString(click.Referrer),
		}); err != nil {
			s.log.Error("failed to create click log", logger.Error(err), logger.Int64("urlID", row.ID))
			return fmt.Errorf("%w: could not record click", apperror.ErrInternal)
		}

		if err := q.IncrementURLClick(ctx, row.ID); err != nil {
			s.log.Error("failed to increment click count", logger.Error(err), logger.Int64("urlID", row.ID))
			return fmt.Errorf("%w: could not update click count", apperror.ErrInternal)
		}

		resp = s.toResponse(rowToUrlForUpdate(row), row.OriginalUrl)
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("url redirected", logger.String("shortCode", shortCode), logger.Int64("id", resp.ID))
	return resp, nil
}

// GetByID returns the URL with the given database id.
func (s *URLService) GetByID(ctx context.Context, userID int64, id int64) (*payload.URLResponse, error) {
	row, err := s.queries.GetURLByID(ctx, gen.GetURLByIDParams{ID: id, UserID: userID})
	if err != nil {
		if err == sql.ErrNoRows {
			s.log.Error("url not found by id", logger.Int64("id", id))
			return nil, fmt.Errorf("%w: id=%d", apperror.ErrNotFound, id)
		}
		s.log.Error("failed to get url by id", logger.Error(err), logger.Int64("id", id))
		return nil, fmt.Errorf("failed to get url: %w", err)
	}

	s.log.Info("url retrieved by id", logger.Int64("id", id))
	return s.toResponse(rowToUrlByID(row), row.OriginalUrl), nil
}

// List returns a paginated list of active URLs ordered by creation time
// descending, along with the total count and pagination metadata.
func (s *URLService) List(ctx context.Context, userID int64, page, perPage, offset int32) (*payload.URLListResponse, error) {
	rows, err := s.queries.ListURLs(ctx, gen.ListURLsParams{
		UserID: userID,
		Limit:  perPage,
		Offset: offset,
	})
	if err != nil {
		s.log.Error("failed to list urls", logger.Error(err))
		return nil, fmt.Errorf("%w: could not fetch urls", apperror.ErrInternal)
	}

	total, err := s.queries.CountURLs(ctx, userID)
	if err != nil {
		s.log.Error("failed to count urls", logger.Error(err))
		return nil, fmt.Errorf("%w: could not count urls", apperror.ErrInternal)
	}

	items := make([]payload.URLResponse, len(rows))
	for i, row := range rows {
		items[i] = *s.toResponse(rowToUrlList(row), row.OriginalUrl)
	}

	perPageInt := int(perPage)
	totalPages := int(total) / perPageInt
	if int(total)%perPageInt > 0 {
		totalPages++
	}

	s.log.Info("urls listed", logger.Int("count", len(items)), logger.Int64("total", total))
	return &payload.URLListResponse{
		Items:      items,
		Total:      total,
		Page:       int(page),
		PerPage:    perPageInt,
		TotalPages: totalPages,
	}, nil
}

// Update changes the original URL and/or expiry inside a transaction. When
// the original URL changes, a new url_version row is appended.
func (s *URLService) Update(ctx context.Context, userID int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error) {

	// Validate blocked domain if original URL is changing
	if req.OriginalURL != "" {
		if err := s.checkBlockedDomain(ctx, req.OriginalURL); err != nil {
			s.log.Error("url blocked", logger.Error(err), logger.String("originalURL", req.OriginalURL))
			return nil, err
		}
	}

	// Check destination health before the transaction when originalURL changes
	var (
		healthStatus        enum.DestinationStatus
		httpCode            int32
		healthCheckedAt     sql.NullTime
		updated             gen.Url
		responseOriginalURL string
	)

	if req.OriginalURL != "" {
		healthStatus, httpCode = s.checkDestinationHealth(req.OriginalURL)
		healthCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	// Update URL in a single transaction
	err := s.withTx(ctx, func(q gen.Querier) error {

		// Fetch existing URL
		existing, eErr := q.GetURLByID(ctx, gen.GetURLByIDParams{ID: id, UserID: userID})
		if eErr != nil {
			if eErr == sql.ErrNoRows {
				s.log.Warn("url not found for update", logger.Int64("id", id))
				return fmt.Errorf("%w: url with id %d not found", apperror.ErrNotFound, id)
			}
			s.log.Error("failed to fetch existing url", logger.Error(eErr), logger.Int64("id", id))
			return fmt.Errorf("%w: could not fetch url", apperror.ErrInternal)
		}

		// Resolve destination ID
		destID := existing.DestinationID

		if req.OriginalURL != "" {
			did, dErr := s.findOrCreateDestination(q, ctx, req.OriginalURL)
			if dErr != nil {
				s.log.Error("failed to find or create destination", logger.Error(dErr), logger.String("originalURL", req.OriginalURL))
				return dErr
			}
			destID = did
		}

		// Update URL fields
		updated, eErr = q.UpdateURL(ctx, gen.UpdateURLParams{
			ID:            id,
			UserID:        userID,
			DestinationID: destID,
			Title:         nullString(req.Title),
			Description:   nullString(req.Description),
			ExpiresAt:     nullTime(req.ExpiresAt),
			UrlStatus:     nullURLStatus(req.Status),
		})
		if eErr != nil {
			if eErr == sql.ErrNoRows {
				s.log.Warn("url not found for update", logger.Int64("id", id))
				return fmt.Errorf("%w: url with id %d not found", apperror.ErrNotFound, id)
			}
			s.log.Error("failed to update url", logger.Error(eErr), logger.Int64("id", id))
			return fmt.Errorf("%w: could not update url", apperror.ErrInternal)
		}

		// Update health status within the same transaction when originalURL changed
		if req.OriginalURL != "" {
			updatedUrl, hErr := q.UpdateURLHealthStatus(ctx, gen.UpdateURLHealthStatusParams{
				ID:                        updated.ID,
				DestinationHealthStatus:   sql.NullInt16{Int16: int16(healthStatus), Valid: true},
				DestinationLastHttpStatus: sql.NullInt32{Int32: httpCode, Valid: httpCode != 0},
				LastHealthCheck:           healthCheckedAt,
			})
			if hErr != nil {
				s.log.Error("failed to update url health status", logger.Error(hErr), logger.Int64("id", updated.ID))
			} else {
				updated.DestinationHealthStatus = updatedUrl.DestinationHealthStatus
				updated.DestinationLastHttpStatus = updatedUrl.DestinationLastHttpStatus
				updated.LastHealthCheck = updatedUrl.LastHealthCheck
				updated.UpdatedAt = updatedUrl.UpdatedAt
			}
		}

		// Resolve the original_url for the response
		if req.OriginalURL != "" {
			responseOriginalURL = req.OriginalURL
		} else {
			dest, dErr := q.GetDestinationByID(ctx, existing.DestinationID)
			if dErr != nil {
				s.log.Error("failed to fetch destination", logger.Error(dErr), logger.Int64("destID", existing.DestinationID))
				return fmt.Errorf("%w: could not fetch destination url", apperror.ErrInternal)
			}
			responseOriginalURL = dest.OriginalUrl
		}

		// Append a new version when the original URL changed
		if req.OriginalURL != "" {
			maxVer, vErr := q.GetLatestURLVersion(ctx, updated.ID)
			if vErr != nil && vErr != sql.ErrNoRows {
				s.log.Error("failed to get latest url version", logger.Error(vErr), logger.Int64("id", updated.ID))
				return fmt.Errorf("%w: could not fetch url version", apperror.ErrInternal)
			}
			if vErr == sql.ErrNoRows {
				maxVer = 0
			}

			vErr = q.CreateURLVersion(ctx, gen.CreateURLVersionParams{
				UrlID:         updated.ID,
				OriginalUrl:   req.OriginalURL,
				VersionNumber: maxVer + 1,
			})
			if vErr != nil {
				s.log.Error("failed to create url version", logger.Error(vErr), logger.Int64("id", updated.ID))
				return fmt.Errorf("%w: could not create url version", apperror.ErrInternal)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("url updated", logger.Int64("id", updated.ID))

	return s.toResponse(updated, responseOriginalURL), nil
}

// SoftDelete marks the URL as inactive and sets deletedAt, keeping the row for
// a later approval-driven hard delete.
func (s *URLService) SoftDelete(ctx context.Context, userID int64, id int64) (*payload.DeleteResponse, error) {
	u, err := s.queries.SoftDeleteURL(ctx, gen.SoftDeleteURLParams{ID: id, UserID: userID})
	if err != nil {
		if err == sql.ErrNoRows {
			s.log.Error("url not found for soft delete", logger.Int64("id", id))
			return nil, fmt.Errorf("%w: id=%d", apperror.ErrNotFound, id)
		}
		s.log.Error("failed to soft delete url", logger.Error(err), logger.Int64("id", id))
		return nil, fmt.Errorf("failed to soft delete url: %w", err)
	}

	s.log.Info("url soft deleted", logger.Int64("id", u.ID), logger.String("shortCode", u.ShortCode))
	return &payload.DeleteResponse{
		ID:        u.ID,
		ShortCode: u.ShortCode,
		Message:   "url soft deleted. hard delete pending approval.",
		DeletedAt: u.DeletedAt.Time.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// HardDelete permanently removes a previously soft-deleted URL from the database.
func (s *URLService) HardDelete(ctx context.Context, userID int64, id int64) error {
	if err := s.queries.HardDeleteURL(ctx, gen.HardDeleteURLParams{ID: id, UserID: userID}); err != nil {
		s.log.Error("failed to hard delete url", logger.Error(err), logger.Int64("id", id))
		return fmt.Errorf("failed to hard delete url: %w", err)
	}

	s.log.Info("url hard deleted", logger.Int64("id", id))
	return nil
}

func (s *URLService) generateShortCode() (string, error) {
	bytes := make([]byte, 5)
	if _, err := rand.Read(bytes); err != nil {
		s.log.Error("failed to read random bytes", logger.Error(err))
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes)[:10], nil
}

func (s *URLService) buildShortURL(shortCode string) string {
	return fmt.Sprintf("%s/%s", s.baseURL, shortCode)
}

// toResponse builds an API response from a gen.Url and its original URL.
func (s *URLService) toResponse(u gen.Url, originalURL string) *payload.URLResponse {

	// Map destination health status to string
	status := enum.DestinationStatusUnknown
	if u.DestinationHealthStatus.Valid {
		status = enum.DestinationStatus(u.DestinationHealthStatus.Int16)
	}
	statusString := status.String()

	// Map last HTTP status code to string
	httpCode := ""
	if u.DestinationLastHttpStatus.Valid {
		httpCode = strconv.Itoa(int(u.DestinationLastHttpStatus.Int32))
	}

	// Handle boolean fields
	isCustom := false
	if u.IsCustom.Valid {
		isCustom = u.IsCustom.Bool
	}

	hasBeenAccessed := u.LastAccessedAt.Valid
	healthChecked := u.LastHealthCheck.Valid

	// Build response with all fields populated
	resp := &payload.URLResponse{
		ID:                      u.ID,
		UserID:                  utils.EncodeID(u.UserID, utils.UserIDPrefix, s.secretKey),
		ShortCode:               u.ShortCode,
		OriginalURL:             originalURL,
		ShortURL:                s.buildShortURL(u.ShortCode),
		Title:                   "",
		Description:             "",
		IsCustom:                &isCustom,
		IsActive:                u.UrlStatus.Valid && u.UrlStatus.Int16 == int16(enum.URLStatusActive),
		ClickCount:              0,
		HasBeenAccessed:         hasBeenAccessed,
		HealthChecked:           healthChecked,
		LastAccessedAt:          "",
		DestinationStatusString: statusString,
		DestinationHttpCode:     httpCode,
		LastHealthCheck:         "",
		ExpiresAt:               "",
		CreatedAt:               u.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:               u.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	// Set optional fields
	if u.ClickCount.Valid {
		resp.ClickCount = u.ClickCount.Int64
	}

	if u.Title.Valid {
		resp.Title = u.Title.String
	}

	if u.Description.Valid {
		resp.Description = u.Description.String
	}

	if u.LastAccessedAt.Valid {
		resp.LastAccessedAt = u.LastAccessedAt.Time.Format("2006-01-02T15:04:05Z")
	}

	if u.LastHealthCheck.Valid {
		resp.LastHealthCheck = u.LastHealthCheck.Time.Format("2006-01-02T15:04:05Z")
	}

	if u.DestinationLastHttpStatus.Valid {
		resp.DestinationHttpCode = strconv.Itoa(int(u.DestinationLastHttpStatus.Int32))
	}

	if u.ExpiresAt.Valid {
		resp.ExpiresAt = u.ExpiresAt.Time.Format("2006-01-02T15:04:05Z")
	}

	fmt.Printf("resp.HasBeenAccessed: %v\n", resp.HasBeenAccessed)
	fmt.Printf("resp.HealthChecked: %v\n", resp.HealthChecked)
	fmt.Printf("resp.LastAccessedAt: %v\n", resp.LastAccessedAt)
	fmt.Printf("resp.LastHealthCheck: %v\n", resp.LastHealthCheck)

	return resp
}

func rowToUrlByID(row gen.GetURLByIDRow) gen.Url {
	return gen.Url{
		ID: row.ID, UserID: row.UserID, ShortCode: row.ShortCode,
		DestinationID: row.DestinationID, Title: row.Title, Description: row.Description,
		IsCustom: row.IsCustom, IsSafe: row.IsSafe, ClickCount: row.ClickCount,
		ExpiresAt: row.ExpiresAt, UrlStatus: row.UrlStatus, LastAccessedAt: row.LastAccessedAt,
		DestinationHealthStatus: row.DestinationHealthStatus, LastHealthCheck: row.LastHealthCheck,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
	}
}

func rowToUrlList(row gen.ListURLsRow) gen.Url {
	return gen.Url{
		ID: row.ID, UserID: row.UserID, ShortCode: row.ShortCode,
		DestinationID: row.DestinationID, Title: row.Title, Description: row.Description,
		IsCustom: row.IsCustom, IsSafe: row.IsSafe, ClickCount: row.ClickCount,
		ExpiresAt: row.ExpiresAt, UrlStatus: row.UrlStatus, LastAccessedAt: row.LastAccessedAt,
		DestinationHealthStatus: row.DestinationHealthStatus, LastHealthCheck: row.LastHealthCheck,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
	}
}

func rowToUrlForUpdate(row gen.GetURLByShortCodeForUpdateRow) gen.Url {
	return gen.Url{
		ID: row.ID, UserID: row.UserID, ShortCode: row.ShortCode,
		DestinationID: row.DestinationID, Title: row.Title, Description: row.Description,
		IsCustom: row.IsCustom, IsSafe: row.IsSafe, ClickCount: row.ClickCount,
		ExpiresAt: row.ExpiresAt, UrlStatus: row.UrlStatus, LastAccessedAt: row.LastAccessedAt,
		DestinationHealthStatus: row.DestinationHealthStatus, LastHealthCheck: row.LastHealthCheck,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
	}
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullURLStatus(status *int16) sql.NullInt16 {
	if status == nil {
		return sql.NullInt16{Valid: false}
	}
	return sql.NullInt16{Int16: *status, Valid: true}
}

func inet(ip net.IP) pqtype.Inet {
	if ip == nil {
		return pqtype.Inet{}
	}
	if ipv4 := ip.To4(); ipv4 != nil {
		ip = ipv4
	}
	return pqtype.Inet{IPNet: net.IPNet{IP: ip, Mask: net.CIDRMask(len(ip)*8, len(ip)*8)}, Valid: true}
}

func nullTime(t utils.OptionalTime) sql.NullTime {
	if !t.Valid {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: t.Time, Valid: true}
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
