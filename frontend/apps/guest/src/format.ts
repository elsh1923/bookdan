import { convertEtb, formatEthiopian, formatMoney, isoToEthiopian, type Currency } from '@bookdan/domain';
import type { Language } from '@bookdan/i18n';
import { useDisplay } from './display';

/** The approximate amount in the guest's display currency, or null when they see ETB (or there is no fresh rate). */
function approx(santim: number, currency: Currency): string | null {
  const d = useDisplay.getState();
  if (currency !== 'ETB' || d.currency === 'ETB' || !d.birrPerUnit) return null;
  return formatMoney({ amount_santim: convertEtb(santim, d.birrPerUnit), currency: d.currency });
}

/** The exact amount in the currency it is charged in. Never converted. */
export function etbOnly(santim: number, currency: Currency = 'ETB'): string {
  return formatMoney({ amount_santim: santim, currency });
}

/**
 * A price for browsing (search, hotel and room prices): in the guest's display currency, with a "≈" when it is converted from ETB,
 * using the rate the server provides. Without a fresh rate it is the ETB price.
 */
export function price(santim: number, currency: Currency = 'ETB'): string {
  const a = approx(santim, currency);
  return a ? `≈ ${a}` : etbOnly(santim, currency);
}

/**
 * An amount that is charged or refunded: always the exact ETB amount, with the approximate display amount after it when the guest
 * is viewing another currency, so what leaves their account is never in doubt.
 */
export function priceExact(santim: number, currency: Currency = 'ETB'): string {
  const a = approx(santim, currency);
  return a ? `${etbOnly(santim, currency)} (≈ ${a})` : etbOnly(santim, currency);
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
