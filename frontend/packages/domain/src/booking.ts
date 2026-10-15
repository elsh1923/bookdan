import type { Booking } from './schemas';

type StateOf = Pick<Booking, 'status' | 'payment'>;

/** The guest has started paying and the result has not come back yet. */
export function isPaymentInFlight(b: StateOf): boolean {
  return b.status === 'HELD' && b.payment === 'pending';
}

/**
 * After the app was closed mid-payment, should we take the guest back to this booking on the next launch?
 * Yes while the payment is unresolved or the hotel has not answered yet. Anything settled stays quiet.
 */
export function shouldResumeOnLaunch(b: StateOf): boolean {
  return isPaymentInFlight(b) || b.status === 'PAID_AWAITING_HOTEL';
}

/** Statuses the booking screen keeps polling on: it can still change without the guest doing anything. */
export function isStillChanging(b: StateOf): boolean {
  return b.status === 'HELD' || b.status === 'PAID_AWAITING_HOTEL';
}
