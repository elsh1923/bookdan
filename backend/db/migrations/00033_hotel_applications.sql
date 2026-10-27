-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A hotel applies to be listed (the /apply page). An admin reviews it; approving creates a draft hotel and invites the applicant as its owner by email.
-- Nothing is public until the admin has completed the hotel (rooms, prices, business details, location) and published it.
CREATE TABLE hotel_applications (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  status         text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
  hotel_name     text NOT NULL CHECK (length(hotel_name) BETWEEN 2 AND 120),
  hotel_type     text NOT NULL CHECK (hotel_type IN ('hotel', 'guesthouse', 'lodge', 'resort')),
  city           text NOT NULL CHECK (length(city) BETWEEN 2 AND 80),
  area           text NOT NULL CHECK (length(area) BETWEEN 2 AND 120),
  address        text NOT NULL CHECK (length(address) BETWEEN 2 AND 200),
  hotel_phone    text NOT NULL CHECK (length(hotel_phone) BETWEEN 5 AND 30),
  description    text NOT NULL DEFAULT '' CHECK (length(description) <= 2000),
  rooms_count    int NOT NULL CHECK (rooms_count BETWEEN 1 AND 1000),
  pay_at_hotel   boolean NOT NULL DEFAULT false,
  owner_name     text NOT NULL CHECK (length(owner_name) BETWEEN 2 AND 120),
  owner_email    text NOT NULL CHECK (owner_email ~ '^[^@\s]+@[^@\s]+\.[^@\s]+$' AND length(owner_email) <= 200),
  owner_phone    text NOT NULL CHECK (length(owner_phone) BETWEEN 5 AND 30),
  legal_name     text NOT NULL CHECK (length(legal_name) BETWEEN 2 AND 160),
  license_number text NOT NULL CHECK (length(license_number) BETWEEN 2 AND 80),
  ip             inet,
  created_at     timestamptz NOT NULL DEFAULT now(),
  decided_at     timestamptz,
  decided_by     text,
  decision_note  text CHECK (length(decision_note) <= 1000),
  property_id    uuid REFERENCES properties (id),
  CONSTRAINT decided_together CHECK ((status = 'pending') = (decided_at IS NULL))
);
CREATE INDEX hotel_applications_queue_idx ON hotel_applications (created_at) WHERE status = 'pending';
CREATE INDEX hotel_applications_ip_idx ON hotel_applications (ip, created_at);
CREATE INDEX hotel_applications_email_idx ON hotel_applications (lower(owner_email), status);
ALTER TABLE hotel_applications ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.hotel_applications;
