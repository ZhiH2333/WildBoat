package match

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists match / DS entities in PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a PostgreSQL-backed match store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// EnsureServer upserts an active dedicated server by credential hash.
func (s *Store) EnsureServer(ctx context.Context, credentialHash []byte, label string) (DedicatedServer, error) {
	var server DedicatedServer
	err := s.pool.QueryRow(ctx, `
		INSERT INTO dedicated_servers (credential_hash, label)
		VALUES ($1, $2)
		ON CONFLICT (credential_hash) DO UPDATE
		SET label = EXCLUDED.label,
		    updated_at = now(),
		    revoked_at = NULL
		RETURNING id, credential_hash, label, created_at, updated_at, revoked_at
	`, credentialHash, label).Scan(
		&server.ID,
		&server.CredentialHash,
		&server.Label,
		&server.CreatedAt,
		&server.UpdatedAt,
		&server.RevokedAt,
	)
	if err != nil {
		return DedicatedServer{}, fmt.Errorf("ensure dedicated server: %w", err)
	}
	return server, nil
}

// GetServerByCredentialHash loads an active server by credential hash.
func (s *Store) GetServerByCredentialHash(ctx context.Context, credentialHash []byte) (DedicatedServer, error) {
	var server DedicatedServer
	err := s.pool.QueryRow(ctx, `
		SELECT id, credential_hash, label, created_at, updated_at, revoked_at
		FROM dedicated_servers
		WHERE credential_hash = $1 AND revoked_at IS NULL
	`, credentialHash).Scan(
		&server.ID,
		&server.CredentialHash,
		&server.Label,
		&server.CreatedAt,
		&server.UpdatedAt,
		&server.RevokedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return DedicatedServer{}, ErrServerAuthInvalid
	}
	if err != nil {
		return DedicatedServer{}, fmt.Errorf("get dedicated server: %w", err)
	}
	return server, nil
}

// RegisterInstance creates or replaces the active instance for a server.
func (s *Store) RegisterInstance(ctx context.Context, serverID uuid.UUID, in RegisterInstanceInput, now time.Time) (GameInstance, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GameInstance{}, fmt.Errorf("begin register instance tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Revoke open tickets and mark prior instances offline for this server.
	_, err = tx.Exec(ctx, `
		UPDATE game_tickets
		SET revoked_at = $2
		WHERE server_id = $1
		  AND consumed_at IS NULL
		  AND revoked_at IS NULL
	`, serverID, now)
	if err != nil {
		return GameInstance{}, fmt.Errorf("revoke tickets on register: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE game_instances
		SET status = $2,
		    active_match_id = NULL,
		    occupied_slots = 0,
		    updated_at = $3
		WHERE server_id = $1
		  AND status <> $2
	`, serverID, StatusOffline, now)
	if err != nil {
		return GameInstance{}, fmt.Errorf("offline prior instances: %w", err)
	}

	// Abort open matches on this server; free player open-membership slots.
	_, err = tx.Exec(ctx, `
		UPDATE match_players mp
		SET join_state = $2
		FROM matches m
		WHERE mp.match_id = m.id
		  AND m.server_id = $1
		  AND m.state IN ($3, $4)
		  AND mp.join_state IN ($5, $6)
	`, serverID, JoinLeft, MatchAllocating, MatchActive, JoinAssigned, JoinAdmitted)
	if err != nil {
		return GameInstance{}, fmt.Errorf("release match players on register: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE matches
		SET state = $2,
		    ended_at = $3,
		    updated_at = $3
		WHERE server_id = $1
		  AND state IN ($4, $5)
	`, serverID, MatchAborted, now, MatchAllocating, MatchActive)
	if err != nil {
		return GameInstance{}, fmt.Errorf("abort open matches on register: %w", err)
	}
	if err := s.AbortRoomsForServerTx(ctx, tx, serverID, now); err != nil {
		return GameInstance{}, err
	}

	capacity := in.Capacity
	if capacity <= 0 {
		capacity = 4
	}
	if capacity > 4 {
		capacity = 4
	}
	status := in.Status
	if status == "" {
		status = StatusStarting
	}

	var inst GameInstance
	err = tx.QueryRow(ctx, `
		INSERT INTO game_instances (
			server_id, host, port, transport, status, protocol_version, server_build,
			capacity, occupied_slots, last_heartbeat_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0,$9,$9,$9)
		RETURNING id, server_id, host, port, transport, status, protocol_version, server_build,
		          capacity, occupied_slots, active_match_id, last_heartbeat_at, created_at, updated_at
	`, serverID, in.Host, in.Port, in.Transport, status, in.ProtocolVersion, in.ServerBuild,
		capacity, now,
	).Scan(
		&inst.ID,
		&inst.ServerID,
		&inst.Host,
		&inst.Port,
		&inst.Transport,
		&inst.Status,
		&inst.ProtocolVersion,
		&inst.ServerBuild,
		&inst.Capacity,
		&inst.OccupiedSlots,
		&inst.ActiveMatchID,
		&inst.LastHeartbeatAt,
		&inst.CreatedAt,
		&inst.UpdatedAt,
	)
	if err != nil {
		return GameInstance{}, fmt.Errorf("insert game instance: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return GameInstance{}, fmt.Errorf("commit register instance: %w", err)
	}
	return inst, nil
}

// GetInstanceForServer loads an instance owned by serverID.
func (s *Store) GetInstanceForServer(ctx context.Context, instanceID, serverID uuid.UUID) (GameInstance, error) {
	inst, err := s.scanInstance(ctx, s.pool, `
		SELECT id, server_id, host, port, transport, status, protocol_version, server_build,
		       capacity, occupied_slots, active_match_id, last_heartbeat_at, created_at, updated_at
		FROM game_instances
		WHERE id = $1 AND server_id = $2
	`, instanceID, serverID)
	if errors.Is(err, pgx.ErrNoRows) {
		return GameInstance{}, ErrNotFound
	}
	return inst, err
}

// Heartbeat updates instance liveness and status.
func (s *Store) Heartbeat(ctx context.Context, instanceID, serverID uuid.UUID, in HeartbeatInput, now time.Time) (GameInstance, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GameInstance{}, fmt.Errorf("begin heartbeat tx: %w", err)
	}
	defer tx.Rollback(ctx)

	inst, err := s.scanInstance(ctx, tx, `
		SELECT id, server_id, host, port, transport, status, protocol_version, server_build,
		       capacity, occupied_slots, active_match_id, last_heartbeat_at, created_at, updated_at
		FROM game_instances
		WHERE id = $1 AND server_id = $2
		FOR UPDATE
	`, instanceID, serverID)
	if errors.Is(err, pgx.ErrNoRows) {
		return GameInstance{}, ErrNotFound
	}
	if err != nil {
		return GameInstance{}, err
	}

	status := in.Status
	if status == "" {
		status = inst.Status
	}
	if !IsValidInstanceStatus(status) {
		return GameInstance{}, ErrInvalidInput
	}

	var activeMatch any
	if in.ActiveMatchID != nil {
		activeMatch = *in.ActiveMatchID
	} else {
		activeMatch = inst.ActiveMatchID
	}

	err = tx.QueryRow(ctx, `
		UPDATE game_instances
		SET status = $3,
		    last_heartbeat_at = $4,
		    active_match_id = $5,
		    updated_at = $4
		WHERE id = $1 AND server_id = $2
		RETURNING id, server_id, host, port, transport, status, protocol_version, server_build,
		          capacity, occupied_slots, active_match_id, last_heartbeat_at, created_at, updated_at
	`, instanceID, serverID, status, now, activeMatch).Scan(
		&inst.ID,
		&inst.ServerID,
		&inst.Host,
		&inst.Port,
		&inst.Transport,
		&inst.Status,
		&inst.ProtocolVersion,
		&inst.ServerBuild,
		&inst.Capacity,
		&inst.OccupiedSlots,
		&inst.ActiveMatchID,
		&inst.LastHeartbeatAt,
		&inst.CreatedAt,
		&inst.UpdatedAt,
	)
	if err != nil {
		return GameInstance{}, fmt.Errorf("update heartbeat: %w", err)
	}

	if status == StatusOffline || status == StatusDraining {
		_, err = tx.Exec(ctx, `
			UPDATE game_tickets
			SET revoked_at = $2
			WHERE game_instance_id = $1
			  AND consumed_at IS NULL
			  AND revoked_at IS NULL
		`, instanceID, now)
		if err != nil {
			return GameInstance{}, fmt.Errorf("revoke tickets on drain/offline: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return GameInstance{}, fmt.Errorf("commit heartbeat: %w", err)
	}
	return inst, nil
}

// AssignPlayer allocates a slot and issues a ticket atomically.
func (s *Store) AssignPlayer(
	ctx context.Context,
	playerID uuid.UUID,
	mode string,
	ticketHash []byte,
	ticketTTL time.Duration,
	heartbeatTTL time.Duration,
	now time.Time,
) (Assignment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Assignment{}, fmt.Errorf("begin assign tx: %w", err)
	}
	defer tx.Rollback(ctx)

	heartbeatFloor := now.Add(-heartbeatTTL)

	// If player already has an open match membership, reissue ticket for that match when mode matches.
	var existingMatchID uuid.UUID
	var existingInstanceID uuid.UUID
	var existingServerID uuid.UUID
	var existingMode string
	var existingHost string
	var existingPort int
	var existingTransport string
	var existingProtocol int
	var existingBuild string
	err = tx.QueryRow(ctx, `
		SELECT m.id, m.game_instance_id, m.server_id, m.mode,
		       gi.host, gi.port, gi.transport, gi.protocol_version, gi.server_build
		FROM match_players mp
		JOIN matches m ON m.id = mp.match_id
		JOIN game_instances gi ON gi.id = m.game_instance_id
		WHERE mp.player_id = $1
		  AND mp.join_state IN ($2, $3)
		  AND m.state IN ($4, $5)
		FOR UPDATE OF mp, m, gi
	`, playerID, JoinAssigned, JoinAdmitted, MatchAllocating, MatchActive).Scan(
		&existingMatchID,
		&existingInstanceID,
		&existingServerID,
		&existingMode,
		&existingHost,
		&existingPort,
		&existingTransport,
		&existingProtocol,
		&existingBuild,
	)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, fmt.Errorf("lookup existing assignment: %w", err)
	}
	if err == nil {
		if existingMode != mode {
			return Assignment{}, ErrInvalidInput
		}
		expiresAt := now.Add(ticketTTL)
		if err := s.revokeOpenTicketsTx(ctx, tx, existingMatchID, playerID, now); err != nil {
			return Assignment{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO game_tickets (
				token_hash, player_id, match_id, game_instance_id, server_id, expires_at, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, ticketHash, playerID, existingMatchID, existingInstanceID, existingServerID, expiresAt, now); err != nil {
			return Assignment{}, fmt.Errorf("insert reissue ticket: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return Assignment{}, fmt.Errorf("commit reissue assign: %w", err)
		}
		return Assignment{
			GameInstanceID:  existingInstanceID,
			MatchID:         existingMatchID,
			ServerID:        existingServerID,
			Host:            existingHost,
			Port:            existingPort,
			Transport:       existingTransport,
			TicketExpiresAt: expiresAt,
			ProtocolVersion: existingProtocol,
			ServerBuild:     existingBuild,
			Mode:            existingMode,
		}, nil
	}

	// Serialize on the ready instance row so concurrent assigns cannot overfill
	// and do not spuriously observe capacity-full via SKIP LOCKED.
	inst, err := s.scanInstance(ctx, tx, `
		SELECT id, server_id, host, port, transport, status, protocol_version, server_build,
		       capacity, occupied_slots, active_match_id, last_heartbeat_at, created_at, updated_at
		FROM game_instances
		WHERE status = $1
		  AND last_heartbeat_at IS NOT NULL
		  AND last_heartbeat_at >= $2
		  AND occupied_slots < capacity
		ORDER BY last_heartbeat_at DESC
		LIMIT 1
		FOR UPDATE
	`, StatusReady, heartbeatFloor)
	if errors.Is(err, pgx.ErrNoRows) {
		// Distinguish unavailable vs full without SKIP LOCKED.
		var liveCount int
		_ = tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM game_instances
			WHERE status IN ($1, $2, $3)
			  AND last_heartbeat_at IS NOT NULL
			  AND last_heartbeat_at >= $4
		`, StatusReady, StatusFull, StatusDraining, heartbeatFloor).Scan(&liveCount)
		if liveCount == 0 {
			return Assignment{}, ErrGameServerUnavailable
		}
		return Assignment{}, ErrGameCapacityFull
	}
	if err != nil {
		return Assignment{}, err
	}
	if inst.OccupiedSlots >= inst.Capacity {
		return Assignment{}, ErrGameCapacityFull
	}

	var matchID uuid.UUID
	var matchMode string
	var matchState string
	err = tx.QueryRow(ctx, `
		SELECT id, mode, state
		FROM matches
		WHERE game_instance_id = $1
		  AND state IN ($2, $3)
		ORDER BY created_at ASC
		LIMIT 1
		FOR UPDATE
	`, inst.ID, MatchAllocating, MatchActive).Scan(&matchID, &matchMode, &matchState)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, fmt.Errorf("lookup open match: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO matches (
				game_instance_id, server_id, mode, state, max_players, started_at, created_at, updated_at
			) VALUES ($1,$2,$3,$4,$5,$6,$6,$6)
			RETURNING id
		`, inst.ID, inst.ServerID, mode, MatchAllocating, inst.Capacity, now).Scan(&matchID)
		if err != nil {
			return Assignment{}, fmt.Errorf("insert match: %w", err)
		}
		matchMode = mode
		matchState = MatchAllocating
		_, err = tx.Exec(ctx, `
			UPDATE game_instances
			SET active_match_id = $2, updated_at = $3
			WHERE id = $1
		`, inst.ID, matchID, now)
		if err != nil {
			return Assignment{}, fmt.Errorf("set active match: %w", err)
		}
	} else if matchMode != mode {
		return Assignment{}, ErrGameCapacityFull
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO match_players (match_id, player_id, join_state, assigned_at)
		VALUES ($1, $2, $3, $4)
	`, matchID, playerID, JoinAssigned, now)
	if err != nil {
		if isUniqueViolation(err) {
			return Assignment{}, ErrGameCapacityFull
		}
		return Assignment{}, fmt.Errorf("insert match player: %w", err)
	}

	var occupied int
	err = tx.QueryRow(ctx, `
		UPDATE game_instances
		SET occupied_slots = occupied_slots + 1,
		    status = CASE WHEN occupied_slots + 1 >= capacity THEN $3 ELSE status END,
		    updated_at = $2
		WHERE id = $1 AND occupied_slots < capacity
		RETURNING occupied_slots
	`, inst.ID, now, StatusFull).Scan(&occupied)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, ErrGameCapacityFull
	}
	if err != nil {
		return Assignment{}, fmt.Errorf("increment occupied slots: %w", err)
	}
	_ = occupied

	if matchState == MatchAllocating {
		_, err = tx.Exec(ctx, `
			UPDATE matches SET state = $2, updated_at = $3 WHERE id = $1 AND state = $4
		`, matchID, MatchActive, now, MatchAllocating)
		if err != nil {
			return Assignment{}, fmt.Errorf("activate match: %w", err)
		}
	}

	expiresAt := now.Add(ticketTTL)
	_, err = tx.Exec(ctx, `
		INSERT INTO game_tickets (
			token_hash, player_id, match_id, game_instance_id, server_id, expires_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, ticketHash, playerID, matchID, inst.ID, inst.ServerID, expiresAt, now)
	if err != nil {
		return Assignment{}, fmt.Errorf("insert ticket: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return Assignment{}, fmt.Errorf("commit assign: %w", err)
	}

	return Assignment{
		GameInstanceID:  inst.ID,
		MatchID:         matchID,
		ServerID:        inst.ServerID,
		Host:            inst.Host,
		Port:            inst.Port,
		Transport:       inst.Transport,
		TicketExpiresAt: expiresAt,
		ProtocolVersion: inst.ProtocolVersion,
		ServerBuild:     inst.ServerBuild,
		Mode:            mode,
	}, nil
}

func (s *Store) revokeOpenTicketsTx(ctx context.Context, tx pgx.Tx, matchID, playerID uuid.UUID, now time.Time) error {
	_, err := tx.Exec(ctx, `
		UPDATE game_tickets
		SET revoked_at = $3
		WHERE match_id = $1
		  AND player_id = $2
		  AND consumed_at IS NULL
		  AND revoked_at IS NULL
	`, matchID, playerID, now)
	if err != nil {
		return fmt.Errorf("revoke prior tickets: %w", err)
	}
	return nil
}

// ValidateAndConsumeTicket validates binding and consumes the ticket once.
func (s *Store) ValidateAndConsumeTicket(
	ctx context.Context,
	serverID uuid.UUID,
	tokenHash []byte,
	playerID, matchID, instanceID uuid.UUID,
	now time.Time,
) (GameTicket, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return GameTicket{}, fmt.Errorf("begin validate ticket tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var ticket GameTicket
	err = tx.QueryRow(ctx, `
		SELECT id, token_hash, player_id, match_id, game_instance_id, server_id,
		       expires_at, consumed_at, revoked_at, created_at
		FROM game_tickets
		WHERE token_hash = $1
		FOR UPDATE
	`, tokenHash).Scan(
		&ticket.ID,
		&ticket.TokenHash,
		&ticket.PlayerID,
		&ticket.MatchID,
		&ticket.GameInstanceID,
		&ticket.ServerID,
		&ticket.ExpiresAt,
		&ticket.ConsumedAt,
		&ticket.RevokedAt,
		&ticket.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GameTicket{}, ErrTicketInvalid
	}
	if err != nil {
		return GameTicket{}, fmt.Errorf("load ticket: %w", err)
	}

	if ticket.ServerID != serverID {
		return GameTicket{}, ErrTicketInvalid
	}
	if ticket.PlayerID != playerID || ticket.MatchID != matchID || ticket.GameInstanceID != instanceID {
		return GameTicket{}, ErrTicketInvalid
	}
	if ticket.RevokedAt != nil {
		return GameTicket{}, ErrTicketRevoked
	}
	if ticket.ConsumedAt != nil {
		return GameTicket{}, ErrTicketReplayed
	}
	if !ticket.ExpiresAt.After(now) {
		return GameTicket{}, ErrTicketExpired
	}

	err = tx.QueryRow(ctx, `
		UPDATE game_tickets
		SET consumed_at = $2
		WHERE id = $1 AND consumed_at IS NULL AND revoked_at IS NULL AND expires_at > $2
		RETURNING id, token_hash, player_id, match_id, game_instance_id, server_id,
		          expires_at, consumed_at, revoked_at, created_at
	`, ticket.ID, now).Scan(
		&ticket.ID,
		&ticket.TokenHash,
		&ticket.PlayerID,
		&ticket.MatchID,
		&ticket.GameInstanceID,
		&ticket.ServerID,
		&ticket.ExpiresAt,
		&ticket.ConsumedAt,
		&ticket.RevokedAt,
		&ticket.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return GameTicket{}, ErrTicketReplayed
	}
	if err != nil {
		return GameTicket{}, fmt.Errorf("consume ticket: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE match_players
		SET join_state = $3, admitted_at = $4
		WHERE match_id = $1 AND player_id = $2 AND join_state = $5
	`, matchID, playerID, JoinAdmitted, now, JoinAssigned)
	if err != nil {
		return GameTicket{}, fmt.Errorf("mark player admitted: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return GameTicket{}, fmt.Errorf("commit validate ticket: %w", err)
	}
	return ticket, nil
}

// SettlementPlayerResult is one player line in a settlement payload.
type SettlementPlayerResult struct {
	PlayerID uuid.UUID
	TeamID   string
	Kills    int
	Deaths   int
	Score    int
}

// ProcessSettlement applies an idempotent trusted settlement.
func (s *Store) ProcessSettlement(
	ctx context.Context,
	serverID uuid.UUID,
	matchID uuid.UUID,
	settlementID uuid.UUID,
	result string,
	startedAt, endedAt time.Time,
	players []SettlementPlayerResult,
	payload json.RawMessage,
	now time.Time,
) (already bool, err error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin settlement tx: %w", err)
	}
	defer tx.Rollback(ctx)

	var existing Settlement
	err = tx.QueryRow(ctx, `
		SELECT settlement_id, match_id, game_instance_id, server_id, result, payload_json, status, received_at, processed_at
		FROM match_settlements
		WHERE settlement_id = $1
	`, settlementID).Scan(
		&existing.SettlementID,
		&existing.MatchID,
		&existing.GameInstanceID,
		&existing.ServerID,
		&existing.Result,
		&existing.PayloadJSON,
		&existing.Status,
		&existing.ReceivedAt,
		&existing.ProcessedAt,
	)
	if err == nil {
		if existing.MatchID != matchID || existing.ServerID != serverID {
			return false, ErrForbidden
		}
		if err := tx.Commit(ctx); err != nil {
			return false, fmt.Errorf("commit idempotent settlement: %w", err)
		}
		return true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("lookup settlement: %w", err)
	}

	var match Match
	err = tx.QueryRow(ctx, `
		SELECT id, room_id, game_instance_id, server_id, mode, state, max_players, started_at, ended_at, created_at, updated_at
		FROM matches
		WHERE id = $1
		FOR UPDATE
	`, matchID).Scan(
		&match.ID,
		&match.RoomID,
		&match.GameInstanceID,
		&match.ServerID,
		&match.Mode,
		&match.State,
		&match.MaxPlayers,
		&match.StartedAt,
		&match.EndedAt,
		&match.CreatedAt,
		&match.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	if err != nil {
		return false, fmt.Errorf("load match: %w", err)
	}
	if match.ServerID != serverID {
		return false, ErrForbidden
	}
	if match.State != MatchAllocating && match.State != MatchActive {
		return false, ErrMatchNotSettleable
	}

	// Ensure no other settlement already claimed this match.
	var other uuid.UUID
	err = tx.QueryRow(ctx, `SELECT settlement_id FROM match_settlements WHERE match_id = $1`, matchID).Scan(&other)
	if err == nil {
		return false, ErrMatchNotSettleable
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("lookup match settlement: %w", err)
	}

	for _, p := range players {
		tag, execErr := tx.Exec(ctx, `
			UPDATE match_players
			SET team_id = $3, kills = $4, deaths = $5, score = $6, join_state = $7
			WHERE match_id = $1 AND player_id = $2
		`, matchID, p.PlayerID, p.TeamID, p.Kills, p.Deaths, p.Score, JoinLeft)
		if execErr != nil {
			return false, fmt.Errorf("update match player result: %w", execErr)
		}
		if tag.RowsAffected() == 0 {
			return false, ErrInvalidInput
		}
		_, execErr = tx.Exec(ctx, `
			INSERT INTO player_stats (player_id, matches_played, kills, deaths, score, updated_at)
			VALUES ($1, 1, $2, $3, $4, $5)
			ON CONFLICT (player_id) DO UPDATE
			SET matches_played = player_stats.matches_played + 1,
			    kills = player_stats.kills + EXCLUDED.kills,
			    deaths = player_stats.deaths + EXCLUDED.deaths,
			    score = player_stats.score + EXCLUDED.score,
			    updated_at = EXCLUDED.updated_at
		`, p.PlayerID, p.Kills, p.Deaths, p.Score, now)
		if execErr != nil {
			return false, fmt.Errorf("upsert player stats: %w", execErr)
		}
	}

	// Mark any remaining roster as left without stats (DS omitted them).
	_, err = tx.Exec(ctx, `
		UPDATE match_players
		SET join_state = $2
		WHERE match_id = $1 AND join_state IN ($3, $4)
	`, matchID, JoinLeft, JoinAssigned, JoinAdmitted)
	if err != nil {
		return false, fmt.Errorf("close remaining roster: %w", err)
	}

	finalState := MatchCompleted
	if result == "aborted" {
		finalState = MatchAborted
	}
	_, err = tx.Exec(ctx, `
		UPDATE matches
		SET state = $2, started_at = COALESCE(started_at, $3), ended_at = $4, updated_at = $5
		WHERE id = $1
	`, matchID, finalState, startedAt, endedAt, now)
	if err != nil {
		return false, fmt.Errorf("close match: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE game_instances
		SET occupied_slots = 0,
		    active_match_id = NULL,
		    status = CASE WHEN status = $3 THEN $4 ELSE status END,
		    updated_at = $2
		WHERE id = $1
	`, match.GameInstanceID, now, StatusFull, StatusReady)
	if err != nil {
		return false, fmt.Errorf("free instance capacity: %w", err)
	}

	_, err = tx.Exec(ctx, `
		UPDATE game_tickets
		SET revoked_at = $2
		WHERE match_id = $1 AND consumed_at IS NULL AND revoked_at IS NULL
	`, matchID, now)
	if err != nil {
		return false, fmt.Errorf("revoke leftover tickets: %w", err)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO match_settlements (
			settlement_id, match_id, game_instance_id, server_id, result, payload_json, status, received_at, processed_at
		) VALUES ($1,$2,$3,$4,$5,$6,'processed',$7,$7)
	`, settlementID, matchID, match.GameInstanceID, serverID, result, payload, now)
	if err != nil {
		if isUniqueViolation(err) {
			// Lost race on settlement_id or match_id — re-read for idempotency.
			err2 := tx.QueryRow(ctx, `
				SELECT settlement_id FROM match_settlements WHERE settlement_id = $1 AND match_id = $2 AND server_id = $3
			`, settlementID, matchID, serverID).Scan(&settlementID)
			if err2 == nil {
				if err := tx.Commit(ctx); err != nil {
					return false, err
				}
				return true, nil
			}
			return false, ErrMatchNotSettleable
		}
		return false, fmt.Errorf("insert settlement: %w", err)
	}

	if err := s.CloseRoomsForMatchTx(ctx, tx, matchID, now); err != nil {
		return false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit settlement: %w", err)
	}
	return false, nil
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func (s *Store) scanInstance(ctx context.Context, q querier, sql string, args ...any) (GameInstance, error) {
	var inst GameInstance
	err := q.QueryRow(ctx, sql, args...).Scan(
		&inst.ID,
		&inst.ServerID,
		&inst.Host,
		&inst.Port,
		&inst.Transport,
		&inst.Status,
		&inst.ProtocolVersion,
		&inst.ServerBuild,
		&inst.Capacity,
		&inst.OccupiedSlots,
		&inst.ActiveMatchID,
		&inst.LastHeartbeatAt,
		&inst.CreatedAt,
		&inst.UpdatedAt,
	)
	if err != nil {
		return GameInstance{}, err
	}
	return inst, nil
}
