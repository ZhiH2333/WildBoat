package handlers

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
	"github.com/ZhiH2333/WildBoat/internal/identity"
)

// Me handles authenticated profile and session routes.
type Me struct {
	Service *identity.Service
}

type patchProfileRequest struct {
	DisplayName *string `json:"display_name"`
	AvatarID    *string `json:"avatar_id"`
	Bio         *string `json:"bio"`
}

// Get handles GET /v1/me.
func (h Me) Get(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	accountID, ok := middleware.AccountIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	me, err := h.Service.Me(r.Context(), accountID)
	if err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, me)
}

// PatchProfile handles PATCH /v1/me/profile.
func (h Me) PatchProfile(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	accountID, ok := middleware.AccountIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	var req patchProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	player, err := h.Service.UpdateProfile(r.Context(), accountID, identity.UpdateProfileInput{
		DisplayName: req.DisplayName,
		AvatarID:    req.AvatarID,
		Bio:         req.Bio,
	})
	if err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"player": player})
}

// ListSessions handles GET /v1/me/sessions.
func (h Me) ListSessions(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	accountID, ok := middleware.AccountIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	sessionID, _ := middleware.SessionIDFromContext(r.Context())
	sessions, err := h.Service.ListSessions(r.Context(), accountID, sessionID)
	if err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"sessions": sessions})
}

// DeleteSession handles DELETE /v1/me/sessions/{session_id}.
func (h Me) DeleteSession(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	accountID, ok := middleware.AccountIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	rawID := r.PathValue("session_id")
	sessionID, err := uuid.Parse(rawID)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "invalid session_id", requestID, nil)
		return
	}
	if err := h.Service.RevokeSession(r.Context(), accountID, sessionID); err != nil {
		writeIdentityError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"ok": true})
}
