package handlers

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
	"github.com/ZhiH2333/WildBoat/internal/room"
)

// Rooms handles multiplayer lobby routes.
type Rooms struct {
	Service *room.Service
}

type createRoomRequest struct {
	Mode       string `json:"mode"`
	Visibility string `json:"visibility"`
	MaxPlayers int    `json:"max_players"`
}

type joinByCodeRequest struct {
	RoomCode string `json:"room_code"`
}

type readyRequest struct {
	Ready *bool `json:"ready"`
}

// Create handles POST /v1/rooms.
func (h Rooms) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	var req createRoomRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	if req.Visibility == "" {
		req.Visibility = room.VisibilityPrivate
	}
	if req.Visibility != room.VisibilityPrivate {
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "visibility must be private for this endpoint", requestID, nil)
		return
	}
	detail, err := h.Service.CreatePrivate(r.Context(), playerID, req.Mode, req.MaxPlayers)
	if err != nil {
		writeRoomError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusCreated, detail)
}

// JoinByCode handles POST /v1/rooms/join-by-code.
func (h Rooms) JoinByCode(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	var req joinByCodeRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	detail, err := h.Service.JoinByCode(r.Context(), playerID, req.RoomCode)
	if err != nil {
		writeRoomError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, detail)
}

// Current handles GET /v1/rooms/current.
func (h Rooms) Current(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	detail, err := h.Service.Current(r.Context(), playerID)
	if err != nil {
		writeRoomError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, detail)
}

// Get handles GET /v1/rooms/{room_id}.
func (h Rooms) Get(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	roomID, err := uuid.Parse(r.PathValue("room_id"))
	if err != nil {
		response.Error(w, http.StatusNotFound, "ROOM_NOT_FOUND", "room not found", requestID, nil)
		return
	}
	detail, err := h.Service.Get(r.Context(), roomID, playerID)
	if err != nil {
		writeRoomError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, detail)
}

// Leave handles POST /v1/rooms/{room_id}/leave.
func (h Rooms) Leave(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	roomID, err := uuid.Parse(r.PathValue("room_id"))
	if err != nil {
		response.Error(w, http.StatusNotFound, "ROOM_NOT_FOUND", "room not found", requestID, nil)
		return
	}
	if err := h.Service.Leave(r.Context(), roomID, playerID); err != nil {
		writeRoomError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Ready handles PUT /v1/rooms/{room_id}/ready.
func (h Rooms) Ready(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	roomID, err := uuid.Parse(r.PathValue("room_id"))
	if err != nil {
		response.Error(w, http.StatusNotFound, "ROOM_NOT_FOUND", "room not found", requestID, nil)
		return
	}
	var req readyRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	if req.Ready == nil {
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "ready is required", requestID, nil)
		return
	}
	detail, err := h.Service.SetReady(r.Context(), roomID, playerID, *req.Ready)
	if err != nil {
		writeRoomError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, detail)
}

// Start handles POST /v1/rooms/{room_id}/start.
func (h Rooms) Start(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	roomID, err := uuid.Parse(r.PathValue("room_id"))
	if err != nil {
		response.Error(w, http.StatusNotFound, "ROOM_NOT_FOUND", "room not found", requestID, nil)
		return
	}
	result, err := h.Service.Start(r.Context(), roomID, playerID)
	if err != nil {
		writeRoomError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}

// Assignment handles POST /v1/rooms/{room_id}/assignment.
func (h Rooms) Assignment(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	roomID, err := uuid.Parse(r.PathValue("room_id"))
	if err != nil {
		response.Error(w, http.StatusNotFound, "ROOM_NOT_FOUND", "room not found", requestID, nil)
		return
	}
	result, err := h.Service.ClaimAssignment(r.Context(), roomID, playerID)
	if err != nil {
		writeRoomError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}

func writeRoomError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case errors.Is(err, room.ErrInvalidInput):
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "request validation failed", requestID, nil)
	case errors.Is(err, room.ErrUnauthorized):
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
	case errors.Is(err, room.ErrForbidden):
		response.Error(w, http.StatusForbidden, "FORBIDDEN", "forbidden", requestID, nil)
	case errors.Is(err, room.ErrNotFound):
		response.Error(w, http.StatusNotFound, "ROOM_NOT_FOUND", "room not found", requestID, nil)
	case errors.Is(err, room.ErrCodeInvalid):
		response.Error(w, http.StatusNotFound, "ROOM_CODE_INVALID", "room code is invalid or expired", requestID, nil)
	case errors.Is(err, room.ErrFull):
		response.Error(w, http.StatusConflict, "ROOM_FULL", "room is full", requestID, nil)
	case errors.Is(err, room.ErrNotJoinable):
		response.Error(w, http.StatusConflict, "ROOM_NOT_JOINABLE", "room is not joinable in its current state", requestID, nil)
	case errors.Is(err, room.ErrAlreadyIn):
		response.Error(w, http.StatusConflict, "ROOM_ALREADY_IN", "player is already in an active room", requestID, nil)
	case errors.Is(err, room.ErrNotReady):
		response.Error(w, http.StatusConflict, "ROOM_NOT_READY", "not all members are ready", requestID, nil)
	case errors.Is(err, room.ErrAlreadyStarted):
		response.Error(w, http.StatusConflict, "ROOM_ALREADY_STARTED", "room has already started", requestID, nil)
	case errors.Is(err, room.ErrNotOwner):
		response.Error(w, http.StatusForbidden, "NOT_ROOM_OWNER", "only the room owner can perform this action", requestID, nil)
	case errors.Is(err, room.ErrAssignmentNotReady):
		response.Error(w, http.StatusConflict, "ASSIGNMENT_NOT_READY", "room assignment is not ready yet", requestID, nil)
	case errors.Is(err, room.ErrMatchAlreadyActive):
		response.Error(w, http.StatusConflict, "MATCH_ALREADY_ACTIVE", "player already has an active match", requestID, nil)
	case errors.Is(err, room.ErrGameServerUnavailable):
		response.Error(w, http.StatusServiceUnavailable, "GAME_SERVER_UNAVAILABLE", "no dedicated server is currently available", requestID, nil)
	case errors.Is(err, room.ErrGameCapacityFull):
		response.Error(w, http.StatusConflict, "GAME_CAPACITY_FULL", "no game slot is currently available", requestID, nil)
	default:
		response.Error(w, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error", requestID, nil)
	}
}
