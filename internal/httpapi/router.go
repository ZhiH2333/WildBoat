package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/handlers"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
	"github.com/ZhiH2333/WildBoat/internal/identity"
	"github.com/ZhiH2333/WildBoat/internal/match"
	"github.com/ZhiH2333/WildBoat/internal/room"
)

// NewRouter wires public HTTP routes for the modular monolith API.
func NewRouter(
	logger *slog.Logger,
	health handlers.Health,
	idSvc *identity.Service,
	matchSvc *match.Service,
	roomSvc *room.Service,
	authLimiter *middleware.RateLimiter,
) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health.Healthz)
	mux.HandleFunc("GET /readyz", health.Readyz)

	auth := handlers.Auth{Service: idSvc}
	me := handlers.Me{Service: idSvc}
	profile := handlers.Profile{Service: matchSvc}
	games := handlers.Games{Service: matchSvc}
	server := handlers.Server{Service: matchSvc}
	rooms := handlers.Rooms{Service: roomSvc}
	mm := handlers.Matchmaking{Service: roomSvc}
	requireAuth := middleware.RequireAuth(idSvc)
	requireServer := middleware.RequireServerAuth(matchSvc)

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
	mux.Handle("GET /v1/me/stats", requireAuth(http.HandlerFunc(profile.Stats)))
	mux.Handle("GET /v1/me/matches", requireAuth(http.HandlerFunc(profile.Matches)))

	mux.Handle("POST /v1/games/assign", requireAuth(http.HandlerFunc(games.Assign)))

	mux.Handle("POST /v1/rooms", requireAuth(limit(rooms.Create)))
	mux.Handle("POST /v1/rooms/join-by-code", requireAuth(limit(rooms.JoinByCode)))
	mux.Handle("GET /v1/rooms/current", requireAuth(http.HandlerFunc(rooms.Current)))
	mux.Handle("GET /v1/rooms/{room_id}", requireAuth(http.HandlerFunc(rooms.Get)))
	mux.Handle("POST /v1/rooms/{room_id}/leave", requireAuth(limit(rooms.Leave)))
	mux.Handle("PUT /v1/rooms/{room_id}/ready", requireAuth(http.HandlerFunc(rooms.Ready)))
	mux.Handle("POST /v1/rooms/{room_id}/start", requireAuth(limit(rooms.Start)))
	mux.Handle("POST /v1/rooms/{room_id}/assignment", requireAuth(limit(rooms.Assignment)))

	mux.Handle("POST /v1/matchmaking/quick", requireAuth(limit(mm.Quick)))
	mux.Handle("POST /v1/matchmaking/cancel", requireAuth(limit(mm.Cancel)))

	mux.Handle("POST /v1/server/instances/register", requireServer(http.HandlerFunc(server.RegisterInstance)))
	mux.Handle("POST /v1/server/instances/{game_instance_id}/heartbeat", requireServer(http.HandlerFunc(server.Heartbeat)))
	mux.Handle("POST /v1/server/tickets/validate", requireServer(http.HandlerFunc(server.ValidateTicket)))
	mux.Handle("POST /v1/server/matches/{match_id}/settlement", requireServer(http.HandlerFunc(server.SettleMatch)))

	var handler http.Handler = mux
	handler = middleware.AccessLog(logger)(handler)
	handler = middleware.RequestID(handler)
	handler = http.MaxBytesHandler(handler, 1<<20) // 1 MiB body limit
	return handler
}
