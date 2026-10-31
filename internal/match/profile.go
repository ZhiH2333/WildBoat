package match

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// PlayerStats is aggregate settlement-backed counters.
type PlayerStats struct {
	PlayerID      uuid.UUID  `json:"player_id"`
	MatchesPlayed int        `json:"matches_played"`
	Kills         int        `json:"kills"`
	Deaths        int        `json:"deaths"`
	Score         int64      `json:"score"`
	UpdatedAt     *time.Time `json:"updated_at"`
}

// MatchHistoryItem is one settled match row for the player.
type MatchHistoryItem struct {
	MatchID   uuid.UUID  `json:"match_id"`
	Mode      string     `json:"mode"`
	Result    string     `json:"result"`
	JoinState string     `json:"join_state"`
	Kills     int        `json:"kills"`
	Deaths    int        `json:"deaths"`
	Score     int        `json:"score"`
	TeamID    string     `json:"team_id"`
	StartedAt *time.Time `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at"`
	RoomID    *uuid.UUID `json:"room_id"`
}

type historyCursor struct {
	EndedAt time.Time `json:"e"`
	MatchID uuid.UUID `json:"m"`
}

// GetPlayerStats returns zeros when the player has never settled.
func (s *Store) GetPlayerStats(ctx context.Context, playerID uuid.UUID) (PlayerStats, error) {
	var stats PlayerStats
	stats.PlayerID = playerID
	err := s.pool.QueryRow(ctx, `
		SELECT matches_played, kills, deaths, score, updated_at
		FROM player_stats
		WHERE player_id = $1
	`, playerID).Scan(&stats.MatchesPlayed, &stats.Kills, &stats.Deaths, &stats.Score, &stats.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlayerStats{PlayerID: playerID}, nil
	}
	if err != nil {
		return PlayerStats{}, fmt.Errorf("get player stats: %w", err)
	}
	return stats, nil
}

// ListSettledMatches returns settlement-backed history newest-first.
func (s *Store) ListSettledMatches(ctx context.Context, playerID uuid.UUID, limit int, cursor string) ([]MatchHistoryItem, string, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	args := []any{playerID, limit + 1}
	where := `
		ms.settlement_id IS NOT NULL
		AND mp.player_id = $1
	`
	if cursor != "" {
		cur, err := decodeHistoryCursor(cursor)
		if err != nil {
			return nil, "", ErrInvalidInput
		}
		where += ` AND (COALESCE(m.ended_at, m.updated_at), m.id) < ($3, $4)`
		args = append(args, cur.EndedAt, cur.MatchID)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT m.id, m.mode, ms.result, mp.join_state, mp.kills, mp.deaths, mp.score, mp.team_id,
		       m.started_at, m.ended_at, m.room_id
		FROM match_players mp
		JOIN matches m ON m.id = mp.match_id
		JOIN match_settlements ms ON ms.match_id = m.id
		WHERE `+where+`
		ORDER BY COALESCE(m.ended_at, m.updated_at) DESC, m.id DESC
		LIMIT $2
	`, args...)
	if err != nil {
		return nil, "", fmt.Errorf("list settled matches: %w", err)
	}
	defer rows.Close()

	items := make([]MatchHistoryItem, 0, limit)
	for rows.Next() {
		var item MatchHistoryItem
		if err := rows.Scan(
			&item.MatchID,
			&item.Mode,
			&item.Result,
			&item.JoinState,
			&item.Kills,
			&item.Deaths,
			&item.Score,
			&item.TeamID,
			&item.StartedAt,
			&item.EndedAt,
			&item.RoomID,
		); err != nil {
			return nil, "", fmt.Errorf("scan match history: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var next string
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		ended := time.Time{}
		if last.EndedAt != nil {
			ended = *last.EndedAt
		}
		next, err = encodeHistoryCursor(historyCursor{EndedAt: ended, MatchID: last.MatchID})
		if err != nil {
			return nil, "", err
		}
	}
	return items, next, nil
}

// GetPlayerStats service wrapper.
func (s *Service) GetPlayerStats(ctx context.Context, playerID uuid.UUID) (PlayerStats, error) {
	return s.store.GetPlayerStats(ctx, playerID)
}

// ListMatchHistory service wrapper.
func (s *Service) ListMatchHistory(ctx context.Context, playerID uuid.UUID, limit int, cursor string) ([]MatchHistoryItem, string, error) {
	return s.store.ListSettledMatches(ctx, playerID, limit, cursor)
}

func encodeHistoryCursor(c historyCursor) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func decodeHistoryCursor(raw string) (historyCursor, error) {
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return historyCursor{}, err
	}
	var c historyCursor
	if err := json.Unmarshal(b, &c); err != nil {
		return historyCursor{}, err
	}
	if c.MatchID == uuid.Nil {
		return historyCursor{}, ErrInvalidInput
	}
	return c, nil
}
