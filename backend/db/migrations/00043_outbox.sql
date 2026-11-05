-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Notifications (phone alerts and emails) are written here first and then sent, so a failed send is tried again with growing pauses instead of being lost, and a
-- server that stops in the middle of a send does not lose the message. The text of a message is erased as soon as it is delivered (it can hold a guest's name or an
-- address); the row stays for a week for counting and for finding problems.
CREATE TABLE outbox (
  id              bigserial PRIMARY KEY,
  kind            text NOT NULL CHECK (kind IN ('push', 'email')),
  payload         jsonb NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  -- A new-booking alert is useless once the request has run out, so each message says how long it is worth sending.
  expires_at      timestamptz NOT NULL,
  attempts        integer NOT NULL DEFAULT 0,
  next_attempt_at timestamptz NOT NULL DEFAULT now(),
  sent_at         timestamptz,
  failed_at       timestamptz,
  last_error      text
);
CREATE INDEX outbox_due_idx ON outbox (next_attempt_at) WHERE sent_at IS NULL AND failed_at IS NULL;
CREATE INDEX outbox_created_idx ON outbox (created_at);
ALTER TABLE outbox ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.outbox;
