package identity_test

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/ZhiH2333/WildBoat/internal/identity"
	"github.com/ZhiH2333/WildBoat/internal/persistence"
)

func testService(t *testing.T) (*identity.Service, context.Context) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping identity integration tests")
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
		15*time.Minute,
		"wildboat-test",
	)
	if err != nil {
		t.Fatalf("NewAccessTokenManager: %v", err)
	}
	svc := identity.NewService(identity.NewStore(db.Pool), access, identity.Config{
		AccessTTL:        15 * time.Minute,
		RefreshTTL:       24 * time.Hour,
		VerifyTTL:        time.Hour,
		PasswordResetTTL: time.Hour,
		EmailMode:        identity.EmailModeDev,
		AppEnv:           "test",
		ReuseGrace:       30 * time.Second,
	}, slog.Default())
	return svc, ctx
}

func uniqueEmail(t *testing.T) string {
	t.Helper()
	return "player-" + time.Now().UTC().Format("20060102150405.000000000") + "@example.com"
}

func registerVerified(t *testing.T, svc *identity.Service, ctx context.Context, email, password, display string) identity.RegisterResult {
	t.Helper()
	reg, err := svc.Register(ctx, identity.RegisterInput{
		Email:       email,
		Password:    password,
		DisplayName: display,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if reg.DevVerificationToken == "" {
		t.Fatal("expected dev verification token")
	}
	if err := svc.VerifyEmail(ctx, reg.DevVerificationToken); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	return reg
}

func TestRegisterDuplicateEmail(t *testing.T) {
	svc, ctx := testService(t)
	email := uniqueEmail(t)
	_ = registerVerified(t, svc, ctx, email, "password-one", "PilotA")

	_, err := svc.Register(ctx, identity.RegisterInput{
		Email:       email,
		Password:    "password-two",
		DisplayName: "PilotB",
	})
	if err != identity.ErrEmailTaken {
		t.Fatalf("err = %v, want ErrEmailTaken", err)
	}
}

func TestLoginSuccessAndFailure(t *testing.T) {
	svc, ctx := testService(t)
	email := uniqueEmail(t)
	password := "correct-password"
	_ = registerVerified(t, svc, ctx, email, password, "Pilot")

	ok, err := svc.Login(ctx, identity.LoginInput{Email: email, Password: password})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if ok.Tokens.AccessToken == "" || ok.Tokens.RefreshToken == "" {
		t.Fatal("expected tokens")
	}

	_, err = svc.Login(ctx, identity.LoginInput{Email: email, Password: "wrong-password"})
	if err != identity.ErrInvalidCredentials {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestLoginRequiresVerification(t *testing.T) {
	svc, ctx := testService(t)
	email := uniqueEmail(t)
	_, err := svc.Register(ctx, identity.RegisterInput{
		Email:       email,
		Password:    "password-one",
		DisplayName: "Pilot",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	_, err = svc.Login(ctx, identity.LoginInput{Email: email, Password: "password-one"})
	if err != identity.ErrAccountNotVerified {
		t.Fatalf("err = %v, want ErrAccountNotVerified", err)
	}
}

func TestRefreshRotationAndReuse(t *testing.T) {
	svc, ctx := testService(t)
	email := uniqueEmail(t)
	_ = registerVerified(t, svc, ctx, email, "password-one", "Pilot")

	login, err := svc.Login(ctx, identity.LoginInput{Email: email, Password: "password-one"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	oldRefresh := login.Tokens.RefreshToken

	refreshed, err := svc.Refresh(ctx, identity.RefreshInput{RefreshToken: oldRefresh})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if refreshed.Tokens.RefreshToken == oldRefresh {
		t.Fatal("refresh token must rotate")
	}

	// Within reuse grace, presenting the previous token fails without revoking the rotated session.
	_, err = svc.Refresh(ctx, identity.RefreshInput{RefreshToken: oldRefresh})
	if err != identity.ErrInvalidCredentials {
		t.Fatalf("reuse err = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.Refresh(ctx, identity.RefreshInput{RefreshToken: refreshed.Tokens.RefreshToken}); err != nil {
		t.Fatalf("rotated token should remain valid within grace: %v", err)
	}
}

func TestRefreshReuseAfterGraceRevokesAll(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping identity integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := persistence.MigrateUp(databaseURL); err != nil {
		t.Fatalf("MigrateUp: %v", err)
	}
	db, err := persistence.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer db.Close()

	access, err := identity.NewAccessTokenManager(
		[]byte("test-signing-key-32-bytes-minimum!!"),
		15*time.Minute,
		"wildboat-test",
	)
	if err != nil {
		t.Fatalf("NewAccessTokenManager: %v", err)
	}
	svc := identity.NewService(identity.NewStore(db.Pool), access, identity.Config{
		AccessTTL:  15 * time.Minute,
		RefreshTTL: 24 * time.Hour,
		EmailMode:  identity.EmailModeDev,
		AppEnv:     "test",
		ReuseGrace: time.Millisecond,
	}, slog.Default())

	email := uniqueEmail(t)
	_ = registerVerified(t, svc, ctx, email, "password-one", "Pilot")
	login, err := svc.Login(ctx, identity.LoginInput{Email: email, Password: "password-one"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	oldRefresh := login.Tokens.RefreshToken
	refreshed, err := svc.Refresh(ctx, identity.RefreshInput{RefreshToken: oldRefresh})
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	time.Sleep(5 * time.Millisecond)

	_, err = svc.Refresh(ctx, identity.RefreshInput{RefreshToken: oldRefresh})
	if err != identity.ErrInvalidCredentials {
		t.Fatalf("reuse err = %v, want ErrInvalidCredentials", err)
	}
	_, err = svc.Refresh(ctx, identity.RefreshInput{RefreshToken: refreshed.Tokens.RefreshToken})
	if err != identity.ErrInvalidCredentials {
		t.Fatalf("post-reuse refresh err = %v, want ErrInvalidCredentials", err)
	}
}

func TestSessionRevocation(t *testing.T) {
	svc, ctx := testService(t)
	email := uniqueEmail(t)
	reg := registerVerified(t, svc, ctx, email, "password-one", "Pilot")

	login, err := svc.Login(ctx, identity.LoginInput{Email: email, Password: "password-one"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if err := svc.Logout(ctx, reg.AccountID, login.Tokens.SessionID); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if err := svc.EnsureSessionActive(ctx, login.Tokens.SessionID); err != identity.ErrUnauthorized {
		t.Fatalf("EnsureSessionActive err = %v, want ErrUnauthorized", err)
	}

	login2, err := svc.Login(ctx, identity.LoginInput{Email: email, Password: "password-one"})
	if err != nil {
		t.Fatalf("Login2: %v", err)
	}
	if err := svc.LogoutAll(ctx, reg.AccountID); err != nil {
		t.Fatalf("LogoutAll: %v", err)
	}
	if err := svc.EnsureSessionActive(ctx, login2.Tokens.SessionID); err != identity.ErrUnauthorized {
		t.Fatalf("after logout-all err = %v, want ErrUnauthorized", err)
	}
}

func TestProfileAuthorizationBoundary(t *testing.T) {
	svc, ctx := testService(t)
	email := uniqueEmail(t)
	reg := registerVerified(t, svc, ctx, email, "password-one", "Pilot")

	name := "NewName"
	player, err := svc.UpdateProfile(ctx, reg.AccountID, identity.UpdateProfileInput{DisplayName: &name})
	if err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if player.DisplayName != "NewName" {
		t.Fatalf("display_name = %q", player.DisplayName)
	}

	me, err := svc.Me(ctx, reg.AccountID)
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if me.Player.DisplayName != "NewName" {
		t.Fatalf("me display_name = %q", me.Player.DisplayName)
	}
}

func TestPasswordResetFlow(t *testing.T) {
	svc, ctx := testService(t)
	email := uniqueEmail(t)
	_ = registerVerified(t, svc, ctx, email, "password-one", "Pilot")

	forgot, err := svc.ForgotPassword(ctx, identity.ForgotPasswordInput{Email: email})
	if err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	if forgot.DevResetToken == "" {
		t.Fatal("expected dev reset token")
	}
	if err := svc.ResetPassword(ctx, identity.ResetPasswordInput{
		Token:       forgot.DevResetToken,
		NewPassword: "password-two",
	}); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	_, err = svc.Login(ctx, identity.LoginInput{Email: email, Password: "password-one"})
	if err != identity.ErrInvalidCredentials {
		t.Fatalf("old password err = %v", err)
	}
	if _, err := svc.Login(ctx, identity.LoginInput{Email: email, Password: "password-two"}); err != nil {
		t.Fatalf("new password login: %v", err)
	}
}
