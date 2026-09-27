package enum

import "testing"

func TestOAuthProviderString(t *testing.T) {
	tests := []struct {
		name     string
		provider OAuthProvider
		want     string
	}{
		{name: "system", provider: OAuthProviderSystem, want: "SYSTEM"},
		{name: "google", provider: OAuthProviderGoogle, want: "GOOGLE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.provider.String(); got != tt.want {
				t.Fatalf("OAuthProvider.String() = %q, want %q", got, tt.want)
			}
		})
	}
}
