// Package payload defines the request and response contracts shared between
// the HTTP handlers and the service layer.
package payload

import "github.com/vicky/url-shortner/internal/utils"

// CreateURLRequest is the request body for creating a short URL.
type CreateURLRequest struct {
	OriginalURL string              `json:"originalURL" validate:"required,url"` // The long URL to shorten. Required.
	CustomCode  string              `json:"customCode,omitempty"`                // Optional custom short code. If empty, a random 10-char code is generated.
	Title       string              `json:"title,omitempty"`                     // Optional title for the short URL.
	Description string              `json:"description,omitempty"`               // Optional description for the short URL.
	ExpiresAt   utils.UnixMilliTime `json:"expiresAt,omitempty"`                 // Optional expiration time (RFC3339 or Unix milliseconds).
}

// UpdateURLRequest is the request body for updating an existing URL.
type UpdateURLRequest struct {
	OriginalURL string              `json:"originalURL"`           // The new long URL value.
	Title       string              `json:"title,omitempty"`       // The new title value.
	Description string              `json:"description,omitempty"` // The new description value.
	Status      *int16              `json:"status,omitempty"`      // New URL status: 0=Disabled, 1=Active, 2=Expired, 3=Deleted.
	ExpiresAt   utils.UnixMilliTime `json:"expiresAt,omitempty"`   // Optional new expiration time (RFC3339 or Unix milliseconds).
}

// URLResponse represents a single URL as returned by the API.
type URLResponse struct {
	ID                      int64  `json:"id"`          // Database identifier.
	UserID                  string `json:"userId"`      // HMAC-encoded display user id (e.g. "USR_...").
	ShortCode               string `json:"shortCode"`   // Short code used in the short URL path.
	OriginalURL             string `json:"originalURL"` // The original long URL.
	ShortURL                string `json:"shortURL"`    // The generated short URL.
	Title                   string `json:"title"`       // Optional title.
	Description             string `json:"description"` // Optional description.
	IsCustom                *bool  `json:"isCustom"`    // Whether a custom code was used.
	IsActive                bool   `json:"isActive"`    // Whether the URL is active.
	ClickCount              int64  `json:"clickCount"`  // Number of times the short URL was hit.
	HasBeenAccessed         bool   `json:"hasBeenAccessed"`
	HealthChecked           bool   `json:"healthChecked"`
	LastAccessedAt          string `json:"lastAccessedAt"` // Last time the short URL was redirected (RFC3339).
	DestinationStatusString string `json:"destinationStatus"`
	DestinationHttpCode     string `json:"destinationHttpCode"` // Health status of the destination.
	LastHealthCheck         string `json:"lastHealthCheck"`     // Last health-check timestamp (RFC3339).
	ExpiresAt               string `json:"expiresAt"`           // Expiration timestamp (RFC3339).
	CreatedAt               string `json:"createdAt"`           // Creation timestamp (RFC3339).
	UpdatedAt               string `json:"updatedAt"`           // Last update timestamp (RFC3339).
}

// URLListResponse is the paginated list of URLs returned by the API.
type URLListResponse struct {
	Items      []URLResponse `json:"items"`      // URLs on the current page.
	Total      int64         `json:"total"`      // Total number of active URLs.
	Page       int           `json:"page"`       // Current page number (1-based).
	PerPage    int           `json:"perPage"`    // Number of items per page.
	TotalPages int           `json:"totalPages"` // Total number of pages.
}

// DeleteResponse describes the result of a soft delete operation.
type DeleteResponse struct {
	ID        int64  `json:"id"`        // Database identifier of the deleted URL.
	ShortCode string `json:"shortCode"` // Short code of the deleted URL.
	Message   string `json:"message"`   // Human-readable outcome message.
	DeletedAt string `json:"deletedAt"` // Timestamp of the soft delete (RFC3339).
}

// ClickLogEntry represents a single click log record.
type ClickLogEntry struct {
	ID         int64  `json:"id"`
	ClickedAt  string `json:"clickedAt"` // RFC3339 timestamp.
	IPAddress  string `json:"ipAddress"`
	UserAgent  string `json:"userAgent"`
	Referrer   string `json:"referrer"`
	Browser    string `json:"browser"`
	DeviceType string `json:"deviceType"`
}

// ClickStats represents aggregate click statistics for a URL.
type ClickStats struct {
	TotalClicks    int64  `json:"totalClicks"`
	UniqueVisitors int64  `json:"uniqueVisitors"`
	FirstClickedAt string `json:"firstClickedAt"` // RFC3339 or empty.
	LastClickedAt  string `json:"lastClickedAt"`  // RFC3339 or empty.
}

// ReferrerStat represents a referrer with its click count.
type ReferrerStat struct {
	Referrer string `json:"referrer"`
	Count    int64  `json:"count"`
}

// DailyClickStat represents click count per day.
type DailyClickStat struct {
	Date   string `json:"date"` // YYYY-MM-DD.
	Clicks int64  `json:"clicks"`
}

// ClickLogsResponse is the paginated list of click logs for a URL.
type ClickLogsResponse struct {
	Items      []ClickLogEntry `json:"items"`
	Total      int64           `json:"total"`
	Page       int             `json:"page"`
	PerPage    int             `json:"perPage"`
	TotalPages int             `json:"totalPages"`
}

// AnalyticsResponse contains all analytics data for a URL.
type AnalyticsResponse struct {
	Stats      ClickStats       `json:"stats"`
	Referrers  []ReferrerStat   `json:"referrers"`
	DailyStats []DailyClickStat `json:"dailyStats"`
}
