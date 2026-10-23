-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A guest can cancel before the hotel has answered. (Cancelling a confirmed booking was already allowed.)
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

-- What a cancellation refunded: the whole payment, or 0 when it came too late for a refund. The status stays CANCELLED_REFUNDED
-- either way; this number says what happened to the money.
ALTER TABLE bookings ADD COLUMN refund_santim bigint CHECK (refund_santim IS NULL OR refund_santim >= 0);
ALTER TABLE bookings ADD COLUMN cancelled_at timestamptz;

-- A review is at most 1000 characters.
ALTER TABLE reviews ADD CONSTRAINT reviews_text_length CHECK (length(text) <= 1000);

-- +goose Down
ALTER TABLE app.reviews DROP CONSTRAINT IF EXISTS reviews_text_length;
ALTER TABLE app.bookings DROP COLUMN IF EXISTS cancelled_at, DROP COLUMN IF EXISTS refund_santim;
