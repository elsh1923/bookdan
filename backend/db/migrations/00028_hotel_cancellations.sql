-- +goose Up
SET LOCAL search_path = app, extensions, public;

-- Who ended a booking, and why. A hotel can cancel a confirmed booking (the guest is refunded in full and the hotel bears it), or report that
-- it cannot host the guest (NEEDS_RELOCATION: support finds another hotel, the guest can cancel for a full refund, or the booking is restored).
ALTER TABLE bookings ADD COLUMN cancelled_by text CHECK (cancelled_by IN ('guest', 'hotel', 'admin'));
ALTER TABLE bookings ADD COLUMN cancel_reason text;
-- Bookings cancelled before this column existed were cancelled by their guests.
UPDATE bookings SET cancelled_by = 'guest' WHERE status = 'CANCELLED_REFUNDED' AND cancelled_at IS NOT NULL AND cancelled_by IS NULL;

-- A relocation is a support request opened for the guest by the system.
ALTER TABLE support_tickets DROP CONSTRAINT IF EXISTS support_tickets_kind_check;
ALTER TABLE support_tickets ADD CONSTRAINT support_tickets_kind_check CHECK (kind IN ('question', 'problem', 'dispute', 'relocation'));
ALTER TABLE support_tickets DROP CONSTRAINT IF EXISTS support_tickets_outcome_check;
ALTER TABLE support_tickets ADD CONSTRAINT support_tickets_outcome_check
  CHECK (outcome IN ('rejected', 'refunded_hotel_pays', 'refunded_platform_pays', 'refunded_partial_hotel_pays', 'refunded_partial_platform_pays', 'withdrawn',
                     'relocation_restored', 'relocation_refunded', 'relocation_cancelled'));
-- At most one open relocation per booking.
CREATE UNIQUE INDEX support_one_open_relocation ON support_tickets (booking_id) WHERE kind = 'relocation' AND status <> 'closed';

-- +goose Down
DROP INDEX IF EXISTS app.support_one_open_relocation;
ALTER TABLE app.support_tickets DROP CONSTRAINT IF EXISTS support_tickets_outcome_check;
ALTER TABLE app.support_tickets ADD CONSTRAINT support_tickets_outcome_check
  CHECK (outcome IN ('rejected', 'refunded_hotel_pays', 'refunded_platform_pays', 'refunded_partial_hotel_pays', 'refunded_partial_platform_pays', 'withdrawn'));
ALTER TABLE app.support_tickets DROP CONSTRAINT IF EXISTS support_tickets_kind_check;
ALTER TABLE app.support_tickets ADD CONSTRAINT support_tickets_kind_check CHECK (kind IN ('question', 'problem', 'dispute'));
ALTER TABLE app.bookings DROP COLUMN IF EXISTS cancel_reason;
ALTER TABLE app.bookings DROP COLUMN IF EXISTS cancelled_by;
