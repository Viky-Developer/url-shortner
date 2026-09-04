package enum

import (
	"fmt"
	"strings"
)

// URLStatus represents the lifecycle state of a URL.
type URLStatus int16

const (
	// URLStatusDisabled indicates the URL is disabled.
	URLStatusDisabled URLStatus = 0
	// URLStatusActive indicates the URL is active.
	URLStatusActive URLStatus = 1
	// URLStatusExpired indicates the URL has expired.
	URLStatusExpired URLStatus = 2
	// URLStatusDeleted indicates the URL was soft-deleted.
	URLStatusDeleted URLStatus = 3
)

// String returns the string representation of the URLStatus.
func (s URLStatus) String() string {
	switch s {
	case URLStatusDisabled:
		return "Disabled"
	case URLStatusActive:
		return "Active"
	case URLStatusExpired:
		return "Expired"
	case URLStatusDeleted:
		return "Deleted"
	default:
		return "Unknown"
	}
}

// ParseURLStatus converts a status string to a URLStatus value.
// Accepts both upper and lowercase (e.g. "ACTIVE", "active").
func ParseURLStatus(s string) (URLStatus, error) {
	switch strings.ToUpper(s) {
	case "ACTIVE":
		return URLStatusActive, nil
	case "EXPIRED":
		return URLStatusExpired, nil
	case "DELETED":
		return URLStatusDeleted, nil
	case "DISABLED":
		return URLStatusDisabled, nil
	default:
		return 0, fmt.Errorf("invalid status: %s", s)
	}
}
