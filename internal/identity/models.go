package identity

import (
	"net/netip"
	"time"

	"github.com/google/uuid"
)

// Account status values.
const (
	AccountPendingVerification = "pending_verification"
	AccountActive              = "active"
	AccountLocked              = "locked"
	AccountClosed              = "closed"
)

// Email action purposes.
const (
	PurposeVerifyEmail   = "verify_email"
	PurposePasswordReset = "password_reset"
)

// Account is a WPass credential record.
type Account struct {
	ID              uuid.UUID
	EmailNormalized string
	PasswordHash    string
	Status          string
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Player is the public WPG player identity bound to an account.
type Player struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	DisplayName    string
	AvatarID       string
	Bio            string
	ProfileVersion int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Session tracks a refresh-token family member for an account.
type Session struct {
	ID                       uuid.UUID
	AccountID                uuid.UUID
	RefreshTokenHash         []byte
	PreviousRefreshTokenHash []byte
	RotatedAt                *time.Time
	UserAgent                string
	IP                       *netip.Addr
	CreatedAt                time.Time
	ExpiresAt                time.Time
	RevokedAt                *time.Time
	RevokeReason             *string
}

// EmailActionToken is a single-use email verification or password-reset secret.
type EmailActionToken struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	TokenHash  []byte
	Purpose    string
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

// TokenPair is issued to clients after login or refresh.
type TokenPair struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
	SessionID        uuid.UUID
}

// SessionView is a safe session summary for API responses.
type SessionView struct {
	ID        uuid.UUID  `json:"session_id"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UserAgent string     `json:"user_agent,omitempty"`
	IP        string     `json:"ip,omitempty"`
	Current   bool       `json:"current"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

// PlayerPublic is the public player profile payload.
type PlayerPublic struct {
	PlayerID    uuid.UUID `json:"player_id"`
	DisplayName string    `json:"display_name"`
	AvatarID    string    `json:"avatar_id"`
	Bio         string    `json:"bio,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MeResponse is returned by GET /v1/me.
type MeResponse struct {
	AccountID     uuid.UUID    `json:"account_id"`
	Email         string       `json:"email"`
	Status        string       `json:"status"`
	EmailVerified bool         `json:"email_verified"`
	Player        PlayerPublic `json:"player"`
}
