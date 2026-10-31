package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/ZhiH2333/WildBoat/internal/httpapi"
	"github.com/ZhiH2333/WildBoat/internal/httpapi/handlers"
	"github.com/ZhiH2333/WildBoat/internal/identity"
	"github.com/ZhiH2333/WildBoat/internal/match"
	"github.com/ZhiH2333/WildBoat/internal/persistence"
	"github.com/ZhiH2333/WildBoat/internal/room"
)

const testDSCredential = "handlers-ds-credential-32bytes!!!"

func testAPI(t *testing.T) (http.Handler, *identity.Service, *match.Service) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL not set; skipping auth HTTP integration tests")
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
		t.Fatalf("access manager: %v", err)
	}
	svc := identity.NewService(identity.NewStore(db.Pool), access, identity.Config{
		AccessTTL:  time.Minute,
		RefreshTTL: 24 * time.Hour,
		EmailMode:  identity.EmailModeDev,
		AppEnv:     "test",
	}, slog.Default())

	matchStore := match.NewStore(db.Pool)
	matchSvc := match.NewService(matchStore, match.Config{
		TicketTTL:        time.Minute,
		HeartbeatTTL:     30 * time.Second,
		DefaultCapacity:  4,
		ServerCredential: testDSCredential,
		ServerLabel:      "test",
	}, slog.Default())
	if err := matchSvc.EnsureBootstrapServer(ctx); err != nil {
		t.Fatalf("EnsureBootstrapServer: %v", err)
	}
	roomSvc := room.NewService(room.NewStore(db.Pool), matchStore, room.Config{
		IdleTTL:      30 * time.Minute,
		TicketTTL:    time.Minute,
		HeartbeatTTL: 30 * time.Second,
	}, slog.Default())

	router := httpapi.NewRouter(slog.Default(), handlers.Health{DB: db}, svc, matchSvc, roomSvc, nil)
	return router, svc, matchSvc
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any, token string) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	var out map[string]any
	if rr.Body.Len() > 0 {
		if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
			t.Fatalf("decode response %s: %v body=%s", path, err, rr.Body.String())
		}
	}
	return rr.Code, out
}

func TestAuthHTTPLifecycle(t *testing.T) {
	h, _, _ := testAPI(t)
	email := "http-" + time.Now().UTC().Format("20060102150405.000000000") + "@example.com"

	code, reg := doJSON(t, h, http.MethodPost, "/v1/auth/register", map[string]any{
		"email":        email,
		"password":     "password-one",
		"display_name": "Pilot",
	}, "")
	if code != http.StatusCreated {
		t.Fatalf("register status=%d body=%v", code, reg)
	}
	devToken, _ := reg["dev_verification_token"].(string)
	if devToken == "" {
		t.Fatal("missing dev_verification_token")
	}

	code, _ = doJSON(t, h, http.MethodPost, "/v1/auth/email/verify", map[string]any{
		"token": devToken,
	}, "")
	if code != http.StatusOK {
		t.Fatalf("verify status=%d", code)
	}

	code, login := doJSON(t, h, http.MethodPost, "/v1/auth/login", map[string]any{
		"email":    email,
		"password": "password-one",
	}, "")
	if code != http.StatusOK {
		t.Fatalf("login status=%d body=%v", code, login)
	}
	tokens := login["tokens"].(map[string]any)
	access := tokens["access_token"].(string)
	refresh := tokens["refresh_token"].(string)

	code, me := doJSON(t, h, http.MethodGet, "/v1/me", nil, access)
	if code != http.StatusOK {
		t.Fatalf("me status=%d body=%v", code, me)
	}

	code, _ = doJSON(t, h, http.MethodPatch, "/v1/me/profile", map[string]any{
		"display_name": "Renamed",
	}, access)
	if code != http.StatusOK {
		t.Fatalf("patch profile status=%d", code)
	}

	code, sessions := doJSON(t, h, http.MethodGet, "/v1/me/sessions", nil, access)
	if code != http.StatusOK {
		t.Fatalf("sessions status=%d body=%v", code, sessions)
	}

	code, refreshed := doJSON(t, h, http.MethodPost, "/v1/auth/refresh", map[string]any{
		"refresh_token": refresh,
	}, "")
	if code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%v", code, refreshed)
	}
	newTokens := refreshed["tokens"].(map[string]any)
	newAccess := newTokens["access_token"].(string)

	code, _ = doJSON(t, h, http.MethodPost, "/v1/auth/logout", nil, newAccess)
	if code != http.StatusOK {
		t.Fatalf("logout status=%d", code)
	}

	code, denied := doJSON(t, h, http.MethodGet, "/v1/me", nil, newAccess)
	if code != http.StatusUnauthorized {
		t.Fatalf("post-logout me status=%d body=%v", code, denied)
	}
}

func TestAuthHTTPRejectsExpiredAccessToken(t *testing.T) {
	h, svc, _ := testAPI(t)
	email := "exp-" + time.Now().UTC().Format("20060102150405.000000000") + "@example.com"

	reg, err := svc.Register(context.Background(), identity.RegisterInput{
		Email: email, Password: "password-one", DisplayName: "Pilot",
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := svc.VerifyEmail(context.Background(), reg.DevVerificationToken); err != nil {
		t.Fatalf("VerifyEmail: %v", err)
	}
	login, err := svc.Login(context.Background(), identity.LoginInput{Email: email, Password: "password-one"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Build an already-expired access token for the same session principals.
	mgr, err := identity.NewAccessTokenManager(
		[]byte("test-signing-key-32-bytes-minimum!!"),
		time.Minute,
		"wildboat-test",
	)
	if err != nil {
		t.Fatalf("mgr: %v", err)
	}
	expired, _, err := mgr.Issue(
		login.Me.AccountID,
		login.Me.Player.PlayerID,
		login.Tokens.SessionID,
		time.Now().UTC().Add(-2*time.Hour),
	)
	if err != nil {
		t.Fatalf("Issue expired: %v", err)
	}

	code, body := doJSON(t, h, http.MethodGet, "/v1/me", nil, expired)
	if code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%v", code, body)
	}
}

func TestAuthHTTPUnauthorizedProfile(t *testing.T) {
	h, _, _ := testAPI(t)
	code, body := doJSON(t, h, http.MethodGet, "/v1/me", nil, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%v", code, body)
	}
	code, body = doJSON(t, h, http.MethodPatch, "/v1/me/profile", map[string]any{
		"display_name": "Nope",
	}, "not-a-token")
	if code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%v", code, body)
	}
}
