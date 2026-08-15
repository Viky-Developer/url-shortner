// Package payload defines the request and response contracts shared between
// the HTTP handlers and the service layer.
package payload

import "github.com/vicky/url-shortner/internal/utils"

// CreateURLRequest is the request body for creating a short URL.
type CreateURLRequest struct {
	OriginalURL string             `json:"originalURL" binding:"required"` // The long URL to shorten. Required.
	CustomCode  string             `json:"customCode,omitempty"`           // Optional custom short code. If empty, a random 10-char code is generated.
	Title       string             `json:"title,omitempty"`                // Optional title for the short URL.
	Description string             `json:"description,omitempty"`          // Optional description for the short URL.
	ExpiresAt   utils.OptionalTime `json:"expiresAt,omitzero"`             // Optional expiration time for the short URL.
}

// UpdateURLRequest is the request body for updating an existing URL.
type UpdateURLRequest struct {
	OriginalURL string             `json:"originalURL"`           // The new long URL value.
	Title       string             `json:"title,omitempty"`       // The new title value.
	Description string             `json:"description,omitempty"` // The new description value.
	IsActive    *bool              `json:"isActive,omitempty"`    // Manually activate/deactivate the URL.
	ExpiresAt   utils.OptionalTime `json:"expiresAt,omitempty"`   // Optional new expiration time.
}

// URLResponse represents a single URL as returned by the API.
type URLResponse struct {
	ID                int64  `json:"id"`                          // Database identifier.
	UserID            string `json:"userId"`                      // HMAC-encoded display user id (e.g. "USR_...").
	ShortCode         string `json:"shortCode"`                   // Short code used in the short URL path.
	OriginalURL       string `json:"originalURL"`                 // The original long URL.
	ShortURL          string `json:"shortURL"`                    // The generated short URL.
	Title             string `json:"title,omitempty"`             // Optional title.
	Description       string `json:"description,omitempty"`       // Optional description.
	IsCustom          *bool  `json:"isCustom,omitempty"`          // Whether a custom code was used.
	IsActive          bool   `json:"isActive"`                    // Whether the URL is active.
	ClickCount        int64  `json:"clickCount"`                  // Number of times the short URL was hit.
	LastAccessedAt    string `json:"lastAccessedAt,omitempty"`    // Last time the short URL was redirected (RFC3339).
	DestinationStatus *int16 `json:"destinationStatus,omitempty"` // Health status of the destination.
	LastHealthCheck   string `json:"lastHealthCheck,omitempty"`   // Last health-check timestamp (RFC3339).
	ExpiresAt         string `json:"expiresAt,omitempty"`         // Expiration timestamp (RFC3339).
	CreatedAt         string `json:"createdAt"`                   // Creation timestamp (RFC3339).
	UpdatedAt         string `json:"updatedAt"`                   // Last update timestamp (RFC3339).
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
