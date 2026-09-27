package enum

// OAuthProvider identifies the provider used to authenticate a user.
type OAuthProvider string

const (
	// OAuthProviderSystem identifies password-based authentication managed by the application.
	OAuthProviderSystem OAuthProvider = "SYSTEM"

	// OAuthProviderGoogle identifies Google OAuth/OpenID Connect accounts.
	OAuthProviderGoogle OAuthProvider = "GOOGLE"
)

// String returns the database representation of the OAuth provider.
func (p OAuthProvider) String() string {
	return string(p)
}
