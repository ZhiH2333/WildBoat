package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/handlers"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
	"github.com/ZhiH2333/WildBoat/internal/identity"
)

// NewRouter wires public HTTP routes for the modular monolith API.
func NewRouter(logger *slog.Logger, health handlers.Health, idSvc *identity.Service, authLimiter *middleware.RateLimiter) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health.Healthz)
	mux.HandleFunc("GET /readyz", health.Readyz)

	auth := handlers.Auth{Service: idSvc}
	me := handlers.Me{Service: idSvc}
	requireAuth := middleware.RequireAuth(idSvc)

	limit := func(h http.HandlerFunc) http.Handler {
		if authLimiter == nil {
			return h
		}
		return authLimiter.Limit(h)
	}

	mux.Handle("POST /v1/auth/register", limit(auth.Register))
	mux.Handle("POST /v1/auth/login", limit(auth.Login))
	mux.Handle("POST /v1/auth/refresh", limit(auth.Refresh))
	mux.Handle("POST /v1/auth/email/verify", limit(auth.VerifyEmail))
	mux.Handle("POST /v1/auth/password/forgot", limit(auth.ForgotPassword))
	mux.Handle("POST /v1/auth/password/reset", limit(auth.ResetPassword))

	mux.Handle("POST /v1/auth/logout", requireAuth(http.HandlerFunc(auth.Logout)))
	mux.Handle("POST /v1/auth/logout-all", requireAuth(http.HandlerFunc(auth.LogoutAll)))
	mux.Handle("GET /v1/me", requireAuth(http.HandlerFunc(me.Get)))
	mux.Handle("PATCH /v1/me/profile", requireAuth(http.HandlerFunc(me.PatchProfile)))
	mux.Handle("GET /v1/me/sessions", requireAuth(http.HandlerFunc(me.ListSessions)))
	mux.Handle("DELETE /v1/me/sessions/{session_id}", requireAuth(http.HandlerFunc(me.DeleteSession)))

	var handler http.Handler = mux
	handler = middleware.AccessLog(logger)(handler)
	handler = middleware.RequestID(handler)
	handler = http.MaxBytesHandler(handler, 1<<20) // 1 MiB body limit
	return handler
}
