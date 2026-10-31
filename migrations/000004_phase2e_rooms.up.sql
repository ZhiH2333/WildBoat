-- Phase 2E: rooms / room_members + matches.room_id FK.

CREATE TABLE rooms (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    room_code text NOT NULL,
    owner_player_id uuid NOT NULL REFERENCES players (id),
    party_id uuid,
    mode text NOT NULL,
    visibility text NOT NULL,
    state text NOT NULL,
    max_players integer NOT NULL DEFAULT 4,
    match_id uuid REFERENCES matches (id),
    version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    CONSTRAINT rooms_mode_valid CHECK (
        mode IN ('coop_survival', 'coop_endless', 'battle_ffa', 'battle_team', 'battle_kill_race')
    ),
    CONSTRAINT rooms_visibility_valid CHECK (visibility IN ('private', 'public')),
    CONSTRAINT rooms_state_valid CHECK (
        state IN ('open', 'starting', 'in_game', 'closed', 'expired', 'aborted')
    ),
    CONSTRAINT rooms_max_players_valid CHECK (max_players BETWEEN 1 AND 4),
    CONSTRAINT rooms_room_code_format CHECK (
        room_code ~ '^[ABCDEFGHJKLMNPQRSTUVWXYZ23456789]{6}$'
    ),
    CONSTRAINT rooms_version_positive CHECK (version > 0)
);

-- Active (non-terminal) room codes must be unique.
CREATE UNIQUE INDEX rooms_active_code_uidx
    ON rooms (room_code)
    WHERE state IN ('open', 'starting', 'in_game');

CREATE INDEX rooms_owner_idx ON rooms (owner_player_id);
CREATE INDEX rooms_public_open_idx
    ON rooms (mode, updated_at)
    WHERE visibility = 'public' AND state = 'open';
CREATE INDEX rooms_expire_open_idx
    ON rooms (expires_at)
    WHERE state = 'open';

CREATE TABLE room_members (
    room_id uuid NOT NULL REFERENCES rooms (id) ON DELETE CASCADE,
    player_id uuid NOT NULL REFERENCES players (id),
    ready boolean NOT NULL DEFAULT false,
    joined_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (room_id, player_id)
);

-- A player may belong to at most one room at a time (members are deleted on leave/close).
CREATE UNIQUE INDEX room_members_player_uidx ON room_members (player_id);

CREATE INDEX room_members_room_joined_idx ON room_members (room_id, joined_at);

-- Existing nullable matches.room_id rows remain valid; add FK once rooms exist.
ALTER TABLE matches
    ADD CONSTRAINT matches_room_id_fkey
    FOREIGN KEY (room_id) REFERENCES rooms (id);

INSERT INTO app_meta (key, value)
VALUES ('schema_phase2e_rooms', 'phase2e')
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = now();
