package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"
)

type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
}

type Google struct {
	config      oauth2.Config
	userInfoURL string
}

func NewGoogle(cfg GoogleConfig) *Google {
	return &Google{config: oauth2.Config{
		ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret, RedirectURL: cfg.RedirectURL,
		Endpoint: oauth2.Endpoint{
			AuthURL: cfg.AuthURL, TokenURL: cfg.TokenURL,
		},
		Scopes: []string{"openid", "email", "profile"},
	}, userInfoURL: cfg.UserInfoURL}
}

func (g *Google) AuthorizationURL(state string) string {
	return g.config.AuthCodeURL(state, oauth2.AccessTypeOnline)
}

func (g *Google) Identity(ctx context.Context, code string) (*Identity, error) {
	token, err := g.config.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("exchange google authorization code: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.userInfoURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create google userinfo request: %w", err)
	}
	resp, err := g.config.Client(ctx, token).Do(req)
	if err != nil {
		return nil, fmt.Errorf("request google userinfo: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google userinfo returned status %d", resp.StatusCode)
	}

	var profile struct {
		Subject       string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("decode google userinfo: %w", err)
	}
	if profile.Subject == "" || profile.Email == "" || !profile.EmailVerified {
		return nil, fmt.Errorf("google account is missing a verified identity")
	}
	return &Identity{
		Subject: profile.Subject, Email: strings.ToLower(strings.TrimSpace(profile.Email)),
		EmailVerified: profile.EmailVerified, Name: profile.Name,
	}, nil
}
