package handlers_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/persistence"
)

func registerLogin(t *testing.T, h http.Handler, name string) (access string, playerID string) {
	t.Helper()
	email := fmt.Sprintf("%s-%d@example.com", name, time.Now().UnixNano())
	code, reg := doJSON(t, h, http.MethodPost, "/v1/auth/register", map[string]any{
		"email":        email,
		"password":     "password-one",
		"display_name": name,
	}, "")
	if code != http.StatusCreated {
		t.Fatalf("register status=%d body=%v", code, reg)
	}
	devToken := reg["dev_verification_token"].(string)
	code, _ = doJSON(t, h, http.MethodPost, "/v1/auth/email/verify", map[string]any{"token": devToken}, "")
	if code != http.StatusOK {
		t.Fatalf("verify status=%d", code)
	}
	code, login := doJSON(t, h, http.MethodPost, "/v1/auth/login", map[string]any{
		"email": email, "password": "password-one",
	}, "")
	if code != http.StatusOK {
		t.Fatalf("login status=%d body=%v", code, login)
	}
	tokens := login["tokens"].(map[string]any)
	me := login["me"].(map[string]any)
	player := me["player"].(map[string]any)
	return tokens["access_token"].(string), player["player_id"].(string)
}

func offlineAllGameInstances(t *testing.T) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := persistence.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect for offline-all: %v", err)
	}
	defer db.Close()
	if _, err := db.Pool.Exec(ctx, `
		UPDATE game_instances
		SET status = 'offline', updated_at = now()
		WHERE status IN ('ready', 'full', 'starting', 'draining')
	`); err != nil {
		t.Fatalf("offline all instances: %v", err)
	}
}

func registerReadyInstance(t *testing.T, h http.Handler, capacity int) string {
	t.Helper()
	// Isolate from a concurrent local Docker DS sharing DATABASE_URL.
	offlineAllGameInstances(t)
	code, reg := doJSON(t, h, http.MethodPost, "/v1/server/instances/register", map[string]any{
		"host":             "127.0.0.1",
		"port":             24567,
		"transport":        "enet_udp",
		"protocol_version": 1,
		"server_build":     "test-build",
		"capacity":         capacity,
		"status":           "starting",
	}, testDSCredential)
	if code != http.StatusOK {
		t.Fatalf("register instance status=%d body=%v", code, reg)
	}
	instanceID := reg["game_instance_id"].(string)
	code, hb := doJSON(t, h, http.MethodPost, "/v1/server/instances/"+instanceID+"/heartbeat", map[string]any{
		"status": "ready",
	}, testDSCredential)
	if code != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%v", code, hb)
	}
	return instanceID
}

func forceNoAssignableInstance(t *testing.T, h http.Handler) {
	t.Helper()
	// Register then mark offline so leftover ready instances from other tests cannot assign.
	instanceID := registerReadyInstance(t, h, 4)
	code, body := doJSON(t, h, http.MethodPost, "/v1/server/instances/"+instanceID+"/heartbeat", map[string]any{
		"status": "offline",
	}, testDSCredential)
	if code != http.StatusOK {
		t.Fatalf("offline heartbeat status=%d body=%v", code, body)
	}
	offlineAllGameInstances(t)
}

func TestAssignRequiresAuth(t *testing.T) {
	h, _, _ := testAPI(t)
	code, body := doJSON(t, h, http.MethodPost, "/v1/games/assign", map[string]any{
		"mode": "coop_survival",
	}, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%v", code, body)
	}
}

func TestAssignAndTicketLifecycle(t *testing.T) {
	h, _, _ := testAPI(t)
	instanceID := registerReadyInstance(t, h, 4)
	access, playerID := registerLogin(t, h, "AsgPilot")

	code, asg := doJSON(t, h, http.MethodPost, "/v1/games/assign", map[string]any{
		"mode": "coop_survival",
	}, access)
	if code != http.StatusOK {
		t.Fatalf("assign status=%d body=%v", code, asg)
	}
	if asg["game_instance_id"] != instanceID {
		t.Fatalf("instance=%v want %s", asg["game_instance_id"], instanceID)
	}
	ticket, _ := asg["game_ticket"].(string)
	matchID, _ := asg["match_id"].(string)
	if ticket == "" || matchID == "" {
		t.Fatalf("missing ticket/match in %v", asg)
	}

	code, validated := doJSON(t, h, http.MethodPost, "/v1/server/tickets/validate", map[string]any{
		"game_ticket":      ticket,
		"player_id":        playerID,
		"match_id":         matchID,
		"game_instance_id": instanceID,
	}, testDSCredential)
	if code != http.StatusOK {
		t.Fatalf("validate status=%d body=%v", code, validated)
	}
	if validated["valid"] != true {
		t.Fatalf("expected valid ticket, got %v", validated)
	}

	// Replay must fail.
	code, replay := doJSON(t, h, http.MethodPost, "/v1/server/tickets/validate", map[string]any{
		"game_ticket":      ticket,
		"player_id":        playerID,
		"match_id":         matchID,
		"game_instance_id": instanceID,
	}, testDSCredential)
	if code != http.StatusConflict {
		t.Fatalf("replay status=%d body=%v", code, replay)
	}
	errObj := replay["error"].(map[string]any)
	if errObj["code"] != "TICKET_REPLAYED" {
		t.Fatalf("code=%v want TICKET_REPLAYED", errObj["code"])
	}

	// Wrong match binding: fabricate a different match id.
	access2, player2 := registerLogin(t, h, "AsgTwo")
	code, asg2 := doJSON(t, h, http.MethodPost, "/v1/games/assign", map[string]any{
		"mode": "coop_survival",
	}, access2)
	if code != http.StatusOK {
		t.Fatalf("assign2 status=%d body=%v", code, asg2)
	}
	ticket2 := asg2["game_ticket"].(string)
	code, wrong := doJSON(t, h, http.MethodPost, "/v1/server/tickets/validate", map[string]any{
		"game_ticket":      ticket2,
		"player_id":        player2,
		"match_id":         uuid.NewString(),
		"game_instance_id": instanceID,
	}, testDSCredential)
	if code != http.StatusBadRequest {
		t.Fatalf("wrong binding status=%d body=%v", code, wrong)
	}
}

func TestAssignRejectedWhenDSUnavailable(t *testing.T) {
	h, _, _ := testAPI(t)
	forceNoAssignableInstance(t, h)
	access, _ := registerLogin(t, h, "NoDS")
	code, body := doJSON(t, h, http.MethodPost, "/v1/games/assign", map[string]any{
		"mode": "coop_survival",
	}, access)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%v", code, body)
	}
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "GAME_SERVER_UNAVAILABLE" {
		t.Fatalf("code=%v", errObj["code"])
	}
}

func TestAssignRejectedWhenFull(t *testing.T) {
	h, _, _ := testAPI(t)
	_ = registerReadyInstance(t, h, 1)
	access1, _ := registerLogin(t, h, "FullOne")
	access2, _ := registerLogin(t, h, "FullTwo")

	code, asg := doJSON(t, h, http.MethodPost, "/v1/games/assign", map[string]any{
		"mode": "battle_ffa",
	}, access1)
	if code != http.StatusOK {
		t.Fatalf("first assign status=%d body=%v", code, asg)
	}
	code, body := doJSON(t, h, http.MethodPost, "/v1/games/assign", map[string]any{
		"mode": "battle_ffa",
	}, access2)
	if code != http.StatusConflict {
		t.Fatalf("second assign status=%d body=%v", code, body)
	}
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "GAME_CAPACITY_FULL" {
		t.Fatalf("code=%v", errObj["code"])
	}
}

func TestConcurrentAssignDoesNotOverfill(t *testing.T) {
	h, _, _ := testAPI(t)
	_ = registerReadyInstance(t, h, 2)

	const n = 8
	accesses := make([]string, n)
	for i := 0; i < n; i++ {
		accesses[i], _ = registerLogin(t, h, fmt.Sprintf("C%d", i))
	}

	var mu sync.Mutex
	okCount := 0
	fullCount := 0
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			code, _ := doJSON(t, h, http.MethodPost, "/v1/games/assign", map[string]any{
				"mode": "coop_endless",
			}, token)
			mu.Lock()
			defer mu.Unlock()
			switch code {
			case http.StatusOK:
				okCount++
			case http.StatusConflict:
				fullCount++
			default:
				t.Errorf("unexpected status %d", code)
			}
		}(accesses[i])
	}
	wg.Wait()
	if okCount != 2 {
		t.Fatalf("okCount=%d want 2 (fullCount=%d)", okCount, fullCount)
	}
	if fullCount != n-2 {
		t.Fatalf("fullCount=%d want %d", fullCount, n-2)
	}
}

func TestSettlementAuthAndIdempotency(t *testing.T) {
	h, _, _ := testAPI(t)
	instanceID := registerReadyInstance(t, h, 4)
	access, playerID := registerLogin(t, h, "Settle")

	code, asg := doJSON(t, h, http.MethodPost, "/v1/games/assign", map[string]any{
		"mode": "coop_survival",
	}, access)
	if code != http.StatusOK {
		t.Fatalf("assign status=%d body=%v", code, asg)
	}
	matchID := asg["match_id"].(string)
	ticket := asg["game_ticket"].(string)

	code, _ = doJSON(t, h, http.MethodPost, "/v1/server/tickets/validate", map[string]any{
		"game_ticket": ticket, "player_id": playerID, "match_id": matchID, "game_instance_id": instanceID,
	}, testDSCredential)
	if code != http.StatusOK {
		t.Fatalf("validate status=%d", code)
	}

	settlementID := uuid.NewString()
	payload := map[string]any{
		"settlement_id": settlementID,
		"result":        "completed",
		"started_at":    time.Now().UTC().Add(-5 * time.Minute),
		"ended_at":      time.Now().UTC(),
		"players": []map[string]any{{
			"player_id": playerID,
			"team_id":   "",
			"kills":     2,
			"deaths":    1,
			"score":     100,
		}},
		"mode_stats": map[string]any{"duration_seconds": 300},
	}

	// Player token cannot settle.
	code, denied := doJSON(t, h, http.MethodPost, "/v1/server/matches/"+matchID+"/settlement", payload, access)
	if code != http.StatusUnauthorized {
		t.Fatalf("player settle status=%d body=%v", code, denied)
	}

	code, first := doJSON(t, h, http.MethodPost, "/v1/server/matches/"+matchID+"/settlement", payload, testDSCredential)
	if code != http.StatusOK {
		t.Fatalf("settle status=%d body=%v", code, first)
	}
	if first["already_processed"] != false {
		t.Fatalf("first already_processed=%v", first["already_processed"])
	}

	code, second := doJSON(t, h, http.MethodPost, "/v1/server/matches/"+matchID+"/settlement", payload, testDSCredential)
	if code != http.StatusOK {
		t.Fatalf("settle retry status=%d body=%v", code, second)
	}
	if second["already_processed"] != true {
		t.Fatalf("second already_processed=%v", second["already_processed"])
	}

	// New settlement on completed match must fail.
	payload2 := map[string]any{
		"settlement_id": uuid.NewString(),
		"result":        "completed",
		"started_at":    time.Now().UTC().Add(-5 * time.Minute),
		"ended_at":      time.Now().UTC(),
		"players": []map[string]any{{
			"player_id": playerID, "kills": 1, "deaths": 0, "score": 10,
		}},
	}
	code, bad := doJSON(t, h, http.MethodPost, "/v1/server/matches/"+matchID+"/settlement", payload2, testDSCredential)
	if code != http.StatusConflict {
		t.Fatalf("invalid match settle status=%d body=%v", code, bad)
	}
}

func TestTicketWrongInstanceRejected(t *testing.T) {
	h, _, _ := testAPI(t)
	_ = registerReadyInstance(t, h, 4)
	access, playerID := registerLogin(t, h, "ExpTix")

	code, asg := doJSON(t, h, http.MethodPost, "/v1/games/assign", map[string]any{
		"mode": "battle_team",
	}, access)
	if code != http.StatusOK {
		t.Fatalf("assign status=%d body=%v", code, asg)
	}
	ticket := asg["game_ticket"].(string)
	matchID := asg["match_id"].(string)
	code, body := doJSON(t, h, http.MethodPost, "/v1/server/tickets/validate", map[string]any{
		"game_ticket":      ticket,
		"player_id":        playerID,
		"match_id":         matchID,
		"game_instance_id": uuid.NewString(),
	}, testDSCredential)
	if code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%v", code, body)
	}
}
