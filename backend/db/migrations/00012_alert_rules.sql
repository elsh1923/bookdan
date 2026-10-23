-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Who gets a booking alert, and when.
--   alert_mode 'everyone':         every person on duty is alerted at once.
--   alert_mode 'front_desk_first': receptionists on duty first; owners only if nobody answers within escalate_after_min minutes
--                                  (or at once if no receptionist can be reached).
ALTER TABLE properties ADD COLUMN alert_mode text NOT NULL DEFAULT 'everyone' CHECK (alert_mode IN ('everyone', 'front_desk_first'));
ALTER TABLE properties ADD COLUMN escalate_after_min int NOT NULL DEFAULT 5 CHECK (escalate_after_min BETWEEN 1 AND 10);

-- Per person: alerts can be switched off, and a shift limits them to certain hours (Addis Ababa time). No shift means always on duty.
-- A shift may run past midnight (22:00 to 06:00).
ALTER TABLE property_staff ADD COLUMN alerts_on boolean NOT NULL DEFAULT true;
ALTER TABLE property_staff ADD COLUMN shift_start time;
ALTER TABLE property_staff ADD COLUMN shift_end time;
ALTER TABLE property_staff ADD CONSTRAINT property_staff_shift CHECK (
  (shift_start IS NULL AND shift_end IS NULL) OR (shift_start IS NOT NULL AND shift_end IS NOT NULL AND shift_start <> shift_end));

-- What was already done for a waiting request, so each step happens once.
ALTER TABLE bookings ADD COLUMN hotel_escalated_at timestamptz;
ALTER TABLE bookings ADD COLUMN hotel_reminded_at timestamptz;

-- +goose Down
ALTER TABLE app.bookings DROP COLUMN IF EXISTS hotel_reminded_at, DROP COLUMN IF EXISTS hotel_escalated_at;
ALTER TABLE app.property_staff DROP CONSTRAINT IF EXISTS property_staff_shift;
ALTER TABLE app.property_staff DROP COLUMN IF EXISTS shift_end, DROP COLUMN IF EXISTS shift_start, DROP COLUMN IF EXISTS alerts_on;
ALTER TABLE app.properties DROP COLUMN IF EXISTS escalate_after_min, DROP COLUMN IF EXISTS alert_mode;
