package enum

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
