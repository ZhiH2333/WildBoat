package identity

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessClaims are short-lived bearer token claims.
type AccessClaims struct {
	AccountID uuid.UUID `json:"aid"`
	PlayerID  uuid.UUID `json:"pid"`
	SessionID uuid.UUID `json:"sid"`
	jwt.RegisteredClaims
}

// AccessTokenManager issues and validates HS256 access tokens.
type AccessTokenManager struct {
	signingKey []byte
	ttl        time.Duration
	issuer     string
}

// NewAccessTokenManager constructs an access-token manager.
func NewAccessTokenManager(signingKey []byte, ttl time.Duration, issuer string) (*AccessTokenManager, error) {
	if len(signingKey) < 32 {
		return nil, errors.New("TOKEN_SIGNING_KEY must be at least 32 bytes")
	}
	if ttl <= 0 {
		return nil, errors.New("access token TTL must be positive")
	}
	if issuer == "" {
		issuer = "wildboat"
	}
	return &AccessTokenManager{signingKey: signingKey, ttl: ttl, issuer: issuer}, nil
}

// TTL returns the configured access-token lifetime.
func (m *AccessTokenManager) TTL() time.Duration {
	return m.ttl
}

// Issue creates a signed access token for the given session principals.
func (m *AccessTokenManager) Issue(accountID, playerID, sessionID uuid.UUID, now time.Time) (token string, expiresAt time.Time, err error) {
	expiresAt = now.Add(m.ttl)
	claims := AccessClaims{
		AccountID: accountID,
		PlayerID:  playerID,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.issuer,
			Subject:   accountID.String(),
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token, err = t.SignedString(m.signingKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return token, expiresAt, nil
}

// Parse validates an access token and returns its claims.
func (m *AccessTokenManager) Parse(token string) (*AccessClaims, error) {
	parsed, err := jwt.ParseWithClaims(token, &AccessClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.signingKey, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		return nil, ErrTokenInvalid
	}
	claims, ok := parsed.Claims.(*AccessClaims)
	if !ok || !parsed.Valid {
		return nil, ErrTokenInvalid
	}
	if claims.AccountID == uuid.Nil || claims.PlayerID == uuid.Nil || claims.SessionID == uuid.Nil {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}
