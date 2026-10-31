package identity

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAccessTokenIssueAndParse(t *testing.T) {
	t.Parallel()

	mgr, err := NewAccessTokenManager([]byte("0123456789abcdef0123456789abcdef"), 15*time.Minute, "wildboat")
	if err != nil {
		t.Fatalf("NewAccessTokenManager: %v", err)
	}
	accountID := uuid.New()
	playerID := uuid.New()
	sessionID := uuid.New()
	now := time.Now().UTC()

	token, exp, err := mgr.Issue(accountID, playerID, sessionID, now)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	if !exp.After(now) {
		t.Fatal("expected future expiry")
	}

	claims, err := mgr.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.AccountID != accountID || claims.PlayerID != playerID || claims.SessionID != sessionID {
		t.Fatalf("claims mismatch: %+v", claims)
	}
}

func TestAccessTokenExpired(t *testing.T) {
	t.Parallel()

	mgr, err := NewAccessTokenManager([]byte("0123456789abcdef0123456789abcdef"), time.Minute, "wildboat")
	if err != nil {
		t.Fatalf("NewAccessTokenManager: %v", err)
	}
	token, _, err := mgr.Issue(uuid.New(), uuid.New(), uuid.New(), time.Now().UTC().Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	_, err = mgr.Parse(token)
	if err != ErrTokenExpired {
		t.Fatalf("err = %v, want ErrTokenExpired", err)
	}
}

func TestAccessTokenRejectsTampering(t *testing.T) {
	t.Parallel()

	mgr, err := NewAccessTokenManager([]byte("0123456789abcdef0123456789abcdef"), time.Minute, "wildboat")
	if err != nil {
		t.Fatalf("NewAccessTokenManager: %v", err)
	}
	token, _, err := mgr.Issue(uuid.New(), uuid.New(), uuid.New(), time.Now().UTC())
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	tampered := token + "x"
	_, err = mgr.Parse(tampered)
	if err != ErrTokenInvalid {
		t.Fatalf("err = %v, want ErrTokenInvalid", err)
	}
}

func TestNewAccessTokenManagerRequiresKey(t *testing.T) {
	t.Parallel()
	_, err := NewAccessTokenManager([]byte("short"), time.Minute, "wildboat")
	if err == nil {
		t.Fatal("expected error for short key")
	}
}
