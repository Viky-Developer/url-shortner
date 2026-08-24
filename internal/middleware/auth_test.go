package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/vicky/url-shortner/external/logger"
	"github.com/vicky/url-shortner/internal/config"
	"github.com/vicky/url-shortner/internal/contextutil"
	"github.com/vicky/url-shortner/internal/service"
	"github.com/vicky/url-shortner/internal/utils"
)

var testCfg = &config.Config{
	JWTSecretKey:       "test-jwt-secret-key-for-middleware-32c",
	UserIDSecretKey:    "test-secret-key",
	AccessTokenExpiry:  15 * time.Minute,
	RefreshTokenExpiry: 7 * 24 * time.Hour,
}

func testAuthService() *service.AuthService {
	log, _ := logger.New(logger.WithLevel("error"))
	return service.NewAuthService(nil, nil, testCfg, service.NoopCache{}, log)
}

func createTestJWT(t *testing.T, jwtKey, encodedUserID string) string {
	t.Helper()

	type claims struct {
		UserID string `json:"user_id"`
		jwt.RegisteredClaims
	}

	c := claims{
		UserID: encodedUserID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Subject:   encodedUserID,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	tokenStr, err := token.SignedString([]byte(jwtKey))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return tokenStr
}

func TestAuthMiddlewareMissingHeader(t *testing.T) {
	log, _ := logger.New(logger.WithLevel("error"))
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(nil, log)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()

	mw(noop).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "authorization header required") {
		t.Errorf("expected missing header error, got %s", w.Body.String())
	}
}

func TestAuthMiddlewareInvalidFormat(t *testing.T) {
	log, _ := logger.New(logger.WithLevel("error"))
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(testAuthService(), log)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()

	mw(noop).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid authorization header format") {
		t.Errorf("expected format error, got %s", w.Body.String())
	}
}

func TestAuthMiddlewareNoBearerPrefix(t *testing.T) {
	log, _ := logger.New(logger.WithLevel("error"))
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(testAuthService(), log)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Token abc123")
	w := httptest.NewRecorder()

	mw(noop).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareValidTokenDecodesUserID(t *testing.T) {
	log, _ := logger.New(logger.WithLevel("error"))

	var capturedUserID int64
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value(contextutil.UserIDKey).(int64)
		if !ok {
			t.Error("expected userID in context")
			return
		}
		capturedUserID = userID
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(testAuthService(), log)

	encodedUserID := utils.EncodeID(100000, utils.UserIDPrefix, testCfg.UserIDSecretKey)
	token := createTestJWT(t, testCfg.JWTSecretKey, encodedUserID)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	mw(noop).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if capturedUserID != 100000 {
		t.Errorf("expected decoded userID 100000, got %d", capturedUserID)
	}
}

func TestAuthMiddlewareMalformedBearer(t *testing.T) {
	log, _ := logger.New(logger.WithLevel("error"))
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(testAuthService(), log)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()

	mw(noop).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareEmptyBearer(t *testing.T) {
	log, _ := logger.New(logger.WithLevel("error"))
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(testAuthService(), log)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer")
	w := httptest.NewRecorder()

	mw(noop).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareCaseInsensitiveBearer(t *testing.T) {
	log, _ := logger.New(logger.WithLevel("error"))
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := AuthMiddleware(testAuthService(), log)

	encodedUserID := utils.EncodeID(42, utils.UserIDPrefix, testCfg.UserIDSecretKey)
	token := createTestJWT(t, testCfg.JWTSecretKey, encodedUserID)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "bearer "+token)
	w := httptest.NewRecorder()

	mw(noop).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthMiddlewareErrorResponse(t *testing.T) {
	log, _ := logger.New(logger.WithLevel("error"))
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called on auth failure")
	})

	mw := AuthMiddleware(testAuthService(), log)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer totally-invalid-token")
	w := httptest.NewRecorder()

	mw(noop).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "invalid or expired token") {
		t.Errorf("expected invalid token error, got %s", body)
	}
}

func TestAuthMiddlewareWrongJWTKey(t *testing.T) {
	log, _ := logger.New(logger.WithLevel("error"))
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with wrong key")
	})

	mw := AuthMiddleware(testAuthService(), log)

	encodedUserID := utils.EncodeID(42, utils.UserIDPrefix, testCfg.UserIDSecretKey)
	token := createTestJWT(t, "wrong-jwt-key-that-does-not-match", encodedUserID)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	mw(noop).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthMiddlewareTamperedEncodedUserID(t *testing.T) {
	log, _ := logger.New(logger.WithLevel("error"))
	noop := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called with tampered token")
	})

	mw := AuthMiddleware(testAuthService(), log)

	token := createTestJWT(t, testCfg.JWTSecretKey, "USR_tampered_invalid")

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	mw(noop).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "invalid token claims") {
		t.Errorf("expected invalid token claims error, got %s", w.Body.String())
	}
}
