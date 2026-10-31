package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/httpapi/middleware"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/response"
	"github.com/ZhiH2333/WildBoat/internal/match"
)

// Server handles dedicated-server-facing routes.
type Server struct {
	Service *match.Service
}

type registerInstanceRequest struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Transport       string `json:"transport"`
	ProtocolVersion int    `json:"protocol_version"`
	ServerBuild     string `json:"server_build"`
	Capacity        int    `json:"capacity"`
	Status          string `json:"status"`
}

type heartbeatRequest struct {
	Status        string  `json:"status"`
	OccupiedSlots *int    `json:"occupied_slots"`
	ActiveMatchID *string `json:"active_match_id"`
}

type validateTicketRequest struct {
	GameTicket     string `json:"game_ticket"`
	PlayerID       string `json:"player_id"`
	MatchID        string `json:"match_id"`
	GameInstanceID string `json:"game_instance_id"`
}

type settlementPlayerRequest struct {
	PlayerID string `json:"player_id"`
	TeamID   string `json:"team_id"`
	Kills    int    `json:"kills"`
	Deaths   int    `json:"deaths"`
	Score    int    `json:"score"`
}

type settlementRequest struct {
	SettlementID string                    `json:"settlement_id"`
	Result       string                    `json:"result"`
	StartedAt    time.Time                 `json:"started_at"`
	EndedAt      time.Time                 `json:"ended_at"`
	Players      []settlementPlayerRequest `json:"players"`
	ModeStats    map[string]any            `json:"mode_stats"`
}

// RegisterInstance handles POST /v1/server/instances/register.
func (h Server) RegisterInstance(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	serverID, ok := middleware.ServerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "SERVER_AUTH_INVALID", "dedicated server authentication required", requestID, nil)
		return
	}
	var req registerInstanceRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	inst, err := h.Service.RegisterInstance(r.Context(), serverID, match.RegisterInstanceInput{
		Host:            req.Host,
		Port:            req.Port,
		Transport:       req.Transport,
		ProtocolVersion: req.ProtocolVersion,
		ServerBuild:     req.ServerBuild,
		Capacity:        req.Capacity,
		Status:          req.Status,
	})
	if err != nil {
		writeMatchError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"server_id":             inst.ServerID,
		"game_instance_id":      inst.ID,
		"status":                inst.Status,
		"capacity":              inst.Capacity,
		"heartbeat_ttl_seconds": h.Service.HeartbeatTTLSeconds(),
	})
}

// Heartbeat handles POST /v1/server/instances/{game_instance_id}/heartbeat.
func (h Server) Heartbeat(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	serverID, ok := middleware.ServerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "SERVER_AUTH_INVALID", "dedicated server authentication required", requestID, nil)
		return
	}
	instanceID, err := uuid.Parse(r.PathValue("game_instance_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "invalid game_instance_id", requestID, nil)
		return
	}
	var req heartbeatRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	var activeMatchID *uuid.UUID
	if req.ActiveMatchID != nil && *req.ActiveMatchID != "" {
		id, parseErr := uuid.Parse(*req.ActiveMatchID)
		if parseErr != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "invalid active_match_id", requestID, nil)
			return
		}
		activeMatchID = &id
	}
	inst, err := h.Service.Heartbeat(r.Context(), serverID, instanceID, match.HeartbeatInput{
		Status:        req.Status,
		OccupiedSlots: req.OccupiedSlots,
		ActiveMatchID: activeMatchID,
	})
	if err != nil {
		writeMatchError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"status":      inst.Status,
		"server_time": time.Now().UTC(),
	})
}

// ValidateTicket handles POST /v1/server/tickets/validate.
func (h Server) ValidateTicket(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	serverID, ok := middleware.ServerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "SERVER_AUTH_INVALID", "dedicated server authentication required", requestID, nil)
		return
	}
	var req validateTicketRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	playerID, err := uuid.Parse(req.PlayerID)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "invalid player_id", requestID, nil)
		return
	}
	matchID, err := uuid.Parse(req.MatchID)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "invalid match_id", requestID, nil)
		return
	}
	instanceID, err := uuid.Parse(req.GameInstanceID)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "invalid game_instance_id", requestID, nil)
		return
	}
	result, err := h.Service.ValidateTicket(r.Context(), serverID, match.ValidateTicketInput{
		GameTicket:     req.GameTicket,
		PlayerID:       playerID,
		MatchID:        matchID,
		GameInstanceID: instanceID,
	})
	if err != nil {
		writeMatchError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}

// SettleMatch handles POST /v1/server/matches/{match_id}/settlement.
func (h Server) SettleMatch(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestIDFromContext(r.Context())
	serverID, ok := middleware.ServerIDFromContext(r.Context())
	if !ok {
		response.Error(w, http.StatusUnauthorized, "SERVER_AUTH_INVALID", "dedicated server authentication required", requestID, nil)
		return
	}
	matchID, err := uuid.Parse(r.PathValue("match_id"))
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "invalid match_id", requestID, nil)
		return
	}
	var req settlementRequest
	if err := decodeJSON(r, &req); err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_JSON", "invalid request body", requestID, nil)
		return
	}
	settlementID, err := uuid.Parse(req.SettlementID)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "invalid settlement_id", requestID, nil)
		return
	}
	players := make([]match.SettlementPlayerInput, 0, len(req.Players))
	for _, p := range req.Players {
		pid, parseErr := uuid.Parse(p.PlayerID)
		if parseErr != nil {
			response.Error(w, http.StatusBadRequest, "INVALID_INPUT", "invalid player_id in players", requestID, nil)
			return
		}
		players = append(players, match.SettlementPlayerInput{
			PlayerID: pid,
			TeamID:   p.TeamID,
			Kills:    p.Kills,
			Deaths:   p.Deaths,
			Score:    p.Score,
		})
	}
	result, err := h.Service.SettleMatch(r.Context(), serverID, match.SettlementInput{
		SettlementID: settlementID,
		MatchID:      matchID,
		Result:       req.Result,
		StartedAt:    req.StartedAt,
		EndedAt:      req.EndedAt,
		Players:      players,
		ModeStats:    req.ModeStats,
	})
	if err != nil {
		writeMatchError(w, requestID, err)
		return
	}
	response.JSON(w, http.StatusOK, result)
}
