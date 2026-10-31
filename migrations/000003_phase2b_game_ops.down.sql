DROP TABLE IF EXISTS player_stats;
DROP TABLE IF EXISTS match_settlements;
DROP TABLE IF EXISTS game_tickets;
DROP TABLE IF EXISTS match_players;

ALTER TABLE IF EXISTS game_instances
    DROP CONSTRAINT IF EXISTS game_instances_active_match_id_fkey;

DROP TABLE IF EXISTS matches;
DROP TABLE IF EXISTS game_instances;
DROP TABLE IF EXISTS dedicated_servers;

DELETE FROM app_meta WHERE key = 'schema_phase2b_game_ops';
