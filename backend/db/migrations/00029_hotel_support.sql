-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Hotel staff can write to BookDan support from the partner app. Their requests are the same tickets as guests' (support_tickets), marked with the hotel
-- they are about, so the admin page shows which hotel is asking.
ALTER TABLE support_tickets ADD COLUMN property_id uuid REFERENCES properties (id);
CREATE INDEX support_tickets_property_idx ON support_tickets (property_id) WHERE property_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS app.support_tickets_property_idx;
ALTER TABLE app.support_tickets DROP COLUMN IF EXISTS property_id;
