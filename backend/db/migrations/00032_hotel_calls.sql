-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A guest taps "Call the hotel" on a confirmed booking: the app tells the server, so BookDan can see how often guests and hotels talk outside the
-- app (the admin page, Calls). The number is shown only once a booking is confirmed; it is not on the hotel's page.
CREATE TABLE hotel_calls (
  id          bigserial PRIMARY KEY,
  booking_id  uuid NOT NULL REFERENCES bookings (id),
  property_id uuid NOT NULL REFERENCES properties (id),
  user_id     uuid NOT NULL REFERENCES users (id),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX hotel_calls_property_idx ON hotel_calls (property_id, created_at DESC);
CREATE INDEX hotel_calls_booking_idx ON hotel_calls (booking_id);
ALTER TABLE hotel_calls ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.hotel_calls;
