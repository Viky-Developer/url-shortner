package payload

// DeleteAccountRequest is the request body for initiating account deletion.
// The user must type "DELETE" exactly to confirm.
type DeleteAccountRequest struct {
	Confirmation string `json:"confirmation" validate:"required"`
}

// AccountStatusResponse represents the account status in API responses.
type AccountStatusResponse struct {
	Status              string  `json:"status"`
	DeletionScheduledAt *string `json:"deletionScheduledAt,omitempty"`
}
