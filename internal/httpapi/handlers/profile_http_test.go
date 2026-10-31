package handlers_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestStatsZerosBeforeSettlement(t *testing.T) {
	h, _, _ := testAPI(t)
	access, playerID := registerLogin(t, h, "StatsZero")
	code, body := doJSON(t, h, http.MethodGet, "/v1/me/stats", nil, access)
	if code != http.StatusOK {
		t.Fatalf("status=%d body=%v", code, body)
	}
	if body["player_id"] != playerID {
		t.Fatalf("player_id=%v", body["player_id"])
	}
	if body["matches_played"].(float64) != 0 || body["kills"].(float64) != 0 || body["deaths"].(float64) != 0 || body["score"].(float64) != 0 {
		t.Fatalf("expected zeros, got %v", body)
	}
	if body["updated_at"] != nil {
		t.Fatalf("updated_at=%v want null", body["updated_at"])
	}
}

func TestStatsAndHistoryAfterSettlement(t *testing.T) {
	h, _, _ := testAPI(t)
	instanceID := registerReadyInstance(t, h, 4)
	access, playerID := registerLogin(t, h, "StatsSettled")

	code, asg := doJSON(t, h, http.MethodPost, "/v1/games/assign", map[string]any{
		"mode": "coop_survival",
	}, access)
	if code != http.StatusOK {
		t.Fatalf("assign status=%d body=%v", code, asg)
	}
	matchID := asg["match_id"].(string)
	ticket := asg["game_ticket"].(string)

	// Unsettled match must not appear in history.
	code, hist := doJSON(t, h, http.MethodGet, "/v1/me/matches?limit=20", nil, access)
	if code != http.StatusOK {
		t.Fatalf("history status=%d body=%v", code, hist)
	}
	if items := hist["items"].([]any); len(items) != 0 {
		t.Fatalf("expected empty history before settlement, got %v", items)
	}

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
			"kills":     3,
			"deaths":    1,
			"score":     420,
		}},
	}
	code, settle := doJSON(t, h, http.MethodPost, "/v1/server/matches/"+matchID+"/settlement", payload, testDSCredential)
	if code != http.StatusOK {
		t.Fatalf("settle status=%d body=%v", code, settle)
	}

	code, stats := doJSON(t, h, http.MethodGet, "/v1/me/stats", nil, access)
	if code != http.StatusOK {
		t.Fatalf("stats status=%d body=%v", code, stats)
	}
	if stats["matches_played"].(float64) != 1 || stats["kills"].(float64) != 3 || stats["deaths"].(float64) != 1 || stats["score"].(float64) != 420 {
		t.Fatalf("stats mismatch: %v", stats)
	}
	if stats["updated_at"] == nil {
		t.Fatal("expected updated_at")
	}

	code, hist = doJSON(t, h, http.MethodGet, "/v1/me/matches?limit=20", nil, access)
	if code != http.StatusOK {
		t.Fatalf("history after status=%d body=%v", code, hist)
	}
	items := hist["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items=%v", items)
	}
	item := items[0].(map[string]any)
	if item["match_id"] != matchID || item["result"] != "completed" {
		t.Fatalf("item=%v", item)
	}
	if item["kills"].(float64) != 3 || item["score"].(float64) != 420 {
		t.Fatalf("item scores=%v", item)
	}
}
