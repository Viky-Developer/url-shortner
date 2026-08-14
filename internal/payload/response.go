package payload

// SuccessResponse is the unified envelope for successful API responses.
type SuccessResponse struct {
	StatusCode int    `json:"statusCode"`        // HTTP status code of the response.
	Message    string `json:"message"`           // Human-readable success message.
	Data       any    `json:"data,omitempty"`    // Payload of the response, omitted when empty.
}

// ErrorResponse is the unified envelope for error responses.
type ErrorResponse struct {
	StatusCode int    `json:"statusCode"` // HTTP status code of the response.
	Message    string `json:"message"`    // Human-readable error message.
}
