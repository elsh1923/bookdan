-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A photo can belong to one room type (shown on that room's card) instead of the hotel's own gallery. Room photos are deleted with
-- their room; the hotel gallery (room_type_id empty) is unchanged.
ALTER TABLE property_photos ADD COLUMN room_type_id uuid REFERENCES room_types (id) ON DELETE CASCADE;
CREATE INDEX property_photos_room_idx ON property_photos (room_type_id, position, created_at) WHERE room_type_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS app.property_photos_room_idx;
ALTER TABLE app.property_photos DROP COLUMN IF EXISTS room_type_id;
