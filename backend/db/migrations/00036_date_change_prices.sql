-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Moving a booking to dates that cost a different amount. A higher price needs a second payment (the difference), so a booking can have a payment of a
-- second kind next to its main one; a pending request holds the new nights until that payment arrives or the request runs out.
ALTER TABLE payments ADD COLUMN kind text NOT NULL DEFAULT 'booking' CHECK (kind IN ('booking', 'date_change'));
DROP INDEX payments_one_pending_per_booking;
DROP INDEX payments_one_success_per_booking;
CREATE UNIQUE INDEX payments_one_pending_per_booking ON payments (booking_id, kind) WHERE status = 'pending';
CREATE UNIQUE INDEX payments_one_success_per_booking ON payments (booking_id, kind) WHERE status = 'succeeded';

CREATE TABLE date_change_requests (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  booking_id         uuid NOT NULL REFERENCES bookings (id),
  payment_id         uuid REFERENCES payments (id),         -- the payment of the difference, once it is started
  old_check_in       date NOT NULL,
  old_check_out      date NOT NULL,
  new_check_in       date NOT NULL,
  new_check_out      date NOT NULL CHECK (new_check_out > new_check_in),
  base_santim        bigint NOT NULL CHECK (base_santim >= 0),       -- the price of the new dates, applied when the payment arrives
  service_santim     bigint NOT NULL CHECK (service_santim >= 0),
  vat_santim         bigint NOT NULL CHECK (vat_santim >= 0),
  total_santim       bigint NOT NULL CHECK (total_santim >= 0),
  deposit_santim     bigint NOT NULL CHECK (deposit_santim >= 0),
  balance_due_santim bigint NOT NULL CHECK (balance_due_santim >= 0),
  pay_santim         bigint NOT NULL CHECK (pay_santim > 0),         -- what the guest pays now
  status             text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'applied', 'expired', 'cancelled', 'refunded')),
  expires_at         timestamptz NOT NULL,
  created_at         timestamptz NOT NULL DEFAULT now(),
  resolved_at        timestamptz
);
CREATE UNIQUE INDEX date_change_one_pending ON date_change_requests (booking_id) WHERE status = 'pending';
CREATE INDEX date_change_due_idx ON date_change_requests (expires_at) WHERE status = 'pending';
CREATE INDEX date_change_payment_idx ON date_change_requests (payment_id) WHERE payment_id IS NOT NULL;
ALTER TABLE date_change_requests ENABLE ROW LEVEL SECURITY;

-- The agreed price still never changes by accident. A date change that costs a different amount reprices the booking, and says so first (app.reprice);
-- the currency and the number of rooms stay fixed whatever happens.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION bookings_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status IS DISTINCT FROM OLD.status AND NOT booking_transition_allowed(OLD.status, NEW.status) THEN
    RAISE EXCEPTION 'illegal booking status change % -> %', OLD.status, NEW.status USING ERRCODE = 'check_violation';
  END IF;
  IF NEW.rooms IS DISTINCT FROM OLD.rooms THEN
    RAISE EXCEPTION 'booking amounts cannot be changed: the number of rooms is part of the price' USING ERRCODE = 'check_violation';
  END IF;
  IF NEW.currency IS DISTINCT FROM OLD.currency THEN
    RAISE EXCEPTION 'booking amounts cannot be changed: the currency is fixed' USING ERRCODE = 'check_violation';
  END IF;
  IF (NEW.base_santim, NEW.service_santim, NEW.vat_santim, NEW.total_santim, NEW.deposit_santim, NEW.balance_due_santim)
     IS DISTINCT FROM
     (OLD.base_santim, OLD.service_santim, OLD.vat_santim, OLD.total_santim, OLD.deposit_santim, OLD.balance_due_santim)
     AND coalesce(current_setting('app.reprice', true), '') <> 'on' THEN
    RAISE EXCEPTION 'booking amounts cannot be changed' USING ERRCODE = 'check_violation';
  END IF;
  NEW.version := OLD.version + 1;
  NEW.updated_at := now();
  RETURN NEW;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.bookings_guard() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status IS DISTINCT FROM OLD.status AND NOT app.booking_transition_allowed(OLD.status, NEW.status) THEN
    RAISE EXCEPTION 'illegal booking status change % -> %', OLD.status, NEW.status USING ERRCODE = 'check_violation';
  END IF;
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
DROP TABLE IF EXISTS app.date_change_requests;
DROP INDEX IF EXISTS app.payments_one_pending_per_booking;
DROP INDEX IF EXISTS app.payments_one_success_per_booking;
CREATE UNIQUE INDEX payments_one_pending_per_booking ON app.payments (booking_id) WHERE status = 'pending';
CREATE UNIQUE INDEX payments_one_success_per_booking ON app.payments (booking_id) WHERE status = 'succeeded';
ALTER TABLE app.payments DROP COLUMN IF EXISTS kind;
