package payload

// RegisterRequest represents the user registration request.
type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName,omitempty"`
}

// LoginRequest represents the user login request.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RefreshTokenRequest represents the token refresh request.
type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// AuthResponse represents the authentication response with tokens.
type AuthResponse struct {
	AccessToken  string       `json:"accessToken"`
	RefreshToken string       `json:"refreshToken"`
	User         UserResponse `json:"user"`
}

// UserResponse represents the user in API responses.
type UserResponse struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	DisplayName     string `json:"displayName,omitempty"`
	PasswordAgeDays int    `json:"passwordAgeDays,omitempty"`
	ChangeSuggested bool   `json:"changeSuggested,omitempty"`
}

// SessionResponse represents a session in API responses.
type SessionResponse struct {
	ID           int64  `json:"id"`
	DeviceType   string `json:"deviceType,omitempty"`
	DeviceName   string `json:"deviceName,omitempty"`
	IPAddress    string `json:"ipAddress,omitempty"`
	LoggedInAt   string `json:"loggedInAt"`
	LastActiveAt string `json:"lastActiveAt"`
}

// UpdatePasswordRequest represents the password update request.
type UpdatePasswordRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// ForgotPasswordRequest represents the forgot-password request.
// Validates the previous password, updates to the new one, and returns tokens.
type ForgotPasswordRequest struct {
	Email           string `json:"email"`
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// UpdatePasswordResponse represents the password update response.
type UpdatePasswordResponse struct {
	Message string `json:"message"`
}

// MaxDeviceErrorResponse is returned when a user has reached the maximum
// number of active devices and must revoke one to log in again.
type MaxDeviceErrorResponse struct {
	StatusCode int               `json:"statusCode"`
	Message    string            `json:"message"`
	Sessions   []SessionResponse `json:"sessions"`
}
