package match

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PlayerTicketHash pairs a player with a pre-generated ticket hash.
type PlayerTicketHash struct {
	PlayerID  uuid.UUID
	TokenHash []byte
}

// RosterAssignmentMeta is assignment data without the raw ticket secret.
type RosterAssignmentMeta struct {
	GameInstanceID  uuid.UUID
	MatchID         uuid.UUID
	ServerID        uuid.UUID
	Host            string
	Port            int
	Transport       string
	TicketExpiresAt time.Time
	ProtocolVersion int
	ServerBuild     string
	Mode            string
	PlayerID        uuid.UUID
}

// HasOpenMatchMembership reports whether the player is already in an open match.
func (s *Store) HasOpenMatchMembership(ctx context.Context, playerID uuid.UUID) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM match_players mp
			JOIN matches m ON m.id = mp.match_id
			WHERE mp.player_id = $1
			  AND mp.join_state IN ($2, $3)
			  AND m.state IN ($4, $5)
		)
	`, playerID, JoinAssigned, JoinAdmitted, MatchAllocating, MatchActive).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check open match membership: %w", err)
	}
	return exists, nil
}

// AssignRosterTx creates a new match bound to roomID for the full roster and issues tickets.
// Requires an open transaction. Does not mutate room rows.
func (s *Store) AssignRosterTx(
	ctx context.Context,
	tx pgx.Tx,
	roomID uuid.UUID,
	mode string,
	roster []PlayerTicketHash,
	ticketTTL time.Duration,
	heartbeatTTL time.Duration,
	now time.Time,
) ([]RosterAssignmentMeta, error) {
	if len(roster) == 0 {
		return nil, ErrInvalidInput
	}
	if len(roster) > 4 {
		return nil, ErrGameCapacityFull
	}

	for _, p := range roster {
		var open bool
		err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM match_players mp
				JOIN matches m ON m.id = mp.match_id
				WHERE mp.player_id = $1
				  AND mp.join_state IN ($2, $3)
				  AND m.state IN ($4, $5)
			)
		`, p.PlayerID, JoinAssigned, JoinAdmitted, MatchAllocating, MatchActive).Scan(&open)
		if err != nil {
			return nil, fmt.Errorf("check roster open match: %w", err)
		}
		if open {
			return nil, ErrMatchAlreadyActive
		}
	}

	heartbeatFloor := now.Add(-heartbeatTTL)
	need := len(roster)

	inst, err := s.scanInstance(ctx, tx, `
		SELECT id, server_id, host, port, transport, status, protocol_version, server_build,
		       capacity, occupied_slots, active_match_id, last_heartbeat_at, created_at, updated_at
		FROM game_instances
		WHERE status = $1
		  AND last_heartbeat_at IS NOT NULL
		  AND last_heartbeat_at >= $2
		  AND occupied_slots = 0
		  AND capacity >= $3
		ORDER BY last_heartbeat_at DESC
		LIMIT 1
		FOR UPDATE
	`, StatusReady, heartbeatFloor, need)
	if errors.Is(err, pgx.ErrNoRows) {
		var liveCount int
		_ = tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM game_instances
			WHERE status IN ($1, $2, $3)
			  AND last_heartbeat_at IS NOT NULL
			  AND last_heartbeat_at >= $4
		`, StatusReady, StatusFull, StatusDraining, heartbeatFloor).Scan(&liveCount)
		if liveCount == 0 {
			return nil, ErrGameServerUnavailable
		}
		return nil, ErrGameCapacityFull
	}
	if err != nil {
		return nil, err
	}

	var matchID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO matches (
			room_id, game_instance_id, server_id, mode, state, max_players, started_at, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$7)
		RETURNING id
	`, roomID, inst.ID, inst.ServerID, mode, MatchActive, inst.Capacity, now).Scan(&matchID)
	if err != nil {
		return nil, fmt.Errorf("insert room match: %w", err)
	}

	tag, err := tx.Exec(ctx, `
		UPDATE game_instances
		SET active_match_id = $2,
		    occupied_slots = $3,
		    status = CASE WHEN $3 >= capacity THEN $5 ELSE status END,
		    updated_at = $4
		WHERE id = $1 AND occupied_slots = 0 AND capacity >= $3
	`, inst.ID, matchID, need, now, StatusFull)
	if err != nil {
		return nil, fmt.Errorf("claim instance for room match: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return nil, ErrGameCapacityFull
	}

	expiresAt := now.Add(ticketTTL)
	out := make([]RosterAssignmentMeta, 0, len(roster))
	for _, p := range roster {
		_, err = tx.Exec(ctx, `
			INSERT INTO match_players (match_id, player_id, join_state, assigned_at)
			VALUES ($1, $2, $3, $4)
		`, matchID, p.PlayerID, JoinAssigned, now)
		if err != nil {
			if isUniqueViolation(err) {
				return nil, ErrMatchAlreadyActive
			}
			return nil, fmt.Errorf("insert room match player: %w", err)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO game_tickets (
				token_hash, player_id, match_id, game_instance_id, server_id, expires_at, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, p.TokenHash, p.PlayerID, matchID, inst.ID, inst.ServerID, expiresAt, now)
		if err != nil {
			return nil, fmt.Errorf("insert room ticket: %w", err)
		}
		out = append(out, RosterAssignmentMeta{
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
			PlayerID:        p.PlayerID,
		})
	}
	return out, nil
}

// ReissueTicketTx revokes prior unconsumed tickets and inserts a new hash for player/match.
func (s *Store) ReissueTicketTx(
	ctx context.Context,
	tx pgx.Tx,
	playerID, matchID uuid.UUID,
	tokenHash []byte,
	ticketTTL time.Duration,
	now time.Time,
) (RosterAssignmentMeta, error) {
	var meta RosterAssignmentMeta
	err := tx.QueryRow(ctx, `
		SELECT m.id, m.game_instance_id, m.server_id, m.mode,
		       gi.host, gi.port, gi.transport, gi.protocol_version, gi.server_build
		FROM match_players mp
		JOIN matches m ON m.id = mp.match_id
		JOIN game_instances gi ON gi.id = m.game_instance_id
		WHERE mp.player_id = $1
		  AND mp.match_id = $2
		  AND mp.join_state IN ($3, $4)
		  AND m.state IN ($5, $6)
		FOR UPDATE OF mp, m, gi
	`, playerID, matchID, JoinAssigned, JoinAdmitted, MatchAllocating, MatchActive).Scan(
		&meta.MatchID,
		&meta.GameInstanceID,
		&meta.ServerID,
		&meta.Mode,
		&meta.Host,
		&meta.Port,
		&meta.Transport,
		&meta.ProtocolVersion,
		&meta.ServerBuild,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RosterAssignmentMeta{}, ErrNotFound
	}
	if err != nil {
		return RosterAssignmentMeta{}, fmt.Errorf("load match for reissue: %w", err)
	}
	meta.PlayerID = playerID
	if err := s.revokeOpenTicketsTx(ctx, tx, matchID, playerID, now); err != nil {
		return RosterAssignmentMeta{}, err
	}
	expiresAt := now.Add(ticketTTL)
	_, err = tx.Exec(ctx, `
		INSERT INTO game_tickets (
			token_hash, player_id, match_id, game_instance_id, server_id, expires_at, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7)
	`, tokenHash, playerID, matchID, meta.GameInstanceID, meta.ServerID, expiresAt, now)
	if err != nil {
		return RosterAssignmentMeta{}, fmt.Errorf("insert reissue room ticket: %w", err)
	}
	meta.TicketExpiresAt = expiresAt
	return meta, nil
}

// CloseRoomsForMatchTx marks rooms bound to matchID as closed and clears members.
func (s *Store) CloseRoomsForMatchTx(ctx context.Context, tx pgx.Tx, matchID uuid.UUID, now time.Time) error {
	rows, err := tx.Query(ctx, `
		SELECT id FROM rooms WHERE match_id = $1 AND state IN ('starting', 'in_game')
		FOR UPDATE
	`, matchID)
	if err != nil {
		return fmt.Errorf("list rooms for match: %w", err)
	}
	defer rows.Close()
	var roomIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan room id: %w", err)
		}
		roomIDs = append(roomIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, roomID := range roomIDs {
		if _, err := tx.Exec(ctx, `DELETE FROM room_members WHERE room_id = $1`, roomID); err != nil {
			return fmt.Errorf("clear room members on settle: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rooms
			SET state = 'closed', updated_at = $2, version = version + 1
			WHERE id = $1
		`, roomID, now); err != nil {
			return fmt.Errorf("close room on settle: %w", err)
		}
	}
	return nil
}

// AbortRoomsForServerTx closes lobby rooms whose matches were aborted for a server.
func (s *Store) AbortRoomsForServerTx(ctx context.Context, tx pgx.Tx, serverID uuid.UUID, now time.Time) error {
	_, err := tx.Exec(ctx, `
		DELETE FROM room_members rm
		USING rooms r
		JOIN matches m ON m.id = r.match_id
		WHERE rm.room_id = r.id
		  AND m.server_id = $1
		  AND r.state IN ('starting', 'in_game')
	`, serverID)
	if err != nil {
		return fmt.Errorf("clear rooms on server re-register: %w", err)
	}
	_, err = tx.Exec(ctx, `
		UPDATE rooms r
		SET state = 'aborted', updated_at = $2, version = version + 1
		FROM matches m
		WHERE r.match_id = m.id
		  AND m.server_id = $1
		  AND r.state IN ('starting', 'in_game')
	`, serverID, now)
	if err != nil {
		return fmt.Errorf("abort rooms on server re-register: %w", err)
	}
	return nil
}
