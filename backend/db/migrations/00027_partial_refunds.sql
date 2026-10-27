-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A partial refund returns part of a payment while the booking stands (a dispute settled for part of the price, or a goodwill credit).
-- It is owed until the gateway has accepted it. The payment keeps its status 'succeeded'; what is still refundable is the payment amount
-- minus the partial refunds (see partialrefund.go). Who bears it is 'hotel' (its share and BookDan's commission shrink in proportion) or
-- 'platform' (BookDan pays from its own money, ledger expense dispute_expense, and the hotel keeps its share).
CREATE TABLE partial_refunds (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  payment_id    uuid NOT NULL REFERENCES payments (id),
  booking_id    uuid NOT NULL REFERENCES bookings (id),
  amount_santim bigint NOT NULL CHECK (amount_santim > 0),
  payer         text NOT NULL CHECK (payer IN ('hotel', 'platform')),
  reason        text NOT NULL CHECK (length(reason) BETWEEN 3 AND 1000),
  ticket_id     uuid REFERENCES support_tickets (id),
  status        text NOT NULL DEFAULT 'owed' CHECK (status IN ('owed', 'paid')),
  created_by    text NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  paid_at       timestamptz,
  CONSTRAINT paid_together CHECK ((status = 'paid') = (paid_at IS NOT NULL))
);
CREATE INDEX partial_refunds_payment_idx ON partial_refunds (payment_id);
CREATE INDEX partial_refunds_owed_idx ON partial_refunds (created_at) WHERE status = 'owed';
ALTER TABLE partial_refunds ENABLE ROW LEVEL SECURITY;

UPDATE ledger_accounts SET description = 'Refunds BookDan paid from its own money (a dispute settled, or a goodwill credit), so the hotel keeps its share' WHERE code = 'dispute_expense';

ALTER TABLE support_tickets DROP CONSTRAINT IF EXISTS support_tickets_outcome_check;
ALTER TABLE support_tickets ADD CONSTRAINT support_tickets_outcome_check
  CHECK (outcome IN ('rejected', 'refunded_hotel_pays', 'refunded_platform_pays', 'refunded_partial_hotel_pays', 'refunded_partial_platform_pays', 'withdrawn'));

-- +goose Down
ALTER TABLE app.support_tickets DROP CONSTRAINT IF EXISTS support_tickets_outcome_check;
ALTER TABLE app.support_tickets ADD CONSTRAINT support_tickets_outcome_check
  CHECK (outcome IN ('rejected', 'refunded_hotel_pays', 'refunded_platform_pays', 'withdrawn'));
DROP TABLE IF EXISTS app.partial_refunds;
