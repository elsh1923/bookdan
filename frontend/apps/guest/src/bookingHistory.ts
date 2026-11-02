import type { Booking } from '@bookdan/domain';

// Stays that are still to come or in progress.
const ACTIVE = new Set(['HELD', 'PAID_AWAITING_HOTEL', 'CONFIRMED', 'CHECKED_IN', 'NEEDS_RELOCATION', 'DISPUTED']);
// Bookings that were cancelled by the guest or the hotel (the money was returned).
const CANCELLED = new Set(['CANCELLED_REFUNDED']);
// Bookings that never went through: the payment or hold ran out, or the hotel declined (the money was returned).
const FAILED = new Set(['EXPIRED', 'DECLINED_REFUNDED']);

/**
 * Sorts the guest's bookings into four groups: upcoming (soonest check-in first), past (finished or no-show, newest first),
 * cancelled and failed (newest first).
 */
export function splitBookings(items: Booking[], today: string) {
  const byCheckIn = (a: Booking, b: Booking) => (a.check_in < b.check_in ? -1 : a.check_in > b.check_in ? 1 : 0);
  const newestFirst = (a: Booking, b: Booking) => byCheckIn(b, a);
  const upcoming = items.filter((b) => ACTIVE.has(b.status) && b.check_out >= today).sort(byCheckIn);
  const cancelled = items.filter((b) => CANCELLED.has(b.status)).sort(newestFirst);
  const failed = items.filter((b) => FAILED.has(b.status)).sort(newestFirst);
  const shown = new Set([...upcoming, ...cancelled, ...failed]);
  const past = items.filter((b) => !shown.has(b)).sort(newestFirst);
  return { upcoming, past, cancelled, failed };
}
