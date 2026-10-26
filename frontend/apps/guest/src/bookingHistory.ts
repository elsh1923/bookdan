import type { Booking } from '@bookdan/domain';

// Stays that are still to come or in progress. Anything else (finished, cancelled, declined, no-show) is history.
const ACTIVE = new Set(['HELD', 'PAID_AWAITING_HOTEL', 'CONFIRMED', 'CHECKED_IN', 'NEEDS_RELOCATION', 'DISPUTED']);

/** Splits the guest's bookings into the stays still to come and the history: upcoming by check-in (soonest first), past newest first. */
export function splitBookings(items: Booking[], today: string) {
  const upcoming = items.filter((b) => ACTIVE.has(b.status) && b.check_out >= today);
  const past = items.filter((b) => !upcoming.includes(b));
  const byCheckIn = (a: Booking, b: Booking) => (a.check_in < b.check_in ? -1 : a.check_in > b.check_in ? 1 : 0);
  return { upcoming: upcoming.sort(byCheckIn), past: past.sort((a, b) => byCheckIn(b, a)) };
}
