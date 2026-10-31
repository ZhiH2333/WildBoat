package handlers

import (
	"net/http"
	"strconv"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
	"github.com/ZhiH2333/WildBoat/internal/match"
)

// Profile handles authenticated stats / match history reads.
type Profile struct {
	Service *match.Service
}

// Stats handles GET /v1/me/stats.
func (h Profile) Stats(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	stats, err := h.Service.GetPlayerStats(r.Context(), playerID)
	if err != nil {
		writeMatchError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, stats)
}

// Matches handles GET /v1/me/matches.
func (h Profile) Matches(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	playerID, ok := middleware.PlayerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "AUTH_REQUIRED", "authentication required", requestID, nil)
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "invalid limit", requestID, nil)
			return
		}
		limit = n
	}
	cursor := r.URL.Query().Get("cursor")
	items, next, err := h.Service.ListMatchHistory(r.Context(), playerID, limit, cursor)
	if err != nil {
		writeMatchError(w, requestID, err)
		return
	}
	if items == nil {
		items = []match.MatchHistoryItem{}
	}
	var nextCursor any
	if next != "" {
		nextCursor = next
	} else {
		nextCursor = nil
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"items":       items,
		"next_cursor": nextCursor,
	})
}
