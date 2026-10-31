package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
)

// Pinger checks dependency readiness.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Health exposes process and dependency health endpoints.
type Health struct {
	DB Pinger
}

type healthResponse struct {
	Status string `json:"status"`
}

// Healthz reports process liveness.
func (h Health) Healthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", middleware.RequestIDFromContext(r.Context()), nil)
		return
	}
	response.JSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

// Readyz reports dependency readiness, including PostgreSQL.
func (h Health) Readyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Error(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", middleware.RequestIDFromContext(r.Context()), nil)
		return
	}

	requestID := middleware.RequestIDFromContext(r.Context())
	if h.DB == nil {
		response.Error(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database is not configured", requestID, nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.DB.Ping(ctx); err != nil {
		response.Error(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "database is not ready", requestID, nil)
		return
	}

	response.JSON(w, http.StatusOK, healthResponse{Status: "ready"})
}
