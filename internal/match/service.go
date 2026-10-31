package match

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/security"
)

// Config holds match service runtime settings.
type Config struct {
	TicketTTL        time.Duration
	HeartbeatTTL     time.Duration
	DefaultCapacity  int
	ServerCredential string
	ServerLabel      string
}

// RegisterInstanceInput is the DS registration payload.
type RegisterInstanceInput struct {
	Host            string
	Port            int
	Transport       string
	ProtocolVersion int
	ServerBuild     string
	Capacity        int
	Status          string
}

// HeartbeatInput is the DS heartbeat payload.
type HeartbeatInput struct {
	Status        string
	OccupiedSlots *int
	ActiveMatchID *uuid.UUID
}

// ValidateTicketInput is the DS ticket validation payload.
type ValidateTicketInput struct {
	GameTicket     string
	PlayerID       uuid.UUID
	MatchID        uuid.UUID
	GameInstanceID uuid.UUID
}

// ValidateTicketResult is returned after successful consume.
type ValidateTicketResult struct {
	Valid          bool      `json:"valid"`
	PlayerID       uuid.UUID `json:"player_id"`
	MatchID        uuid.UUID `json:"match_id"`
	GameInstanceID uuid.UUID `json:"game_instance_id"`
	ServerID       uuid.UUID `json:"server_id"`
	ConsumedAt     time.Time `json:"consumed_at"`
}

// SettlementPlayerInput is one player result line.
type SettlementPlayerInput struct {
	PlayerID uuid.UUID `json:"player_id"`
	TeamID   string    `json:"team_id"`
	Kills    int       `json:"kills"`
	Deaths   int       `json:"deaths"`
	Score    int       `json:"score"`
}

// SettlementInput is the DS settlement payload.
type SettlementInput struct {
	SettlementID uuid.UUID
	MatchID      uuid.UUID
	Result       string
	StartedAt    time.Time
	EndedAt      time.Time
	Players      []SettlementPlayerInput
	ModeStats    map[string]any
}

// SettlementResult is returned after settlement processing.
type SettlementResult struct {
	OK               bool      `json:"ok"`
	SettlementID     uuid.UUID `json:"settlement_id"`
	MatchID          uuid.UUID `json:"match_id"`
	AlreadyProcessed bool      `json:"already_processed"`
}

// Service implements Phase 2B game assignment / DS / ticket / settlement use cases.
type Service struct {
	store *Store
	cfg   Config
	log   *slog.Logger
	now   func() time.Time
}

// NewService constructs the match application service.
func NewService(store *Store, cfg Config, log *slog.Logger) *Service {
	if cfg.TicketTTL <= 0 {
		cfg.TicketTTL = 60 * time.Second
	}
	if cfg.HeartbeatTTL <= 0 {
		cfg.HeartbeatTTL = 30 * time.Second
	}
	if cfg.DefaultCapacity <= 0 {
		cfg.DefaultCapacity = 4
	}
	if cfg.DefaultCapacity > 4 {
		cfg.DefaultCapacity = 4
	}
	if cfg.ServerLabel == "" {
		cfg.ServerLabel = "default"
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store: store,
		cfg:   cfg,
		log:   log,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

// HeartbeatTTLSeconds exposes the configured heartbeat TTL for DS clients.
func (s *Service) HeartbeatTTLSeconds() int {
	return int(s.cfg.HeartbeatTTL / time.Second)
}

// EnsureBootstrapServer upserts the configured dedicated server credential.
func (s *Service) EnsureBootstrapServer(ctx context.Context) error {
	cred := strings.TrimSpace(s.cfg.ServerCredential)
	if cred == "" {
		return ErrInvalidInput
	}
	hash := security.HashOpaqueToken(cred)
	_, err := s.store.EnsureServer(ctx, hash, s.cfg.ServerLabel)
	return err
}

// AuthenticateServer validates the dedicated server credential and returns server_id.
func (s *Service) AuthenticateServer(ctx context.Context, credential string) (uuid.UUID, error) {
	cred := strings.TrimSpace(credential)
	if cred == "" {
		return uuid.Nil, ErrServerAuthInvalid
	}
	server, err := s.store.GetServerByCredentialHash(ctx, security.HashOpaqueToken(cred))
	if err != nil {
		return uuid.Nil, err
	}
	return server.ID, nil
}

// RegisterInstance registers or replaces the active game instance for a server.
func (s *Service) RegisterInstance(ctx context.Context, serverID uuid.UUID, in RegisterInstanceInput) (GameInstance, error) {
	in.Host = strings.TrimSpace(in.Host)
	in.ServerBuild = strings.TrimSpace(in.ServerBuild)
	in.Transport = strings.TrimSpace(in.Transport)
	if in.Transport == "" {
		in.Transport = TransportENetUDP
	}
	if in.Host == "" || in.Port < 1 || in.Port > 65535 || in.ServerBuild == "" {
		return GameInstance{}, ErrInvalidInput
	}
	if in.Transport != TransportENetUDP {
		return GameInstance{}, ErrInvalidInput
	}
	if in.ProtocolVersion <= 0 {
		in.ProtocolVersion = 1
	}
	if in.Capacity <= 0 {
		in.Capacity = s.cfg.DefaultCapacity
	}
	if in.Capacity > 4 {
		in.Capacity = 4
	}
	if in.Status == "" {
		in.Status = StatusStarting
	}
	if !IsValidInstanceStatus(in.Status) {
		return GameInstance{}, ErrInvalidInput
	}

	inst, err := s.store.RegisterInstance(ctx, serverID, in, s.now())
	if err != nil {
		return GameInstance{}, err
	}
	s.log.Info("game_instance_registered",
		"server_id", serverID.String(),
		"game_instance_id", inst.ID.String(),
		"status", inst.Status,
		"capacity", inst.Capacity,
	)
	return inst, nil
}

// Heartbeat records DS liveness.
func (s *Service) Heartbeat(ctx context.Context, serverID, instanceID uuid.UUID, in HeartbeatInput) (GameInstance, error) {
	if in.Status != "" && !IsValidInstanceStatus(in.Status) {
		return GameInstance{}, ErrInvalidInput
	}
	inst, err := s.store.Heartbeat(ctx, instanceID, serverID, in, s.now())
	if err != nil {
		return GameInstance{}, err
	}
	return inst, nil
}

// Assign creates or resumes a match assignment for an authenticated player.
func (s *Service) Assign(ctx context.Context, playerID uuid.UUID, mode string) (Assignment, error) {
	mode = strings.TrimSpace(mode)
	if !IsValidMode(mode) {
		return Assignment{}, ErrInvalidInput
	}
	raw, hash, err := security.NewOpaqueToken()
	if err != nil {
		return Assignment{}, err
	}
	asg, err := s.store.AssignPlayer(ctx, playerID, mode, hash, s.cfg.TicketTTL, s.cfg.HeartbeatTTL, s.now())
	if err != nil {
		return Assignment{}, err
	}
	asg.GameTicket = raw
	s.log.Info("game_assigned",
		"player_id", playerID.String(),
		"match_id", asg.MatchID.String(),
		"game_instance_id", asg.GameInstanceID.String(),
		"mode", asg.Mode,
	)
	return asg, nil
}

// ValidateTicket consumes a one-time ticket for DS admission.
func (s *Service) ValidateTicket(ctx context.Context, serverID uuid.UUID, in ValidateTicketInput) (ValidateTicketResult, error) {
	raw := strings.TrimSpace(in.GameTicket)
	if raw == "" || in.PlayerID == uuid.Nil || in.MatchID == uuid.Nil || in.GameInstanceID == uuid.Nil {
		return ValidateTicketResult{}, ErrInvalidInput
	}
	ticket, err := s.store.ValidateAndConsumeTicket(
		ctx,
		serverID,
		security.HashOpaqueToken(raw),
		in.PlayerID,
		in.MatchID,
		in.GameInstanceID,
		s.now(),
	)
	if err != nil {
		return ValidateTicketResult{}, err
	}
	consumed := s.now()
	if ticket.ConsumedAt != nil {
		consumed = *ticket.ConsumedAt
	}
	return ValidateTicketResult{
		Valid:          true,
		PlayerID:       ticket.PlayerID,
		MatchID:        ticket.MatchID,
		GameInstanceID: ticket.GameInstanceID,
		ServerID:       ticket.ServerID,
		ConsumedAt:     consumed,
	}, nil
}

// SettleMatch processes an idempotent trusted settlement from the DS.
func (s *Service) SettleMatch(ctx context.Context, serverID uuid.UUID, in SettlementInput) (SettlementResult, error) {
	if in.SettlementID == uuid.Nil || in.MatchID == uuid.Nil {
		return SettlementResult{}, ErrInvalidInput
	}
	result := strings.TrimSpace(in.Result)
	if result != "completed" && result != "aborted" {
		return SettlementResult{}, ErrInvalidInput
	}
	if in.EndedAt.Before(in.StartedAt) {
		return SettlementResult{}, ErrInvalidInput
	}
	if len(in.Players) == 0 {
		return SettlementResult{}, ErrInvalidInput
	}
	for _, p := range in.Players {
		if p.PlayerID == uuid.Nil || p.Kills < 0 || p.Deaths < 0 {
			return SettlementResult{}, ErrInvalidInput
		}
	}

	rows := make([]SettlementPlayerResult, 0, len(in.Players))
	for _, p := range in.Players {
		rows = append(rows, SettlementPlayerResult{
			PlayerID: p.PlayerID,
			TeamID:   strings.TrimSpace(p.TeamID),
			Kills:    p.Kills,
			Deaths:   p.Deaths,
			Score:    p.Score,
		})
	}

	payload, err := json.Marshal(map[string]any{
		"settlement_id": in.SettlementID,
		"match_id":      in.MatchID,
		"result":        result,
		"started_at":    in.StartedAt.UTC(),
		"ended_at":      in.EndedAt.UTC(),
		"players":       in.Players,
		"mode_stats":    in.ModeStats,
	})
	if err != nil {
		return SettlementResult{}, err
	}

	already, err := s.store.ProcessSettlement(
		ctx,
		serverID,
		in.MatchID,
		in.SettlementID,
		result,
		in.StartedAt.UTC(),
		in.EndedAt.UTC(),
		rows,
		payload,
		s.now(),
	)
	if err != nil {
		return SettlementResult{}, err
	}
	s.log.Info("match_settled",
		"settlement_id", in.SettlementID.String(),
		"match_id", in.MatchID.String(),
		"server_id", serverID.String(),
		"already_processed", already,
	)
	return SettlementResult{
		OK:               true,
		SettlementID:     in.SettlementID,
		MatchID:          in.MatchID,
		AlreadyProcessed: already,
	}, nil
}
