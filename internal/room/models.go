package room

import (
	"time"

	"github.com/google/uuid"
)

// Room states.
const (
	StateOpen     = "open"
	StateStarting = "starting"
	StateInGame   = "in_game"
	StateClosed   = "closed"
	StateExpired  = "expired"
	StateAborted  = "aborted"
)

// Visibility values.
const (
	VisibilityPrivate = "private"
	VisibilityPublic  = "public"
)

// Room is a multiplayer lobby.
type Room struct {
	ID            uuid.UUID
	RoomCode      string
	OwnerPlayerID uuid.UUID
	PartyID       *uuid.UUID
	Mode          string
	Visibility    string
	State         string
	MaxPlayers    int
	MatchID       *uuid.UUID
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ExpiresAt     time.Time
}

// Member is a lobby seat.
type Member struct {
	RoomID      uuid.UUID
	PlayerID    uuid.UUID
	DisplayName string
	AvatarID    string
	Ready       bool
	JoinedAt    time.Time
	LastSeenAt  time.Time
}

// IsTerminal reports whether the room can no longer be joined.
func IsTerminal(state string) bool {
	switch state {
	case StateClosed, StateExpired, StateAborted:
		return true
	default:
		return false
	}
}

// IsActive reports whether the room counts toward one-active-room-per-player.
func IsActive(state string) bool {
	switch state {
	case StateOpen, StateStarting, StateInGame:
		return true
	default:
		return false
	}
}
