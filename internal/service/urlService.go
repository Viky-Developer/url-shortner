// Package service contains the business logic for the URL shortener,
// isolated from HTTP concerns and depending only on the sqlc-generated
// query interface and the payload contracts.
package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/lib/pq"
	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/utils"
)

const maxShortCodeAttempts = 5

// URLService implements the URL shortening business logic.
type URLService struct {
	queries   gen.Querier
	baseURL   string
	secretKey string
	log       logger.Logger
}

// NewURLService constructs a URLService with the given querier, base URL used
// to build short URLs, HMAC secret key for display id encoding, and logger.
func NewURLService(queries gen.Querier, baseURL, secretKey string, log logger.Logger) *URLService {
	return &URLService{
		queries:   queries,
		baseURL:   strings.TrimRight(baseURL, "/"),
		secretKey: secretKey,
		log:       log,
	}
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

// Create stores a new URL. If no custom code is provided, a random 10-character
// short code is generated, retrying on collision up to maxShortCodeAttempts
// times. If a custom code is provided and already exists, it returns an error.
func (s *URLService) Create(ctx context.Context, userID int64, req payload.CreateURLRequest) (*payload.URLResponse, error) {
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

	for attempt := range maxShortCodeAttempts {
		created, err := s.queries.CreateURL(ctx, gen.CreateURLParams{
			UserID:      userID,
			ShortCode:   code,
			OriginalUrl: req.OriginalURL,
			IsCustom:    sql.NullBool{Bool: custom, Valid: true},
			ExpiresAt:   nullTime(req.ExpiresAt),
		})
		if err != nil {
			if custom && isDuplicateKey(err) {
				s.log.Warn("short code already taken", logger.Error(err))
				return nil, fmt.Errorf("%w: short code already taken", apperror.ErrConflict)
			}
			if !custom && isDuplicateKey(err) && attempt < maxShortCodeAttempts-1 {
				code, _ = s.generateShortCode()
				continue
			}
			s.log.Error("failed to create url", logger.Error(err))
			return nil, fmt.Errorf("failed to create url: %w", err)
		}

		s.log.Info("url created", logger.Int64("id", created.ID), logger.String("shortCode", created.ShortCode))
		return s.toResponse(created), nil
	}

	return nil, fmt.Errorf("failed to create url: %w", apperror.ErrInternal)
}

// isDuplicateKey reports whether err is a Postgres unique-constraint violation.
func isDuplicateKey(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == pgerrcode.UniqueViolation
}

// GetByShortCode returns the active URL matching the given short code for the
// given user.
func (s *URLService) GetByShortCode(ctx context.Context, userID int64, shortCode string) (*payload.URLResponse, error) {
	u, err := s.queries.GetURLByShortCode(ctx, gen.GetURLByShortCodeParams{
		UserID:    userID,
		ShortCode: shortCode,
	})
	if err != nil {
		if err == sql.ErrNoRows {
			s.log.Error("url not found by shortCode", logger.String("shortCode", shortCode))
			return nil, fmt.Errorf("%w: %s", apperror.ErrNotFound, shortCode)
		}
		s.log.Error("failed to get url by shortCode", logger.Error(err), logger.String("shortCode", shortCode))
		return nil, fmt.Errorf("failed to get url: %w", err)
	}

	s.log.Info("url retrieved by shortCode", logger.String("shortCode", shortCode))
	return s.toResponse(u), nil
}

// GetByID returns the URL with the given database id.
func (s *URLService) GetByID(ctx context.Context, userID int64, id int64) (*payload.URLResponse, error) {
	u, err := s.queries.GetURLByID(ctx, gen.GetURLByIDParams{ID: id, UserID: userID})
	if err != nil {
		if err == sql.ErrNoRows {
			s.log.Error("url not found by id", logger.Int64("id", id))
			return nil, fmt.Errorf("%w: id=%d", apperror.ErrNotFound, id)
		}
		s.log.Error("failed to get url by id", logger.Error(err), logger.Int64("id", id))
		return nil, fmt.Errorf("failed to get url: %w", err)
	}

	s.log.Info("url retrieved by id", logger.Int64("id", id))
	return s.toResponse(u), nil
}

// List returns a paginated list of active URLs ordered by creation time
// descending, along with the total count and pagination metadata.
func (s *URLService) List(ctx context.Context, userID int64, page, perPage, offset int32) (*payload.URLListResponse, error) {
	urls, err := s.queries.ListURLs(ctx, gen.ListURLsParams{
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

	items := make([]payload.URLResponse, len(urls))
	for i, u := range urls {
		items[i] = *s.toResponse(u)
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

// Update changes the original URL and expiry of the URL with the given id.
func (s *URLService) Update(ctx context.Context, userID int64, id int64, req payload.UpdateURLRequest) (*payload.URLResponse, error) {
	updated, err := s.queries.UpdateURL(ctx, gen.UpdateURLParams{
		ID:          id,
		UserID:      userID,
		OriginalUrl: req.OriginalURL,
		ExpiresAt:   nullTime(req.ExpiresAt),
	})
	if err != nil {
		if err == sql.ErrNoRows {
			s.log.Error("url not found for update", logger.Int64("id", id))
			return nil, fmt.Errorf("%w: id=%d", apperror.ErrNotFound, id)
		}
		s.log.Error("failed to update url", logger.Error(err), logger.Int64("id", id))
		return nil, fmt.Errorf("failed to update url: %w", err)
	}

	s.log.Info("url updated", logger.Int64("id", updated.ID))
	return s.toResponse(updated), nil
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

// generateShortCode returns a random 10-character hexadecimal short code
// derived from crypto/rand.
func (s *URLService) generateShortCode() (string, error) {
	bytes := make([]byte, 5)
	if _, err := rand.Read(bytes); err != nil {
		s.log.Error("failed to read random bytes", logger.Error(err))
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return hex.EncodeToString(bytes)[:10], nil
}

// buildShortURL composes the full short URL from the configured base URL and
// the given short code.  The userId is intentionally NOT part of the short URL
// path, so the value is short and shareable.
func (s *URLService) buildShortURL(shortCode string) string {
	return fmt.Sprintf("%s/%s", s.baseURL, shortCode)
}

// toResponse maps a database Url model to the API URLResponse payload.
func (s *URLService) toResponse(u gen.Url) *payload.URLResponse {
	resp := &payload.URLResponse{
		ID:          u.ID,
		UserID:      utils.EncodeID(u.UserID, utils.UserIDPrefix, s.secretKey),
		ShortCode:   u.ShortCode,
		OriginalURL: u.OriginalUrl,
		ShortURL:    s.buildShortURL(u.ShortCode),
		IsActive:    u.IsActive.Valid && u.IsActive.Bool,
		CreatedAt:   u.CreatedAt.Time.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   u.UpdatedAt.Time.Format("2006-01-02T15:04:05Z"),
	}

	if u.IsCustom.Valid {
		resp.IsCustom = &u.IsCustom.Bool
	}

	if u.ExpiresAt.Valid {
		resp.ExpiresAt = u.ExpiresAt.Time.Format("2006-01-02T15:04:05Z")
	}

	return resp
}

// nullTime converts a utils.OptionalTime to a sql.NullTime, marking it
// invalid when the time was not provided.
func nullTime(t utils.OptionalTime) sql.NullTime {
	if !t.Valid {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: t.Time, Valid: true}
}
