-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A refund used to hold a database transaction (and a connection and a row lock) open while it waited for the payment gateway. Now a refund is first CLAIMED in a
-- short transaction, then sent to the gateway with nothing held, then recorded in another short transaction. The claim is a time limit: if the server stops in
-- between, the next pass takes the refund over and asks the gateway again with the same reference, which the gateway answers once.
--
--   refund_amount_santim   what the full refund of this payment pays, fixed when the refund starts so every retry pays the same
--   refund_started_at      set at the first claim; from then on no partial refund may be added to this payment
--   refund_claimed_until   another pass leaves the refund alone until this time (also the pause before a retry)
ALTER TABLE payments
  ADD COLUMN refund_amount_santim bigint CHECK (refund_amount_santim IS NULL OR refund_amount_santim >= 0),
  ADD COLUMN refund_started_at    timestamptz,
  ADD COLUMN refund_claimed_until timestamptz,
  ADD COLUMN refund_attempts      integer NOT NULL DEFAULT 0,
  ADD COLUMN refund_last_error    text;

ALTER TABLE partial_refunds
  ADD COLUMN claimed_until timestamptz,
  ADD COLUMN attempts      integer NOT NULL DEFAULT 0,
  ADD COLUMN last_error    text;

-- +goose Down
ALTER TABLE app.partial_refunds DROP COLUMN IF EXISTS last_error, DROP COLUMN IF EXISTS attempts, DROP COLUMN IF EXISTS claimed_until;
ALTER TABLE app.payments DROP COLUMN IF EXISTS refund_last_error, DROP COLUMN IF EXISTS refund_attempts, DROP COLUMN IF EXISTS refund_claimed_until,
  DROP COLUMN IF EXISTS refund_started_at, DROP COLUMN IF EXISTS refund_amount_santim;
