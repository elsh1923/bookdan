-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- What BookDan knows about the business behind a hotel: who it is legally, who to contact, where to pay it.
CREATE TABLE hotel_business (
  property_id          uuid PRIMARY KEY REFERENCES properties (id) ON DELETE CASCADE,
  legal_name           text NOT NULL,
  license_number       text NOT NULL,
  tin                  text,
  contact_name         text NOT NULL,
  contact_phone        text NOT NULL,
  contact_email        text NOT NULL,
  payout_method        text NOT NULL CHECK (payout_method IN ('telebirr', 'cbe_birr', 'bank_transfer')),
  payout_account_name  text NOT NULL,
  payout_enc           bytea NOT NULL,
  payout_last4         text NOT NULL,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER hotel_business_touch BEFORE UPDATE ON hotel_business FOR EACH ROW EXECUTE FUNCTION touch_updated_at();
ALTER TABLE hotel_business ENABLE ROW LEVEL SECURITY;

-- Staff: when they were invited and when they first signed in (so the admin can see who has joined).
ALTER TABLE property_staff ADD COLUMN invited_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE property_staff ADD COLUMN joined_at timestamptz;

-- +goose Down
ALTER TABLE app.property_staff DROP COLUMN IF EXISTS joined_at, DROP COLUMN IF EXISTS invited_at;
DROP TABLE IF EXISTS app.hotel_business;
