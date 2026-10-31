package match_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/identity"
	"github.com/ZhiH2333/WildBoat/internal/match"
	"github.com/ZhiH2333/WildBoat/internal/persistence"
)

func setupMatchService(t *testing.T) (*match.Service, *identity.Service, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping match integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	if err := persistence.MigrateUp(databaseURL); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	db, err := persistence.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(db.Close)

	access, err := identity.NewAccessTokenManager(
		[]byte("test-signing-key-32-bytes-minimum!!"),
		time.Minute,
		"wildboat-test",
	)
	if err != nil {
		t.Fatalf("access: %v", err)
	}
	idSvc := identity.NewService(identity.NewStore(db.Pool), access, identity.Config{
		AccessTTL:  time.Minute,
		RefreshTTL: time.Hour,
		EmailMode:  identity.EmailModeDev,
		AppEnv:     "test",
	}, slog.Default())

	const dsCredential = "matchpkg-ds-credential-32bytes!!"
	matchSvc := match.NewService(match.NewStore(db.Pool), match.Config{
		TicketTTL:        50 * time.Millisecond,
		HeartbeatTTL:     30 * time.Second,
		DefaultCapacity:  4,
		ServerCredential: dsCredential,
		ServerLabel:      "integration",
	}, slog.Default())
	if err := matchSvc.EnsureBootstrapServer(ctx); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return matchSvc, idSvc, ctx
}

func matchDSCredential() string {
	return "matchpkg-ds-credential-32bytes!!"
}

func createActivePlayer(t *testing.T, idSvc *identity.Service, ctx context.Context, name string) uuid.UUID {
	t.Helper()
	email := name + "-" + uuid.NewString() + "@example.com"
	reg, err := idSvc.Register(ctx, identity.RegisterInput{
		Email: email, Password: "password-one", DisplayName: name,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := idSvc.VerifyEmail(ctx, reg.DevVerificationToken); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	return reg.Player.PlayerID
}

func TestExpiredTicketValidation(t *testing.T) {
	matchSvc, idSvc, ctx := setupMatchService(t)
	serverID, err := matchSvc.AuthenticateServer(ctx, matchDSCredential())
	if err != nil {
		t.Fatalf("AuthenticateServer: %v", err)
	}
	inst, err := matchSvc.RegisterInstance(ctx, serverID, match.RegisterInstanceInput{
		Host: "127.0.0.1", Port: 24567, Transport: match.TransportENetUDP,
		ProtocolVersion: 1, ServerBuild: "test", Capacity: 4, Status: match.StatusStarting,
	})
	if err != nil {
		t.Fatalf("RegisterInstance: %v", err)
	}
	if _, err := matchSvc.Heartbeat(ctx, serverID, inst.ID, match.HeartbeatInput{Status: match.StatusReady}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	playerID := createActivePlayer(t, idSvc, ctx, "Exp")
	asg, err := matchSvc.Assign(ctx, playerID, match.ModeCoopSurvival)
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}

	time.Sleep(80 * time.Millisecond)
	_, err = matchSvc.ValidateTicket(ctx, serverID, match.ValidateTicketInput{
		GameTicket:     asg.GameTicket,
		PlayerID:       playerID,
		MatchID:        asg.MatchID,
		GameInstanceID: asg.GameInstanceID,
	})
	if err != match.ErrTicketExpired {
		t.Fatalf("ValidateTicket err=%v want ErrTicketExpired", err)
	}
}

func TestTicketRevokedOnReregister(t *testing.T) {
	matchSvc, idSvc, ctx := setupMatchService(t)
	serverID, err := matchSvc.AuthenticateServer(ctx, matchDSCredential())
	if err != nil {
		t.Fatalf("AuthenticateServer: %v", err)
	}
	inst, err := matchSvc.RegisterInstance(ctx, serverID, match.RegisterInstanceInput{
		Host: "127.0.0.1", Port: 24567, Transport: match.TransportENetUDP,
		ProtocolVersion: 1, ServerBuild: "test", Capacity: 4, Status: match.StatusReady,
	})
	if err != nil {
		t.Fatalf("RegisterInstance: %v", err)
	}
	if _, err := matchSvc.Heartbeat(ctx, serverID, inst.ID, match.HeartbeatInput{Status: match.StatusReady}); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}

	playerID := createActivePlayer(t, idSvc, ctx, "Rev")
	asg, err := matchSvc.Assign(ctx, playerID, match.ModeBattleFFA)
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}

	_, err = matchSvc.RegisterInstance(ctx, serverID, match.RegisterInstanceInput{
		Host: "127.0.0.1", Port: 24568, Transport: match.TransportENetUDP,
		ProtocolVersion: 1, ServerBuild: "test-2", Capacity: 4, Status: match.StatusStarting,
	})
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}

	_, err = matchSvc.ValidateTicket(ctx, serverID, match.ValidateTicketInput{
		GameTicket:     asg.GameTicket,
		PlayerID:       playerID,
		MatchID:        asg.MatchID,
		GameInstanceID: asg.GameInstanceID,
	})
	if err != match.ErrTicketRevoked && err != match.ErrTicketInvalid {
		t.Fatalf("ValidateTicket err=%v want revoked/invalid", err)
	}
}
