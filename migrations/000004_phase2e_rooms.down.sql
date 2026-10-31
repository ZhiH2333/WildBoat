ALTER TABLE IF EXISTS matches
    DROP CONSTRAINT IF EXISTS matches_room_id_fkey;

DROP TABLE IF EXISTS room_members;
DROP TABLE IF EXISTS rooms;

DELETE FROM app_meta WHERE key = 'schema_phase2e_rooms';
