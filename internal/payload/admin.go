package payload

// BlockedDomainResponse represents a blocked domain entry.
type BlockedDomainResponse struct {
	ID        int64  `json:"id"`
	Domain    string `json:"domain"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"createdAt"`
}

// CreateBlockedDomainRequest is the request body for blocking a domain.
type CreateBlockedDomainRequest struct {
	Domain string `json:"domain" binding:"required"`
	Reason string `json:"reason,omitempty"`
}

// BlockedIPRangeResponse represents a blocked IP range entry.
type BlockedIPRangeResponse struct {
	ID          int64  `json:"id"`
	CIDR        string `json:"cidr"`
	Description string `json:"description"`
}

// CreateBlockedIPRangeRequest is the request body for blocking an IP range.
type CreateBlockedIPRangeRequest struct {
	CIDR        string `json:"cidr" binding:"required"`
	Description string `json:"description" binding:"required"`
}
