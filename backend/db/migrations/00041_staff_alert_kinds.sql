-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Which optional alerts a person at a hotel does not want on their phone, chosen by that person. New booking requests (and their reminders) are not optional:
-- they are controlled by the person's alerts switch and shift. The optional kinds are changes (a guest cancelled or moved their dates), disputes (a guest
-- opened one) and reviews (a guest reviewed the hotel; owners only).
ALTER TABLE property_staff
  ADD COLUMN muted_kinds text[] NOT NULL DEFAULT '{}',
  ADD CONSTRAINT property_staff_muted_kinds_known CHECK (muted_kinds <@ ARRAY['changes', 'disputes', 'reviews']::text[]);

-- +goose Down
ALTER TABLE app.property_staff DROP CONSTRAINT IF EXISTS property_staff_muted_kinds_known, DROP COLUMN IF EXISTS muted_kinds;
