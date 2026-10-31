package handlers_test

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/ZhiH2333/WildBoat/internal/room"
)

func createReadyRoom(t *testing.T, h http.Handler, access string, mode string) (roomID, roomCode string) {
	t.Helper()
	code, body := doJSON(t, h, http.MethodPost, "/v1/rooms", map[string]any{
		"mode": mode, "visibility": "private", "max_players": 4,
	}, access)
	if code != http.StatusCreated {
		t.Fatalf("create room status=%d body=%v", code, body)
	}
	r := body["room"].(map[string]any)
	return r["room_id"].(string), r["room_code"].(string)
}

func setReady(t *testing.T, h http.Handler, access, roomID string, ready bool) {
	t.Helper()
	code, body := doJSON(t, h, http.MethodPut, "/v1/rooms/"+roomID+"/ready", map[string]any{
		"ready": ready,
	}, access)
	if code != http.StatusOK {
		t.Fatalf("ready status=%d body=%v", code, body)
	}
}

func TestCreateRoomGeneratesValidCode(t *testing.T) {
	h, _, _ := testAPI(t)
	access, playerID := registerLogin(t, h, "Host")
	code, body := doJSON(t, h, http.MethodPost, "/v1/rooms", map[string]any{
		"mode": "coop_survival", "visibility": "private", "max_players": 4,
	}, access)
	if code != http.StatusCreated {
		t.Fatalf("status=%d body=%v", code, body)
	}
	r := body["room"].(map[string]any)
	roomCode := r["room_code"].(string)
	if !room.IsValidRoomCodeFormat(roomCode) {
		t.Fatalf("invalid room_code %q", roomCode)
	}
	if r["owner_player_id"] != playerID {
		t.Fatalf("owner=%v want %s", r["owner_player_id"], playerID)
	}
	if int(r["member_count"].(float64)) != 1 {
		t.Fatalf("member_count=%v", r["member_count"])
	}
	members := body["members"].([]any)
	if len(members) != 1 {
		t.Fatalf("members=%d", len(members))
	}
	m0 := members[0].(map[string]any)
	if m0["ready"] != false || m0["is_owner"] != true {
		t.Fatalf("member=%v", m0)
	}
}

func TestJoinInvalidExpiredClosedCode(t *testing.T) {
	h, _, _ := testAPI(t)
	access, _ := registerLogin(t, h, "Joiner")

	code, body := doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
		"room_code": "IIIIII",
	}, access)
	if code != http.StatusNotFound {
		t.Fatalf("ambiguous code status=%d body=%v", code, body)
	}
	if body["error"].(map[string]any)["code"] != "ROOM_CODE_INVALID" {
		t.Fatalf("code=%v", body["error"])
	}

	code, body = doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
		"room_code": "ZZZZZZ",
	}, access)
	if code != http.StatusNotFound || body["error"].(map[string]any)["code"] != "ROOM_CODE_INVALID" {
		t.Fatalf("unknown code status=%d body=%v", code, body)
	}

	host, _ := registerLogin(t, h, "CloseHost")
	roomID, roomCode := createReadyRoom(t, h, host, "coop_survival")
	code, _ = doJSON(t, h, http.MethodPost, "/v1/rooms/"+roomID+"/leave", map[string]any{}, host)
	if code != http.StatusOK {
		t.Fatalf("leave status=%d", code)
	}
	code, body = doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
		"room_code": roomCode,
	}, access)
	if code != http.StatusNotFound || body["error"].(map[string]any)["code"] != "ROOM_CODE_INVALID" {
		t.Fatalf("closed code status=%d body=%v", code, body)
	}
}

func TestTwoPlayersJoinSameRoom(t *testing.T) {
	h, _, _ := testAPI(t)
	host, hostID := registerLogin(t, h, "TwoHost")
	guest, guestID := registerLogin(t, h, "TwoGuest")
	roomID, roomCode := createReadyRoom(t, h, host, "battle_ffa")

	code, body := doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
		"room_code": roomCode,
	}, guest)
	if code != http.StatusOK {
		t.Fatalf("join status=%d body=%v", code, body)
	}
	if body["room"].(map[string]any)["room_id"] != roomID {
		t.Fatalf("room_id mismatch")
	}
	members := body["members"].([]any)
	if len(members) != 2 {
		t.Fatalf("members=%d", len(members))
	}
	ids := map[string]bool{}
	for _, raw := range members {
		ids[raw.(map[string]any)["player_id"].(string)] = true
	}
	if !ids[hostID] || !ids[guestID] {
		t.Fatalf("members missing players: %v", ids)
	}

	// Idempotent re-join.
	code, body = doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
		"room_code": roomCode,
	}, guest)
	if code != http.StatusOK {
		t.Fatalf("rejoin status=%d body=%v", code, body)
	}
	if len(body["members"].([]any)) != 2 {
		t.Fatalf("rejoin duplicated members")
	}
}

func TestCapacityFourOkFifthRejected(t *testing.T) {
	h, _, _ := testAPI(t)
	host, _ := registerLogin(t, h, "CapHost")
	_, roomCode := createReadyRoom(t, h, host, "coop_endless")

	for i := 0; i < 3; i++ {
		access, _ := registerLogin(t, h, fmt.Sprintf("Cap%d", i))
		code, body := doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
			"room_code": roomCode,
		}, access)
		if code != http.StatusOK {
			t.Fatalf("join %d status=%d body=%v", i, code, body)
		}
	}
	fifth, _ := registerLogin(t, h, "CapFifth")
	code, body := doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
		"room_code": roomCode,
	}, fifth)
	if code != http.StatusConflict || body["error"].(map[string]any)["code"] != "ROOM_FULL" {
		t.Fatalf("fifth status=%d body=%v", code, body)
	}
}

func TestConcurrentJoinDoesNotOverfill(t *testing.T) {
	h, _, _ := testAPI(t)
	host, _ := registerLogin(t, h, "ConcHost")
	code, body := doJSON(t, h, http.MethodPost, "/v1/rooms", map[string]any{
		"mode": "coop_survival", "visibility": "private", "max_players": 2,
	}, host)
	if code != http.StatusCreated {
		t.Fatalf("create status=%d body=%v", code, body)
	}
	roomCode := body["room"].(map[string]any)["room_code"].(string)

	const n = 8
	accesses := make([]string, n)
	for i := 0; i < n; i++ {
		accesses[i], _ = registerLogin(t, h, fmt.Sprintf("CJ%d", i))
	}

	var mu sync.Mutex
	okCount, fullCount := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(token string) {
			defer wg.Done()
			st, resp := doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
				"room_code": roomCode,
			}, token)
			mu.Lock()
			defer mu.Unlock()
			switch st {
			case http.StatusOK:
				okCount++
			case http.StatusConflict:
				if resp["error"].(map[string]any)["code"] == "ROOM_FULL" {
					fullCount++
				}
			default:
				t.Errorf("unexpected status %d body=%v", st, resp)
			}
		}(accesses[i])
	}
	wg.Wait()
	// Host already occupies 1 of 2 seats; exactly one joiner should succeed.
	if okCount != 1 {
		t.Fatalf("okCount=%d want 1 (fullCount=%d)", okCount, fullCount)
	}
}

func TestPlayerCannotJoinTwoActiveRooms(t *testing.T) {
	h, _, _ := testAPI(t)
	a1, _ := registerLogin(t, h, "TwoA")
	a2, _ := registerLogin(t, h, "TwoB")
	_, _ = createReadyRoom(t, h, a1, "coop_survival")
	_, code2 := createReadyRoom(t, h, a2, "coop_survival")

	st, body := doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
		"room_code": code2,
	}, a1)
	if st != http.StatusConflict || body["error"].(map[string]any)["code"] != "ROOM_ALREADY_IN" {
		t.Fatalf("status=%d body=%v", st, body)
	}

	st, body = doJSON(t, h, http.MethodPost, "/v1/rooms", map[string]any{
		"mode": "battle_ffa", "visibility": "private", "max_players": 4,
	}, a1)
	if st != http.StatusConflict || body["error"].(map[string]any)["code"] != "ROOM_ALREADY_IN" {
		t.Fatalf("second create status=%d body=%v", st, body)
	}
}

func TestReadyUnreadyAndStartGates(t *testing.T) {
	h, _, _ := testAPI(t)
	_ = registerReadyInstance(t, h, 4)
	host, _ := registerLogin(t, h, "ReadyHost")
	guest, _ := registerLogin(t, h, "ReadyGuest")
	roomID, roomCode := createReadyRoom(t, h, host, "coop_survival")
	code, _ := doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
		"room_code": roomCode,
	}, guest)
	if code != http.StatusOK {
		t.Fatalf("join status=%d", code)
	}

	setReady(t, h, host, roomID, true)
	setReady(t, h, host, roomID, true) // idempotent

	code, body := doJSON(t, h, http.MethodPost, "/v1/rooms/"+roomID+"/start", map[string]any{}, host)
	if code != http.StatusConflict || body["error"].(map[string]any)["code"] != "ROOM_NOT_READY" {
		t.Fatalf("start not ready status=%d body=%v", code, body)
	}

	code, body = doJSON(t, h, http.MethodPost, "/v1/rooms/"+roomID+"/start", map[string]any{}, guest)
	if code != http.StatusForbidden || body["error"].(map[string]any)["code"] != "NOT_ROOM_OWNER" {
		t.Fatalf("non-owner start status=%d body=%v", code, body)
	}

	setReady(t, h, guest, roomID, true)
	setReady(t, h, guest, roomID, false)
	code, body = doJSON(t, h, http.MethodPost, "/v1/rooms/"+roomID+"/start", map[string]any{}, host)
	if code != http.StatusConflict || body["error"].(map[string]any)["code"] != "ROOM_NOT_READY" {
		t.Fatalf("unready start status=%d body=%v", code, body)
	}
}

func TestOwnerLeaveTransfersOwnership(t *testing.T) {
	h, _, _ := testAPI(t)
	host, hostID := registerLogin(t, h, "XferHost")
	guest, guestID := registerLogin(t, h, "XferGuest")
	roomID, roomCode := createReadyRoom(t, h, host, "battle_team")
	code, _ := doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
		"room_code": roomCode,
	}, guest)
	if code != http.StatusOK {
		t.Fatalf("join status=%d", code)
	}
	code, _ = doJSON(t, h, http.MethodPost, "/v1/rooms/"+roomID+"/leave", map[string]any{}, host)
	if code != http.StatusOK {
		t.Fatalf("leave status=%d", code)
	}
	code, body := doJSON(t, h, http.MethodGet, "/v1/rooms/"+roomID, nil, guest)
	if code != http.StatusOK {
		t.Fatalf("get status=%d body=%v", code, body)
	}
	r := body["room"].(map[string]any)
	if r["owner_player_id"] != guestID {
		t.Fatalf("owner=%v want %s (old host %s)", r["owner_player_id"], guestID, hostID)
	}
	if r["state"] != "open" {
		t.Fatalf("state=%v", r["state"])
	}
}

func TestLastMemberLeaveClosesRoom(t *testing.T) {
	h, _, _ := testAPI(t)
	host, _ := registerLogin(t, h, "LastHost")
	roomID, _ := createReadyRoom(t, h, host, "coop_survival")
	code, _ := doJSON(t, h, http.MethodPost, "/v1/rooms/"+roomID+"/leave", map[string]any{}, host)
	if code != http.StatusOK {
		t.Fatalf("leave status=%d", code)
	}
	code, body := doJSON(t, h, http.MethodGet, "/v1/rooms/current", nil, host)
	if code != http.StatusOK {
		t.Fatalf("current status=%d", code)
	}
	if body["room"] != nil {
		t.Fatalf("expected null room, got %v", body["room"])
	}
}

func TestQuickMatchFindOrCreateSameMode(t *testing.T) {
	h, _, _ := testAPI(t)
	a1, _ := registerLogin(t, h, "QM1")
	a2, _ := registerLogin(t, h, "QM2")

	code, body1 := doJSON(t, h, http.MethodPost, "/v1/matchmaking/quick", map[string]any{
		"mode": "battle_ffa",
	}, a1)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("quick1 status=%d body=%v", code, body1)
	}
	roomID := body1["room"].(map[string]any)["room_id"].(string)
	if body1["room"].(map[string]any)["visibility"] != "public" {
		t.Fatalf("visibility=%v", body1["room"])
	}

	code, body2 := doJSON(t, h, http.MethodPost, "/v1/matchmaking/quick", map[string]any{
		"mode": "battle_ffa",
	}, a2)
	if code != http.StatusOK {
		t.Fatalf("quick2 status=%d body=%v", code, body2)
	}
	if body2["room"].(map[string]any)["room_id"] != roomID {
		t.Fatalf("expected same room %s got %v", roomID, body2["room"])
	}

	// Repeat by a1 must not create another room.
	code, body3 := doJSON(t, h, http.MethodPost, "/v1/matchmaking/quick", map[string]any{
		"mode": "battle_ffa",
	}, a1)
	if code != http.StatusOK {
		t.Fatalf("quick repeat status=%d body=%v", code, body3)
	}
	if body3["room"].(map[string]any)["room_id"] != roomID {
		t.Fatalf("repeat created new room")
	}
}

func TestQuickMatchDifferentModeSeparateRooms(t *testing.T) {
	h, _, _ := testAPI(t)
	a1, _ := registerLogin(t, h, "QMMode1")
	a2, _ := registerLogin(t, h, "QMMode2")
	code, body1 := doJSON(t, h, http.MethodPost, "/v1/matchmaking/quick", map[string]any{
		"mode": "coop_survival",
	}, a1)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("quick1 status=%d", code)
	}
	code, body2 := doJSON(t, h, http.MethodPost, "/v1/matchmaking/quick", map[string]any{
		"mode": "battle_kill_race",
	}, a2)
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("quick2 status=%d", code)
	}
	if body1["room"].(map[string]any)["room_id"] == body2["room"].(map[string]any)["room_id"] {
		t.Fatal("different modes shared a room")
	}
}

func TestRoomStartAssignmentAndTicketLifecycle(t *testing.T) {
	h, _, _ := testAPI(t)
	instanceID := registerReadyInstance(t, h, 4)
	host, hostID := registerLogin(t, h, "StartHost")
	guest, guestID := registerLogin(t, h, "StartGuest")
	roomID, roomCode := createReadyRoom(t, h, host, "coop_survival")
	code, _ := doJSON(t, h, http.MethodPost, "/v1/rooms/join-by-code", map[string]any{
		"room_code": roomCode,
	}, guest)
	if code != http.StatusOK {
		t.Fatalf("join=%d", code)
	}
	setReady(t, h, host, roomID, true)
	setReady(t, h, guest, roomID, true)

	code, start := doJSON(t, h, http.MethodPost, "/v1/rooms/"+roomID+"/start", map[string]any{}, host)
	if code != http.StatusOK {
		t.Fatalf("start status=%d body=%v", code, start)
	}
	if start["room"].(map[string]any)["state"] != "in_game" {
		t.Fatalf("room state=%v", start["room"])
	}
	asg := start["assignment"].(map[string]any)
	if asg["game_instance_id"] != instanceID {
		t.Fatalf("instance=%v", asg["game_instance_id"])
	}
	matchID := asg["match_id"].(string)
	hostTicket := asg["game_ticket"].(string)

	code, detail := doJSON(t, h, http.MethodGet, "/v1/rooms/"+roomID, nil, guest)
	if code != http.StatusOK || detail["assignment_available"] != true {
		t.Fatalf("detail status=%d body=%v", code, detail)
	}
	code, claim := doJSON(t, h, http.MethodPost, "/v1/rooms/"+roomID+"/assignment", map[string]any{}, guest)
	if code != http.StatusOK {
		t.Fatalf("assignment status=%d body=%v", code, claim)
	}
	guestAsg := claim["assignment"].(map[string]any)
	guestTicket := guestAsg["game_ticket"].(string)
	if guestAsg["match_id"] != matchID {
		t.Fatalf("match mismatch")
	}
	if guestTicket == hostTicket {
		t.Fatal("tickets should be per-player")
	}

	for _, tc := range []struct {
		ticket   string
		playerID string
	}{
		{hostTicket, hostID},
		{guestTicket, guestID},
	} {
		code, validated := doJSON(t, h, http.MethodPost, "/v1/server/tickets/validate", map[string]any{
			"game_ticket": tc.ticket, "player_id": tc.playerID, "match_id": matchID, "game_instance_id": instanceID,
		}, testDSCredential)
		if code != http.StatusOK || validated["valid"] != true {
			t.Fatalf("validate %s status=%d body=%v", tc.playerID, code, validated)
		}
		code, replay := doJSON(t, h, http.MethodPost, "/v1/server/tickets/validate", map[string]any{
			"game_ticket": tc.ticket, "player_id": tc.playerID, "match_id": matchID, "game_instance_id": instanceID,
		}, testDSCredential)
		if code != http.StatusConflict || replay["error"].(map[string]any)["code"] != "TICKET_REPLAYED" {
			t.Fatalf("replay status=%d body=%v", code, replay)
		}
	}
}

func TestStartFailsWhenDSUnavailableLeavesOpen(t *testing.T) {
	h, _, _ := testAPI(t)
	forceNoAssignableInstance(t, h)
	host, _ := registerLogin(t, h, "NoDSHost")
	roomID, _ := createReadyRoom(t, h, host, "coop_survival")
	setReady(t, h, host, roomID, true)

	code, body := doJSON(t, h, http.MethodPost, "/v1/rooms/"+roomID+"/start", map[string]any{}, host)
	if code != http.StatusServiceUnavailable || body["error"].(map[string]any)["code"] != "GAME_SERVER_UNAVAILABLE" {
		t.Fatalf("start status=%d body=%v", code, body)
	}
	code, detail := doJSON(t, h, http.MethodGet, "/v1/rooms/"+roomID, nil, host)
	if code != http.StatusOK {
		t.Fatalf("get status=%d", code)
	}
	if detail["room"].(map[string]any)["state"] != "open" {
		t.Fatalf("room state=%v want open", detail["room"].(map[string]any)["state"])
	}
	if detail["room"].(map[string]any)["match_id"] != nil {
		t.Fatalf("match_id should be null, got %v", detail["room"].(map[string]any)["match_id"])
	}
}

func TestNonMemberCannotReadRoom(t *testing.T) {
	h, _, _ := testAPI(t)
	host, _ := registerLogin(t, h, "PrivHost")
	outsider, _ := registerLogin(t, h, "Outsider")
	roomID, _ := createReadyRoom(t, h, host, "coop_survival")
	code, body := doJSON(t, h, http.MethodGet, "/v1/rooms/"+roomID, nil, outsider)
	if code != http.StatusNotFound || body["error"].(map[string]any)["code"] != "ROOM_NOT_FOUND" {
		t.Fatalf("status=%d body=%v", code, body)
	}
	code, body = doJSON(t, h, http.MethodGet, "/v1/rooms/"+uuid.NewString(), nil, host)
	if code != http.StatusNotFound {
		t.Fatalf("unknown room status=%d body=%v", code, body)
	}
}
