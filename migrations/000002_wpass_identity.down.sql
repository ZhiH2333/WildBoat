DROP TABLE IF EXISTS email_action_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS players;
DROP TABLE IF EXISTS accounts;

DELETE FROM app_meta WHERE key = 'schema_wpass_identity';
