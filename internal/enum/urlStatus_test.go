package enum

import "testing"

func TestURLStatusString(t *testing.T) {
	tests := []struct {
		status   URLStatus
		expected string
	}{
		{URLStatusDisabled, "Disabled"},
		{URLStatusActive, "Active"},
		{URLStatusExpired, "Expired"},
		{URLStatusDeleted, "Deleted"},
		{URLStatus(99), "Unknown"},
		{URLStatus(-1), "Unknown"},
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
