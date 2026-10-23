-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- When a guest arrived and who at the hotel checked them in (the status history records the change as well).
ALTER TABLE bookings ADD COLUMN checked_in_at timestamptz;
ALTER TABLE bookings ADD COLUMN checked_in_by uuid REFERENCES users (id);

-- +goose Down
ALTER TABLE app.bookings DROP COLUMN IF EXISTS checked_in_by, DROP COLUMN IF EXISTS checked_in_at;
