import { ApiError } from '@bookdan/api-client';

/** Translation key for why a check-in lookup or confirmation failed. */
export function checkinErrorKey(e: unknown): string {
  if (e instanceof ApiError) {
    const known = ['INVALID_VOUCHER', 'VOUCHER_EXPIRED', 'WRONG_HOTEL', 'INVALID_PIN', 'PIN_LOCKED', 'NOT_FOUND', 'INVALID_STATE', 'NETWORK'];
    if (known.includes(e.code)) return `partner.checkin_err_${e.code}`;
  }
  return 'partner.checkin_err_generic';
}
