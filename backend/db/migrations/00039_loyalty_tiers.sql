-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Member tiers: a guest moves up when they have finished stays at this many different hotels. The discount is taken off the
-- room price at checkout (BookDan pays it, like a coupon; the hotel still receives its full share). Editable on the admin page.
CREATE TABLE loyalty_tiers (
  name          text PRIMARY KEY CHECK (name ~ '^[a-z]{3,12}$'),
  min_hotels    integer NOT NULL UNIQUE CHECK (min_hotels >= 1),
  discount_bps  integer NOT NULL CHECK (discount_bps BETWEEN 0 AND 3000),
  sort_order    integer NOT NULL UNIQUE
);
INSERT INTO loyalty_tiers (name, min_hotels, discount_bps, sort_order) VALUES
  ('bronze', 1, 300, 1),
  ('silver', 3, 500, 2),
  ('gold',   6, 800, 3);
ALTER TABLE loyalty_tiers ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.loyalty_tiers;
