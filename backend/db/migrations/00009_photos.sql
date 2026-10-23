-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Hotel photos. They are stored in the database (resized by the server: a main size and a small one for lists, so guests on
-- mobile data download little). Photos are public content: anyone may view them, only the hotel's managers may change them.
CREATE TABLE property_photos (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  property_id  uuid NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
  position     integer NOT NULL DEFAULT 0,           -- 0 is the cover photo
  main         bytea NOT NULL CHECK (octet_length(main) BETWEEN 1 AND 3000000),
  thumb        bytea NOT NULL CHECK (octet_length(thumb) BETWEEN 1 AND 500000),
  width        integer NOT NULL CHECK (width > 0),
  height       integer NOT NULL CHECK (height > 0),
  uploaded_by  uuid REFERENCES users (id),
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX property_photos_property_idx ON property_photos (property_id, position, created_at);

ALTER TABLE property_photos ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.property_photos;
