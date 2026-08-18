package enum

// SessionStatus represents the lifecycle state of a session.
type SessionStatus int16

const (
	// SessionStatusRevoked indicates the session was revoked.
	SessionStatusRevoked SessionStatus = 0
	// SessionStatusActive indicates the session is active.
	SessionStatusActive SessionStatus = 1
	// SessionStatusExpired indicates the session has expired.
	SessionStatusExpired SessionStatus = 2
)

// String returns the string representation of the SessionStatus.
func (s SessionStatus) String() string {
	switch s {
	case SessionStatusRevoked:
		return "Revoked"
	case SessionStatusActive:
		return "Active"
	case SessionStatusExpired:
		return "Expired"
	default:
		return "Unknown"
	}
}
