// Package service contains the business logic for the URL shortener,
// isolated from HTTP concerns and depending only on the sqlc-generated
// query interface and the payload contracts.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
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

// hostHeaderTransport injects a custom Host header into outgoing requests
// while allowing the underlying transport to dial a different address (e.g. a
// resolved IP for SSRF protection). This keeps TLS certificate validation
// against the original hostname.
type hostHeaderTransport struct {
	base       *http.Transport
	customHost string
}

func (t *hostHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Host = t.customHost
	return t.base.RoundTrip(req)
}

func (s *URLService) newSafeHTTPClient(customHost string) *http.Client {
	transport := &http.Transport{
		DialContext:           s.safeDialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}

	// When dialing a resolved IP for SSRF safety, TLS must still validate
	// the certificate against the original hostname. Setting ServerName
	// tells the TLS layer which hostname to check, instead of using the
	// IP address from the URL.
	if customHost != "" {
		transport.TLSClientConfig = &tls.Config{
			ServerName: customHost,
		}
	}

	var rt http.RoundTripper = transport
	if customHost != "" {
		rt = &hostHeaderTransport{base: transport, customHost: customHost}
	}

	return &http.Client{
		Timeout:   10 * time.Second,
		Transport: rt,
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

func defaultPort(u *url.URL) string {
	if u.Port() != "" {
		return u.Port()
	}
	if u.Scheme == "https" {
		return "443"
	}
	return "80"
}

func buildCleanURL(ip net.IP, useHTTPS bool, port string) string {
	scheme := "http"
	if useHTTPS {
		scheme = "https"
	}
	return scheme + "://" + net.JoinHostPort(ip.String(), port)
}

// checkDestinationHealth performs a HEAD request against the destination URL
// and returns the health status, HTTP status code, and whether a valid HTTP
// response was received. When the HEAD request fails entirely (DNS error,
// connection refused, timeout, etc.) statusCode is 0 and responded is false.
// A non-zero statusCode is only returned when the server actually responded
// with an HTTP status code, regardless of whether it was a success or error.
func (s *URLService) checkDestinationHealth(originalURL string) (enum.DestinationStatus, int32, bool) {

	parsedURL, err := url.ParseRequestURI(originalURL)
	if err != nil {
		s.log.Error("invalid URL for health check", logger.Error(err), logger.String("originalURL", utils.SanitizeLog(originalURL)))
		return enum.DestinationStatusUnknown, 0, false
	}
	if err := validateRequestURL(parsedURL); err != nil {
		s.log.Error("rejected URL for health check", logger.Error(err), logger.String("originalURL", utils.SanitizeLog(originalURL)))
		return enum.DestinationStatusUnknown, 0, false
	}

	// Resolve DNS and validate every IP before building the request URL.
	// This breaks the taint chain: the final URL is constructed only from
	// string literals and resolved (validated) IPs, not from user input.
	host := parsedURL.Hostname()
	ips, dnsErr := net.DefaultResolver.LookupIP(context.Background(), "ip", host)
	if dnsErr != nil {
		s.log.Error("DNS resolution failed for health check", logger.Error(dnsErr), logger.String("host", utils.SanitizeLog(host)))
		return enum.DestinationStatusUnknown, 0, false
	}
	var safeIP net.IP
	for _, ip := range ips {
		if !isDisallowedIP(ip, s.blockedIPRanges) {
			safeIP = ip
			break
		}
	}
	if safeIP == nil {
		s.log.Error("all resolved IPs are blocked for health check", logger.String("host", utils.SanitizeLog(host)))
		return enum.DestinationStatusUnknown, 0, false
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return enum.DestinationStatusUnknown, 0, false
	}

	cleanURL := buildCleanURL(safeIP, parsedURL.Scheme == "https", defaultPort(parsedURL))

	client := s.newSafeHTTPClient(host)
	req, err := http.NewRequest(http.MethodHead, cleanURL, nil)
	if err != nil {
		s.log.Error("failed to build health check request", logger.Error(err), logger.String("originalURL", utils.SanitizeLog(originalURL)))
		return enum.DestinationStatusUnknown, 0, false
	}

	resp, err := client.Do(req)
	if err != nil {
		s.log.Error("health check failed", logger.Error(err), logger.String("originalURL", utils.SanitizeLog(originalURL)))
		return enum.DestinationStatusUnknown, 0, false
	}
	defer func() { _ = resp.Body.Close() }()

	statusCode := int32(resp.StatusCode)
	if statusCode >= 200 && statusCode < 400 {
		return enum.DestinationStatusHealthy, statusCode, true
	}
	return enum.DestinationStatusUnhealthy, statusCode, true
}

// cacheKeyRedirectPrefix prefixes every Redis key that stores redirect
// resolution data (keyed by short code).
const cacheKeyRedirectPrefix = "url:redirect:"

// redirectCacheData holds the redirect resolution data stored as a single
// JSON blob per short code key.
type redirectCacheData struct {
	ID          int64  `json:"id"`
	OriginalURL string `json:"original_url"`
	ExpiresAt   string `json:"expires_at,omitempty"`
	URLStatus   int16  `json:"url_status"`
}

// URLRedirectCache is the contract the URL service depends on for caching
// redirect resolution data (shortCode → original URL, expiry, status).
// It uses a single string key per short code with a JSON value.
type URLRedirectCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
	Del(ctx context.Context, key string) error
}

// NoopRedirectCache is a URLRedirectCache implementation that always returns
// cache-miss. It is used as a fallback when cache is unavailable and in tests
// that do not exercise caching.
type NoopRedirectCache struct{}

func (NoopRedirectCache) Get(context.Context, string) (string, error) { return "", fmt.Errorf("noop") }
func (NoopRedirectCache) Set(context.Context, string, string) error   { return nil }
func (NoopRedirectCache) Del(context.Context, string) error           { return nil }

// URLOption configures optional behavior of a URLService.
type URLOption func(*URLService)

// WithRedirectCache sets the cache backend used for redirect lookups.
func WithRedirectCache(c URLRedirectCache) URLOption {
	return func(s *URLService) { s.cache = c }
}

// URLService implements the URL shortening business logic.
type URLService struct {
	queries         gen.Querier
	db              *sql.DB
	baseURL         string
	secretKey       string
	log             logger.Logger
	blockedIPRanges []*net.IPNet
	cache           URLRedirectCache
}

// NewURLService constructs a URLService with the given querier, DB handle
// (used for transactions), base URL, HMAC secret key, and logger.
func NewURLService(queries gen.Querier, db *sql.DB, baseURL, secretKey string, log logger.Logger, opts ...URLOption) *URLService {
	ctx := context.Background()
	blockedRanges := loadBlockedIPRanges(ctx, queries, log)
	svc := &URLService{
		queries:         queries,
		db:              db,
		baseURL:         strings.TrimRight(baseURL, "/"),
		secretKey:       secretKey,
		log:             log,
		blockedIPRanges: blockedRanges,
		cache:           NoopRedirectCache{},
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
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
		return apperror.ErrInternal
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(gen.New(tx)); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		s.log.Error("failed to commit transaction", logger.Error(err))
		return apperror.ErrInternal
	}

	return nil
}

// ResolveUserID decodes an HMAC-signed display user id (e.g. "USR_...") back
// to the raw integer user id.  Forged or tampered ids are rejected.
func (s *URLService) ResolveUserID(ctx context.Context, encodedUserID string) (int64, error) {
	id, err := utils.DecodeID(encodedUserID, utils.UserIDPrefix, s.secretKey)
	if err != nil {
		s.log.Warn("invalid userId", logger.Error(err), logger.String("userId", utils.SanitizeLog(encodedUserID)))
		return 0, apperror.ErrNotFound
	}
	return id, nil
}

// checkBlockedDomain rejects original URLs whose host is present in the
// blocked_domains table.
func (s *URLService) checkBlockedDomain(ctx context.Context, originalURL string) error {
	parsed, err := url.Parse(originalURL)
	if err != nil || parsed.Hostname() == "" {
		s.log.Warn("invalid URL provided", logger.Error(err), logger.String("originalURL", utils.SanitizeLog(originalURL)))
		return apperror.ErrInvalidURL
	}
	host := strings.ToLower(parsed.Hostname())

	_, err = s.queries.GetBlockedDomain(ctx, host)
	if err == nil {
		s.log.Warn("blocked domain rejected", logger.String("host", utils.SanitizeLog(host)))
		return apperror.ErrBlockedDomain
	}
	if err != sql.ErrNoRows {
		s.log.Error("failed to check blocked domain", logger.Error(err), logger.String("host", utils.SanitizeLog(host)))
		return apperror.ErrInternal
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
		s.log.Error("failed to lookup destination", logger.Error(err), logger.String("urlHash", utils.SanitizeLog(urlHash)))
		return 0, apperror.ErrInternal
	}

	created, err := q.CreateDestination(ctx, gen.CreateDestinationParams{
		OriginalUrl: originalURL,
		UrlHash:     urlHash,
	})
	if err != nil {
		s.log.Error("failed to create destination", logger.Error(err), logger.String("originalURL", utils.SanitizeLog(originalURL)))
		return 0, apperror.ErrInternal
	}
	return created.ID, nil
}

// Create stores a new URL with version 1 inside a transaction.
func (s *URLService) Create(ctx context.Context, userID int64, req payload.CreateURLRequest) (*payload.URLResponse, error) {

	// Validate expiresAt
	if err := utils.ValidateExpiresAt(req.ExpiresAt); err != nil {
		return nil, apperror.ErrInvalidPayload
	}

	// Validate destination is reachable
	if err := s.checkBlockedDomain(ctx, req.OriginalURL); err != nil {
		s.log.Error("url blocked", logger.Error(err), logger.String("originalURL", utils.SanitizeLog(req.OriginalURL)))
		return nil, err
	}

	// Resolve short code: custom codes fail on duplicate, auto-generated codes
	// retry until unique (max 5 attempts).
	custom := req.CustomCode != ""
	code := req.CustomCode

	const maxRetries = 5

	var err error

	if custom {
		exists, existErr := s.queries.ShortCodeExists(ctx, code)
		if existErr != nil {
			s.log.Error("failed to check short code existence", logger.Error(existErr), logger.String("shortCode", utils.SanitizeLog(code)))
			return nil, apperror.ErrInternal
		}
		if exists {
			s.log.Warn("custom code already taken", logger.String("shortCode", utils.SanitizeLog(code)))
			return nil, apperror.ErrConflict
		}
	} else {
		for i := range maxRetries {

			code, err = s.generateShortCode()
			if err != nil {
				return nil, err
			}

			exists, existErr := s.queries.ShortCodeExists(ctx, code)
			if existErr != nil {
				s.log.Error("failed to check short code existence", logger.Error(existErr), logger.String("shortCode", utils.SanitizeLog(code)))
				return nil, apperror.ErrInternal
			}

			if !exists {
				break
			}

			s.log.Warn("auto-generated short code collision, retrying",
				logger.String("shortCode", utils.SanitizeLog(code)),
				logger.Int("attempt", i+1),
				logger.Int("maxRetries", maxRetries),
			)
			if i == maxRetries-1 {
				return nil, apperror.ErrInternal
			}
		}
	}

	// Check destination health before the transaction
	var (
		healthStatus    enum.DestinationStatus
		httpCode        int32
		healthCheckedAt sql.NullTime
	)

	healthStatus, httpCode, responded := s.checkDestinationHealth(req.OriginalURL)
	if responded {
		healthCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	// Create URL in a single transaction
	var created gen.Url

	err = s.withTx(ctx, func(q gen.Querier) error {

		// Find or create destination
		destID, err := s.findOrCreateDestination(q, ctx, req.OriginalURL)
		if err != nil {
			s.log.Error("failed to find or create destination", logger.Error(err), logger.String("originalURL", utils.SanitizeLog(req.OriginalURL)))
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
			LastAccessedAt:            healthCheckedAt,
		})
		if err != nil {
			if isDuplicateKey(err) {
				s.log.Warn("short code collision on insert", logger.String("shortCode", utils.SanitizeLog(code)))
				return apperror.ErrConflict
			}
			s.log.Error("failed to create url", logger.Error(err), logger.String("shortCode", utils.SanitizeLog(code)))
			return apperror.ErrInternal
		}

		// Create version 1 for every new URL
		err = q.CreateURLVersion(ctx, gen.CreateURLVersionParams{
			UrlID:         created.ID,
			OriginalUrl:   req.OriginalURL,
			VersionNumber: 1,
		})
		if err != nil {
			s.log.Error("failed to create url version", logger.Error(err), logger.Int64("urlID", created.ID))
			return apperror.ErrInternal
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("url created", logger.Int64("id", created.ID), logger.String("shortCode", utils.SanitizeLog(created.ShortCode)))

	return s.toResponse(created, req.OriginalURL), nil
}

// isDuplicateKey reports whether err is a Postgres unique-constraint violation.
func isDuplicateKey(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

// Redirect resolves a short code to its destination URL and records a click.
//
// It is cache-first: on a cache hit (the same shortCode resolved before) it
// serves the redirect and records the click without taking the SELECT ...
// FOR UPDATE row lock, avoiding the DB round trip on the hottest path. On a
// cache miss it falls back to the transactional DB path and populates the
// cache so subsequent requests can be served from it.
func (s *URLService) Redirect(ctx context.Context, shortCode string, click payload.ClickInfo) (*payload.URLResponse, error) {

	if resp, hit, err := s.redirectFromCache(ctx, shortCode, click); err != nil {
		return nil, err
	} else if hit {
		s.log.Info("url redirected (cache hit)", logger.String("shortCode", utils.SanitizeLog(shortCode)), logger.Int64("id", resp.ID))
		return resp, nil
	}

	var resp *payload.URLResponse
	var urlRow gen.GetURLByShortCodeForUpdateRow

	err := s.withTx(ctx, func(q gen.Querier) error {

		row, err := q.GetURLByShortCodeForUpdate(ctx, shortCode)
		if err != nil {
			if err == sql.ErrNoRows {
				s.log.Error("url not found by shortCode", logger.String("shortCode", utils.SanitizeLog(shortCode)))
				return apperror.ErrNotFound
			}
			s.log.Error("failed to get url by shortCode", logger.Error(err), logger.String("shortCode", utils.SanitizeLog(shortCode)))
			return apperror.ErrInternal
		}

		urlRow = row

		if row.ExpiresAt.Valid && row.ExpiresAt.Time.Before(time.Now().UTC()) {
			s.log.Warn("url expired", logger.Int64("id", row.ID), logger.String("shortCode", utils.SanitizeLog(shortCode)))
			return apperror.ErrURLExpired
		}

		if err := s.recordClick(ctx, q, row.ID, click); err != nil {
			return err
		}

		resp = s.toResponse(rowToUrlForUpdate(row), row.OriginalUrl)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Populate the cache so subsequent redirects for this code hit cache.
	s.cacheRedirect(ctx, shortCode, rowToUrlForUpdate(urlRow), resp.OriginalURL)

	s.log.Info("url redirected", logger.String("shortCode", utils.SanitizeLog(shortCode)), logger.Int64("id", resp.ID))
	return resp, nil
}

// redirectFromCache attempts to serve a redirect from cache. It returns the
// response, whether it was a valid cache hit, and an error when the cached
// entry is present but expired or inactive. When the cache is unavailable or
// the key is missing, hit is false and err is nil so the caller falls back to
// the DB.
func (s *URLService) redirectFromCache(ctx context.Context, shortCode string, click payload.ClickInfo) (*payload.URLResponse, bool, error) {

	raw, err := s.cache.Get(ctx, cacheKeyRedirectPrefix+shortCode)
	if err != nil {
		return nil, false, nil
	}

	var data redirectCacheData
	if json.Unmarshal([]byte(raw), &data) != nil {
		return nil, false, nil
	}

	if data.ExpiresAt != "" {
		if t, parseErr := time.Parse(time.RFC3339, data.ExpiresAt); parseErr == nil && t.Before(time.Now().UTC()) {
			s.log.Warn("cached url expired", logger.Int64("id", data.ID), logger.String("shortCode", utils.SanitizeLog(shortCode)))
			s.invalidateRedirectCache(ctx, shortCode)
			return nil, true, apperror.ErrURLExpired
		}
	}

	if data.URLStatus != int16(enum.URLStatusActive) {
		s.log.Warn("cached url not active", logger.Int64("id", data.ID), logger.String("shortCode", utils.SanitizeLog(shortCode)))
		s.invalidateRedirectCache(ctx, shortCode)
		return nil, true, apperror.ErrNotFound
	}

	// Cache hit: record the click (no row lock) and redirect.
	if err := s.recordClick(ctx, s.queries, data.ID, click); err != nil {
		return nil, false, nil
	}

	u := gen.Url{
		ID:        data.ID,
		ShortCode: shortCode,
		UrlStatus: sql.NullInt16{Int16: data.URLStatus, Valid: true},
	}

	return s.toResponse(u, data.OriginalURL), true, nil
}

// recordClick writes a click log, increments the click counter, and upserts
// daily stats for the given URL using the provided querier.
func (s *URLService) recordClick(ctx context.Context, q gen.Querier, urlID int64, click payload.ClickInfo) error {

	if _, err := q.CreateClickLog(ctx, gen.CreateClickLogParams{
		UrlID:      urlID,
		IpAddress:  inet(click.IP),
		UserAgent:  nullString(click.UserAgent),
		Referrer:   nullString(click.Referrer),
		Browser:    nullString(utils.ParseBrowser(click.UserAgent)),
		DeviceType: nullString(utils.ParseDeviceType(click.UserAgent)),
	}); err != nil {
		s.log.Error("failed to create click log", logger.Error(err), logger.Int64("urlID", urlID))
		return apperror.ErrInternal
	}

	if err := q.IncrementURLClick(ctx, urlID); err != nil {
		s.log.Error("failed to increment click count", logger.Error(err), logger.Int64("urlID", urlID))
		return apperror.ErrInternal
	}

	_ = q.UpsertDailyStats(ctx, gen.UpsertDailyStatsParams{
		UrlID:       urlID,
		StatDate:    time.Now().Truncate(24 * time.Hour),
		TotalClicks: sql.NullInt64{Int64: 1, Valid: true},
	})

	return nil
}

// cacheRedirect stores the primary redirect data for a short code as a single
// JSON value so subsequent lookups can be served without hitting the database.
func (s *URLService) cacheRedirect(ctx context.Context, shortCode string, u gen.Url, originalURL string) {
	if s.cache == nil {
		return
	}
	expiresAt := ""
	if u.ExpiresAt.Valid {
		expiresAt = u.ExpiresAt.Time.UTC().Format(time.RFC3339)
	}
	data := redirectCacheData{
		ID:          u.ID,
		OriginalURL: originalURL,
		ExpiresAt:   expiresAt,
		URLStatus:   u.UrlStatus.Int16,
	}
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	_ = s.cache.Set(ctx, cacheKeyRedirectPrefix+shortCode, string(b))
}

// invalidateRedirectCache removes the cached redirect data for a short code.
func (s *URLService) invalidateRedirectCache(ctx context.Context, shortCode string) {
	if s.cache == nil {
		return
	}
	_ = s.cache.Del(ctx, cacheKeyRedirectPrefix+shortCode)
}

// GetByID returns the URL with the given database id.
func (s *URLService) GetByID(ctx context.Context, userID int64, id int64) (*payload.URLResponse, error) {
	row, err := s.queries.GetURLByID(ctx, gen.GetURLByIDParams{ID: id, UserID: userID})
	if err != nil {
		if err == sql.ErrNoRows {
			s.log.Error("url not found by id", logger.Int64("id", id))
			return nil, apperror.ErrNotFound
		}
		s.log.Error("failed to get url by id", logger.Error(err), logger.Int64("id", id))
		return nil, fmt.Errorf("failed to get url: %w", err)
	}

	s.log.Info("url retrieved by id", logger.Int64("id", id))
	return s.toResponse(rowToUrlByID(row), row.OriginalUrl), nil
}

// List returns a paginated list of URLs ordered by creation time descending,
// along with the total count and pagination metadata. When status is non-nil,
// only URLs matching that status are returned.
func (s *URLService) List(ctx context.Context, userID int64, page, perPage, offset int32, status *int16) ([]any, int64, error) {

	var statusFilter sql.NullInt16
	if status != nil {
		statusFilter = sql.NullInt16{Int16: *status, Valid: true}
	}

	rows, err := s.queries.ListURLs(ctx, gen.ListURLsParams{
		UserID: userID,
		Limit:  perPage,
		Offset: offset,
		Status: statusFilter,
	})
	if err != nil {
		s.log.Error("failed to list urls", logger.Error(err))
		return nil, 0, apperror.ErrInternal
	}

	total, err := s.queries.CountURLs(ctx, gen.CountURLsParams{
		UserID: userID,
		Status: statusFilter,
	})
	if err != nil {
		s.log.Error("failed to count urls", logger.Error(err))
		return nil, 0, apperror.ErrInternal
	}

	items := make([]any, len(rows))
	for i, row := range rows {
		items[i] = *s.toResponse(rowToUrlList(row), row.OriginalUrl)
	}

	s.log.Info("urls listed", logger.Int("count", len(items)), logger.Int64("total", total))

	return items, total, nil
}

// CountByStatus returns the count of URLs per status for the given user.
func (s *URLService) CountByStatus(ctx context.Context, userID int64) (*payload.URLStatusCounts, error) {
	row, err := s.queries.CountURLsByStatus(ctx, userID)
	if err != nil {
		s.log.Error("failed to count urls by status", logger.Error(err))
		return nil, apperror.ErrInternal
	}

	s.log.Info("url status counts",
		logger.Int64("active", row.Active),
		logger.Int64("expired", row.Expired),
		logger.Int64("disabled", row.Disabled),
		logger.Int64("deleted", row.Deleted),
	)

	return &payload.URLStatusCounts{
		Active:   row.Active,
		Expired:  row.Expired,
		Disabled: row.Disabled,
		Deleted:  row.Deleted,
	}, nil
}

// Update changes the original URL and/or expiry inside a transaction. When
// the original URL changes, a new url_version row is appended.
func (s *URLService) Update(ctx context.Context, userID int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error) {

	if !req.ExpiresAt.IsZero() {

		// Validate expiresAt
		if err := utils.ValidateExpiresAt(req.ExpiresAt); err != nil {
			return nil, apperror.ErrInvalidPayload
		}
	}

	// Validate blocked domain if original URL is changing
	if req.OriginalURL != "" {
		if err := s.checkBlockedDomain(ctx, req.OriginalURL); err != nil {
			s.log.Error("url blocked", logger.Error(err), logger.String("originalURL", utils.SanitizeLog(req.OriginalURL)))
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
		var responded bool
		healthStatus, httpCode, responded = s.checkDestinationHealth(req.OriginalURL)
		if responded {
			healthCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}
		}
	}

	// Update URL in a single transaction
	err := s.withTx(ctx, func(q gen.Querier) error {

		// Fetch existing URL
		existing, eErr := q.GetURLByID(ctx, gen.GetURLByIDParams{ID: id, UserID: userID})
		if eErr != nil {
			if eErr == sql.ErrNoRows {
				s.log.Warn("url not found for update", logger.Int64("id", id))
				return apperror.ErrNotFound
			}
			s.log.Error("failed to fetch existing url", logger.Error(eErr), logger.Int64("id", id))
			return apperror.ErrInternal
		}

		// Resolve destination ID
		destID := existing.DestinationID

		if req.OriginalURL != "" {
			did, dErr := s.findOrCreateDestination(q, ctx, req.OriginalURL)
			if dErr != nil {
				s.log.Error("failed to find or create destination", logger.Error(dErr), logger.String("originalURL", utils.SanitizeLog(req.OriginalURL)))
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
				return apperror.ErrNotFound
			}
			s.log.Error("failed to update url", logger.Error(eErr), logger.Int64("id", id))
			return apperror.ErrInternal
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
				return apperror.ErrInternal
			}
			responseOriginalURL = dest.OriginalUrl
		}

		// Append a new version when the original URL changed
		if req.OriginalURL != "" {
			maxVer, vErr := q.GetLatestURLVersion(ctx, updated.ID)
			if vErr != nil && vErr != sql.ErrNoRows {
				s.log.Error("failed to get latest url version", logger.Error(vErr), logger.Int64("id", updated.ID))
				return apperror.ErrInternal
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
				return apperror.ErrInternal
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	s.log.Info("url updated", logger.Int64("id", updated.ID))

	// Invalidate the cache so the next redirect re-resolves from the DB.
	s.invalidateRedirectCache(ctx, updated.ShortCode)

	return s.toResponse(updated, responseOriginalURL), nil
}

// SoftDelete marks the URL as inactive and sets deletedAt, keeping the row for
// a later approval-driven hard delete.
func (s *URLService) SoftDelete(ctx context.Context, userID int64, id int64) (*payload.DeleteResponse, error) {
	u, err := s.queries.SoftDeleteURL(ctx, gen.SoftDeleteURLParams{ID: id, UserID: userID})
	if err != nil {
		if err == sql.ErrNoRows {
			s.log.Error("url not found for soft delete", logger.Int64("id", id))
			return nil, apperror.ErrNotFound
		}
		s.log.Error("failed to soft delete url", logger.Error(err), logger.Int64("id", id))
		return nil, fmt.Errorf("failed to soft delete url: %w", err)
	}

	s.log.Info("url soft deleted", logger.Int64("id", u.ID), logger.String("shortCode", utils.SanitizeLog(u.ShortCode)))
	s.invalidateRedirectCache(ctx, u.ShortCode)
	return &payload.DeleteResponse{
		ID:        u.ID,
		ShortCode: u.ShortCode,
		Message:   "url soft deleted. hard delete pending approval.",
		DeletedAt: u.DeletedAt.Time.Format("2006-01-02T15:04:05Z"),
	}, nil
}

// HardDelete permanently removes a previously soft-deleted URL from the database.
func (s *URLService) HardDelete(ctx context.Context, userID int64, id int64) error {
	u, err := s.queries.GetSoftDeletedURLByID(ctx, gen.GetSoftDeletedURLByIDParams{ID: id, UserID: userID})
	if err != nil {
		if err == sql.ErrNoRows {
			s.log.Warn("soft-deleted url not found for hard delete", logger.Int64("id", id))
			return apperror.ErrNotFound
		}
		s.log.Error("failed to fetch url for hard delete", logger.Error(err), logger.Int64("id", id))
		return fmt.Errorf("failed to fetch url for hard delete: %w", err)
	}

	if err := s.queries.HardDeleteURL(ctx, gen.HardDeleteURLParams{ID: id, UserID: userID}); err != nil {
		s.log.Error("failed to hard delete url", logger.Error(err), logger.Int64("id", id))
		return fmt.Errorf("failed to hard delete url: %w", err)
	}

	s.log.Info("url hard deleted", logger.Int64("id", id), logger.String("shortCode", utils.SanitizeLog(u.ShortCode)))
	s.invalidateRedirectCache(ctx, u.ShortCode)
	return nil
}

// ListClickLogs returns a paginated list of click logs for a URL, optionally
// filtered by a time range. The URL must belong to the given user.
func (s *URLService) ListClickLogs(ctx context.Context, userID, urlID int64, from, to *time.Time, page, perPage, offset int32) ([]any, int64, error) {
	if err := s.ensureOwnership(ctx, userID, urlID); err != nil {
		return nil, 0, err
	}

	total, err := s.queries.CountClickLogsByURL(ctx, gen.CountClickLogsByURLParams{
		UrlID: urlID,
		From:  nullableTime(from),
		To:    nullableTime(to),
	})
	if err != nil {
		s.log.Error("failed to count click logs", logger.Error(err), logger.Int64("urlID", urlID))
		return nil, 0, apperror.ErrInternal
	}

	rows, err := s.queries.ListClickLogsByURL(ctx, gen.ListClickLogsByURLParams{
		UrlID:  urlID,
		From:   nullableTime(from),
		To:     nullableTime(to),
		Limit:  perPage,
		Offset: offset,
	})
	if err != nil {
		s.log.Error("failed to list click logs", logger.Error(err), logger.Int64("urlID", urlID))
		return nil, 0, apperror.ErrInternal
	}

	items := make([]any, len(rows))
	for i, r := range rows {
		items[i] = payload.ClickLogEntry{
			ID:         r.ID,
			ClickedAt:  formatNullTime(r.ClickedAt),
			IPAddress:  r.IpAddress.IPNet.IP.String(),
			UserAgent:  r.UserAgent.String,
			Referrer:   r.Referrer.String,
			Browser:    r.Browser.String,
			DeviceType: r.DeviceType.String,
		}
	}

	// return &payload.ClickLogsResponse{
	// 	Items:      items,
	// 	Total:      total,
	// 	Page:       int(page),
	// 	PerPage:    int(perPage),
	// 	TotalPages: totalPages,
	// }, nil

	return items, total, nil
}

// ListAllClickLogs returns a paginated list of click logs across all of the
// user's URLs, optionally filtered by a time range. Results are ordered by
// most recent click first.
func (s *URLService) ListAllClickLogs(ctx context.Context, userID int64, from, to *time.Time, page, perPage, offset int32) ([]any, int64, error) {

	total, err := s.queries.CountAllClickLogsByUser(ctx, gen.CountAllClickLogsByUserParams{
		UserID: userID,
		From:   nullableTime(from),
		To:     nullableTime(to),
	})
	if err != nil {
		s.log.Error("failed to count all click logs", logger.Error(err), logger.Int64("userID", userID))
		return nil, 0, apperror.ErrInternal
	}

	rows, err := s.queries.ListAllClickLogsByUser(ctx, gen.ListAllClickLogsByUserParams{
		UserID: userID,
		From:   nullableTime(from),
		To:     nullableTime(to),
		Limit:  perPage,
		Offset: offset,
	})
	if err != nil {
		s.log.Error("failed to list all click logs", logger.Error(err), logger.Int64("userID", userID))
		return nil, 0, apperror.ErrInternal
	}

	items := make([]any, len(rows))
	for i, r := range rows {
		items[i] = payload.ClickLogEntry{
			ID:         r.ID,
			URLID:      r.UrlID,
			ShortCode:  r.ShortCode,
			ClickedAt:  formatNullTime(r.ClickedAt),
			IPAddress:  r.IpAddress.IPNet.IP.String(),
			UserAgent:  r.UserAgent.String,
			Referrer:   r.Referrer.String,
			Browser:    r.Browser.String,
			DeviceType: r.DeviceType.String,
		}
	}

	return items, total, nil
}

// GetAnalytics returns aggregate analytics for a URL including stats,
// top referrers, and daily click breakdown.
func (s *URLService) GetAnalytics(ctx context.Context, userID, urlID int64, from, to *time.Time) (*payload.AnalyticsResponse, error) {

	if err := s.ensureOwnership(ctx, userID, urlID); err != nil {
		return nil, err
	}

	stats, err := s.queries.ClickStatsByURL(ctx, gen.ClickStatsByURLParams{
		UrlID: urlID,
		From:  nullableTime(from),
		To:    nullableTime(to),
	})
	if err != nil {
		s.log.Error("failed to get click stats", logger.Error(err), logger.Int64("urlID", urlID))
		return nil, apperror.ErrInternal
	}

	refRows, err := s.queries.TopReferrersByURL(ctx, gen.TopReferrersByURLParams{
		UrlID: urlID,
		From:  nullableTime(from),
		To:    nullableTime(to),
		Limit: 10,
	})
	if err != nil {
		s.log.Error("failed to get top referrers", logger.Error(err), logger.Int64("urlID", urlID))
		return nil, apperror.ErrInternal
	}

	referrers := make([]payload.ReferrerStat, len(refRows))
	for i, r := range refRows {
		referrers[i] = payload.ReferrerStat{
			Referrer: r.Referrer,
			Count:    r.Count,
		}
	}

	var dailyStats []payload.DailyClickStat
	if from != nil && to != nil {
		dailyRows, dErr := s.queries.ClicksByDateRange(ctx, gen.ClicksByDateRangeParams{
			UrlID:       urlID,
			ClickedAt:   nullableTime(from),
			ClickedAt_2: nullableTime(to),
		})
		if dErr != nil {
			s.log.Error("failed to get daily clicks", logger.Error(dErr), logger.Int64("urlID", urlID))
			return nil, apperror.ErrInternal
		}
		dailyStats = make([]payload.DailyClickStat, len(dailyRows))
		for i, r := range dailyRows {
			dailyStats[i] = payload.DailyClickStat{
				Date:   r.Date.Format("2006-01-02"),
				Clicks: r.Clicks,
			}
		}
	}

	return &payload.AnalyticsResponse{
		Stats: payload.ClickStats{
			TotalClicks:    stats.TotalClicks,
			UniqueVisitors: stats.UniqueVisitors,
			FirstClickedAt: formatInterfaceTime(stats.FirstClickedAt),
			LastClickedAt:  formatInterfaceTime(stats.LastClickedAt),
		},
		Referrers:  referrers,
		DailyStats: dailyStats,
	}, nil
}

// GetAllAnalytics returns aggregate analytics across all of the user's URLs
// including overall stats, top referrers, and per-day click breakdown.
func (s *URLService) GetAllAnalytics(ctx context.Context, userID int64, from, to *time.Time) (*payload.AnalyticsResponse, error) {

	stats, err := s.queries.ClickStatsByUser(ctx, gen.ClickStatsByUserParams{
		UserID: userID,
		From:   nullableTime(from),
		To:     nullableTime(to),
	})
	if err != nil {
		s.log.Error("failed to get click stats by user", logger.Error(err), logger.Int64("userID", userID))
		return nil, apperror.ErrInternal
	}

	refRows, err := s.queries.TopReferrersByUser(ctx, gen.TopReferrersByUserParams{
		UserID: userID,
		From:   nullableTime(from),
		To:     nullableTime(to),
		Limit:  10,
	})
	if err != nil {
		s.log.Error("failed to get top referrers by user", logger.Error(err), logger.Int64("userID", userID))
		return nil, apperror.ErrInternal
	}

	referrers := make([]payload.ReferrerStat, len(refRows))
	for i, r := range refRows {
		referrers[i] = payload.ReferrerStat{
			Referrer: r.Referrer,
			Count:    r.Count,
		}
	}

	var dailyStats []payload.DailyClickStat
	if from != nil && to != nil {
		dailyRows, dErr := s.queries.ClicksByDateRangeByUser(ctx, gen.ClicksByDateRangeByUserParams{
			UserID:      userID,
			ClickedAt:   nullableTime(from),
			ClickedAt_2: nullableTime(to),
		})
		if dErr != nil {
			s.log.Error("failed to get daily clicks by user", logger.Error(dErr), logger.Int64("userID", userID))
			return nil, apperror.ErrInternal
		}
		dailyStats = make([]payload.DailyClickStat, len(dailyRows))
		for i, r := range dailyRows {
			dailyStats[i] = payload.DailyClickStat{
				Date:   r.Date.Format("2006-01-02"),
				Clicks: r.Clicks,
			}
		}
	}

	return &payload.AnalyticsResponse{
		Stats: payload.ClickStats{
			TotalClicks:    stats.TotalClicks,
			UniqueVisitors: stats.UniqueVisitors,
			FirstClickedAt: formatInterfaceTime(stats.FirstClickedAt),
			LastClickedAt:  formatInterfaceTime(stats.LastClickedAt),
		},
		Referrers:  referrers,
		DailyStats: dailyStats,
	}, nil
}

// GetCumulativeClickCounts returns the per-day click totals across all of the
// user's URLs for the trailing `days` days (oldest first), including days with
// zero clicks so the caller always receives a full contiguous series.
func (s *URLService) GetCumulativeClickCounts(ctx context.Context, userID int64, days int) (*payload.CumulativeClickCounts, error) {
	if days <= 0 {
		days = 7
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	from := today.AddDate(0, 0, -(days - 1))
	to := today.AddDate(0, 0, 1)

	rows, err := s.queries.CumulativeClickCounts(ctx, gen.CumulativeClickCountsParams{
		UserID: userID,
		From:   nullableTime(&from),
		To:     nullableTime(&to),
	})
	if err != nil {
		s.log.Error("failed to get cumulative click counts", logger.Error(err), logger.Int64("userID", userID), logger.Int("days", days))
		return nil, apperror.ErrInternal
	}

	byDate := make(map[string]int64, len(rows))
	for _, r := range rows {
		byDate[r.Date.Format("2006-01-02")] = r.Clicks
	}

	resp := &payload.CumulativeClickCounts{Days: days, Items: make([]payload.DailyClickStat, 0, days)}
	for i := 0; i < days; i++ {
		date := from.AddDate(0, 0, i).Format("2006-01-02")
		clicks := byDate[date]
		resp.Total += clicks
		resp.Items = append(resp.Items, payload.DailyClickStat{Date: date, Clicks: clicks})
	}

	return resp, nil
}

// ensureOwnership verifies the URL belongs to the user. Returns nil on success.
func (s *URLService) ensureOwnership(ctx context.Context, userID, urlID int64) error {
	_, err := s.queries.GetURLByID(ctx, gen.GetURLByIDParams{ID: urlID, UserID: userID})
	if err != nil {
		if err == sql.ErrNoRows {
			return apperror.ErrNotFound
		}
		return apperror.ErrInternal
	}
	return nil
}

func nullableTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

func formatNullTime(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return t.Time.Format("2006-01-02T15:04:05Z")
}

func formatInterfaceTime(v interface{}) string {
	switch t := v.(type) {
	case time.Time:
		return t.Format("2006-01-02T15:04:05Z")
	default:
		return ""
	}
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

	// Compute URL status string from url_status and expires_at
	urlStatus := "ACTIVE"
	if u.UrlStatus.Valid {
		switch enum.URLStatus(u.UrlStatus.Int16) {
		case enum.URLStatusDisabled:
			urlStatus = "DISABLED"
		case enum.URLStatusExpired:
			urlStatus = "EXPIRED"
		case enum.URLStatusDeleted:
			urlStatus = "DELETED"
		case enum.URLStatusActive:
			if u.ExpiresAt.Valid && u.ExpiresAt.Time.Before(time.Now().UTC()) {
				urlStatus = "EXPIRED"
			}
		}
	}

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
		Status:                  urlStatus,
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

func nullTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}

func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
