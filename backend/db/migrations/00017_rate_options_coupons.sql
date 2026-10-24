-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- ---- Room options. A room type is sold as room only, or with breakfast, and either with free cancellation or non-refundable (cheaper).
--   breakfast_santim:  what breakfast adds per room per night; empty = no breakfast offered.
--   nonrefundable_bps: how much cheaper the room is when the guest accepts no refund (500 = 5%); empty = not offered.
ALTER TABLE room_types ADD COLUMN breakfast_santim  bigint CHECK (breakfast_santim > 0);
ALTER TABLE room_types ADD COLUMN nonrefundable_bps integer CHECK (nonrefundable_bps BETWEEN 100 AND 5000);

-- ---- Coupons. A percent coupon holds basis points (1000 = 10%), a fixed one holds santim. The discount comes off the room charges
-- before service and VAT, and BookDan pays for it: the hotel still receives its full share.
CREATE TABLE coupons (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  code                text NOT NULL UNIQUE CHECK (code ~ '^[A-Z0-9]{3,20}$'),
  description         text NOT NULL DEFAULT '',
  kind                text NOT NULL CHECK (kind IN ('percent', 'fixed')),
  value               bigint NOT NULL CHECK (value > 0),
  max_discount_santim bigint CHECK (max_discount_santim > 0),
  min_subtotal_santim bigint NOT NULL DEFAULT 0 CHECK (min_subtotal_santim >= 0),
  starts_at           timestamptz,
  ends_at             timestamptz,
  max_uses            integer CHECK (max_uses > 0),
  per_user_limit      integer NOT NULL DEFAULT 1 CHECK (per_user_limit > 0),
  first_booking_only  boolean NOT NULL DEFAULT false,
  property_id         uuid REFERENCES properties (id),
  active              boolean NOT NULL DEFAULT true,
  created_at          timestamptz NOT NULL DEFAULT now(),
  CHECK (kind <> 'percent' OR value <= 10000)
);
ALTER TABLE coupons ENABLE ROW LEVEL SECURITY;

ALTER TABLE quotes   ADD COLUMN meal            text    NOT NULL DEFAULT 'room_only' CHECK (meal IN ('room_only', 'breakfast'));
ALTER TABLE quotes   ADD COLUMN refundable      boolean NOT NULL DEFAULT true;
ALTER TABLE quotes   ADD COLUMN coupon_id       uuid REFERENCES coupons (id);
ALTER TABLE quotes   ADD COLUMN discount_santim bigint  NOT NULL DEFAULT 0 CHECK (discount_santim >= 0);  -- how much cheaper the total is thanks to the coupon (VAT included)

ALTER TABLE bookings ADD COLUMN meal            text    NOT NULL DEFAULT 'room_only' CHECK (meal IN ('room_only', 'breakfast'));
ALTER TABLE bookings ADD COLUMN refundable      boolean NOT NULL DEFAULT true;
ALTER TABLE bookings ADD COLUMN coupon_id       uuid REFERENCES coupons (id);
ALTER TABLE bookings ADD COLUMN discount_santim bigint  NOT NULL DEFAULT 0 CHECK (discount_santim >= 0);

-- Each use of a coupon, tied to its booking. A use is released when the booking ends without the guest keeping the stay.
CREATE TABLE coupon_redemptions (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  coupon_id        uuid NOT NULL REFERENCES coupons (id),
  user_id          uuid NOT NULL REFERENCES users (id),
  booking_id       uuid NOT NULL UNIQUE REFERENCES bookings (id),
  discount_santim  bigint NOT NULL CHECK (discount_santim >= 0),
  created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX coupon_redemptions_coupon_idx ON coupon_redemptions (coupon_id);
CREATE INDEX coupon_redemptions_user_idx ON coupon_redemptions (user_id, coupon_id);
ALTER TABLE coupon_redemptions ENABLE ROW LEVEL SECURITY;

-- What coupons cost BookDan. It is the other side of the hotel's full share when the guest paid less.
INSERT INTO ledger_accounts (code, kind, description) VALUES ('coupon_expense', 'expense', 'Discounts BookDan paid for with coupons, so the hotel still receives its full share');

-- +goose Down
DELETE FROM app.ledger_accounts WHERE code = 'coupon_expense';
DROP TABLE IF EXISTS app.coupon_redemptions;
ALTER TABLE app.bookings DROP COLUMN IF EXISTS discount_santim, DROP COLUMN IF EXISTS coupon_id, DROP COLUMN IF EXISTS refundable, DROP COLUMN IF EXISTS meal;
ALTER TABLE app.quotes DROP COLUMN IF EXISTS discount_santim, DROP COLUMN IF EXISTS coupon_id, DROP COLUMN IF EXISTS refundable, DROP COLUMN IF EXISTS meal;
DROP TABLE IF EXISTS app.coupons;
ALTER TABLE app.room_types DROP COLUMN IF EXISTS nonrefundable_bps, DROP COLUMN IF EXISTS breakfast_santim;
