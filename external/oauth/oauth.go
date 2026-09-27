// Package oauth provides integrations with external OAuth identity providers.
package oauth

import "context"

// Identity is a verified profile returned by an OAuth provider.
type Identity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// Provider defines the external OAuth behavior used by the application.
type Provider interface {
	AuthorizationURL(state string) string
	Identity(ctx context.Context, code string) (*Identity, error)
}
