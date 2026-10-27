-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A payment that needs a person. Today there is one kind: the gateway says a guest paid, but the amount or the currency is not what we
-- asked for. The booking is NOT confirmed, the money sits with the gateway, and the admins and support are told (see payment_issues.go).
-- One open row per payment and kind: repeated checks of the same payment never raise it twice.
CREATE TABLE payment_issues (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  payment_id  uuid NOT NULL REFERENCES payments (id),
  booking_id  uuid NOT NULL REFERENCES bookings (id),
  kind        text NOT NULL CHECK (kind IN ('amount_mismatch')),
  details     jsonb NOT NULL DEFAULT '{}'::jsonb,   -- what we asked for and what the gateway reports; never a card or a key
  created_at  timestamptz NOT NULL DEFAULT now(),
  resolved_at timestamptz,
  resolved_by text,                                 -- the admin's email
  note        text CHECK (length(note) <= 1000),
  UNIQUE (payment_id, kind),
  CONSTRAINT resolved_together CHECK ((resolved_at IS NULL) = (resolved_by IS NULL))
);
CREATE INDEX payment_issues_open_idx ON payment_issues (created_at) WHERE resolved_at IS NULL;
ALTER TABLE payment_issues ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.payment_issues;
