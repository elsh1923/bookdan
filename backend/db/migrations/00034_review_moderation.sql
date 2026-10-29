-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Taking down an abusive review. The row stays (an admin can restore it and the audit trail keeps the reason), but a hidden review is not shown to
-- anyone but its author, is not counted in the hotel's rating, and cannot be answered.
ALTER TABLE reviews ADD COLUMN hidden_at timestamptz;
ALTER TABLE reviews ADD COLUMN hidden_by uuid REFERENCES users (id);
ALTER TABLE reviews ADD COLUMN hidden_reason text CHECK (length(hidden_reason) BETWEEN 3 AND 300);
ALTER TABLE reviews ADD CONSTRAINT review_hidden_together CHECK ((hidden_at IS NULL) = (hidden_reason IS NULL) AND (hidden_at IS NULL) = (hidden_by IS NULL));
CREATE INDEX reviews_hidden_idx ON reviews (hidden_at DESC) WHERE hidden_at IS NOT NULL;

CREATE OR REPLACE VIEW property_ratings AS
  SELECT property_id, round(avg(rating)::numeric, 1) AS rating, count(*)::integer AS review_count
  FROM reviews WHERE hidden_at IS NULL GROUP BY property_id;

-- +goose Down
CREATE OR REPLACE VIEW app.property_ratings AS
  SELECT property_id, round(avg(rating)::numeric, 1) AS rating, count(*)::integer AS review_count
  FROM app.reviews GROUP BY property_id;
DROP INDEX IF EXISTS app.reviews_hidden_idx;
ALTER TABLE app.reviews DROP CONSTRAINT IF EXISTS review_hidden_together;
ALTER TABLE app.reviews DROP COLUMN IF EXISTS hidden_reason, DROP COLUMN IF EXISTS hidden_by, DROP COLUMN IF EXISTS hidden_at;
