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
	"net/url"
	"strings"
	"time"

	"net/http"
	"strconv"

	"github.com/jackc/pgerrcode"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"
	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/utils"
)

// DestinationStatus represents the health status of a URL destination.
type DestinationStatus int16

const (
	// DestinationStatusUnknown indicates the destination has not been checked.
	DestinationStatusUnknown DestinationStatus = 0
	// DestinationStatusHealthy indicates the destination is healthy.
	DestinationStatusHealthy DestinationStatus = 1
	// DestinationStatusUnhealthy indicates the destination is unhealthy.
	DestinationStatusUnhealthy DestinationStatus = 2
)

// String returns the string representation of the DestinationStatus.
func (s DestinationStatus) String() string {
	switch s {
	case DestinationStatusHealthy:
		return "Healthy"
	case DestinationStatusUnhealthy:
		return "Unhealthy"
	default:
		return "Unknown / Not Checked"
	}
}

// checkDestinationHealth sends a HEAD request to the originalURL and returns the health status.
func (s *URLService) checkDestinationHealth(originalURL string) (DestinationStatus, int32) {
	parsedURL, err := url.ParseRequestURI(originalURL)
	if err != nil {
		s.log.Error("invalid URL for health check", logger.Error(err), logger.String("originalURL", originalURL))
		return DestinationStatusUnknown, 0
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Head(parsedURL.String())
	if err != nil {
		s.log.Error("health check failed", logger.Error(err), logger.String("originalURL", originalURL))
		return DestinationStatusUnknown, 0
	}
	defer func() { _ = resp.Body.Close() }()

	// Determine health status based on HTTP status code
	statusCode := int32(resp.StatusCode)
	if statusCode >= 200 && statusCode < 400 {
		return DestinationStatusHealthy, statusCode
	}
	return DestinationStatusUnhealthy, statusCode
}

const maxShortCodeAttempts = 5

// URLService implements the URL shortening business logic.
type URLService struct {
	queries   gen.Querier
	db        *sql.DB
	baseURL   string
	secretKey string
	log       logger.Logger
}

// NewURLService constructs a URLService with the given querier, DB handle
// (used for transactions), base URL, HMAC secret key, and logger.
func NewURLService(queries gen.Querier, db *sql.DB, baseURL, secretKey string, log logger.Logger) *URLService {
	return &URLService{
		queries:   queries,
		db:        db,
		baseURL:   strings.TrimRight(baseURL, "/"),
		secretKey: secretKey,
		log:       log,
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
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := fn(gen.New(tx)); err != nil {
		return err
	}
	return tx.Commit()
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
		return fmt.Errorf("%w: invalid URL", apperror.ErrInvalidURL)
	}
	host := strings.ToLower(parsed.Hostname())

	_, err = s.queries.GetBlockedDomain(ctx, host)
	if err == nil {
		s.log.Warn("blocked domain rejected", logger.String("host", host))
		return fmt.Errorf("%w: domain is blocked", apperror.ErrInvalidURL)
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check blocked domain: %w", err)
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
		return 0, fmt.Errorf("lookup destination: %w", err)
	}

	created, err := q.CreateDestination(ctx, gen.CreateDestinationParams{
		OriginalUrl: originalURL,
		UrlHash:     urlHash,
	})
	if err != nil {
		return 0, fmt.Errorf("create destination: %w", err)
	}
	return created.ID, nil
}

// Create stores a new URL with version 1 inside a transaction.
func (s *URLService) Create(ctx context.Context, userID int64, req payload.CreateURLRequest) (*payload.URLResponse, error) {
	if err := s.checkBlockedDomain(ctx, req.OriginalURL); err != nil {
		s.log.Error("url blocked", logger.Error(err), logger.String("originalURL", req.OriginalURL))
		return nil, err
	}

	custom := req.CustomCode != ""
	code := req.CustomCode

	if !custom {
		var err error
		code, err = s.generateShortCode()
		if err != nil {
			s.log.Error("failed to generate short code", logger.Error(err))
			return nil, err
		}
	}

	// Check destination health before the transaction
	var healthStatus DestinationStatus
	var httpCode int32
	var healthCheckedAt sql.NullTime

	healthStatus, httpCode = s.checkDestinationHealth(req.OriginalURL)
	healthCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}

	var created gen.Url
	err := s.withTx(ctx, func(q gen.Querier) error {
		destID, err := s.findOrCreateDestination(q, ctx, req.OriginalURL)
		if err != nil {
			return err
		}

		for attempt := range maxShortCodeAttempts {
			created, err = q.CreateURL(ctx, gen.CreateURLParams{
				UserID:              userID,
				ShortCode:           code,
				DestinationID:       destID,
				Title:               nullString(req.Title),
				Description:         nullString(req.Description),
				IsCustom:            sql.NullBool{Bool: custom, Valid: true},
				ExpiresAt:           nullTime(req.ExpiresAt),
				DestinationStatus:   sql.NullInt16{Int16: int16(healthStatus), Valid: true},
				DestinationHttpCode: sql.NullInt32{Int32: httpCode, Valid: httpCode != 0},
				LastHealthCheck:     healthCheckedAt,
			})
			if err != nil {
				if custom && isDuplicateKey(err) {
					s.log.Warn("short code already taken", logger.Error(err))
					return fmt.Errorf("%w: short code already taken", apperror.ErrConflict)
				}
				if !custom && isDuplicateKey(err) && attempt < maxShortCodeAttempts-1 {
					code, _ = s.generateShortCode()
					continue
				}
				return fmt.Errorf("create url: %w", err)
			}

			// Version 1 for every new URL.
			err = q.CreateURLVersion(ctx, gen.CreateURLVersionParams{
				UrlID:         created.ID,
				OriginalUrl:   req.OriginalURL,
				VersionNumber: 1,
			})
			if err != nil {
				return fmt.Errorf("create url version: %w", err)
			}
			return nil
		}
		return fmt.Errorf("%w: failed to create url", apperror.ErrInternal)
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
			return fmt.Errorf("failed to get url: %w", err)
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
			return fmt.Errorf("create click log: %w", err)
		}

		if err := q.IncrementURLClick(ctx, row.ID); err != nil {
			s.log.Error("failed to increment click count", logger.Error(err), logger.Int64("urlID", row.ID))
			return fmt.Errorf("increment click count: %w", err)
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
		return nil, fmt.Errorf("failed to list urls: %w", err)
	}

	total, err := s.queries.CountURLs(ctx, userID)
	if err != nil {
		s.log.Error("failed to count urls", logger.Error(err))
		return nil, fmt.Errorf("failed to count urls: %w", err)
	}
	if err != nil {
		s.log.Error("failed to count urls", logger.Error(err))
		return nil, fmt.Errorf("failed to count urls: %w", err)
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

// Update changes the original URL and/or expiry inside a transaction.  When
// the original URL changes, a new url_version row is appended.
func (s *URLService) Update(ctx context.Context, userID int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error) {
	if req.OriginalURL != "" {
		if err := s.checkBlockedDomain(ctx, req.OriginalURL); err != nil {
			s.log.Error("url blocked", logger.Error(err), logger.String("originalURL", req.OriginalURL))
			return nil, err
		}
	}

	// Check destination health before the transaction when originalURL changes
	var healthStatus DestinationStatus
	var httpCode int32
	var healthCheckedAt sql.NullTime
	if req.OriginalURL != "" {
		healthStatus, httpCode = s.checkDestinationHealth(req.OriginalURL)
		healthCheckedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	var updated gen.Url
	var responseOriginalURL string

	err := s.withTx(ctx, func(q gen.Querier) error {
		existing, eErr := q.GetURLByID(ctx, gen.GetURLByIDParams{ID: id, UserID: userID})
		if eErr != nil {
			if eErr == sql.ErrNoRows {
				return fmt.Errorf("%w: id=%d", apperror.ErrNotFound, id)
			}
			return fmt.Errorf("fetch existing url: %w", eErr)
		}

		destID := existing.DestinationID
		if req.OriginalURL != "" {
			did, dErr := s.findOrCreateDestination(q, ctx, req.OriginalURL)
			if dErr != nil {
				return dErr
			}
			destID = did
		}

		var uErr error
		updated, uErr = q.UpdateURL(ctx, gen.UpdateURLParams{
			ID:            id,
			UserID:        userID,
			DestinationID: destID,
			Title:         nullString(req.Title),
			Description:   nullString(req.Description),
			ExpiresAt:     nullTime(req.ExpiresAt),
			IsActive:      nullBool(req.IsActive),
		})
		if uErr != nil {
			if uErr == sql.ErrNoRows {
				return fmt.Errorf("%w: id=%d", apperror.ErrNotFound, id)
			}
			return fmt.Errorf("update url: %w", uErr)
		}

		// Update health status within the same transaction when originalURL changed
		if req.OriginalURL != "" {
			updatedUrl, hErr := q.UpdateURLHealthStatus(ctx, gen.UpdateURLHealthStatusParams{
				ID:                  updated.ID,
				DestinationStatus:   sql.NullInt16{Int16: int16(healthStatus), Valid: true},
				DestinationHttpCode: sql.NullInt32{Int32: httpCode, Valid: httpCode != 0},
				LastHealthCheck:     healthCheckedAt,
			})
			if hErr != nil {
				s.log.Error("failed to update URL health status", logger.Error(hErr), logger.Int64("id", updated.ID))
			} else {
				updated.DestinationStatus = updatedUrl.DestinationStatus
				updated.DestinationHttpCode = updatedUrl.DestinationHttpCode
				updated.LastHealthCheck = updatedUrl.LastHealthCheck
				updated.UpdatedAt = updatedUrl.UpdatedAt
			}
		}

		// Resolve the original_url for the response.
		if req.OriginalURL != "" {
			responseOriginalURL = req.OriginalURL
		} else {
			dest, dErr := q.GetDestinationByID(ctx, existing.DestinationID)
			if dErr != nil {
				return fmt.Errorf("fetch destination: %w", dErr)
			}
			responseOriginalURL = dest.OriginalUrl
		}

		// Append a new version when the original URL changed.
		if req.OriginalURL != "" {
			var maxVer int32
			maxVer, eErr = q.GetLatestURLVersion(ctx, updated.ID)
			if eErr != nil && eErr != sql.ErrNoRows {
				return fmt.Errorf("get latest version: %w", eErr)
			}
			if eErr == sql.ErrNoRows {
				maxVer = 0
			}
			vErr := q.CreateURLVersion(ctx, gen.CreateURLVersionParams{
				UrlID:         updated.ID,
				OriginalUrl:   req.OriginalURL,
				VersionNumber: maxVer + 1,
			})
			if vErr != nil {
				return fmt.Errorf("create url version: %w", vErr)
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

	// Map destination status to string
	status := DestinationStatusUnknown
	if u.DestinationStatus.Valid {
		status = DestinationStatus(u.DestinationStatus.Int16)
	}
	statusString := status.String()

	// Map HTTP code to string
	httpCode := ""
	if u.DestinationHttpCode.Valid {
		httpCode = strconv.Itoa(int(u.DestinationHttpCode.Int32))
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
		IsActive:                u.IsActive.Valid && u.IsActive.Bool,
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

	if u.DestinationHttpCode.Valid {
		resp.DestinationHttpCode = strconv.Itoa(int(u.DestinationHttpCode.Int32))
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
		ExpiresAt: row.ExpiresAt, IsActive: row.IsActive, LastAccessedAt: row.LastAccessedAt,
		DestinationStatus: row.DestinationStatus, LastHealthCheck: row.LastHealthCheck,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
	}
}

func rowToUrlList(row gen.ListURLsRow) gen.Url {
	return gen.Url{
		ID: row.ID, UserID: row.UserID, ShortCode: row.ShortCode,
		DestinationID: row.DestinationID, Title: row.Title, Description: row.Description,
		IsCustom: row.IsCustom, IsSafe: row.IsSafe, ClickCount: row.ClickCount,
		ExpiresAt: row.ExpiresAt, IsActive: row.IsActive, LastAccessedAt: row.LastAccessedAt,
		DestinationStatus: row.DestinationStatus, LastHealthCheck: row.LastHealthCheck,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
	}
}

func rowToUrlForUpdate(row gen.GetURLByShortCodeForUpdateRow) gen.Url {
	return gen.Url{
		ID: row.ID, UserID: row.UserID, ShortCode: row.ShortCode,
		DestinationID: row.DestinationID, Title: row.Title, Description: row.Description,
		IsCustom: row.IsCustom, IsSafe: row.IsSafe, ClickCount: row.ClickCount,
		ExpiresAt: row.ExpiresAt, IsActive: row.IsActive, LastAccessedAt: row.LastAccessedAt,
		DestinationStatus: row.DestinationStatus, LastHealthCheck: row.LastHealthCheck,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, DeletedAt: row.DeletedAt,
	}
}

func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullBool(b *bool) sql.NullBool {
	if b == nil {
		return sql.NullBool{Valid: false}
	}
	return sql.NullBool{Bool: *b, Valid: true}
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
