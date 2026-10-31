package handlers

import (
	"net/http"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
	"github.com/ZhiH2333/WildBoat/internal/room"
)

// Matchmaking handles Quick Match routes.
type Matchmaking struct {
	Service *room.Service
}

type quickMatchRequest struct {
	Mode string `json:"mode"`
}

// Quick handles POST /v1/matchmaking/quick.
func (h Matchmaking) Quick(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	var req quickMatchRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	detail, created, err := h.Service.QuickMatch(r.Context(), playerID, req.Mode)
	if err != nil {
		writeRoomError(w, requestID, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	response.JSON(w, status, detail)
}

// Cancel handles POST /v1/matchmaking/cancel.
func (h Matchmaking) Cancel(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	if err := h.Service.CancelMatchmaking(r.Context(), playerID); err != nil {
		writeRoomError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"ok": true})
}
