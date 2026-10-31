package room

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ZhiH2333/WildBoat/internal/match"
)

// Store persists rooms in PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs a room store.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Begin starts a transaction.
func (s *Store) Begin(ctx context.Context) (pgx.Tx, error) {
	return s.pool.Begin(ctx)
}

// Pool exposes the underlying pool for cross-store transactions.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

func scanRoom(row pgx.Row) (Room, error) {
	var r Room
	err := row.Scan(
		&r.ID,
		&r.RoomCode,
		&r.OwnerPlayerID,
		&r.PartyID,
		&r.Mode,
		&r.Visibility,
		&r.State,
		&r.MaxPlayers,
		&r.MatchID,
		&r.Version,
		&r.CreatedAt,
		&r.UpdatedAt,
		&r.ExpiresAt,
	)
	return r, err
}

const roomColumns = `id, room_code, owner_player_id, party_id, mode, visibility, state, max_players,
	match_id, version, created_at, updated_at, expires_at`

// ExpireIdleRooms marks idle open rooms as expired and clears members.
func (s *Store) ExpireIdleRooms(ctx context.Context, now time.Time) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id FROM rooms
		WHERE state = $1 AND expires_at <= $2
		FOR UPDATE
	`, StateOpen, now)
	if err != nil {
		return 0, fmt.Errorf("list expired rooms: %w", err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, id := range ids {
		if _, err := tx.Exec(ctx, `DELETE FROM room_members WHERE room_id = $1`, id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rooms SET state = $2, updated_at = $3, version = version + 1 WHERE id = $1
		`, id, StateExpired, now); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

func (s *Store) expireRoomIfNeededTx(ctx context.Context, tx pgx.Tx, r *Room, now time.Time) error {
	if r.State != StateOpen || r.ExpiresAt.After(now) {
		return nil
	}
	if _, err := tx.Exec(ctx, `DELETE FROM room_members WHERE room_id = $1`, r.ID); err != nil {
		return fmt.Errorf("clear expired room members: %w", err)
	}
	err := tx.QueryRow(ctx, `
		UPDATE rooms SET state = $2, updated_at = $3, version = version + 1
		WHERE id = $1
		RETURNING `+roomColumns+`
	`, r.ID, StateExpired, now).Scan(
		&r.ID, &r.RoomCode, &r.OwnerPlayerID, &r.PartyID, &r.Mode, &r.Visibility, &r.State,
		&r.MaxPlayers, &r.MatchID, &r.Version, &r.CreatedAt, &r.UpdatedAt, &r.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("expire room: %w", err)
	}
	return nil
}

// CreatePrivateRoom inserts a private lobby with the creator as owner.
func (s *Store) CreatePrivateRoom(
	ctx context.Context,
	ownerID uuid.UUID,
	mode string,
	maxPlayers int,
	idleTTL time.Duration,
	now time.Time,
) (Room, []Member, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Room{}, nil, err
	}
	defer tx.Rollback(ctx)

	if err := s.ensurePlayerFreeTx(ctx, tx, ownerID); err != nil {
		return Room{}, nil, err
	}
	if err := s.ensureNoOpenMatchTx(ctx, tx, ownerID); err != nil {
		return Room{}, nil, err
	}

	expiresAt := now.Add(idleTTL)
	var room Room
	for attempt := 0; attempt < 8; attempt++ {
		code, err := GenerateRoomCode()
		if err != nil {
			return Room{}, nil, err
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO rooms (
				room_code, owner_player_id, mode, visibility, state, max_players,
				created_at, updated_at, expires_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8)
			RETURNING `+roomColumns+`
		`, code, ownerID, mode, VisibilityPrivate, StateOpen, maxPlayers, now, expiresAt).Scan(
			&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
			&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
		)
		if err == nil {
			break
		}
		if isUniqueViolation(err) {
			continue
		}
		return Room{}, nil, fmt.Errorf("insert room: %w", err)
	}
	if room.ID == uuid.Nil {
		return Room{}, nil, fmt.Errorf("room code collision exhausted")
	}

	member, err := s.insertMemberTx(ctx, tx, room.ID, ownerID, false, now)
	if err != nil {
		return Room{}, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Room{}, nil, err
	}
	return room, []Member{member}, nil
}

func (s *Store) ensurePlayerFreeTx(ctx context.Context, tx pgx.Tx, playerID uuid.UUID) error {
	var existing uuid.UUID
	err := tx.QueryRow(ctx, `SELECT room_id FROM room_members WHERE player_id = $1`, playerID).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check active room membership: %w", err)
	}
	return ErrAlreadyIn
}

func (s *Store) ensureNoOpenMatchTx(ctx context.Context, tx pgx.Tx, playerID uuid.UUID) error {
	var open bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM match_players mp
			JOIN matches m ON m.id = mp.match_id
			WHERE mp.player_id = $1
			  AND mp.join_state IN ('assigned', 'admitted')
			  AND m.state IN ('allocating', 'active')
		)
	`, playerID).Scan(&open)
	if err != nil {
		return fmt.Errorf("check open match: %w", err)
	}
	if open {
		return ErrMatchAlreadyActive
	}
	return nil
}

func (s *Store) insertMemberTx(ctx context.Context, tx pgx.Tx, roomID, playerID uuid.UUID, ready bool, now time.Time) (Member, error) {
	var m Member
	err := tx.QueryRow(ctx, `
		INSERT INTO room_members (room_id, player_id, ready, joined_at, last_seen_at)
		VALUES ($1, $2, $3, $4, $4)
		RETURNING room_id, player_id, ready, joined_at, last_seen_at
	`, roomID, playerID, ready, now).Scan(&m.RoomID, &m.PlayerID, &m.Ready, &m.JoinedAt, &m.LastSeenAt)
	if err != nil {
		if isUniqueViolation(err) {
			return Member{}, ErrAlreadyIn
		}
		return Member{}, fmt.Errorf("insert room member: %w", err)
	}
	_ = tx.QueryRow(ctx, `SELECT display_name, avatar_id FROM players WHERE id = $1`, playerID).Scan(&m.DisplayName, &m.AvatarID)
	return m, nil
}

// JoinByCode adds a player to an open room identified by code.
func (s *Store) JoinByCode(ctx context.Context, playerID uuid.UUID, code string, now time.Time) (Room, []Member, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Room{}, nil, err
	}
	defer tx.Rollback(ctx)

	var room Room
	err = tx.QueryRow(ctx, `
		SELECT `+roomColumns+` FROM rooms WHERE room_code = $1
		ORDER BY CASE WHEN state IN ('open','starting','in_game') THEN 0 ELSE 1 END, created_at DESC
		LIMIT 1
		FOR UPDATE
	`, code).Scan(
		&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
		&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, nil, ErrCodeInvalid
	}
	if err != nil {
		return Room{}, nil, fmt.Errorf("lookup room by code: %w", err)
	}

	if err := s.expireRoomIfNeededTx(ctx, tx, &room, now); err != nil {
		return Room{}, nil, err
	}
	if IsTerminal(room.State) {
		return Room{}, nil, ErrCodeInvalid
	}

	// Idempotent: already a member of this room.
	var already uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT player_id FROM room_members WHERE room_id = $1 AND player_id = $2
	`, room.ID, playerID).Scan(&already)
	if err == nil {
		members, err := s.listMembersTx(ctx, tx, room.ID)
		if err != nil {
			return Room{}, nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Room{}, nil, err
		}
		return room, members, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Room{}, nil, err
	}

	if room.State != StateOpen {
		return Room{}, nil, ErrNotJoinable
	}

	if err := s.ensurePlayerFreeTx(ctx, tx, playerID); err != nil {
		return Room{}, nil, err
	}
	if err := s.ensureNoOpenMatchTx(ctx, tx, playerID); err != nil {
		return Room{}, nil, err
	}

	var count int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM room_members WHERE room_id = $1`, room.ID).Scan(&count); err != nil {
		return Room{}, nil, err
	}
	if count >= room.MaxPlayers {
		return Room{}, nil, ErrFull
	}

	if _, err := s.insertMemberTx(ctx, tx, room.ID, playerID, false, now); err != nil {
		return Room{}, nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE rooms SET updated_at = $2, version = version + 1 WHERE id = $1
	`, room.ID, now); err != nil {
		return Room{}, nil, err
	}
	room.UpdatedAt = now
	room.Version++

	members, err := s.listMembersTx(ctx, tx, room.ID)
	if err != nil {
		return Room{}, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Room{}, nil, err
	}
	return room, members, nil
}

func (s *Store) listMembersTx(ctx context.Context, tx pgx.Tx, roomID uuid.UUID) ([]Member, error) {
	rows, err := tx.Query(ctx, `
		SELECT rm.room_id, rm.player_id, p.display_name, p.avatar_id, rm.ready, rm.joined_at, rm.last_seen_at
		FROM room_members rm
		JOIN players p ON p.id = rm.player_id
		WHERE rm.room_id = $1
		ORDER BY rm.joined_at ASC
	`, roomID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.RoomID, &m.PlayerID, &m.DisplayName, &m.AvatarID, &m.Ready, &m.JoinedAt, &m.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetRoomForMember loads a room if the player is a current member.
func (s *Store) GetRoomForMember(ctx context.Context, roomID, playerID uuid.UUID, now time.Time) (Room, []Member, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Room{}, nil, err
	}
	defer tx.Rollback(ctx)

	var room Room
	err = tx.QueryRow(ctx, `SELECT `+roomColumns+` FROM rooms WHERE id = $1 FOR UPDATE`, roomID).Scan(
		&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
		&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, nil, ErrNotFound
	}
	if err != nil {
		return Room{}, nil, err
	}
	if err := s.expireRoomIfNeededTx(ctx, tx, &room, now); err != nil {
		return Room{}, nil, err
	}

	var memberCheck uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT player_id FROM room_members WHERE room_id = $1 AND player_id = $2
	`, roomID, playerID).Scan(&memberCheck)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, nil, ErrNotFound
	}
	if err != nil {
		return Room{}, nil, err
	}

	members, err := s.listMembersTx(ctx, tx, roomID)
	if err != nil {
		return Room{}, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Room{}, nil, err
	}
	return room, members, nil
}

// GetCurrentRoom returns the player's active room if any.
func (s *Store) GetCurrentRoom(ctx context.Context, playerID uuid.UUID, now time.Time) (Room, []Member, bool, error) {
	var roomID uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT room_id FROM room_members WHERE player_id = $1`, playerID).Scan(&roomID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, nil, false, nil
	}
	if err != nil {
		return Room{}, nil, false, err
	}
	room, members, err := s.GetRoomForMember(ctx, roomID, playerID, now)
	if errors.Is(err, ErrNotFound) {
		return Room{}, nil, false, nil
	}
	if err != nil {
		return Room{}, nil, false, err
	}
	return room, members, true, nil
}

// Leave removes the player from the room, transferring ownership or closing as needed.
func (s *Store) Leave(ctx context.Context, roomID, playerID uuid.UUID, now time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var room Room
	err = tx.QueryRow(ctx, `SELECT `+roomColumns+` FROM rooms WHERE id = $1 FOR UPDATE`, roomID).Scan(
		&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
		&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `DELETE FROM room_members WHERE room_id = $1 AND player_id = $2`, roomID, playerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil // idempotent
	}

	var remaining int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM room_members WHERE room_id = $1`, roomID).Scan(&remaining); err != nil {
		return err
	}
	if remaining == 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE rooms SET state = $2, updated_at = $3, version = version + 1 WHERE id = $1
		`, roomID, StateClosed, now); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	if room.OwnerPlayerID == playerID {
		var newOwner uuid.UUID
		err = tx.QueryRow(ctx, `
			SELECT player_id FROM room_members WHERE room_id = $1 ORDER BY joined_at ASC LIMIT 1
		`, roomID).Scan(&newOwner)
		if err != nil {
			return fmt.Errorf("pick new owner: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE rooms SET owner_player_id = $2, updated_at = $3, version = version + 1 WHERE id = $1
		`, roomID, newOwner, now); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE rooms SET updated_at = $2, version = version + 1 WHERE id = $1
		`, roomID, now); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// SetReady updates ready flag for a member in an open room.
func (s *Store) SetReady(ctx context.Context, roomID, playerID uuid.UUID, ready bool, now time.Time) (Room, []Member, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Room{}, nil, err
	}
	defer tx.Rollback(ctx)

	var room Room
	err = tx.QueryRow(ctx, `SELECT `+roomColumns+` FROM rooms WHERE id = $1 FOR UPDATE`, roomID).Scan(
		&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
		&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, nil, ErrNotFound
	}
	if err != nil {
		return Room{}, nil, err
	}
	if err := s.expireRoomIfNeededTx(ctx, tx, &room, now); err != nil {
		return Room{}, nil, err
	}
	if room.State != StateOpen {
		return Room{}, nil, ErrNotJoinable
	}

	tag, err := tx.Exec(ctx, `
		UPDATE room_members SET ready = $3, last_seen_at = $4
		WHERE room_id = $1 AND player_id = $2
	`, roomID, playerID, ready, now)
	if err != nil {
		return Room{}, nil, err
	}
	if tag.RowsAffected() == 0 {
		return Room{}, nil, ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
		UPDATE rooms SET updated_at = $2, version = version + 1 WHERE id = $1
	`, roomID, now); err != nil {
		return Room{}, nil, err
	}
	room.UpdatedAt = now
	room.Version++

	members, err := s.listMembersTx(ctx, tx, roomID)
	if err != nil {
		return Room{}, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Room{}, nil, err
	}
	return room, members, nil
}

// FindOrCreatePublicRoom implements Quick Match find-or-create.
func (s *Store) FindOrCreatePublicRoom(
	ctx context.Context,
	playerID uuid.UUID,
	mode string,
	maxPlayers int,
	idleTTL time.Duration,
	now time.Time,
) (Room, []Member, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Room{}, nil, false, err
	}
	defer tx.Rollback(ctx)

	// If already in a room: prefer return when same mode public; else conflict.
	var existingRoomID uuid.UUID
	err = tx.QueryRow(ctx, `SELECT room_id FROM room_members WHERE player_id = $1`, playerID).Scan(&existingRoomID)
	if err == nil {
		var room Room
		err = tx.QueryRow(ctx, `SELECT `+roomColumns+` FROM rooms WHERE id = $1 FOR UPDATE`, existingRoomID).Scan(
			&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
			&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
		)
		if err != nil {
			return Room{}, nil, false, err
		}
		if err := s.expireRoomIfNeededTx(ctx, tx, &room, now); err != nil {
			return Room{}, nil, false, err
		}
		if IsActive(room.State) && room.Visibility == VisibilityPublic && room.Mode == mode {
			members, err := s.listMembersTx(ctx, tx, room.ID)
			if err != nil {
				return Room{}, nil, false, err
			}
			if err := tx.Commit(ctx); err != nil {
				return Room{}, nil, false, err
			}
			return room, members, false, nil
		}
		if IsActive(room.State) {
			return Room{}, nil, false, ErrAlreadyIn
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Room{}, nil, false, err
	}

	if err := s.ensureNoOpenMatchTx(ctx, tx, playerID); err != nil {
		return Room{}, nil, false, err
	}

	// Find joinable public room of same mode.
	var room Room
	err = tx.QueryRow(ctx, `
		SELECT `+roomColumns+`
		FROM rooms r
		WHERE r.visibility = $1
		  AND r.state = $2
		  AND r.mode = $3
		  AND r.expires_at > $4
		  AND (SELECT COUNT(*) FROM room_members rm WHERE rm.room_id = r.id) < r.max_players
		ORDER BY r.created_at ASC
		LIMIT 1
		FOR UPDATE OF r
	`, VisibilityPublic, StateOpen, mode, now).Scan(
		&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
		&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
	)
	if err == nil {
		var count int
		if err := tx.QueryRow(ctx, `SELECT COUNT(*) FROM room_members WHERE room_id = $1`, room.ID).Scan(&count); err != nil {
			return Room{}, nil, false, err
		}
		if count >= room.MaxPlayers {
			return Room{}, nil, false, ErrFull
		}
		if _, err := s.insertMemberTx(ctx, tx, room.ID, playerID, false, now); err != nil {
			return Room{}, nil, false, err
		}
		if _, err := tx.Exec(ctx, `UPDATE rooms SET updated_at = $2, version = version + 1 WHERE id = $1`, room.ID, now); err != nil {
			return Room{}, nil, false, err
		}
		room.UpdatedAt = now
		room.Version++
		members, err := s.listMembersTx(ctx, tx, room.ID)
		if err != nil {
			return Room{}, nil, false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Room{}, nil, false, err
		}
		return room, members, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Room{}, nil, false, fmt.Errorf("find public room: %w", err)
	}

	// Create new public room.
	expiresAt := now.Add(idleTTL)
	for attempt := 0; attempt < 8; attempt++ {
		code, genErr := GenerateRoomCode()
		if genErr != nil {
			return Room{}, nil, false, genErr
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO rooms (
				room_code, owner_player_id, mode, visibility, state, max_players,
				created_at, updated_at, expires_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$7,$8)
			RETURNING `+roomColumns+`
		`, code, playerID, mode, VisibilityPublic, StateOpen, maxPlayers, now, expiresAt).Scan(
			&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
			&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
		)
		if err == nil {
			break
		}
		if isUniqueViolation(err) {
			continue
		}
		return Room{}, nil, false, fmt.Errorf("insert public room: %w", err)
	}
	if room.ID == uuid.Nil {
		return Room{}, nil, false, fmt.Errorf("public room code collision exhausted")
	}
	member, err := s.insertMemberTx(ctx, tx, room.ID, playerID, false, now)
	if err != nil {
		return Room{}, nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Room{}, nil, false, err
	}
	return room, []Member{member}, true, nil
}

// StartMatch binds DS allocation into the room transactionally.
func (s *Store) StartMatch(
	ctx context.Context,
	roomID, ownerID uuid.UUID,
	matchStore *match.Store,
	ticketTTL, heartbeatTTL time.Duration,
	now time.Time,
	newTicket func() (raw string, hash []byte, err error),
) (Room, []Member, map[uuid.UUID]string, match.RosterAssignmentMeta, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, err
	}
	defer tx.Rollback(ctx)

	var room Room
	err = tx.QueryRow(ctx, `SELECT `+roomColumns+` FROM rooms WHERE id = $1 FOR UPDATE`, roomID).Scan(
		&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
		&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, ErrNotFound
	}
	if err != nil {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, err
	}

	var isMember uuid.UUID
	err = tx.QueryRow(ctx, `SELECT player_id FROM room_members WHERE room_id = $1 AND player_id = $2`, roomID, ownerID).Scan(&isMember)
	if errors.Is(err, pgx.ErrNoRows) {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, ErrNotFound
	}
	if err != nil {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, err
	}
	if room.OwnerPlayerID != ownerID {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, ErrNotOwner
	}

	// Idempotent start while already in_game: reissue owner ticket.
	if room.State == StateInGame && room.MatchID != nil {
		raw, hash, err := newTicket()
		if err != nil {
			return Room{}, nil, nil, match.RosterAssignmentMeta{}, err
		}
		meta, err := matchStore.ReissueTicketTx(ctx, tx, ownerID, *room.MatchID, hash, ticketTTL, now)
		if err != nil {
			if errors.Is(err, match.ErrNotFound) {
				return Room{}, nil, nil, match.RosterAssignmentMeta{}, ErrAlreadyStarted
			}
			return Room{}, nil, nil, match.RosterAssignmentMeta{}, mapMatchErr(err)
		}
		members, err := s.listMembersTx(ctx, tx, roomID)
		if err != nil {
			return Room{}, nil, nil, match.RosterAssignmentMeta{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Room{}, nil, nil, match.RosterAssignmentMeta{}, err
		}
		return room, members, map[uuid.UUID]string{ownerID: raw}, meta, nil
	}

	if room.State == StateStarting || room.State == StateInGame {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, ErrAlreadyStarted
	}
	if room.State != StateOpen {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, ErrNotJoinable
	}

	members, err := s.listMembersTx(ctx, tx, roomID)
	if err != nil {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, err
	}
	if len(members) < 1 {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, ErrNotReady
	}
	for _, m := range members {
		if !m.Ready {
			return Room{}, nil, nil, match.RosterAssignmentMeta{}, ErrNotReady
		}
	}

	roster := make([]match.PlayerTicketHash, 0, len(members))
	rawByPlayer := make(map[uuid.UUID]string, len(members))
	for _, m := range members {
		raw, hash, err := newTicket()
		if err != nil {
			return Room{}, nil, nil, match.RosterAssignmentMeta{}, err
		}
		rawByPlayer[m.PlayerID] = raw
		roster = append(roster, match.PlayerTicketHash{PlayerID: m.PlayerID, TokenHash: hash})
	}

	metas, err := matchStore.AssignRosterTx(ctx, tx, roomID, room.Mode, roster, ticketTTL, heartbeatTTL, now)
	if err != nil {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, mapMatchErr(err)
	}
	if len(metas) == 0 {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, ErrGameServerUnavailable
	}

	matchID := metas[0].MatchID
	err = tx.QueryRow(ctx, `
		UPDATE rooms
		SET state = $2, match_id = $3, updated_at = $4, version = version + 1
		WHERE id = $1
		RETURNING `+roomColumns+`
	`, roomID, StateInGame, matchID, now).Scan(
		&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
		&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
	)
	if err != nil {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, fmt.Errorf("set room in_game: %w", err)
	}

	var ownerMeta match.RosterAssignmentMeta
	for _, meta := range metas {
		if meta.PlayerID == ownerID {
			ownerMeta = meta
			break
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return Room{}, nil, nil, match.RosterAssignmentMeta{}, err
	}
	return room, members, rawByPlayer, ownerMeta, nil
}

// ClaimAssignment reissues a ticket for a room member after start.
func (s *Store) ClaimAssignment(
	ctx context.Context,
	roomID, playerID uuid.UUID,
	matchStore *match.Store,
	ticketTTL time.Duration,
	now time.Time,
	newTicket func() (raw string, hash []byte, err error),
) (match.RosterAssignmentMeta, string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return match.RosterAssignmentMeta{}, "", err
	}
	defer tx.Rollback(ctx)

	var room Room
	err = tx.QueryRow(ctx, `SELECT `+roomColumns+` FROM rooms WHERE id = $1 FOR UPDATE`, roomID).Scan(
		&room.ID, &room.RoomCode, &room.OwnerPlayerID, &room.PartyID, &room.Mode, &room.Visibility, &room.State,
		&room.MaxPlayers, &room.MatchID, &room.Version, &room.CreatedAt, &room.UpdatedAt, &room.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return match.RosterAssignmentMeta{}, "", ErrNotFound
	}
	if err != nil {
		return match.RosterAssignmentMeta{}, "", err
	}

	var isMember uuid.UUID
	err = tx.QueryRow(ctx, `SELECT player_id FROM room_members WHERE room_id = $1 AND player_id = $2`, roomID, playerID).Scan(&isMember)
	if errors.Is(err, pgx.ErrNoRows) {
		return match.RosterAssignmentMeta{}, "", ErrNotFound
	}
	if err != nil {
		return match.RosterAssignmentMeta{}, "", err
	}

	if room.State == StateOpen {
		return match.RosterAssignmentMeta{}, "", ErrAssignmentNotReady
	}
	if room.State != StateStarting && room.State != StateInGame {
		return match.RosterAssignmentMeta{}, "", ErrNotJoinable
	}
	if room.MatchID == nil {
		return match.RosterAssignmentMeta{}, "", ErrAssignmentNotReady
	}

	raw, hash, err := newTicket()
	if err != nil {
		return match.RosterAssignmentMeta{}, "", err
	}
	meta, err := matchStore.ReissueTicketTx(ctx, tx, playerID, *room.MatchID, hash, ticketTTL, now)
	if err != nil {
		return match.RosterAssignmentMeta{}, "", mapMatchErr(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return match.RosterAssignmentMeta{}, "", err
	}
	return meta, raw, nil
}

func mapMatchErr(err error) error {
	switch {
	case errors.Is(err, match.ErrGameServerUnavailable):
		return ErrGameServerUnavailable
	case errors.Is(err, match.ErrGameCapacityFull):
		return ErrGameCapacityFull
	case errors.Is(err, match.ErrMatchAlreadyActive):
		return ErrMatchAlreadyActive
	case errors.Is(err, match.ErrInvalidInput):
		return ErrInvalidInput
	case errors.Is(err, match.ErrNotFound):
		return ErrNotFound
	default:
		return err
	}
}
