package handlers

import (
	"errors"
	"net/http"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
	"github.com/ZhiH2333/WildBoat/internal/match"
)

// Games handles player-facing game assignment routes.
type Games struct {
	Service *match.Service
}

type assignRequest struct {
	Mode string `json:"mode"`
}

// Assign handles POST /v1/games/assign.
func (h Games) Assign(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	var req assignRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	asg, err := h.Service.Assign(r.Context(), playerID, req.Mode)
	if err != nil {
		writeMatchError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, asg)
}

func writeMatchError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, match.ErrInvalidInput):
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "request validation failed", requestID, nil)
	case errors.Is(err, match.ErrUnauthorized):
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
	case errors.Is(err, match.ErrForbidden):
		response.Error(w, http.StatusForbidden, "FORBIDDEN", "forbidden", requestID, nil)
	case errors.Is(err, match.ErrNotFound):
		response.Error(w, http.StatusNotFound, "NOT_FOUND", "resource not found", requestID, nil)
	case errors.Is(err, match.ErrServerAuthInvalid):
		response.Error(w, http.StatusUnauthorized, "SERVER_AUTH_INVALID", "dedicated server authentication failed", requestID, nil)
	case errors.Is(err, match.ErrGameServerUnavailable):
		response.Error(w, http.StatusServiceUnavailable, "GAME_SERVER_UNAVAILABLE", "no dedicated server is currently available", requestID, nil)
	case errors.Is(err, match.ErrGameCapacityFull):
		response.Error(w, http.StatusConflict, "GAME_CAPACITY_FULL", "no game slot is currently available", requestID, nil)
	case errors.Is(err, match.ErrTicketInvalid):
		response.Error(w, http.StatusBadRequest, "TICKET_INVALID", "game ticket is invalid", requestID, nil)
	case errors.Is(err, match.ErrTicketExpired):
		response.Error(w, http.StatusConflict, "TICKET_EXPIRED", "game ticket has expired", requestID, nil)
	case errors.Is(err, match.ErrTicketReplayed):
		response.Error(w, http.StatusConflict, "TICKET_REPLAYED", "game ticket was already used", requestID, nil)
	case errors.Is(err, match.ErrTicketRevoked):
		response.Error(w, http.StatusConflict, "TICKET_REVOKED", "game ticket was revoked", requestID, nil)
	case errors.Is(err, match.ErrMatchNotSettleable):
		response.Error(w, http.StatusConflict, "MATCH_NOT_SETTLEABLE", "match cannot be settled in its current state", requestID, nil)
	default:
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", requestID, nil)
	}
}
