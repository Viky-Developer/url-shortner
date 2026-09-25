package enum

// OAuthProvider identifies an external authentication provider.
type OAuthProvider string

const (
	// OAuthProviderGoogle identifies Google OAuth/OpenID Connect accounts.
	OAuthProviderGoogle OAuthProvider = "GOOGLE"
)

// String returns the database representation of the OAuth provider.
func (p OAuthProvider) String() string {
	return string(p)
}
