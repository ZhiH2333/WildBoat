-- Phase 2B: Dedicated Server registry, matches, tickets, settlements.

CREATE TABLE dedicated_servers (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    credential_hash bytea NOT NULL,
    label text NOT NULL DEFAULT 'default',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    CONSTRAINT dedicated_servers_credential_hash_len CHECK (octet_length(credential_hash) = 32),
    CONSTRAINT dedicated_servers_label_nonempty CHECK (char_length(label) > 0)
);

CREATE UNIQUE INDEX dedicated_servers_credential_hash_uidx ON dedicated_servers (credential_hash);
CREATE INDEX dedicated_servers_active_idx ON dedicated_servers (id) WHERE revoked_at IS NULL;

CREATE TABLE game_instances (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id uuid NOT NULL REFERENCES dedicated_servers (id),
    host text NOT NULL,
    port integer NOT NULL,
    transport text NOT NULL DEFAULT 'enet_udp',
    status text NOT NULL,
    protocol_version integer NOT NULL DEFAULT 1,
    server_build text NOT NULL,
    capacity integer NOT NULL DEFAULT 4,
    occupied_slots integer NOT NULL DEFAULT 0,
    active_match_id uuid,
    last_heartbeat_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT game_instances_host_nonempty CHECK (char_length(host) > 0),
    CONSTRAINT game_instances_port_valid CHECK (port BETWEEN 1 AND 65535),
    CONSTRAINT game_instances_transport_valid CHECK (transport IN ('enet_udp')),
    CONSTRAINT game_instances_status_valid CHECK (
        status IN ('starting', 'ready', 'full', 'draining', 'offline')
    ),
    CONSTRAINT game_instances_protocol_version_positive CHECK (protocol_version > 0),
    CONSTRAINT game_instances_server_build_nonempty CHECK (char_length(server_build) > 0),
    CONSTRAINT game_instances_capacity_valid CHECK (capacity BETWEEN 1 AND 4),
    CONSTRAINT game_instances_occupied_slots_valid CHECK (
        occupied_slots >= 0 AND occupied_slots <= capacity
    )
);

CREATE INDEX game_instances_server_id_idx ON game_instances (server_id);
CREATE INDEX game_instances_assignable_idx
    ON game_instances (status, last_heartbeat_at)
    WHERE status IN ('ready', 'full');

CREATE TABLE matches (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id uuid,
    game_instance_id uuid NOT NULL REFERENCES game_instances (id),
    server_id uuid NOT NULL REFERENCES dedicated_servers (id),
    mode text NOT NULL,
    state text NOT NULL,
    max_players integer NOT NULL DEFAULT 4,
    started_at timestamptz,
    ended_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT matches_mode_valid CHECK (
        mode IN ('coop_survival', 'coop_endless', 'battle_ffa', 'battle_team', 'battle_kill_race')
    ),
    CONSTRAINT matches_state_valid CHECK (
        state IN ('allocating', 'active', 'completed', 'aborted')
    ),
    CONSTRAINT matches_max_players_valid CHECK (max_players BETWEEN 1 AND 4)
);

CREATE INDEX matches_instance_open_idx
    ON matches (game_instance_id, state)
    WHERE state IN ('allocating', 'active');
CREATE INDEX matches_server_id_idx ON matches (server_id);

-- Deferred FK: active_match_id references matches after matches exists.
ALTER TABLE game_instances
    ADD CONSTRAINT game_instances_active_match_id_fkey
    FOREIGN KEY (active_match_id) REFERENCES matches (id);

CREATE TABLE match_players (
    match_id uuid NOT NULL REFERENCES matches (id) ON DELETE CASCADE,
    player_id uuid NOT NULL REFERENCES players (id),
    team_id text NOT NULL DEFAULT '',
    join_state text NOT NULL DEFAULT 'assigned',
    kills integer NOT NULL DEFAULT 0,
    deaths integer NOT NULL DEFAULT 0,
    score integer NOT NULL DEFAULT 0,
    assigned_at timestamptz NOT NULL DEFAULT now(),
    admitted_at timestamptz,
    PRIMARY KEY (match_id, player_id),
    CONSTRAINT match_players_join_state_valid CHECK (
        join_state IN ('assigned', 'admitted', 'left')
    ),
    CONSTRAINT match_players_kills_nonneg CHECK (kills >= 0),
    CONSTRAINT match_players_deaths_nonneg CHECK (deaths >= 0)
);

CREATE INDEX match_players_player_id_idx ON match_players (player_id);

-- At most one open match membership per player.
CREATE UNIQUE INDEX match_players_one_open_match_uidx
    ON match_players (player_id)
    WHERE join_state IN ('assigned', 'admitted');

CREATE TABLE game_tickets (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash bytea NOT NULL,
    player_id uuid NOT NULL REFERENCES players (id),
    match_id uuid NOT NULL REFERENCES matches (id) ON DELETE CASCADE,
    game_instance_id uuid NOT NULL REFERENCES game_instances (id),
    server_id uuid NOT NULL REFERENCES dedicated_servers (id),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT game_tickets_token_hash_len CHECK (octet_length(token_hash) = 32),
    CONSTRAINT game_tickets_expires_after_created CHECK (expires_at > created_at)
);

CREATE UNIQUE INDEX game_tickets_token_hash_uidx ON game_tickets (token_hash);
CREATE INDEX game_tickets_match_player_idx ON game_tickets (match_id, player_id);
CREATE INDEX game_tickets_instance_open_idx
    ON game_tickets (game_instance_id)
    WHERE consumed_at IS NULL AND revoked_at IS NULL;

CREATE TABLE match_settlements (
    settlement_id uuid PRIMARY KEY,
    match_id uuid NOT NULL REFERENCES matches (id),
    game_instance_id uuid NOT NULL REFERENCES game_instances (id),
    server_id uuid NOT NULL REFERENCES dedicated_servers (id),
    result text NOT NULL,
    payload_json jsonb NOT NULL,
    status text NOT NULL DEFAULT 'processed',
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT match_settlements_result_valid CHECK (result IN ('completed', 'aborted')),
    CONSTRAINT match_settlements_status_valid CHECK (status IN ('processed'))
);

CREATE UNIQUE INDEX match_settlements_match_id_uidx ON match_settlements (match_id);

CREATE TABLE player_stats (
    player_id uuid PRIMARY KEY REFERENCES players (id) ON DELETE CASCADE,
    matches_played integer NOT NULL DEFAULT 0,
    kills integer NOT NULL DEFAULT 0,
    deaths integer NOT NULL DEFAULT 0,
    score bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT player_stats_matches_played_nonneg CHECK (matches_played >= 0),
    CONSTRAINT player_stats_kills_nonneg CHECK (kills >= 0),
    CONSTRAINT player_stats_deaths_nonneg CHECK (deaths >= 0)
);

INSERT INTO app_meta (key, value)
VALUES ('schema_phase2b_game_ops', 'phase2b')
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = now();
