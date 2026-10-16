-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Guests give their identity details once, in the guest app. When a guest requests a room, a frozen copy of that
-- profile (including the ID) travels with the booking, and the hotel sees it in the partner app.
--
-- This is sensitive personal data:
--   * the full ID number is stored ENCRYPTED by the server (AES-GCM, key held outside the database), so a
--     database leak alone does not expose it;
--   * a keyed hash lets a hotel recognise a returning guest without being able to read the number;
--   * lists and the apps show only the last four characters; the full number needs an explicit, audited "reveal"
--     by staff of the hotel that holds the booking;
--   * a hotel's copy is never edited; after the retention period the full number is erased and only the last four remain.

-- The guest's own profile: one per guest. The guest can change it or delete it any time.
CREATE TABLE guest_profiles (
  user_id        uuid PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
  full_name      text NOT NULL CHECK (length(btrim(full_name)) >= 2),
  id_type        text NOT NULL CHECK (id_type IN ('national_id', 'passport', 'driving_licence', 'residence_permit', 'other')),
  id_number_enc  bytea NOT NULL,         -- ciphertext
  id_number_hash bytea NOT NULL,         -- keyed hash, cannot be reversed
  id_last4       text NOT NULL CHECK (id_last4 ~ '^[A-Z0-9/-]{1,4}$'),
  nationality    text NOT NULL CHECK (length(btrim(nationality)) >= 2),
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER guest_profiles_touch BEFORE UPDATE ON guest_profiles FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- A booking must prove which hotel it belongs to, so the copy below can only be attached to the right hotel.
ALTER TABLE bookings ADD CONSTRAINT bookings_id_property_key UNIQUE (id, property_id);

-- The copy the hotel sees: the guest's profile at the moment they requested the room.
CREATE TABLE booking_guests (
  booking_id     uuid PRIMARY KEY,
  property_id    uuid NOT NULL,
  full_name      text NOT NULL CHECK (length(btrim(full_name)) >= 2),
  contact        text NOT NULL,          -- the phone or email the guest signed in with
  nationality    text NOT NULL,
  id_type        text NOT NULL CHECK (id_type IN ('national_id', 'passport', 'driving_licence', 'residence_permit', 'other')),
  id_number_enc  bytea,                  -- ciphertext; null once erased
  id_number_hash bytea NOT NULL,
  id_last4       text NOT NULL CHECK (id_last4 ~ '^[A-Z0-9/-]{1,4}$'),
  created_at     timestamptz NOT NULL DEFAULT now(),
  purged_at      timestamptz,
  CONSTRAINT erased_means_no_ciphertext CHECK ((purged_at IS NULL) = (id_number_enc IS NOT NULL)),
  -- The booking and the hotel must match, so one hotel can never be handed another hotel's guest.
  FOREIGN KEY (booking_id, property_id) REFERENCES bookings (id, property_id)
);
CREATE INDEX booking_guests_property_idx ON booking_guests (property_id, created_at DESC);
CREATE INDEX booking_guests_hash_idx ON booking_guests (property_id, id_number_hash);

-- A hotel's copy is a record, not a draft: nothing changes except the one-way erasure of the ID number.
-- +goose StatementBegin
CREATE FUNCTION booking_guests_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    RAISE EXCEPTION 'a booking''s guest details cannot be deleted' USING ERRCODE = 'restrict_violation';
  END IF;
  IF OLD.purged_at IS NOT NULL THEN
    RAISE EXCEPTION 'this ID number was already erased and the record cannot change' USING ERRCODE = 'restrict_violation';
  END IF;
  IF NEW.id_number_enc IS NOT NULL OR NEW.purged_at IS NULL THEN
    RAISE EXCEPTION 'booking guest details are append-only; only erasing the ID number is allowed' USING ERRCODE = 'restrict_violation';
  END IF;
  IF (NEW.booking_id, NEW.property_id, NEW.full_name, NEW.contact, NEW.nationality, NEW.id_type,
      NEW.id_number_hash, NEW.id_last4, NEW.created_at)
     IS DISTINCT FROM
     (OLD.booking_id, OLD.property_id, OLD.full_name, OLD.contact, OLD.nationality, OLD.id_type,
      OLD.id_number_hash, OLD.id_last4, OLD.created_at) THEN
    RAISE EXCEPTION 'erasing an ID number must not change anything else' USING ERRCODE = 'restrict_violation';
  END IF;
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER booking_guests_guard BEFORE UPDATE OR DELETE ON booking_guests
  FOR EACH ROW EXECUTE FUNCTION booking_guests_guard();
CREATE TRIGGER booking_guests_no_truncate BEFORE TRUNCATE ON booking_guests
  FOR EACH STATEMENT EXECUTE FUNCTION forbid_change();

-- Erases the full ID number from every hotel copy whose stay ended before the given date. A scheduled job calls this
-- with the retention date the owner chose. There is deliberately no default: how long the law requires a hotel to keep
-- the full number is a decision for the owner (docs/design.md, DECISION 5). Returns how many were erased.
-- +goose StatementBegin
CREATE FUNCTION purge_id_numbers(ended_before date) RETURNS integer LANGUAGE plpgsql AS $$
DECLARE erased integer;
BEGIN
  UPDATE booking_guests bg
     SET id_number_enc = NULL, purged_at = now()
    FROM bookings b
   WHERE b.id = bg.booking_id AND bg.purged_at IS NULL AND b.check_out < ended_before;
  GET DIAGNOSTICS erased = ROW_COUNT;
  RETURN erased;
END $$;
-- +goose StatementEnd

ALTER TABLE guest_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE booking_guests ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP FUNCTION IF EXISTS app.purge_id_numbers(date);
DROP TABLE IF EXISTS app.booking_guests, app.guest_profiles CASCADE;
DROP FUNCTION IF EXISTS app.booking_guests_guard();
ALTER TABLE app.bookings DROP CONSTRAINT IF EXISTS bookings_id_property_key;
