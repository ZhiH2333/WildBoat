-- Phase 2: WPass identity foundation.

CREATE TABLE accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email_normalized text NOT NULL,
    password_hash text NOT NULL,
    status text NOT NULL,
    email_verified_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT accounts_email_normalized_nonempty CHECK (char_length(email_normalized) > 0),
    CONSTRAINT accounts_status_valid CHECK (status IN ('pending_verification', 'active', 'locked', 'closed'))
);

CREATE UNIQUE INDEX accounts_email_normalized_uidx ON accounts (email_normalized);

CREATE TABLE players (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    display_name text NOT NULL,
    avatar_id text NOT NULL DEFAULT 'default',
    bio text NOT NULL DEFAULT '',
    profile_version integer NOT NULL DEFAULT 1,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT players_display_name_nonempty CHECK (char_length(display_name) BETWEEN 2 AND 32),
    CONSTRAINT players_avatar_id_nonempty CHECK (char_length(avatar_id) > 0),
    CONSTRAINT players_profile_version_positive CHECK (profile_version > 0)
);

CREATE UNIQUE INDEX players_account_id_uidx ON players (account_id);

CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    refresh_token_hash bytea NOT NULL,
    previous_refresh_token_hash bytea,
    rotated_at timestamptz,
    user_agent text NOT NULL DEFAULT '',
    ip inet,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    revoke_reason text,
    CONSTRAINT sessions_expires_after_created CHECK (expires_at > created_at),
    CONSTRAINT sessions_revoke_reason_when_revoked CHECK (
        (revoked_at IS NULL AND revoke_reason IS NULL)
        OR (revoked_at IS NOT NULL AND revoke_reason IS NOT NULL)
    )
);

CREATE UNIQUE INDEX sessions_refresh_token_hash_uidx ON sessions (refresh_token_hash);
CREATE INDEX sessions_account_id_active_idx ON sessions (account_id) WHERE revoked_at IS NULL;
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

CREATE TABLE email_action_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id uuid NOT NULL REFERENCES accounts (id) ON DELETE CASCADE,
    token_hash bytea NOT NULL,
    purpose text NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT email_action_tokens_purpose_valid CHECK (purpose IN ('verify_email', 'password_reset')),
    CONSTRAINT email_action_tokens_expires_after_created CHECK (expires_at > created_at)
);

CREATE UNIQUE INDEX email_action_tokens_token_hash_uidx ON email_action_tokens (token_hash);
CREATE INDEX email_action_tokens_account_purpose_idx
    ON email_action_tokens (account_id, purpose)
    WHERE consumed_at IS NULL;

INSERT INTO app_meta (key, value)
VALUES ('schema_wpass_identity', 'phase2')
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = now();
