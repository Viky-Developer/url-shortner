package enum

// DestinationStatus represents the health status of a URL destination.
type DestinationStatus int16

const (
	// DestinationStatusUnknown indicates the destination has not been checked.
	DestinationStatusUnknown DestinationStatus = 0
	// DestinationStatusHealthy indicates the destination is healthy.
	DestinationStatusHealthy DestinationStatus = 1
	// DestinationStatusUnhealthy indicates the destination is unhealthy.
	DestinationStatusUnhealthy DestinationStatus = 2
)

// String returns the string representation of the DestinationStatus.
func (s DestinationStatus) String() string {
	switch s {
	case DestinationStatusHealthy:
		return "Healthy"
	case DestinationStatusUnhealthy:
		return "Unhealthy"
	default:
		return "Unknown / Not Checked"
	}
}
