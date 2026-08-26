// Package handler contains the HTTP handlers for the authentication API.
package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/contextutil"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/response"
	"github.com/vicky/url-shortner/internal/utils"
	"github.com/vicky/url-shortner/internal/validation"
)

// AuthService is the contract the handlers depend on for auth business logic.
type AuthService interface {
	Register(ctx context.Context, req *payload.RegisterRequest, deviceType, deviceName, ipAddress, country, city, userAgent string) (*payload.AuthResponse, error)
	Login(ctx context.Context, req payload.LoginRequest, deviceType, deviceName, ipAddress, country, city, userAgent string) (*payload.AuthResponse, error)
	ForgotPassword(ctx context.Context, req payload.ForgotPasswordRequest, ipAddress, userAgent string) error
	UpdatePassword(ctx context.Context, userID int64, req payload.UpdatePasswordRequest, sessionID int64, ipAddress, userAgent string) error
	RefreshToken(ctx context.Context, refreshToken string, sessionID int64) (*payload.RefreshTokenResponse, error)
	Logout(ctx context.Context, refreshToken string, userID, sessionID int64) error
	ListSessions(ctx context.Context, userID int64) ([]payload.SessionResponse, error)
	RevokeSession(ctx context.Context, sessionID, userID int64) error
	RevokeOtherDevices(ctx context.Context, userID, currentSessionID int64) error
	RevokeAllSessions(ctx context.Context, userID int64) error
}

// AuthHandler holds the dependencies required by the auth HTTP handlers.
type AuthHandler struct {
	authService AuthService
	log         logger.Logger
}

// NewAuthHandler constructs an AuthHandler with the given service and logger.
func NewAuthHandler(authService AuthService, log logger.Logger) *AuthHandler {
	return &AuthHandler{authService: authService, log: log}
}

// Register handles POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {

	req, ok := validation.BindAndValidate[payload.RegisterRequest](r, w)
	if !ok {
		return
	}

	ipAddress := clientIP(r).String()
	userAgent := r.UserAgent()
	deviceType := r.Header.Get("X-Device-Type")
	deviceName := r.Header.Get("X-Device-Name")
	country := r.Header.Get("X-Country")
	city := r.Header.Get("X-City")

	resp, err := h.authService.Register(r.Context(), req, deviceType, deviceName, ipAddress, country, city, userAgent)
	if err != nil {
		h.log.Error("registration failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("user registered", logger.String("email", utils.SanitizeLog(req.Email)))
	response.Success(w, http.StatusCreated, "user registered", []any{resp})
}

// Login handles POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	req, ok := validation.BindAndValidate[payload.LoginRequest](r, w)
	if !ok {
		return
	}

	deviceType := r.Header.Get("X-Device-Type")
	deviceName := r.Header.Get("X-Device-Name")
	ipAddress := clientIP(r).String()
	userAgent := r.UserAgent()
	country := r.Header.Get("X-Country")
	city := r.Header.Get("X-City")

	resp, err := h.authService.Login(r.Context(), *req, deviceType, deviceName, ipAddress, country, city, userAgent)
	if err != nil {
		h.log.Error("login failed", logger.Error(err), logger.String("email", utils.SanitizeLog(req.Email)))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("user logged in", logger.String("email", utils.SanitizeLog(req.Email)))
	response.Success(w, http.StatusOK, "login successful", []any{resp})
}

// ForgotPassword handles POST /api/v1/auth/forgot-password
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	
	req, ok := validation.BindAndValidate[payload.ForgotPasswordRequest](r, w)
	if !ok {
		return
	}

	ipAddress := clientIP(r).String()
	userAgent := r.UserAgent()

	err := h.authService.ForgotPassword(r.Context(), *req, ipAddress, userAgent)
	if err != nil {
		h.log.Error("forgot password failed", logger.Error(err), logger.String("email", utils.SanitizeLog(req.Email)))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("password reset via forgot-password", logger.String("email", utils.SanitizeLog(req.Email)))
	response.Success(w, http.StatusOK, "password updated successfully", nil)
}

// UpdatePassword handles POST /api/v1/auth/update-password
func (h *AuthHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	req, ok := validation.BindAndValidate[payload.UpdatePasswordRequest](r, w)
	if !ok {
		return
	}

	userID, ok := h.getUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: user not authenticated", apperror.ErrUnauthorized))
		return
	}
	sessionID, _ := h.getSessionIDFromContext(r)

	ipAddress := clientIP(r).String()
	userAgent := r.UserAgent()

	err := h.authService.UpdatePassword(r.Context(), userID, *req, sessionID, ipAddress, userAgent)
	if err != nil {
		h.log.Error("update password failed", logger.Error(err), logger.Int64("userID", userID))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("password updated", logger.Int64("userID", userID))
	response.Success(w, http.StatusOK, "password updated successfully, all sessions revoked", nil)
}

// RefreshToken handles POST /api/v1/auth/refresh
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	req, ok := validation.BindAndValidate[payload.RefreshTokenRequest](r, w)
	if !ok {
		return
	}

	// sessionID is 0 when the client sent no access token; the middleware
	// populates it from a valid-signature (possibly expired) JWT.
	sessionID, _ := r.Context().Value(contextutil.SessionIDKey).(int64)

	resp, err := h.authService.RefreshToken(r.Context(), req.RefreshToken, sessionID)
	if err != nil {
		h.log.Error("token refresh failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "token refreshed", []any{resp})
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	req, ok := validation.BindAndValidate[payload.RefreshTokenRequest](r, w)
	if !ok {
		return
	}

	userID, ok := h.getUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	sessionID, ok := h.getSessionIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	err := h.authService.Logout(r.Context(), req.RefreshToken, userID, sessionID)
	if err != nil {
		h.log.Error("logout failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "logged out", []any{})
}

// ListSessions handles GET /api/v1/auth/sessions
func (h *AuthHandler) ListSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.getUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	sessions, err := h.authService.ListSessions(r.Context(), userID)
	if err != nil {
		h.log.Error("list sessions failed", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, err)
		return
	}

	items := make([]any, len(sessions))
	for i, s := range sessions {
		items[i] = s
	}

	response.Success(w, http.StatusOK, "sessions listed", items)
}

// RevokeSession handles DELETE /api/v1/auth/sessions/{id}
func (h *AuthHandler) RevokeSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.getUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	sessionIDStr := r.PathValue("id")
	sessionID, err := utils.ParseID(sessionIDStr)
	if err != nil {
		h.log.Error("invalid session id", logger.Error(err))
		response.Error(w, http.StatusBadRequest, fmt.Errorf("%w: invalid session id", apperror.ErrInvalidPayload))
		return
	}

	err = h.authService.RevokeSession(r.Context(), sessionID, userID)
	if err != nil {
		h.log.Error("revoke session failed", logger.Error(err))
		response.Error(w, http.StatusInternalServerError, err)
		return
	}

	response.Success(w, http.StatusOK, "session revoked", []any{})
}

// RevokeOtherDevices handles POST /api/v1/auth/sessions/revoke-others
// Revokes all active sessions except the one identified by the current user's
// session (extracted from the JWT access token).
func (h *AuthHandler) RevokeOtherDevices(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.getUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	sessionID, ok := h.getSessionIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: session required", apperror.ErrUnauthorized))
		return
	}

	err := h.authService.RevokeOtherDevices(r.Context(), userID, sessionID)
	if err != nil {
		h.log.Error("revoke other devices failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "all other sessions revoked", []any{})
}

// RevokeAllSessions handles POST /api/v1/auth/sessions/revoke-all
// Revokes every active session for the user, forcing re-login on all devices.
func (h *AuthHandler) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.getUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	err := h.authService.RevokeAllSessions(r.Context(), userID)
	if err != nil {
		h.log.Error("revoke all sessions failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "all sessions revoked, please re-login", []any{})
}

// getUserIDFromContext extracts the user ID from the request context.
// This is set by the JWT auth middleware.
func (h *AuthHandler) getUserIDFromContext(r *http.Request) (int64, bool) {
	userID, ok := r.Context().Value(contextutil.UserIDKey).(int64)
	return userID, ok
}

// getSessionIDFromContext extracts the session ID from the request context.
// The middleware stores it under the SessionIDKey context key.
func (h *AuthHandler) getSessionIDFromContext(r *http.Request) (int64, bool) {
	sessionID, ok := r.Context().Value(contextutil.SessionIDKey).(int64)
	return sessionID, ok
}
