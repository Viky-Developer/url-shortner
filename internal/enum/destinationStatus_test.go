package enum

import "testing"

func TestDestinationStatusString(t *testing.T) {
	tests := []struct {
		status   DestinationStatus
		expected string
	}{
		{DestinationStatusHealthy, "Healthy"},
		{DestinationStatusUnhealthy, "Unhealthy"},
		{DestinationStatusUnknown, "Unknown / Not Checked"},
		{DestinationStatus(99), "Unknown / Not Checked"},
		{DestinationStatus(-1), "Unknown / Not Checked"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			got := tt.status.String()
			if got != tt.expected {
				t.Fatalf("expected %q, got %q", tt.expected, got)
			}
		})
	}
}
