-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- A guest asks for help; the platform's support answers. One ticket is one conversation. A ticket can be about a booking.
-- status: open = waiting for support, answered = waiting for the guest, closed = finished (the guest can open a new one).
-- kind: question and problem are help requests; dispute is a guest challenging a charge or a stay (see disputes in support.go).
CREATE TABLE support_tickets (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id    uuid NOT NULL REFERENCES users (id),
  booking_id uuid REFERENCES bookings (id),
  kind       text NOT NULL CHECK (kind IN ('question', 'problem', 'dispute')),
  subject    text NOT NULL CHECK (length(subject) BETWEEN 3 AND 120),
  status     text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'answered', 'closed')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  closed_at  timestamptz,
  closed_by  text,                                  -- 'guest' or the staff member's email
  CONSTRAINT closed_together CHECK ((status = 'closed') = (closed_at IS NOT NULL))
);
CREATE INDEX support_tickets_user_idx ON support_tickets (user_id, updated_at DESC);
CREATE INDEX support_tickets_queue_idx ON support_tickets (updated_at) WHERE status <> 'closed';
CREATE INDEX support_tickets_booking_idx ON support_tickets (booking_id) WHERE booking_id IS NOT NULL;

CREATE TABLE support_messages (
  id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  ticket_id  uuid NOT NULL REFERENCES support_tickets (id),
  from_staff boolean NOT NULL,
  author     text NOT NULL,                         -- 'guest' or the staff member's email
  body       text NOT NULL CHECK (length(body) <= 2000),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX support_messages_ticket_idx ON support_messages (ticket_id, created_at);

ALTER TABLE support_tickets ENABLE ROW LEVEL SECURITY;
ALTER TABLE support_messages ENABLE ROW LEVEL SECURITY;

-- +goose Down
DROP TABLE IF EXISTS app.support_messages;
DROP TABLE IF EXISTS app.support_tickets;
