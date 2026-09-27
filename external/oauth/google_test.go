package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newGoogleTestProvider(serverURL string) *Google {
	return NewGoogle(GoogleConfig{
		ClientID: "client-id", ClientSecret: "client-secret", RedirectURL: serverURL + "/callback",
		AuthURL: serverURL + "/authorize", TokenURL: serverURL + "/token", UserInfoURL: serverURL + "/userinfo",
	})
}

func TestGoogleAuthorizationURL(t *testing.T) {
	provider := newGoogleTestProvider("https://provider.test")
	rawURL := provider.AuthorizationURL("state-value")
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	if parsed.Path != "/authorize" {
		t.Fatalf("path = %q, want /authorize", parsed.Path)
	}
	query := parsed.Query()
	if query.Get("client_id") != "client-id" || query.Get("redirect_uri") == "" {
		t.Fatalf("authorization query missing client configuration: %v", query)
	}
	if query.Get("state") != "state-value" || query.Get("scope") != "openid email profile" {
		t.Fatalf("authorization query missing security parameters: %v", query)
	}
}

func TestGoogleIdentitySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if r.Form.Get("code") != "valid-code" || r.Form.Get("grant_type") != "authorization_code" {
				t.Fatalf("unexpected token request: %v", r.Form)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"provider-token","token_type":"Bearer","expires_in":3600}`))
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer provider-token" {
				t.Fatalf("unexpected authorization header %q", r.Header.Get("Authorization"))
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sub":"subject-1","email":" User@Example.COM ","email_verified":true,"name":"Example User"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	identity, err := newGoogleTestProvider(server.URL).Identity(context.Background(), "valid-code")
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if identity.Subject != "subject-1" || identity.Email != "user@example.com" || identity.Name != "Example User" {
		t.Fatalf("unexpected identity: %+v", identity)
	}
}

func TestGoogleIdentityRejectsTokenExchangeFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid code", http.StatusBadRequest)
	}))
	defer server.Close()

	_, err := newGoogleTestProvider(server.URL).Identity(context.Background(), "invalid-code")
	if err == nil || !strings.Contains(err.Error(), "exchange google authorization code") {
		t.Fatalf("expected wrapped token exchange error, got %v", err)
	}
}

func TestGoogleIdentityRejectsUserInfoFailure(t *testing.T) {
	server := oauthTestServer(t, http.StatusBadGateway, `{"error":"unavailable"}`)
	defer server.Close()

	_, err := newGoogleTestProvider(server.URL).Identity(context.Background(), "valid-code")
	if err == nil || !strings.Contains(err.Error(), "userinfo returned status") {
		t.Fatalf("expected user-info status error, got %v", err)
	}
}

func TestGoogleIdentityRejectsInvalidProfiles(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "unverified email", body: `{"sub":"subject-1","email":"user@example.com","email_verified":false}`},
		{name: "missing subject", body: `{"email":"user@example.com","email_verified":true}`},
		{name: "missing email", body: `{"sub":"subject-1","email_verified":true}`},
		{name: "malformed JSON", body: `{not-json`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := oauthTestServer(t, http.StatusOK, tt.body)
			defer server.Close()
			if _, err := newGoogleTestProvider(server.URL).Identity(context.Background(), "valid-code"); err == nil {
				t.Fatal("expected invalid provider profile to fail")
			}
		})
	}
}

func oauthTestServer(t *testing.T, userInfoStatus int, userInfoBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"provider-token","token_type":"Bearer"}`))
		case "/userinfo":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(userInfoStatus)
			_, _ = w.Write([]byte(userInfoBody))
		default:
			http.NotFound(w, r)
		}
	}))
}
