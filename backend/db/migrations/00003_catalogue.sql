-- +goose Up
SET LOCAL search_path = app, extensions, public;

CREATE TABLE properties (
  id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name                    text NOT NULL,
  type                    text NOT NULL CHECK (type IN ('hotel', 'guesthouse', 'lodge', 'resort')),
  city                    text NOT NULL,
  area                    text NOT NULL,
  address                 text NOT NULL,
  location                geography(Point, 4326) NOT NULL,
  phone                   text NOT NULL,
  description             text NOT NULL DEFAULT '',
  verified                boolean NOT NULL DEFAULT false,
  pay_at_hotel            boolean NOT NULL DEFAULT false,
  -- If the hotel has not synced for this long, the platform may hide its rooms online.
  freshness_limit_minutes integer NOT NULL DEFAULT 1440 CHECK (freshness_limit_minutes > 0),
  last_synced_at          timestamptz,
  house_rules             jsonb NOT NULL DEFAULT '[]'::jsonb,
  amenities               text[] NOT NULL DEFAULT '{}'
    CHECK (amenities <@ ARRAY['generator', 'hot_water', 'wifi', 'parking', 'pay_at_hotel']::text[]),
  status                  text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'active', 'hidden')),
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER properties_touch BEFORE UPDATE ON properties FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
CREATE INDEX properties_location_idx ON properties USING gist (location);
CREATE INDEX properties_city_idx ON properties (city) WHERE status = 'active';

-- Who may act for a hotel. Every partner endpoint checks this, so a hotel only ever sees its own data.
CREATE TABLE property_staff (
  property_id uuid NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
  user_id     uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
  role        text NOT NULL CHECK (role IN ('owner', 'receptionist')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (property_id, user_id)
);
CREATE INDEX property_staff_user_idx ON property_staff (user_id);

-- "Near Meskel Square": properties within a radius of a landmark.
CREATE TABLE landmarks (
  id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  name     text NOT NULL,
  name_am  text,
  city     text NOT NULL,
  location geography(Point, 4326) NOT NULL
);
CREATE INDEX landmarks_location_idx ON landmarks USING gist (location);
CREATE INDEX landmarks_name_idx ON landmarks (lower(name));

CREATE TABLE room_types (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  property_id uuid NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
  name        text NOT NULL,
  capacity    integer NOT NULL CHECK (capacity > 0),
  amenities   text[] NOT NULL DEFAULT '{}',
  total_rooms integer NOT NULL CHECK (total_rooms >= 0),
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (property_id, name),
  UNIQUE (id, property_id)  -- lets other tables prove a room type belongs to the same property
);

CREATE TABLE rooms (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  property_id  uuid NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
  room_type_id uuid NOT NULL,
  number       text NOT NULL,
  status       text NOT NULL DEFAULT 'vacant' CHECK (status IN ('vacant', 'occupied', 'dirty', 'out_of_service')),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (property_id, number),
  -- A room can only be of a room type that belongs to its own hotel.
  FOREIGN KEY (room_type_id, property_id) REFERENCES room_types (id, property_id) ON DELETE CASCADE
);
CREATE TRIGGER rooms_touch BEFORE UPDATE ON rooms FOR EACH ROW EXECUTE FUNCTION touch_updated_at();

-- Nightly price for a room type over a date range. Ranges for one room type may not overlap, so there is
-- never any doubt which price applies on a night.
CREATE TABLE rate_plans (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  room_type_id         uuid NOT NULL REFERENCES room_types (id) ON DELETE CASCADE,
  currency             text NOT NULL DEFAULT 'ETB' CHECK (currency IN ('ETB', 'USD', 'EUR')),
  nightly_price_santim bigint NOT NULL CHECK (nightly_price_santim > 0),
  valid_from           date NOT NULL,
  valid_to             date NOT NULL,
  CHECK (valid_to >= valid_from),
  EXCLUDE USING gist (room_type_id WITH =, daterange(valid_from, valid_to, '[]') WITH &&)
);

-- Holiday boosts, in basis points (1000 = +10%).
CREATE TABLE rate_boosts (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  property_id uuid NOT NULL REFERENCES properties (id) ON DELETE CASCADE,
  starts_on   date NOT NULL,
  ends_on     date NOT NULL,
  percent_bp  integer NOT NULL CHECK (percent_bp BETWEEN -5000 AND 20000),
  label       text,
  CHECK (ends_on >= starts_on)
);
CREATE INDEX rate_boosts_property_idx ON rate_boosts (property_id, starts_on, ends_on);

-- The numbers behind every quote. ONE row, so a rate change never needs a deploy.
-- PLACEHOLDERS: the product owner has not confirmed these yet (design doc, DECISION 1). The quote code
-- must refuse to price a booking in production while confirmed is false.
CREATE TABLE pricing_config (
  id             integer PRIMARY KEY DEFAULT 1 CHECK (id = 1),
  service_bps    integer NOT NULL CHECK (service_bps BETWEEN 0 AND 10000),
  vat_bps        integer NOT NULL CHECK (vat_bps BETWEEN 0 AND 10000),
  deposit_bps    integer NOT NULL CHECK (deposit_bps BETWEEN 0 AND 10000),  -- share paid online; the rest at the hotel
  commission_bps integer NOT NULL CHECK (commission_bps BETWEEN 0 AND 10000),
  commission_vat_bps integer NOT NULL CHECK (commission_vat_bps BETWEEN 0 AND 10000),
  confirmed      boolean NOT NULL DEFAULT false,
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER pricing_config_touch BEFORE UPDATE ON pricing_config FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
-- Mirrors what the mobile mock does today: 15% VAT, the full amount paid online. Not a business decision.
INSERT INTO pricing_config (service_bps, vat_bps, deposit_bps, commission_bps, commission_vat_bps, confirmed)
VALUES (0, 1500, 10000, 0, 1500, false);

-- +goose Down
DROP TABLE IF EXISTS app.pricing_config, app.rate_boosts, app.rate_plans, app.rooms, app.room_types,
  app.landmarks, app.property_staff, app.properties CASCADE;
