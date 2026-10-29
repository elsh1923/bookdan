-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Saved hotels (the guest's wishlist) kept on the server, so they follow the guest to a new phone.
CREATE TABLE saved_properties (
  user_id     uuid NOT NULL REFERENCES users (id),
  property_id uuid NOT NULL REFERENCES properties (id),
  saved_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, property_id)
);
CREATE INDEX saved_properties_user_idx ON saved_properties (user_id, saved_at DESC);
ALTER TABLE saved_properties ENABLE ROW LEVEL SECURITY;

-- Sign-in codes are limited per address already; this makes counting them per client address cheap too.
CREATE INDEX otp_challenges_ip_idx ON otp_challenges (ip, created_at DESC) WHERE ip IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS app.otp_challenges_ip_idx;
DROP TABLE IF EXISTS app.saved_properties;
