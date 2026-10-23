-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A payout is not about one booking, so its ledger transaction names the hotel directly.
ALTER TABLE ledger_transactions ADD COLUMN property_id uuid REFERENCES properties (id);

-- Money BookDan has paid out to a hotel (recorded by a platform admin after the transfer was made).
CREATE TABLE hotel_payouts (
  id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  property_id           uuid NOT NULL REFERENCES properties (id),
  amount_santim         bigint NOT NULL CHECK (amount_santim > 0),
  currency              text NOT NULL DEFAULT 'ETB' CHECK (currency IN ('ETB', 'USD', 'EUR')),
  method                text NOT NULL CHECK (method IN ('telebirr', 'cbe_birr', 'bank_transfer')),
  reference             text NOT NULL,   -- the transfer's own reference (bank or Telebirr transaction id)
  note                  text,
  paid_at               timestamptz NOT NULL DEFAULT now(),
  paid_by               uuid REFERENCES users (id),
  ledger_transaction_id uuid NOT NULL REFERENCES ledger_transactions (id)
);
CREATE INDEX hotel_payouts_property_idx ON hotel_payouts (property_id, paid_at DESC);
-- A payout is a fact: it is corrected with an opposite ledger adjustment, never edited.
CREATE TRIGGER hotel_payouts_append_only BEFORE UPDATE OR DELETE ON hotel_payouts FOR EACH ROW EXECUTE FUNCTION forbid_change();
ALTER TABLE hotel_payouts ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.hotel_payouts;
ALTER TABLE app.ledger_transactions DROP COLUMN IF EXISTS property_id;
