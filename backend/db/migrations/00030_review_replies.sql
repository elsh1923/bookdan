-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A hotel's public reply to a guest's review: one per review, written by a hotel owner, shown under the review in the guest app.
ALTER TABLE reviews ADD COLUMN hotel_reply text CHECK (length(hotel_reply) BETWEEN 3 AND 1000);
ALTER TABLE reviews ADD COLUMN hotel_reply_at timestamptz;
ALTER TABLE reviews ADD COLUMN hotel_reply_by uuid REFERENCES users (id);
ALTER TABLE reviews ADD CONSTRAINT hotel_reply_together CHECK ((hotel_reply IS NULL) = (hotel_reply_at IS NULL) AND (hotel_reply IS NULL) = (hotel_reply_by IS NULL));

-- +goose Down
ALTER TABLE app.reviews DROP CONSTRAINT IF EXISTS hotel_reply_together;
ALTER TABLE app.reviews DROP COLUMN IF EXISTS hotel_reply_by;
ALTER TABLE app.reviews DROP COLUMN IF EXISTS hotel_reply_at;
ALTER TABLE app.reviews DROP COLUMN IF EXISTS hotel_reply;
