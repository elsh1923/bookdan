import { formatEthiopian, formatMoney, isoToEthiopian, type Currency } from '@bookdan/domain';
import type { Language } from '@bookdan/i18n';

// Prices are shown as stored. USD/EUR display (M) will use server-provided rates,
// never a rate hard-coded in the app, so only ETB renders for now.
export function price(santim: number, currency: Currency = 'ETB'): string {
  return formatMoney({ amount_santim: santim, currency });
}

export function shortDate(iso: string, months: string[]): string {
  const [, m, d] = iso.split('-').map(Number);
  return `${months[m - 1]} ${d}`;
}

/** Ethiopian-calendar line, shown when the language is Amharic. */
export function ethiopianLine(iso: string, lang: Language): string | null {
  if (lang !== 'am') return null;
  return formatEthiopian(isoToEthiopian(iso), 'am');
}
