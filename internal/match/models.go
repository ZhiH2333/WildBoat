package match

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Dedicated server / instance statuses.
const (
	StatusStarting = "starting"
	StatusReady    = "ready"
	StatusFull     = "full"
	StatusDraining = "draining"
	StatusOffline  = "offline"
)

// Match states.
const (
	MatchAllocating = "allocating"
	MatchActive     = "active"
	MatchCompleted  = "completed"
	MatchAborted    = "aborted"
)

// Player join states within a match.
const (
	JoinAssigned = "assigned"
	JoinAdmitted = "admitted"
	JoinLeft     = "left"
)

// Supported game modes.
const (
	ModeCoopSurvival   = "coop_survival"
	ModeCoopEndless    = "coop_endless"
	ModeBattleFFA      = "battle_ffa"
	ModeBattleTeam     = "battle_team"
	ModeBattleKillRace = "battle_kill_race"
)

// Transport for Phase 2B.
const TransportENetUDP = "enet_udp"

// DedicatedServer is a server-to-server workload identity.
type DedicatedServer struct {
	ID             uuid.UUID
	CredentialHash []byte
	Label          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	RevokedAt      *time.Time
}

// GameInstance is a registered allocatable Dedicated Server endpoint.
type GameInstance struct {
	ID              uuid.UUID
	ServerID        uuid.UUID
	Host            string
	Port            int
	Transport       string
	Status          string
	ProtocolVersion int
	ServerBuild     string
	Capacity        int
	OccupiedSlots   int
	ActiveMatchID   *uuid.UUID
	LastHeartbeatAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Match is a game session on an instance.
type Match struct {
	ID             uuid.UUID
	RoomID         *uuid.UUID
	GameInstanceID uuid.UUID
	ServerID       uuid.UUID
	Mode           string
	State          string
	MaxPlayers     int
	StartedAt      *time.Time
	EndedAt        *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// MatchPlayer is a roster entry.
type MatchPlayer struct {
	MatchID    uuid.UUID
	PlayerID   uuid.UUID
	TeamID     string
	JoinState  string
	Kills      int
	Deaths     int
	Score      int
	AssignedAt time.Time
	AdmittedAt *time.Time
}

// GameTicket is the persisted ticket metadata (hash only).
type GameTicket struct {
	ID             uuid.UUID
	TokenHash      []byte
	PlayerID       uuid.UUID
	MatchID        uuid.UUID
	GameInstanceID uuid.UUID
	ServerID       uuid.UUID
	ExpiresAt      time.Time
	ConsumedAt     *time.Time
	RevokedAt      *time.Time
	CreatedAt      time.Time
}

// Settlement is a processed trusted result.
type Settlement struct {
	SettlementID   uuid.UUID
	MatchID        uuid.UUID
	GameInstanceID uuid.UUID
	ServerID       uuid.UUID
	Result         string
	PayloadJSON    json.RawMessage
	Status         string
	ReceivedAt     time.Time
	ProcessedAt    time.Time
}

// Assignment is returned to authenticated players.
type Assignment struct {
	GameInstanceID  uuid.UUID `json:"game_instance_id"`
	MatchID         uuid.UUID `json:"match_id"`
	ServerID        uuid.UUID `json:"server_id"`
	Host            string    `json:"host"`
	Port            int       `json:"port"`
	Transport       string    `json:"transport"`
	GameTicket      string    `json:"game_ticket"`
	TicketExpiresAt time.Time `json:"ticket_expires_at"`
	ProtocolVersion int       `json:"protocol_version"`
	ServerBuild     string    `json:"server_build"`
	Mode            string    `json:"mode"`
}

// ValidModes lists assignable modes.
func ValidModes() []string {
	return []string{
		ModeCoopSurvival,
		ModeCoopEndless,
		ModeBattleFFA,
		ModeBattleTeam,
		ModeBattleKillRace,
	}
}

// IsValidMode reports whether mode is allowed.
func IsValidMode(mode string) bool {
	for _, m := range ValidModes() {
		if m == mode {
			return true
		}
	}
	return false
}

// IsValidInstanceStatus reports whether status is allowed.
func IsValidInstanceStatus(status string) bool {
	switch status {
	case StatusStarting, StatusReady, StatusFull, StatusDraining, StatusOffline:
		return true
	default:
		return false
	}
}
