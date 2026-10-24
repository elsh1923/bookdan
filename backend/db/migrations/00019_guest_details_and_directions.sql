-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Who is travelling: how many of the guests are children (the rest are adults; each room needs an adult).
ALTER TABLE quotes   ADD COLUMN children integer NOT NULL DEFAULT 0 CHECK (children >= 0);
ALTER TABLE bookings ADD COLUMN children integer NOT NULL DEFAULT 0 CHECK (children >= 0);

-- Things the guest asks the hotel for ("late arrival", "high floor"), and whether the person staying is not the one who booked.
-- guest_name already holds the name of the person staying; for someone else the hotel also gets their phone number.
ALTER TABLE bookings ADD COLUMN special_requests text CHECK (length(special_requests) <= 400);
ALTER TABLE bookings ADD COLUMN booked_for_other boolean NOT NULL DEFAULT false;
ALTER TABLE bookings ADD COLUMN guest_phone text CHECK (length(guest_phone) <= 30);

-- What a hotel writes for its guests: its description in Amharic, and how to find it in words (addresses here are often
-- "behind the church, near the pharmacy"), in English and Amharic.
ALTER TABLE properties ADD COLUMN description_am text NOT NULL DEFAULT '';
ALTER TABLE properties ADD COLUMN directions     text NOT NULL DEFAULT '';
ALTER TABLE properties ADD COLUMN directions_am  text NOT NULL DEFAULT '';

-- The list of hotel features the database accepts grows with the app: air conditioning, pool, restaurant, gym, spa, airport shuttle,
-- laundry, a 24-hour front desk and 24-hour water, on top of the first five.
ALTER TABLE properties DROP CONSTRAINT properties_amenities_check;
ALTER TABLE properties ADD CONSTRAINT properties_amenities_check CHECK (amenities <@ ARRAY[
  'generator', 'hot_water', 'wifi', 'parking', 'pay_at_hotel',
  'ac', 'pool', 'restaurant', 'gym', 'spa', 'airport_shuttle', 'laundry', 'front_desk_24h', 'water_tank']::text[]);

-- +goose Down
ALTER TABLE app.properties DROP CONSTRAINT IF EXISTS properties_amenities_check;
ALTER TABLE app.properties ADD CONSTRAINT properties_amenities_check CHECK (amenities <@ ARRAY['generator', 'hot_water', 'wifi', 'parking', 'pay_at_hotel']::text[]);
ALTER TABLE app.properties DROP COLUMN IF EXISTS directions_am, DROP COLUMN IF EXISTS directions, DROP COLUMN IF EXISTS description_am;
ALTER TABLE app.bookings DROP COLUMN IF EXISTS guest_phone, DROP COLUMN IF EXISTS booked_for_other, DROP COLUMN IF EXISTS special_requests, DROP COLUMN IF EXISTS children;
ALTER TABLE app.quotes DROP COLUMN IF EXISTS children;
