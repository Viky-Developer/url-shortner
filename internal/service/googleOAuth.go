package service

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"

	"golang.org/x/crypto/bcrypt"

	externaloauth "github.com/vicky/url-shortner/external/oauth"
	"github.com/vicky/url-shortner/internal/apperror"
	gen "github.com/vicky/url-shortner/internal/db/gen"
	"github.com/vicky/url-shortner/internal/enum"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/utils"
)

func (s *AuthService) GoogleAuthURL(state string) string {
	return s.googleOAuth.AuthorizationURL(state)
}

func (s *AuthService) LoginWithGoogle(ctx context.Context, code, deviceType, deviceName, ipAddress, country, city, userAgent string) (*payload.AuthResponse, error) {
	identity, err := s.googleOAuth.Identity(ctx, code)
	if err != nil {
		s.log.Warn("google authentication failed")
		return nil, apperror.ErrUnauthorized
	}

	user, err := s.queries.GetOAuthUser(ctx, gen.GetOAuthUserParams{
		Provider:        enum.OAuthProviderGoogle.String(),
		ProviderSubject: identity.Subject,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.ErrInternal
	}

	if errors.Is(err, sql.ErrNoRows) {
		user, err = s.createOrLinkGoogleUser(ctx, identity)
		if err != nil {
			return nil, err
		}
	}

	displayUserID := user.DisplayUserID.String
	if displayUserID == "" {
		displayUserID = utils.EncodeID(user.ID, utils.UserIDPrefix, s.cfg.UserIDSecretKey)
		if _, err := s.queries.UpdateUserDisplayID(ctx, gen.UpdateUserDisplayIDParams{ID: user.ID, DisplayUserID: utils.NullString(displayUserID)}); err != nil {
			return nil, apperror.ErrInternal
		}
	}

	tokens, err := s.GenerateTokens(ctx, user.ID, displayUserID, user.Email, user.DisplayUserName.String, user.Role, deviceType, deviceName, ipAddress, country, city, userAgent)
	if err != nil {
		return nil, err
	}

	return &payload.AuthResponse{
		Token: payload.RefreshTokenResponse{AccessToken: tokens.AccessToken, RefreshToken: tokens.RefreshToken},
		User: payload.UserResponse{
			ID: displayUserID, Email: user.Email, DisplayName: user.DisplayUserName.String,
			Role: user.Role, Status: user.Status,
		},
	}, nil
}

func (s *AuthService) createOrLinkGoogleUser(ctx context.Context, identity *externaloauth.Identity) (gen.GetOAuthUserRow, error) {
	var result gen.GetOAuthUserRow
	err := s.withAuthTx(ctx, func(q gen.Querier) error {
		existing, lookupErr := q.GetUserByEmail(ctx, identity.Email)
		var userID int64
		if lookupErr == nil {
			userID = existing.ID
		} else if !errors.Is(lookupErr, sql.ErrNoRows) {
			return lookupErr
		} else {
			randomPassword := make([]byte, 32)
			if _, err := rand.Read(randomPassword); err != nil {
				return err
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(randomPassword)), bcrypt.DefaultCost)
			if err != nil {
				return err
			}
			created, err := q.CreateUser(ctx, gen.CreateUserParams{
				Email: identity.Email, PasswordHash: string(hash), DisplayUserName: utils.NullString(identity.Name),
			})
			if err != nil {
				return err
			}
			userID = created.ID
		}

		if err := q.CreateOAuthAccount(ctx, gen.CreateOAuthAccountParams{
			UserID: userID, Provider: enum.OAuthProviderGoogle.String(), ProviderSubject: identity.Subject, ProviderEmail: identity.Email,
		}); err != nil {
			return err
		}

		var err error
		result, err = q.GetOAuthUser(ctx, gen.GetOAuthUserParams{Provider: enum.OAuthProviderGoogle.String(), ProviderSubject: identity.Subject})
		return err
	})
	if err != nil {
		s.log.Error("failed to create or link google account")
		return gen.GetOAuthUserRow{}, apperror.ErrInternal
	}
	return result, nil
}
