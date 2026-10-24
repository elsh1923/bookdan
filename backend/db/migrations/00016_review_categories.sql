-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Optional sub-ratings a guest can give along with the overall stars (1 to 5, empty when the guest skipped them).
ALTER TABLE reviews ADD COLUMN rating_cleanliness smallint CHECK (rating_cleanliness BETWEEN 1 AND 5);
ALTER TABLE reviews ADD COLUMN rating_location    smallint CHECK (rating_location BETWEEN 1 AND 5);
ALTER TABLE reviews ADD COLUMN rating_staff       smallint CHECK (rating_staff BETWEEN 1 AND 5);
ALTER TABLE reviews ADD COLUMN rating_value       smallint CHECK (rating_value BETWEEN 1 AND 5);

-- The times a hotel gives for arriving and leaving (Addis Ababa time), shown on the hotel page and in the booking.
ALTER TABLE properties ADD COLUMN check_in_time  time NOT NULL DEFAULT '14:00';
ALTER TABLE properties ADD COLUMN check_out_time time NOT NULL DEFAULT '12:00';

-- The demo hotels' house rules already say "check-in from 12:00, check-out until 11:00": make the times agree.
UPDATE properties SET check_in_time = '12:00', check_out_time = '11:00' WHERE house_rules::text LIKE '%Check-in from 12:00, check-out until 11:00%';

-- +goose Down
ALTER TABLE app.properties DROP COLUMN IF EXISTS check_out_time, DROP COLUMN IF EXISTS check_in_time;
ALTER TABLE app.reviews DROP COLUMN IF EXISTS rating_value, DROP COLUMN IF EXISTS rating_staff,
  DROP COLUMN IF EXISTS rating_location, DROP COLUMN IF EXISTS rating_cleanliness;
