-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A photo of the guest's ID (national ID, passport...): the front and, for a card, the back. Like the ID number, it is stored
-- ENCRYPTED by the server (AES-GCM, key held outside the database), the hotel sees it only through an audited view, and the
-- hotel's copy is erased with the ID number after the retention period. The server re-draws every upload as a plain JPEG, which
-- removes the location and camera details a phone puts in a picture.

-- The guest's own photos. The guest can replace or delete them any time; deleting the account deletes them.
CREATE TABLE guest_id_photos (
  user_id    uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  side       text NOT NULL CHECK (side IN ('front', 'back')),
  data_enc   bytea NOT NULL,             -- ciphertext of a JPEG
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, side)
);

-- The copy a hotel sees: the guest's photos at the moment they requested the room. Later changes to the guest's own photos
-- do not change it, and it is erased (data_enc null, purged_at set) when the ID number is.
CREATE TABLE booking_id_photos (
  booking_id  uuid NOT NULL,
  property_id uuid NOT NULL,
  side        text NOT NULL CHECK (side IN ('front', 'back')),
  data_enc    bytea,
  created_at  timestamptz NOT NULL DEFAULT now(),
  purged_at   timestamptz,
  PRIMARY KEY (booking_id, side),
  CONSTRAINT id_photo_erased_means_no_ciphertext CHECK ((purged_at IS NULL) = (data_enc IS NOT NULL)),
  FOREIGN KEY (booking_id, property_id) REFERENCES bookings (id, property_id)
);
CREATE INDEX booking_id_photos_live_idx ON booking_id_photos (booking_id) WHERE purged_at IS NULL;

ALTER TABLE guest_id_photos ENABLE ROW LEVEL SECURITY;
ALTER TABLE booking_id_photos ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.booking_id_photos, app.guest_id_photos;
