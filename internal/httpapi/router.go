package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/handlers"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
)

// NewRouter wires public HTTP routes for the modular monolith API.
func NewRouter(logger *slog.Logger, health handlers.Health) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", health.Healthz)
	mux.HandleFunc("/readyz", health.Readyz)

	var handler http.Handler = mux
	handler = middleware.AccessLog(logger)(handler)
	handler = middleware.RequestID(handler)
	handler = http.MaxBytesHandler(handler, 1<<20) // 1 MiB body limit
	return handler
}
