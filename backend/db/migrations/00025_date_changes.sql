-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A guest can move a confirmed booking to other dates once, at the same price (see datechange.go). The booking keeps its price, its
-- payment and its ledger entries; only the nights change. The first dates are kept here, and every change is also in the audit log.
ALTER TABLE bookings ADD COLUMN date_changes int NOT NULL DEFAULT 0 CHECK (date_changes BETWEEN 0 AND 1);
ALTER TABLE bookings ADD COLUMN original_check_in date;
ALTER TABLE bookings ADD COLUMN original_check_out date;
ALTER TABLE bookings ADD CONSTRAINT original_dates_together CHECK ((original_check_in IS NULL) = (original_check_out IS NULL));

-- +goose Down
ALTER TABLE app.bookings DROP CONSTRAINT IF EXISTS original_dates_together;
ALTER TABLE app.bookings DROP COLUMN IF EXISTS original_check_out;
ALTER TABLE app.bookings DROP COLUMN IF EXISTS original_check_in;
ALTER TABLE app.bookings DROP COLUMN IF EXISTS date_changes;
