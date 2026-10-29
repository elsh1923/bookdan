export type Currency = 'ETB' | 'USD' | 'EUR';

export interface Money {
  amount_santim: number;
  currency: Currency;
}

/** Money is always an integer in santim (1 ETB = 100 santim). Never floats. */
export function etb(birr: number): Money {
  return { amount_santim: Math.round(birr * 100), currency: 'ETB' };
}

const SYMBOL: Record<Currency, string> = { ETB: 'ETB', USD: '$', EUR: '€' };

/** Display only. Groups thousands by hand so output is identical on every JS engine. */
export function formatMoney(m: Money): string {
  const negative = m.amount_santim < 0;
  const abs = Math.abs(m.amount_santim);
  const whole = Math.floor(abs / 100);
  const cents = abs % 100;
  const grouped = String(whole).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  const body = cents === 0 ? grouped : `${grouped}.${String(cents).padStart(2, '0')}`;
  const text = m.currency === 'ETB' ? `${body} ETB` : `${SYMBOL[m.currency]}${body}`;
  return negative ? `-${text}` : text;
}

/**
 * Display currencies (see currencies.ts). BookDan charges and refunds in ETB only; another currency is an approximate amount shown next to the birr price,
 * computed with a rate the server provides (never a rate written in the app).
 */

/** An ETB amount in santim as an amount of the display currency in cents, rounded half up. birrPerUnit is ETB for one dollar or euro. */
export function convertEtb(santim: number, birrPerUnit: number): number {
  return Math.round(santim / birrPerUnit);
}
