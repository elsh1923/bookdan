-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Disputes. A guest challenges a charge or a stay; the booking becomes DISPUTED (so no money is released to the hotel while it is open)
-- and a support ticket of kind 'dispute' carries the conversation. An admin resolves it:
--   rejected:             nothing changes; the booking goes back to the status it had (disputed_from);
--   refunded_hotel_pays:  the guest is refunded in full and the hotel's share and BookDan's commission are reversed (the booking is cancelled);
--   refunded_platform_pays: the guest is refunded in full from BookDan's own money (expense 'dispute_expense'); the hotel keeps its share.
ALTER TABLE bookings ADD COLUMN disputed_from text
  CHECK (disputed_from IN ('PAID_AWAITING_HOTEL', 'CONFIRMED', 'CHECKED_IN', 'COMPLETED', 'NO_SHOW', 'NEEDS_RELOCATION'));

ALTER TABLE support_tickets ADD COLUMN outcome text
  CHECK (outcome IN ('rejected', 'refunded_hotel_pays', 'refunded_platform_pays', 'withdrawn'));
ALTER TABLE support_tickets ADD COLUMN reason text;

-- A booking has at most one open dispute.
CREATE UNIQUE INDEX support_one_open_dispute ON support_tickets (booking_id) WHERE kind = 'dispute' AND status <> 'closed';

INSERT INTO ledger_accounts (code, kind, description)
VALUES ('dispute_expense', 'expense', 'Refunds BookDan paid from its own money to settle a dispute, so the hotel keeps its share');

-- A resolved dispute puts the booking back where it was, so DISPUTED may lead back to any state a dispute can start from.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION booking_transition_allowed(from_status text, to_status text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
  SELECT (from_status, to_status) IN (
    ('HELD', 'PAID_AWAITING_HOTEL'), ('HELD', 'EXPIRED'), ('PAID_AWAITING_HOTEL', 'CONFIRMED'), ('PAID_AWAITING_HOTEL', 'DECLINED_REFUNDED'),
    ('PAID_AWAITING_HOTEL', 'CANCELLED_REFUNDED'), ('CONFIRMED', 'CHECKED_IN'), ('CONFIRMED', 'COMPLETED'), ('CONFIRMED', 'CANCELLED_REFUNDED'),
    ('CONFIRMED', 'NO_SHOW'), ('CONFIRMED', 'NEEDS_RELOCATION'), ('NEEDS_RELOCATION', 'CONFIRMED'), ('NEEDS_RELOCATION', 'CANCELLED_REFUNDED'),
    ('CHECKED_IN', 'COMPLETED'),
    ('PAID_AWAITING_HOTEL', 'DISPUTED'), ('CONFIRMED', 'DISPUTED'), ('CHECKED_IN', 'DISPUTED'), ('COMPLETED', 'DISPUTED'), ('NO_SHOW', 'DISPUTED'),
    ('NEEDS_RELOCATION', 'DISPUTED'),
    ('DISPUTED', 'CONFIRMED'), ('DISPUTED', 'COMPLETED'), ('DISPUTED', 'CANCELLED_REFUNDED'),
    ('DISPUTED', 'PAID_AWAITING_HOTEL'), ('DISPUTED', 'CHECKED_IN'), ('DISPUTED', 'NO_SHOW'), ('DISPUTED', 'NEEDS_RELOCATION')
  )
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION app.booking_transition_allowed(from_status text, to_status text) RETURNS boolean
LANGUAGE sql IMMUTABLE AS $$
  SELECT (from_status, to_status) IN (
    ('HELD', 'PAID_AWAITING_HOTEL'), ('HELD', 'EXPIRED'), ('PAID_AWAITING_HOTEL', 'CONFIRMED'), ('PAID_AWAITING_HOTEL', 'DECLINED_REFUNDED'),
    ('PAID_AWAITING_HOTEL', 'CANCELLED_REFUNDED'), ('CONFIRMED', 'CHECKED_IN'), ('CONFIRMED', 'COMPLETED'), ('CONFIRMED', 'CANCELLED_REFUNDED'),
    ('CONFIRMED', 'NO_SHOW'), ('CONFIRMED', 'NEEDS_RELOCATION'), ('NEEDS_RELOCATION', 'CONFIRMED'), ('NEEDS_RELOCATION', 'CANCELLED_REFUNDED'),
    ('CHECKED_IN', 'COMPLETED'),
    ('PAID_AWAITING_HOTEL', 'DISPUTED'), ('CONFIRMED', 'DISPUTED'), ('CHECKED_IN', 'DISPUTED'), ('COMPLETED', 'DISPUTED'), ('NO_SHOW', 'DISPUTED'),
    ('NEEDS_RELOCATION', 'DISPUTED'),
    ('DISPUTED', 'CONFIRMED'), ('DISPUTED', 'COMPLETED'), ('DISPUTED', 'CANCELLED_REFUNDED')
  )
$$;
-- +goose StatementEnd
DELETE FROM app.ledger_accounts WHERE code = 'dispute_expense';
DROP INDEX IF EXISTS app.support_one_open_dispute;
ALTER TABLE app.support_tickets DROP COLUMN IF EXISTS reason;
ALTER TABLE app.support_tickets DROP COLUMN IF EXISTS outcome;
ALTER TABLE app.bookings DROP COLUMN IF EXISTS disputed_from;
