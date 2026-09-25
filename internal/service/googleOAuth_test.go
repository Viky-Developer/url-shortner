package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	externaloauth "github.com/vicky/url-shortner/external/oauth"
	"github.com/vicky/url-shortner/internal/apperror"
	gen "github.com/vicky/url-shortner/internal/db/gen"
)

func TestLoginWithGoogleCreateOAuthAccountFailure(t *testing.T) {
	lookups := 0
	q := &mockQuerier{
		getOAuthUserFn: func(context.Context, gen.GetOAuthUserParams) (gen.GetOAuthUserRow, error) {
			lookups++
			return gen.GetOAuthUserRow{}, sql.ErrNoRows
		},
		emailFn: func(context.Context, string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{ID: 42, Email: "existing@example.com"}, nil
		},
		createOAuthAccountFn: func(context.Context, gen.CreateOAuthAccountParams) error {
			return errors.New("duplicate provider identity")
		},
	}
	provider := &mockGoogleProvider{identity: &externaloauth.Identity{
		Subject: "subject", Email: "existing@example.com", EmailVerified: true,
	}}
	svc := NewAuthService(q, nil, testConfig(), NoopCache{}, testLog(t), provider)

	_, err := svc.LoginWithGoogle(context.Background(), "code", "", "", "", "", "", "")
	if !errors.Is(err, apperror.ErrInternal) {
		t.Fatalf("expected OAuth-account creation failure to return internal error, got %v", err)
	}
	if lookups != 1 {
		t.Fatalf("expected one OAuth identity lookup, got %d", lookups)
	}
}

func TestLoginWithGoogleCreateUserFailure(t *testing.T) {
	q := &mockQuerier{
		getOAuthUserFn: func(context.Context, gen.GetOAuthUserParams) (gen.GetOAuthUserRow, error) {
			return gen.GetOAuthUserRow{}, sql.ErrNoRows
		},
		emailFn: func(context.Context, string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{}, sql.ErrNoRows
		},
		createUserFn: func(context.Context, gen.CreateUserParams) (gen.CreateUserRow, error) {
			return gen.CreateUserRow{}, errors.New("insert user failed")
		},
	}
	provider := &mockGoogleProvider{identity: &externaloauth.Identity{
		Subject: "subject", Email: "new@example.com", EmailVerified: true,
	}}
	svc := NewAuthService(q, nil, testConfig(), NoopCache{}, testLog(t), provider)

	_, err := svc.LoginWithGoogle(context.Background(), "code", "", "", "", "", "", "")
	if !errors.Is(err, apperror.ErrInternal) {
		t.Fatalf("expected user creation failure to return internal error, got %v", err)
	}
}

func TestLoginWithGoogleUpdateDisplayIDFailure(t *testing.T) {
	q := &mockQuerier{
		getOAuthUserFn: func(context.Context, gen.GetOAuthUserParams) (gen.GetOAuthUserRow, error) {
			return gen.GetOAuthUserRow{ID: 88, Email: "user@example.com", Role: "USER", Status: "ACTIVE"}, nil
		},
		updateUserFn: func(context.Context, gen.UpdateUserDisplayIDParams) (gen.UpdateUserDisplayIDRow, error) {
			return gen.UpdateUserDisplayIDRow{}, errors.New("update failed")
		},
	}
	provider := &mockGoogleProvider{identity: &externaloauth.Identity{
		Subject: "subject", Email: "user@example.com", EmailVerified: true,
	}}
	svc := NewAuthService(q, nil, testConfig(), NoopCache{}, testLog(t), provider)

	_, err := svc.LoginWithGoogle(context.Background(), "code", "", "", "", "", "", "")
	if !errors.Is(err, apperror.ErrInternal) {
		t.Fatalf("expected display-ID update failure to return internal error, got %v", err)
	}
}

func TestCreateOrLinkGoogleUserEmailLookupFailure(t *testing.T) {
	q := &mockQuerier{emailFn: func(context.Context, string) (gen.GetUserByEmailRow, error) {
		return gen.GetUserByEmailRow{}, errors.New("lookup failed")
	}}
	svc := NewAuthService(q, nil, testConfig(), NoopCache{}, testLog(t))

	_, err := svc.createOrLinkGoogleUser(context.Background(), &externaloauth.Identity{
		Subject: "subject", Email: "user@example.com", EmailVerified: true,
	})
	if !errors.Is(err, apperror.ErrInternal) {
		t.Fatalf("expected email lookup failure to return internal error, got %v", err)
	}
}

func TestCreateOrLinkGoogleUserReloadFailure(t *testing.T) {
	q := &mockQuerier{
		emailFn: func(context.Context, string) (gen.GetUserByEmailRow, error) {
			return gen.GetUserByEmailRow{ID: 42, Email: "user@example.com"}, nil
		},
		createOAuthAccountFn: func(context.Context, gen.CreateOAuthAccountParams) error { return nil },
		getOAuthUserFn: func(context.Context, gen.GetOAuthUserParams) (gen.GetOAuthUserRow, error) {
			return gen.GetOAuthUserRow{}, errors.New("reload failed")
		},
	}
	svc := NewAuthService(q, nil, testConfig(), NoopCache{}, testLog(t))

	_, err := svc.createOrLinkGoogleUser(context.Background(), &externaloauth.Identity{
		Subject: "subject", Email: "user@example.com", EmailVerified: true,
	})
	if !errors.Is(err, apperror.ErrInternal) {
		t.Fatalf("expected identity reload failure to return internal error, got %v", err)
	}
}
