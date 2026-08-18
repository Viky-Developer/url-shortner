package payload

// Pagination describes the paging metadata of a list response.
type Pagination struct {
	Total      int64 `json:"total"`      // Total number of records.
	Page       int   `json:"page"`       // Current page number (1-based).
	PerPage    int   `json:"perPage"`    // Number of records per page.
	TotalPages int   `json:"totalPages"` // Total number of pages.
}

// SuccessResponse is the unified envelope for successful API responses.
type SuccessResponse struct {
	StatusCode int         `json:"statusCode"`           // HTTP status code of the response.
	Message    string      `json:"message"`              // Human-readable success message.
	Data       []any       `json:"data,omitempty"`       // Payload array, omitted when empty.
	Pagination *Pagination `json:"pagination,omitempty"` // Pagination metadata, omitted when not paged.
}

// ErrorResponse is the unified envelope for error responses.
type ErrorResponse struct {
	StatusCode int    `json:"statusCode"` // HTTP status code of the response.
	Message    string `json:"message"`    // Human-readable error message.
}
