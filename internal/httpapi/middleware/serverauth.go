package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
)

type serverContextKey string

const serverIDKey serverContextKey = "server_id"

// ServerAuthenticator validates dedicated server credentials.
type ServerAuthenticator interface {
	AuthenticateServer(ctx context.Context, credential string) (uuid.UUID, error)
}

// RequireServerAuth rejects requests without a valid dedicated server credential.
func RequireServerAuth(auth ServerAuthenticator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := RequestIDFromContext(r.Context())
			raw := bearerToken(r.Header.Get("Authorization"))
			if raw == "" {
				response.Error(w, http.StatusUnauthorized, "SERVER_AUTH_INVALID", "dedicated server authentication required", requestID, nil)
				return
			}
			serverID, err := auth.AuthenticateServer(r.Context(), raw)
			if err != nil {
				response.Error(w, http.StatusUnauthorized, "SERVER_AUTH_INVALID", "dedicated server authentication failed", requestID, nil)
				return
			}
			ctx := context.WithValue(r.Context(), serverIDKey, serverID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ServerIDFromContext returns the authenticated dedicated server ID.
func ServerIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(serverIDKey).(uuid.UUID)
	return v, ok
}
