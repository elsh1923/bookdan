-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Pay at hotel: the guest pays the hotel directly, so BookDan holds no money for the stay. When such a stay is completed, BookDan books its
-- commission (and the VAT on it) as a debit on property_payable: it reduces what BookDan owes the hotel, and when the hotel has only
-- pay-at-hotel stays the balance turns negative, meaning the hotel owes BookDan. The marker keeps it from being booked twice.
ALTER TABLE bookings ADD COLUMN hotel_commission_booked_at timestamptz;

-- Money a hotel paid to BookDan (the commission on its pay-at-hotel stays): the opposite of a payout, recorded the same way.
CREATE TABLE hotel_receipts (
  id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  property_id           uuid NOT NULL REFERENCES properties (id),
  amount_santim         bigint NOT NULL CHECK (amount_santim > 0),
  reference             text NOT NULL,
  note                  text,
  received_at           timestamptz NOT NULL DEFAULT now(),
  received_by           uuid REFERENCES users (id),
  ledger_transaction_id uuid NOT NULL REFERENCES ledger_transactions (id)
);
CREATE INDEX hotel_receipts_property_idx ON hotel_receipts (property_id, received_at DESC);
CREATE TRIGGER hotel_receipts_append_only BEFORE UPDATE OR DELETE ON hotel_receipts FOR EACH ROW EXECUTE FUNCTION forbid_change();
ALTER TABLE hotel_receipts ENABLE ROW LEVEL SECURITY;

-- Which monthly statements were emailed, so a restart never sends one twice.
CREATE TABLE statement_emails (
  property_id uuid NOT NULL REFERENCES properties (id),
  month       text NOT NULL CHECK (month ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
  sent_at     timestamptz NOT NULL DEFAULT now(),
  recipients  int NOT NULL DEFAULT 0,
  PRIMARY KEY (property_id, month)
);
ALTER TABLE statement_emails ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.statement_emails;
DROP TABLE IF EXISTS app.hotel_receipts;
ALTER TABLE app.bookings DROP COLUMN IF EXISTS hotel_commission_booked_at;
