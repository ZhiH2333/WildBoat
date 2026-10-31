package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
	"github.com/ZhiH2333/WildBoat/internal/identity"
)

type authContextKey string

const (
	accountIDKey authContextKey = "account_id"
	playerIDKey  authContextKey = "player_id"
	sessionIDKey authContextKey = "session_id"
)

// Authenticator validates bearer access tokens and active sessions.
type Authenticator interface {
	ParseAccessToken(token string) (*identity.AccessClaims, error)
	EnsureSessionActive(ctx context.Context, sessionID uuid.UUID) error
}

// RequireAuth rejects requests without a valid access token and active session.
func RequireAuth(auth Authenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := RequestIDFromContext(r.Context())
			raw := bearerToken(r.Header.Get("Authorization"))
			if raw == "" {
				response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
				return
			}
			claims, err := auth.ParseAccessToken(raw)
			if err != nil {
				code, message, status := mapAuthParseError(err)
				response.Error(w, status, code, message, requestID, nil)
				return
			}
			if err := auth.EnsureSessionActive(r.Context(), claims.SessionID); err != nil {
				response.Error(w, http.StatusUnauthorized, "AUTH_SESSION_EXPIRED", "session is no longer valid", requestID, nil)
				return
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, accountIDKey, claims.AccountID)
			ctx = context.WithValue(ctx, playerIDKey, claims.PlayerID)
			ctx = context.WithValue(ctx, sessionIDKey, claims.SessionID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func mapAuthParseError(err error) (code, message string, status int) {
	switch err {
	case identity.ErrTokenExpired:
		return "AUTH_SESSION_EXPIRED", "access token expired", http.StatusUnauthorized
	default:
		return "AUTH_INVALID_TOKEN", "access token is invalid", http.StatusUnauthorized
	}
}

func bearerToken(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// AccountIDFromContext returns the authenticated account ID.
func AccountIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(accountIDKey).(uuid.UUID)
	return v, ok
}

// PlayerIDFromContext returns the authenticated player ID.
func PlayerIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(playerIDKey).(uuid.UUID)
	return v, ok
}

// SessionIDFromContext returns the authenticated session ID.
func SessionIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(sessionIDKey).(uuid.UUID)
	return v, ok
}
