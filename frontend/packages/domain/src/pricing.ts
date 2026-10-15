// Stay pricing in integer santim. The server is the source of truth for the final
// quote; this is used by the mock and to sanity-check what the server returns.

export const VAT_PERCENT = 15;

export interface StayPrice {
  subtotal_santim: number;
  vat_santim: number;
  total_santim: number;
}

export function priceStay(nightlySantim: number, nights: number): StayPrice {
  const subtotal = nightlySantim * nights;
  // Round half up on whole santim; never floats in storage.
  const vat = Math.round((subtotal * VAT_PERCENT) / 100);
  return { subtotal_santim: subtotal, vat_santim: vat, total_santim: subtotal + vat };
}

/** mm:ss for the hold countdown. */
export function formatCountdown(ms: number): string {
  const s = Math.ceil(Math.max(0, ms) / 1000);
  const mm = String(Math.floor(s / 60)).padStart(2, '0');
  const ss = String(s % 60).padStart(2, '0');
  return `${mm}:${ss}`;
}

/** Ethiopian mobile numbers: +2519XXXXXXXX or +2517XXXXXXXX. Accepts 09…/07…/9…/+251… input. */
export function normalizeEthiopianPhone(input: string): string | null {
  const digits = input.replace(/[\s()-]/g, '');
  const m = digits.match(/^(?:\+?251|0)?([79]\d{8})$/);
  return m ? `+251${m[1]}` : null;
}
