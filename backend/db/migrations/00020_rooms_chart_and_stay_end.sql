-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A confirmed booking whose stay is over becomes COMPLETED even if the hotel never used the app to check the guest in (many hotels
-- check guests in at the desk). Everything else about the status rules stays as it was.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION booking_transition_allowed(from_status text, to_status text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
  SELECT (from_status, to_status) IN (
    ('HELD', 'PAID_AWAITING_HOTEL'),
    ('HELD', 'EXPIRED'),
    ('PAID_AWAITING_HOTEL', 'CONFIRMED'),
    ('PAID_AWAITING_HOTEL', 'DECLINED_REFUNDED'),
    ('PAID_AWAITING_HOTEL', 'CANCELLED_REFUNDED'),
    ('CONFIRMED', 'CHECKED_IN'),
    ('CONFIRMED', 'COMPLETED'),
    ('CONFIRMED', 'CANCELLED_REFUNDED'),
    ('CONFIRMED', 'NO_SHOW'),
    ('CONFIRMED', 'NEEDS_RELOCATION'),
    ('NEEDS_RELOCATION', 'CONFIRMED'),
    ('NEEDS_RELOCATION', 'CANCELLED_REFUNDED'),
    ('CHECKED_IN', 'COMPLETED'),
    -- any state where money was taken can be disputed
    ('PAID_AWAITING_HOTEL', 'DISPUTED'), ('CONFIRMED', 'DISPUTED'), ('CHECKED_IN', 'DISPUTED'),
    ('COMPLETED', 'DISPUTED'), ('NO_SHOW', 'DISPUTED'), ('NEEDS_RELOCATION', 'DISPUTED'),
    -- how a dispute is resolved
    ('DISPUTED', 'CONFIRMED'), ('DISPUTED', 'COMPLETED'), ('DISPUTED', 'CANCELLED_REFUNDED')
  )
$$;
-- +goose StatementEnd

-- Rooms a hotel takes out of sale itself: a guest who walks in, or rooms blocked for repairs or a private event. They take rooms from
-- the same inventory as app bookings (inventory.booked), so a room can never be sold twice; this table says why they are taken and
-- lets the hotel give them back.
CREATE TABLE room_blocks (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  property_id   uuid NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
  room_type_id  uuid NOT NULL,
  check_in      date NOT NULL,
  check_out     date NOT NULL,
  rooms         integer NOT NULL CHECK (rooms BETWEEN 1 AND 500),
  kind          text NOT NULL CHECK (kind IN ('walk_in', 'block')),
  guest_name    text,
  guest_phone   text CHECK (length(guest_phone) <= 30),
  note          text CHECK (length(note) <= 300),
  created_by    uuid REFERENCES users (id),
  created_at    timestamptz NOT NULL DEFAULT now(),
  released_at   timestamptz,
  released_by   uuid REFERENCES users (id),
  CHECK (check_out > check_in),
  CONSTRAINT walk_in_has_a_guest CHECK (kind <> 'walk_in' OR length(btrim(coalesce(guest_name, ''))) >= 2),
  FOREIGN KEY (room_type_id, property_id) REFERENCES room_types (id, property_id)
);
CREATE INDEX room_blocks_active_idx ON room_blocks (property_id, check_in) WHERE released_at IS NULL;
ALTER TABLE room_blocks ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.room_blocks;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.booking_transition_allowed(from_status text, to_status text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
  SELECT (from_status, to_status) IN (
    ('HELD', 'PAID_AWAITING_HOTEL'), ('HELD', 'EXPIRED'), ('PAID_AWAITING_HOTEL', 'CONFIRMED'), ('PAID_AWAITING_HOTEL', 'DECLINED_REFUNDED'),
    ('PAID_AWAITING_HOTEL', 'CANCELLED_REFUNDED'), ('CONFIRMED', 'CHECKED_IN'), ('CONFIRMED', 'CANCELLED_REFUNDED'), ('CONFIRMED', 'NO_SHOW'),
    ('CONFIRMED', 'NEEDS_RELOCATION'), ('NEEDS_RELOCATION', 'CONFIRMED'), ('NEEDS_RELOCATION', 'CANCELLED_REFUNDED'), ('CHECKED_IN', 'COMPLETED'),
    ('PAID_AWAITING_HOTEL', 'DISPUTED'), ('CONFIRMED', 'DISPUTED'), ('CHECKED_IN', 'DISPUTED'), ('COMPLETED', 'DISPUTED'), ('NO_SHOW', 'DISPUTED'),
    ('NEEDS_RELOCATION', 'DISPUTED'), ('DISPUTED', 'CONFIRMED'), ('DISPUTED', 'COMPLETED'), ('DISPUTED', 'CANCELLED_REFUNDED')
  )
$$;
-- +goose StatementEnd
