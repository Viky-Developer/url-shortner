package enum

import "testing"

func TestSessionStatusString(t *testing.T) {
	tests := []struct {
		status   SessionStatus
		expected string
	}{
		{SessionStatusRevoked, "Revoked"},
		{SessionStatusActive, "Active"},
		{SessionStatusExpired, "Expired"},
		{SessionStatus(99), "Unknown"},
		{SessionStatus(-1), "Unknown"},
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
