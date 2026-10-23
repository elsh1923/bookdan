-- DEMO DATA for development and staging. Never run in production (cmd/migrate refuses).
-- Generated from the mobile app's demo catalogue so the apps and the database show the same hotels.
-- Safe to run again: existing rows are left alone.
SET search_path = app, extensions, public;

BEGIN;

INSERT INTO properties (id, name, type, city, area, address, location, phone, description, verified, pay_at_hotel, amenities, house_rules, status) VALUES
  ('a0000000-0000-0000-0000-000000000001', 'Meskel Square Palace Hotel', 'hotel', 'Addis Ababa', 'Meskel Square', 'Meskel Square, Addis Ababa', ST_SetSRID(ST_MakePoint(38.7613, 9.0108), 4326)::geography, '+251 11 000 0000', 'A clean, well-run stay close to the main sights, with friendly staff and reliable hot water.', true, false, ARRAY['generator', 'hot_water', 'wifi', 'parking']::text[], '["Check-in from 12:00, check-out until 11:00", "Valid ID required at check-in", "No smoking inside rooms"]'::jsonb, 'active'),
  ('a0000000-0000-0000-0000-000000000002', 'Bole Garden Guesthouse', 'guesthouse', 'Addis Ababa', 'Bole', 'Bole, Addis Ababa', ST_SetSRID(ST_MakePoint(38.7889, 8.9962), 4326)::geography, '+251 11 000 0000', 'A clean, well-run stay close to the main sights, with friendly staff and reliable hot water.', true, true, ARRAY['wifi', 'hot_water', 'pay_at_hotel']::text[], '["Check-in from 12:00, check-out until 11:00", "Valid ID required at check-in", "No smoking inside rooms"]'::jsonb, 'active'),
  ('a0000000-0000-0000-0000-000000000003', 'Piassa Heritage Inn', 'hotel', 'Addis Ababa', 'Piassa', 'Piassa, Addis Ababa', ST_SetSRID(ST_MakePoint(38.7469, 9.0346), 4326)::geography, '+251 11 000 0000', 'A clean, well-run stay close to the main sights, with friendly staff and reliable hot water.', false, false, ARRAY['hot_water', 'wifi']::text[], '["Check-in from 12:00, check-out until 11:00", "Valid ID required at check-in", "No smoking inside rooms"]'::jsonb, 'active'),
  ('a0000000-0000-0000-0000-000000000004', 'Lake Tana View Lodge', 'lodge', 'Bahir Dar', 'Lake Tana shore', 'Lake Tana shore, Bahir Dar', ST_SetSRID(ST_MakePoint(37.3876, 11.5936), 4326)::geography, '+251 11 000 0000', 'A clean, well-run stay close to the main sights, with friendly staff and reliable hot water.', true, true, ARRAY['generator', 'hot_water', 'wifi', 'parking', 'pay_at_hotel']::text[], '["Check-in from 12:00, check-out until 11:00", "Valid ID required at check-in", "No smoking inside rooms"]'::jsonb, 'active'),
  ('a0000000-0000-0000-0000-000000000005', 'Rock Church Guesthouse', 'guesthouse', 'Lalibela', 'Near Bete Giyorgis', 'Near Bete Giyorgis, Lalibela', ST_SetSRID(ST_MakePoint(39.0473, 12.0319), 4326)::geography, '+251 11 000 0000', 'A clean, well-run stay close to the main sights, with friendly staff and reliable hot water.', true, true, ARRAY['generator', 'wifi', 'pay_at_hotel']::text[], '["Check-in from 12:00, check-out until 11:00", "Valid ID required at check-in", "No smoking inside rooms"]'::jsonb, 'active'),
  ('a0000000-0000-0000-0000-000000000006', 'Fasil Castle View Hotel', 'hotel', 'Gondar', 'Fasil Ghebbi', 'Fasil Ghebbi, Gondar', ST_SetSRID(ST_MakePoint(37.4666, 12.6074), 4326)::geography, '+251 11 000 0000', 'A clean, well-run stay close to the main sights, with friendly staff and reliable hot water.', true, false, ARRAY['generator', 'hot_water', 'wifi', 'parking']::text[], '["Check-in from 12:00, check-out until 11:00", "Valid ID required at check-in", "No smoking inside rooms"]'::jsonb, 'active'),
  ('a0000000-0000-0000-0000-000000000007', 'Hawassa Lakeside Resort', 'resort', 'Hawassa', 'Lake Hawassa', 'Lake Hawassa, Hawassa', ST_SetSRID(ST_MakePoint(38.4763, 7.0621), 4326)::geography, '+251 11 000 0000', 'A clean, well-run stay close to the main sights, with friendly staff and reliable hot water.', true, true, ARRAY['generator', 'hot_water', 'wifi', 'parking', 'pay_at_hotel']::text[], '["Check-in from 12:00, check-out until 11:00", "Valid ID required at check-in", "No smoking inside rooms"]'::jsonb, 'active'),
  ('a0000000-0000-0000-0000-000000000008', 'Adama Crossroads Hotel', 'hotel', 'Adama', 'Adama centre', 'Adama centre, Adama', ST_SetSRID(ST_MakePoint(39.2689, 8.5414), 4326)::geography, '+251 11 000 0000', 'A clean, well-run stay close to the main sights, with friendly staff and reliable hot water.', true, false, ARRAY['hot_water', 'wifi', 'parking']::text[], '["Check-in from 12:00, check-out until 11:00", "Valid ID required at check-in", "No smoking inside rooms"]'::jsonb, 'active')
ON CONFLICT (id) DO NOTHING;

INSERT INTO room_types (id, property_id, name, capacity, amenities, total_rooms) VALUES
  ('b0000000-0000-0000-0000-000000000001', 'a0000000-0000-0000-0000-000000000001', 'Standard Double', 2, ARRAY['Private bathroom', 'TV']::text[], 5),
  ('b0000000-0000-0000-0000-000000000002', 'a0000000-0000-0000-0000-000000000001', 'Deluxe King', 2, ARRAY['Private bathroom', 'TV']::text[], 2),
  ('b0000000-0000-0000-0000-000000000003', 'a0000000-0000-0000-0000-000000000001', 'Family Suite', 4, ARRAY['Private bathroom', 'TV']::text[], 1),
  ('b0000000-0000-0000-0000-000000000004', 'a0000000-0000-0000-0000-000000000002', 'Single Room', 1, ARRAY['Private bathroom', 'TV']::text[], 4),
  ('b0000000-0000-0000-0000-000000000005', 'a0000000-0000-0000-0000-000000000002', 'Twin Room', 2, ARRAY['Private bathroom', 'TV']::text[], 3),
  ('b0000000-0000-0000-0000-000000000006', 'a0000000-0000-0000-0000-000000000003', 'Standard Room', 2, ARRAY['Private bathroom', 'TV']::text[], 0),
  ('b0000000-0000-0000-0000-000000000007', 'a0000000-0000-0000-0000-000000000004', 'Garden Room', 2, ARRAY['Private bathroom', 'TV']::text[], 6),
  ('b0000000-0000-0000-0000-000000000008', 'a0000000-0000-0000-0000-000000000004', 'Lake View Room', 2, ARRAY['Private bathroom', 'TV']::text[], 3),
  ('b0000000-0000-0000-0000-000000000009', 'a0000000-0000-0000-0000-000000000004', 'Family Chalet', 5, ARRAY['Private bathroom', 'TV']::text[], 2),
  ('b0000000-0000-0000-0000-000000000010', 'a0000000-0000-0000-0000-000000000005', 'Standard Room', 2, ARRAY['Private bathroom', 'TV']::text[], 4),
  ('b0000000-0000-0000-0000-000000000011', 'a0000000-0000-0000-0000-000000000005', 'Triple Room', 3, ARRAY['Private bathroom', 'TV']::text[], 2),
  ('b0000000-0000-0000-0000-000000000012', 'a0000000-0000-0000-0000-000000000006', 'Standard Double', 2, ARRAY['Private bathroom', 'TV']::text[], 7),
  ('b0000000-0000-0000-0000-000000000013', 'a0000000-0000-0000-0000-000000000006', 'Executive Room', 2, ARRAY['Private bathroom', 'TV']::text[], 2),
  ('b0000000-0000-0000-0000-000000000014', 'a0000000-0000-0000-0000-000000000007', 'Lake View Room', 2, ARRAY['Private bathroom', 'TV']::text[], 5),
  ('b0000000-0000-0000-0000-000000000015', 'a0000000-0000-0000-0000-000000000007', 'Bungalow', 4, ARRAY['Private bathroom', 'TV']::text[], 3),
  ('b0000000-0000-0000-0000-000000000016', 'a0000000-0000-0000-0000-000000000008', 'Standard Room', 2, ARRAY['Private bathroom', 'TV']::text[], 6)
ON CONFLICT (id) DO NOTHING;

-- One year of rates starting today.
INSERT INTO rate_plans (room_type_id, currency, nightly_price_santim, valid_from, valid_to)
SELECT v.room_type_id::uuid, v.currency, v.price, current_date, current_date + 365
FROM (VALUES
  ('b0000000-0000-0000-0000-000000000001', 'ETB', 320000),
  ('b0000000-0000-0000-0000-000000000002', 'ETB', 480000),
  ('b0000000-0000-0000-0000-000000000003', 'ETB', 720000),
  ('b0000000-0000-0000-0000-000000000004', 'ETB', 140000),
  ('b0000000-0000-0000-0000-000000000005', 'ETB', 210000),
  ('b0000000-0000-0000-0000-000000000006', 'ETB', 180000),
  ('b0000000-0000-0000-0000-000000000007', 'ETB', 290000),
  ('b0000000-0000-0000-0000-000000000008', 'ETB', 390000),
  ('b0000000-0000-0000-0000-000000000009', 'ETB', 610000),
  ('b0000000-0000-0000-0000-000000000010', 'ETB', 170000),
  ('b0000000-0000-0000-0000-000000000011', 'ETB', 240000),
  ('b0000000-0000-0000-0000-000000000012', 'ETB', 230000),
  ('b0000000-0000-0000-0000-000000000013', 'ETB', 330000),
  ('b0000000-0000-0000-0000-000000000014', 'ETB', 420000),
  ('b0000000-0000-0000-0000-000000000015', 'ETB', 680000),
  ('b0000000-0000-0000-0000-000000000016', 'ETB', 160000)
) AS v(room_type_id, currency, price)
WHERE NOT EXISTS (SELECT 1 FROM rate_plans r WHERE r.room_type_id = v.room_type_id::uuid);

-- Physical rooms: 101, 102, ... for each room type.
INSERT INTO rooms (property_id, room_type_id, number)
SELECT rt.property_id, rt.id,
       ((row_number() OVER (PARTITION BY rt.property_id ORDER BY rt.name, g.n)) + 100)::text
FROM room_types rt
CROSS JOIN LATERAL generate_series(1, rt.total_rooms) AS g(n)
ON CONFLICT (property_id, number) DO NOTHING;

-- Availability for the next 180 nights.
INSERT INTO inventory (room_type_id, night, capacity)
SELECT rt.id, d::date, rt.total_rooms
FROM room_types rt
CROSS JOIN generate_series(current_date, current_date + 179, interval '1 day') AS d
ON CONFLICT (room_type_id, night) DO NOTHING;

INSERT INTO landmarks (name, name_am, city, location)
SELECT v.name, v.name_am, v.city, ST_SetSRID(ST_MakePoint(v.lng, v.lat), 4326)::geography
FROM (VALUES
  ('Meskel Square', 'መስቀል አደባባይ', 'Addis Ababa', 9.0108::float8, 38.7613::float8),
  ('Bole Airport', 'ቦሌ አየር ማረፊያ', 'Addis Ababa', 8.9779::float8, 38.7993::float8),
  ('Piassa', 'ፒያሳ', 'Addis Ababa', 9.0346::float8, 38.7469::float8),
  ('Lake Tana', 'ጣና ሐይቅ', 'Bahir Dar', 11.6::float8, 37.39::float8),
  ('Bete Giyorgis', 'ቤተ ጊዮርጊስ', 'Lalibela', 12.0319::float8, 39.0473::float8),
  ('Fasil Ghebbi', 'ፋሲል ግቢ', 'Gondar', 12.6074::float8, 37.4666::float8)
) AS v(name, name_am, city, lat, lng)
WHERE NOT EXISTS (SELECT 1 FROM landmarks x WHERE x.name = v.name);

-- A demo hotel owner for the first hotel, so the partner app has someone to sign in as once real sign-in exists.
INSERT INTO users (id, email, display_name, role) VALUES ('c0000000-0000-0000-0000-000000000001', 'owner@bookdan.test', 'Demo Owner', 'owner')
ON CONFLICT (id) DO NOTHING;
-- The demo owner works for every demo hotel, so whichever hotel a tester books, the hotel app receives the request.
INSERT INTO property_staff (property_id, user_id, role)
SELECT p.id, 'c0000000-0000-0000-0000-000000000001', 'owner' FROM properties p
ON CONFLICT DO NOTHING;

COMMIT;
