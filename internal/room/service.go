package room

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/match"
	"github.com/ZhiH2333/WildBoat/internal/security"
)

// Config holds room service runtime settings.
type Config struct {
	IdleTTL      time.Duration
	TicketTTL    time.Duration
	HeartbeatTTL time.Duration
}

// Service implements multiplayer lobby use cases.
type Service struct {
	store      *Store
	matchStore *match.Store
	cfg        Config
	log        *slog.Logger
	now        func() time.Time
}

// NewService constructs the room application service.
func NewService(store *Store, matchStore *match.Store, cfg Config, log *slog.Logger) *Service {
	if cfg.IdleTTL <= 0 {
		cfg.IdleTTL = 30 * time.Minute
	}
	if cfg.TicketTTL <= 0 {
		cfg.TicketTTL = 60 * time.Second
	}
	if cfg.HeartbeatTTL <= 0 {
		cfg.HeartbeatTTL = 30 * time.Second
	}
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store:      store,
		matchStore: matchStore,
		cfg:        cfg,
		log:        log,
		now:        func() time.Time { return time.Now().UTC() },
	}
}

// Detail is the room detail response shape.
type Detail struct {
	Room                *RoomDTO    `json:"room"`
	Members             []MemberDTO `json:"members"`
	Self                *SelfDTO    `json:"self"`
	AssignmentAvailable bool        `json:"assignment_available"`
}

// RoomDTO is the public room JSON.
type RoomDTO struct {
	RoomID        string    `json:"room_id"`
	RoomCode      string    `json:"room_code"`
	OwnerPlayerID string    `json:"owner_player_id"`
	Mode          string    `json:"mode"`
	Visibility    string    `json:"visibility"`
	State         string    `json:"state"`
	MaxPlayers    int       `json:"max_players"`
	MemberCount   int       `json:"member_count"`
	MatchID       *string   `json:"match_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}

// MemberDTO is a public member JSON.
type MemberDTO struct {
	PlayerID    string    `json:"player_id"`
	DisplayName string    `json:"display_name"`
	AvatarID    string    `json:"avatar_id"`
	Ready       bool      `json:"ready"`
	IsOwner     bool      `json:"is_owner"`
	JoinedAt    time.Time `json:"joined_at"`
}

// SelfDTO is the caller's membership summary.
type SelfDTO struct {
	PlayerID string `json:"player_id"`
	Ready    bool   `json:"ready"`
	IsOwner  bool   `json:"is_owner"`
}

// StartResult is returned from host start.
type StartResult struct {
	Room       RoomDTO          `json:"room"`
	Assignment match.Assignment `json:"assignment"`
}

// AssignmentResult wraps a claimed assignment.
type AssignmentResult struct {
	Assignment match.Assignment `json:"assignment"`
}

func (s *Service) toDetail(room Room, members []Member, playerID uuid.UUID) Detail {
	dto := toRoomDTO(room, len(members))
	memberDTOs := make([]MemberDTO, 0, len(members))
	var self *SelfDTO
	for _, m := range members {
		isOwner := m.PlayerID == room.OwnerPlayerID
		memberDTOs = append(memberDTOs, MemberDTO{
			PlayerID:    m.PlayerID.String(),
			DisplayName: m.DisplayName,
			AvatarID:    m.AvatarID,
			Ready:       m.Ready,
			IsOwner:     isOwner,
			JoinedAt:    m.JoinedAt.UTC(),
		})
		if m.PlayerID == playerID {
			self = &SelfDTO{
				PlayerID: playerID.String(),
				Ready:    m.Ready,
				IsOwner:  isOwner,
			}
		}
	}
	assignable := (room.State == StateStarting || room.State == StateInGame) && self != nil
	return Detail{
		Room:                &dto,
		Members:             memberDTOs,
		Self:                self,
		AssignmentAvailable: assignable,
	}
}

func toRoomDTO(room Room, memberCount int) RoomDTO {
	var matchID *string
	if room.MatchID != nil {
		s := room.MatchID.String()
		matchID = &s
	}
	return RoomDTO{
		RoomID:        room.ID.String(),
		RoomCode:      room.RoomCode,
		OwnerPlayerID: room.OwnerPlayerID.String(),
		Mode:          room.Mode,
		Visibility:    room.Visibility,
		State:         room.State,
		MaxPlayers:    room.MaxPlayers,
		MemberCount:   memberCount,
		MatchID:       matchID,
		CreatedAt:     room.CreatedAt.UTC(),
		UpdatedAt:     room.UpdatedAt.UTC(),
		ExpiresAt:     room.ExpiresAt.UTC(),
	}
}

func assignmentFromMeta(meta match.RosterAssignmentMeta, rawTicket string) match.Assignment {
	return match.Assignment{
		GameInstanceID:  meta.GameInstanceID,
		MatchID:         meta.MatchID,
		ServerID:        meta.ServerID,
		Host:            meta.Host,
		Port:            meta.Port,
		Transport:       meta.Transport,
		GameTicket:      rawTicket,
		TicketExpiresAt: meta.TicketExpiresAt.UTC(),
		ProtocolVersion: meta.ProtocolVersion,
		ServerBuild:     meta.ServerBuild,
		Mode:            meta.Mode,
	}
}

// CreatePrivate creates a private room.
func (s *Service) CreatePrivate(ctx context.Context, playerID uuid.UUID, mode string, maxPlayers int) (Detail, error) {
	mode = normalizeMode(mode)
	if !match.IsValidMode(mode) {
		return Detail{}, ErrInvalidInput
	}
	if maxPlayers == 0 {
		maxPlayers = 4
	}
	if maxPlayers < 2 || maxPlayers > 4 {
		return Detail{}, ErrInvalidInput
	}
	room, members, err := s.store.CreatePrivateRoom(ctx, playerID, mode, maxPlayers, s.cfg.IdleTTL, s.now())
	if err != nil {
		return Detail{}, err
	}
	return s.toDetail(room, members, playerID), nil
}

// JoinByCode joins via server-generated room code.
func (s *Service) JoinByCode(ctx context.Context, playerID uuid.UUID, code string) (Detail, error) {
	code = NormalizeRoomCode(code)
	if !IsValidRoomCodeFormat(code) {
		return Detail{}, ErrCodeInvalid
	}
	room, members, err := s.store.JoinByCode(ctx, playerID, code, s.now())
	if err != nil {
		return Detail{}, err
	}
	return s.toDetail(room, members, playerID), nil
}

// Get returns room detail for a member.
func (s *Service) Get(ctx context.Context, roomID, playerID uuid.UUID) (Detail, error) {
	room, members, err := s.store.GetRoomForMember(ctx, roomID, playerID, s.now())
	if err != nil {
		return Detail{}, err
	}
	return s.toDetail(room, members, playerID), nil
}

// Current returns the caller's current room or an empty detail.
func (s *Service) Current(ctx context.Context, playerID uuid.UUID) (Detail, error) {
	room, members, ok, err := s.store.GetCurrentRoom(ctx, playerID, s.now())
	if err != nil {
		return Detail{}, err
	}
	if !ok {
		return Detail{
			Members:             []MemberDTO{},
			AssignmentAvailable: false,
		}, nil
	}
	return s.toDetail(room, members, playerID), nil
}

// Leave leaves a room (idempotent for non-members of existing rooms).
func (s *Service) Leave(ctx context.Context, roomID, playerID uuid.UUID) error {
	return s.store.Leave(ctx, roomID, playerID, s.now())
}

// SetReady updates ready state.
func (s *Service) SetReady(ctx context.Context, roomID, playerID uuid.UUID, ready bool) (Detail, error) {
	room, members, err := s.store.SetReady(ctx, roomID, playerID, ready, s.now())
	if err != nil {
		return Detail{}, err
	}
	return s.toDetail(room, members, playerID), nil
}

// Start starts the match for the room owner.
func (s *Service) Start(ctx context.Context, roomID, playerID uuid.UUID) (StartResult, error) {
	room, members, rawByPlayer, ownerMeta, err := s.store.StartMatch(
		ctx,
		roomID,
		playerID,
		s.matchStore,
		s.cfg.TicketTTL,
		s.cfg.HeartbeatTTL,
		s.now(),
		security.NewOpaqueToken,
	)
	if err != nil {
		return StartResult{}, err
	}
	raw := rawByPlayer[playerID]
	s.log.Info("room_started",
		"room_id", room.ID.String(),
		"match_id", ownerMeta.MatchID.String(),
		"member_count", len(members),
	)
	return StartResult{
		Room:       toRoomDTO(room, len(members)),
		Assignment: assignmentFromMeta(ownerMeta, raw),
	}, nil
}

// ClaimAssignment returns a fresh ticket for a room member after start.
func (s *Service) ClaimAssignment(ctx context.Context, roomID, playerID uuid.UUID) (AssignmentResult, error) {
	meta, raw, err := s.store.ClaimAssignment(
		ctx,
		roomID,
		playerID,
		s.matchStore,
		s.cfg.TicketTTL,
		s.now(),
		security.NewOpaqueToken,
	)
	if err != nil {
		return AssignmentResult{}, err
	}
	return AssignmentResult{Assignment: assignmentFromMeta(meta, raw)}, nil
}

// QuickMatch finds or creates a public lobby for mode.
func (s *Service) QuickMatch(ctx context.Context, playerID uuid.UUID, mode string) (Detail, bool, error) {
	mode = normalizeMode(mode)
	if !match.IsValidMode(mode) {
		return Detail{}, false, ErrInvalidInput
	}
	room, members, created, err := s.store.FindOrCreatePublicRoom(ctx, playerID, mode, 4, s.cfg.IdleTTL, s.now())
	if err != nil {
		return Detail{}, false, err
	}
	return s.toDetail(room, members, playerID), created, nil
}

// CancelMatchmaking leaves the current public room if any.
func (s *Service) CancelMatchmaking(ctx context.Context, playerID uuid.UUID) error {
	room, _, ok, err := s.store.GetCurrentRoom(ctx, playerID, s.now())
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if room.Visibility != VisibilityPublic {
		// Still allow leave of current room via cancel as "leave current".
	}
	return s.store.Leave(ctx, room.ID, playerID, s.now())
}

// SweepExpired expires idle open rooms.
func (s *Service) SweepExpired(ctx context.Context) (int64, error) {
	n, err := s.store.ExpireIdleRooms(ctx, s.now())
	if err != nil {
		return 0, err
	}
	if n > 0 {
		s.log.Info("rooms_expired", "count", n)
	}
	return n, nil
}

func normalizeMode(mode string) string {
	return strings.TrimSpace(mode)
}
