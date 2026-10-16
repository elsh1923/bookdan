-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- One row per room type per night. The CHECK is what makes double booking impossible: no matter what the
-- application does, the database refuses any state where more rooms are promised than exist.
-- A hold does UPDATE ... SET held = held + 1 WHERE held + booked < capacity; zero rows updated = sold out.
CREATE TABLE inventory (
  room_type_id uuid NOT NULL REFERENCES room_types (id) ON DELETE CASCADE,
  night        date NOT NULL,
  capacity     integer NOT NULL CHECK (capacity >= 0),
  held         integer NOT NULL DEFAULT 0,
  booked       integer NOT NULL DEFAULT 0,
  PRIMARY KEY (room_type_id, night),
  CONSTRAINT inventory_never_oversold CHECK (held >= 0 AND booked >= 0 AND held + booked <= capacity)
);

-- The full price, frozen when the guest is shown it. All amounts are whole santim.
CREATE TABLE quotes (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id          uuid NOT NULL REFERENCES users (id),
  property_id      uuid NOT NULL REFERENCES properties (id),
  room_type_id     uuid NOT NULL,
  check_in         date NOT NULL,
  check_out        date NOT NULL,
  guests           integer NOT NULL CHECK (guests > 0),
  rooms            integer NOT NULL DEFAULT 1 CHECK (rooms BETWEEN 1 AND 9),  -- rooms of this type; the amounts cover all of them
  currency         text NOT NULL CHECK (currency IN ('ETB', 'USD', 'EUR')),
  base_santim      bigint NOT NULL CHECK (base_santim >= 0),
  service_santim   bigint NOT NULL CHECK (service_santim >= 0),
  vat_santim       bigint NOT NULL CHECK (vat_santim >= 0),
  total_santim     bigint NOT NULL CHECK (total_santim >= 0),
  deposit_santim   bigint NOT NULL CHECK (deposit_santim >= 0),
  balance_due_santim bigint NOT NULL CHECK (balance_due_santim >= 0),
  price_snapshot   jsonb NOT NULL,  -- per-night prices, boosts and rates used, for audit
  expires_at       timestamptz NOT NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  CHECK (check_out > check_in),
  CONSTRAINT quote_guests_cover_rooms CHECK (guests >= rooms),  -- every room has at least one guest
  CONSTRAINT quote_amounts_add_up CHECK (
    total_santim = base_santim + service_santim + vat_santim
    AND deposit_santim + balance_due_santim = total_santim
  ),
  FOREIGN KEY (room_type_id, property_id) REFERENCES room_types (id, property_id)
);
CREATE INDEX quotes_user_idx ON quotes (user_id, created_at DESC);

CREATE TABLE bookings (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code               text NOT NULL UNIQUE CHECK (code ~ '^BD-[A-Z0-9]{6}$'),  -- what the guest and hotel quote
  quote_id           uuid NOT NULL UNIQUE REFERENCES quotes (id),            -- one booking per quote
  user_id            uuid NOT NULL REFERENCES users (id),
  property_id        uuid NOT NULL REFERENCES properties (id),
  room_type_id       uuid NOT NULL,
  room_id            uuid REFERENCES rooms (id),                             -- assigned later by the hotel
  check_in           date NOT NULL,
  check_out          date NOT NULL,
  guests             integer NOT NULL CHECK (guests > 0),
  rooms              integer NOT NULL DEFAULT 1 CHECK (rooms BETWEEN 1 AND 9),  -- held and sold as this many units of room_type_id
  guest_name         text NOT NULL CHECK (length(btrim(guest_name)) >= 2),
  status             text NOT NULL DEFAULT 'HELD' CHECK (status IN (
    'HELD', 'PAID_AWAITING_HOTEL', 'CONFIRMED', 'CHECKED_IN', 'COMPLETED', 'EXPIRED',
    'DECLINED_REFUNDED', 'CANCELLED_REFUNDED', 'NO_SHOW', 'NEEDS_RELOCATION', 'DISPUTED')),
  currency           text NOT NULL CHECK (currency IN ('ETB', 'USD', 'EUR')),
  base_santim        bigint NOT NULL CHECK (base_santim >= 0),
  service_santim     bigint NOT NULL CHECK (service_santim >= 0),
  vat_santim         bigint NOT NULL CHECK (vat_santim >= 0),
  total_santim       bigint NOT NULL CHECK (total_santim >= 0),
  deposit_santim     bigint NOT NULL CHECK (deposit_santim >= 0),
  balance_due_santim bigint NOT NULL CHECK (balance_due_santim >= 0),
  payment_method     text CHECK (payment_method IN ('telebirr', 'chapa', 'pay_at_hotel')),
  hold_expires_at    timestamptz,  -- while HELD
  hotel_deadline_at  timestamptz,  -- while PAID_AWAITING_HOTEL: answer by then or the guest is refunded
  paid_at            timestamptz,
  decline_reason     text,
  version            integer NOT NULL DEFAULT 0,  -- bumped on every update, for optimistic locking
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  CHECK (check_out > check_in),
  CONSTRAINT booking_guests_cover_rooms CHECK (guests >= rooms),
  CONSTRAINT booking_amounts_add_up CHECK (
    total_santim = base_santim + service_santim + vat_santim
    AND deposit_santim + balance_due_santim = total_santim
  ),
  CONSTRAINT held_has_expiry CHECK (status <> 'HELD' OR hold_expires_at IS NOT NULL),
  CONSTRAINT awaiting_has_deadline CHECK (status <> 'PAID_AWAITING_HOTEL' OR hotel_deadline_at IS NOT NULL),
  FOREIGN KEY (room_type_id, property_id) REFERENCES room_types (id, property_id)
);
CREATE INDEX bookings_user_idx ON bookings (user_id, created_at DESC);
CREATE INDEX bookings_property_idx ON bookings (property_id, check_in);
-- The background workers scan these two constantly; partial indexes keep them tiny.
CREATE INDEX bookings_holds_due_idx ON bookings (hold_expires_at) WHERE status = 'HELD';
CREATE INDEX bookings_deadlines_due_idx ON bookings (hotel_deadline_at) WHERE status = 'PAID_AWAITING_HOTEL';

-- Is this status change allowed? The one place the state machine is written down.
-- +goose StatementBegin
CREATE FUNCTION booking_transition_allowed(from_status text, to_status text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
  SELECT (from_status, to_status) IN (
    ('HELD', 'PAID_AWAITING_HOTEL'),
    ('HELD', 'EXPIRED'),
    ('PAID_AWAITING_HOTEL', 'CONFIRMED'),
    ('PAID_AWAITING_HOTEL', 'DECLINED_REFUNDED'),
    ('CONFIRMED', 'CHECKED_IN'),
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

-- +goose StatementBegin
CREATE FUNCTION bookings_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status IS DISTINCT FROM OLD.status AND NOT booking_transition_allowed(OLD.status, NEW.status) THEN
    RAISE EXCEPTION 'illegal booking status change % -> %', OLD.status, NEW.status USING ERRCODE = 'check_violation';
  END IF;
  -- The agreed price never changes after the booking exists.
  IF NEW.rooms IS DISTINCT FROM OLD.rooms THEN
    RAISE EXCEPTION 'booking amounts cannot be changed: the number of rooms is part of the price' USING ERRCODE = 'check_violation';
  END IF;
  IF (NEW.currency, NEW.base_santim, NEW.service_santim, NEW.vat_santim, NEW.total_santim, NEW.deposit_santim, NEW.balance_due_santim)
     IS DISTINCT FROM
     (OLD.currency, OLD.base_santim, OLD.service_santim, OLD.vat_santim, OLD.total_santim, OLD.deposit_santim, OLD.balance_due_santim) THEN
    RAISE EXCEPTION 'booking amounts cannot be changed' USING ERRCODE = 'check_violation';
  END IF;
  NEW.version := OLD.version + 1;
  NEW.updated_at := now();
  RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER bookings_guard BEFORE UPDATE ON bookings FOR EACH ROW EXECUTE FUNCTION bookings_guard();

-- Every status change is written to the history by the database itself, so no code path can skip it.
-- The caller says who and why with: SET LOCAL app.actor = '...'; SET LOCAL app.reason = '...';
CREATE TABLE booking_events (
  id          bigserial PRIMARY KEY,
  booking_id  uuid NOT NULL REFERENCES bookings (id),
  from_status text,
  to_status   text NOT NULL,
  actor       text,
  reason      text,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX booking_events_booking_idx ON booking_events (booking_id, id);
CREATE TRIGGER booking_events_append_only BEFORE UPDATE OR DELETE ON booking_events FOR EACH ROW EXECUTE FUNCTION forbid_change();
CREATE TRIGGER booking_events_no_truncate BEFORE TRUNCATE ON booking_events FOR EACH STATEMENT EXECUTE FUNCTION forbid_change();

-- +goose StatementBegin
CREATE FUNCTION bookings_log_status() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO booking_events (booking_id, from_status, to_status, actor, reason)
  VALUES (NEW.id,
          CASE WHEN TG_OP = 'UPDATE' THEN OLD.status END,
          NEW.status,
          nullif(current_setting('app.actor', true), ''),
          nullif(current_setting('app.reason', true), ''));
  RETURN NULL;
END $$;
-- +goose StatementEnd
CREATE TRIGGER bookings_log_insert AFTER INSERT ON bookings FOR EACH ROW EXECUTE FUNCTION bookings_log_status();
CREATE TRIGGER bookings_log_update AFTER UPDATE OF status ON bookings
  FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status) EXECUTE FUNCTION bookings_log_status();

-- Safe retries. The same key with the same request replays the saved response; with a different request it
-- is an error (IDEMPOTENCY_MISMATCH).
CREATE TABLE idempotency_keys (
  user_id         uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  key             text NOT NULL CHECK (length(key) BETWEEN 8 AND 255),
  endpoint        text NOT NULL,
  request_hash    bytea NOT NULL,
  response_status integer,        -- null while the first request is still running
  response_body   jsonb,
  created_at      timestamptz NOT NULL DEFAULT now(),
  expires_at      timestamptz NOT NULL DEFAULT now() + interval '48 hours',
  PRIMARY KEY (user_id, key)
);
CREATE INDEX idempotency_keys_expiry_idx ON idempotency_keys (expires_at);

-- One review per stay, written after check-out.
CREATE TABLE reviews (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  booking_id  uuid NOT NULL UNIQUE REFERENCES bookings (id),
  property_id uuid NOT NULL REFERENCES properties (id),
  user_id     uuid NOT NULL REFERENCES users (id),
  rating      integer NOT NULL CHECK (rating BETWEEN 1 AND 5),
  text        text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX reviews_property_idx ON reviews (property_id, created_at DESC);

CREATE VIEW property_ratings AS
  SELECT property_id, round(avg(rating)::numeric, 1) AS rating, count(*)::integer AS review_count
  FROM reviews GROUP BY property_id;

-- +goose Down
DROP VIEW IF EXISTS app.property_ratings;
DROP TABLE IF EXISTS app.reviews, app.idempotency_keys, app.booking_events, app.bookings, app.quotes, app.inventory CASCADE;
DROP FUNCTION IF EXISTS app.bookings_log_status(), app.bookings_guard(), app.booking_transition_allowed(text, text);
