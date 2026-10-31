-- Phase 1 bootstrap: foundational extensions only.
-- Domain tables (accounts, players, rooms, ...) arrive in later phases.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS app_meta (
    key text PRIMARY KEY,
    value text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO app_meta (key, value)
VALUES ('schema_bootstrap', 'phase1')
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
    updated_at = now();
