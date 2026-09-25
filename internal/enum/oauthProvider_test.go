package enum

import "testing"

func TestOAuthProviderString(t *testing.T) {
	if got := OAuthProviderGoogle.String(); got != "GOOGLE" {
		t.Fatalf("OAuthProviderGoogle.String() = %q, want GOOGLE", got)
	}
}
