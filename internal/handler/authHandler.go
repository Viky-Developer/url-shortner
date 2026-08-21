// Package handler contains the HTTP handlers for the authentication API.
package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/apperror"
	"github.com/vicky/url-shortner/internal/contextutil"
	"github.com/vicky/url-shortner/internal/payload"
	"github.com/vicky/url-shortner/internal/response"
	"github.com/vicky/url-shortner/internal/utils"
)

// AuthService is the contract the handlers depend on for auth business logic.
type AuthService interface {
	Register(ctx context.Context, req payload.RegisterRequest, ipAddress, userAgent string) (*payload.AuthResponse, error)
	Login(ctx context.Context, req payload.LoginRequest, deviceType, deviceName, ipAddress, userAgent string) (*payload.AuthResponse, error)
	ForgotPassword(ctx context.Context, req payload.ForgotPasswordRequest, ipAddress, userAgent string) (*payload.AuthResponse, error)
	RefreshToken(ctx context.Context, refreshToken string) (*payload.AuthResponse, error)
	Logout(ctx context.Context, refreshToken string) error
	ListSessions(ctx context.Context, userID int64) ([]payload.SessionResponse, error)
	RevokeSession(ctx context.Context, sessionID, userID int64) error
	UpdatePassword(ctx context.Context, userID int64, req payload.UpdatePasswordRequest, ipAddress, userAgent string) (*payload.UpdatePasswordResponse, error)
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
	var req payload.RegisterRequest
	if err := utils.DecodeBody(r, &req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	if req.Email == "" || req.Password == "" {
		err := fmt.Errorf("%w: email and password are required", apperror.ErrInvalidPayload)
		h.log.Error("missing required fields", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	ipAddress := clientIP(r).String()
	userAgent := r.UserAgent()

	resp, err := h.authService.Register(r.Context(), req, ipAddress, userAgent)
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
	var req payload.LoginRequest
	if err := utils.DecodeBody(r, &req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	if req.Email == "" || req.Password == "" {
		err := fmt.Errorf("%w: email and password are required", apperror.ErrInvalidPayload)
		h.log.Error("missing required fields", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	deviceType := r.Header.Get("X-Device-Type")
	deviceName := r.Header.Get("X-Device-Name")
	ipAddress := clientIP(r).String()
	userAgent := r.UserAgent()

	resp, err := h.authService.Login(r.Context(), req, deviceType, deviceName, ipAddress, userAgent)
	if err != nil {
		var maxErr *apperror.MaxDeviceError
		if errors.As(err, &maxErr) {
			h.log.Warn("login blocked: max devices reached", logger.String("email", utils.SanitizeLog(req.Email)))
			sessions := make([]payload.SessionResponse, len(maxErr.Devices))
			for i, d := range maxErr.Devices {
				sessions[i] = payload.SessionResponse{
					ID: d.ID, DeviceType: d.DeviceType, DeviceName: d.DeviceName,
					IPAddress: d.IPAddress, LoggedInAt: d.LoggedInAt, LastActiveAt: d.LastActiveAt,
				}
			}
			response.JSON(w, http.StatusConflict, payload.MaxDeviceErrorResponse{
				StatusCode: http.StatusConflict,
				Message:    maxErr.Error(),
				Sessions:   sessions,
			})
			return
		}
		h.log.Error("login failed", logger.Error(err), logger.String("email", utils.SanitizeLog(req.Email)))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("user logged in", logger.String("email", utils.SanitizeLog(req.Email)))
	response.Success(w, http.StatusOK, "login successful", []any{resp})
}

// ForgotPassword handles POST /api/v1/auth/forgot-password
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req payload.ForgotPasswordRequest
	if err := utils.DecodeBody(r, &req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	if req.Email == "" || req.CurrentPassword == "" || req.NewPassword == "" {
		err := fmt.Errorf("%w: email, current password and new password are required", apperror.ErrInvalidPayload)
		h.log.Error("missing required fields", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	ipAddress := clientIP(r).String()
	userAgent := r.UserAgent()

	resp, err := h.authService.ForgotPassword(r.Context(), req, ipAddress, userAgent)
	if err != nil {
		var maxErr *apperror.MaxDeviceError
		if errors.As(err, &maxErr) {
			h.log.Warn("forgot password blocked: max devices reached", logger.String("email", utils.SanitizeLog(req.Email)))
			sessions := make([]payload.SessionResponse, len(maxErr.Devices))
			for i, d := range maxErr.Devices {
				sessions[i] = payload.SessionResponse{
					ID: d.ID, DeviceType: d.DeviceType, DeviceName: d.DeviceName,
					IPAddress: d.IPAddress, LoggedInAt: d.LoggedInAt, LastActiveAt: d.LastActiveAt,
				}
			}
			response.JSON(w, http.StatusConflict, payload.MaxDeviceErrorResponse{
				StatusCode: http.StatusConflict,
				Message:    maxErr.Error(),
				Sessions:   sessions,
			})
			return
		}
		h.log.Error("forgot password failed", logger.Error(err), logger.String("email", utils.SanitizeLog(req.Email)))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("password reset via forgot-password", logger.String("email", utils.SanitizeLog(req.Email)))
	response.Success(w, http.StatusOK, "password updated successfully", []any{resp})
}

// RefreshToken handles POST /api/v1/auth/refresh
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req payload.RefreshTokenRequest
	if err := utils.DecodeBody(r, &req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	if req.RefreshToken == "" {
		err := fmt.Errorf("%w: refresh token is required", apperror.ErrInvalidPayload)
		h.log.Error("missing refresh token", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	resp, err := h.authService.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		h.log.Error("token refresh failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	response.Success(w, http.StatusOK, "token refreshed", []any{resp})
}

// Logout handles POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req payload.RefreshTokenRequest
	if err := utils.DecodeBody(r, &req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	if req.RefreshToken == "" {
		err := fmt.Errorf("%w: refresh token is required", apperror.ErrInvalidPayload)
		h.log.Error("missing refresh token", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	err := h.authService.Logout(r.Context(), req.RefreshToken)
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

// UpdatePassword handles PATCH /api/v1/auth/password
func (h *AuthHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.getUserIDFromContext(r)
	if !ok {
		response.Error(w, http.StatusUnauthorized, fmt.Errorf("%w: unauthorized", apperror.ErrUnauthorized))
		return
	}

	var req payload.UpdatePasswordRequest
	if err := utils.DecodeBody(r, &req); err != nil {
		h.log.Error("invalid request body", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		err := fmt.Errorf("%w: current password and new password are required", apperror.ErrInvalidPayload)
		h.log.Error("missing required fields", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	// Validate new password
	if err := utils.ValidatePassword(req.NewPassword); err != nil {
		h.log.Error("invalid new password", logger.Error(err))
		response.Error(w, http.StatusBadRequest, err)
		return
	}

	ipAddress := clientIP(r).String()
	userAgent := r.UserAgent()

	resp, err := h.authService.UpdatePassword(r.Context(), userID, req, ipAddress, userAgent)
	if err != nil {
		h.log.Error("update password failed", logger.Error(err))
		response.Error(w, response.StatusCodeFromError(err), err)
		return
	}

	h.log.Info("password updated", logger.Int64("userID", userID))
	response.Success(w, http.StatusOK, resp.Message, []any{resp})
}

// getUserIDFromContext extracts the user ID from the request context.
// This is set by the JWT auth middleware.
func (h *AuthHandler) getUserIDFromContext(r *http.Request) (int64, bool) {
	userID, ok := r.Context().Value(contextutil.UserIDKey).(int64)
	return userID, ok
}
