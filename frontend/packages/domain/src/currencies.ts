// The currencies a guest can choose to see prices in. BookDan always charges and refunds in ETB; these only turn an ETB price into an approximate amount on
// screen, with a rate the server provides (a currency without a fresh rate is not offered). The server carries the same list (backend/internal/api/fxrates.go:
// displayCurrencyList): keep them in step. The first nine are the popular ones (the Gulf and the usual travel currencies).

export const DISPLAY_CURRENCY_CODES = [
  'AED', 'SAR', 'QAR', 'KWD', 'OMR', 'BHD', 'USD', 'GBP', 'EUR',
  'AUD', 'BRL', 'CAD', 'CHF', 'CNY', 'DJF', 'DKK', 'EGP', 'ILS', 'INR', 'JPY', 'KES', 'MYR', 'NOK', 'NZD', 'SEK', 'SGD', 'TRY', 'TZS', 'UGX', 'ZAR',
] as const;

export type DisplayCurrency = (typeof DISPLAY_CURRENCY_CODES)[number];

/** What the guest sees prices in: birr, or one of the display currencies. */
export type ShownCurrency = 'ETB' | DisplayCurrency;

export interface CurrencyInfo {
  code: DisplayCurrency;
  name: string;
  /** Written before the amount; a currency without a well-known sign is written with its code. */
  symbol: string;
  /** 0 for a currency whose amounts are written without a fraction (yen, shillings of East Africa). */
  decimals: 0 | 2;
  popular: boolean;
}

const NAMES: Record<DisplayCurrency, [string, string, 0 | 2]> = {
  AED: ['United Arab Emirates Dirham', 'AED ', 2],
  SAR: ['Saudi Riyal', 'SAR ', 2],
  QAR: ['Qatari Riyal', 'QAR ', 2],
  KWD: ['Kuwaiti Dinar', 'KWD ', 2],
  OMR: ['Omani Rial', 'OMR ', 2],
  BHD: ['Bahraini Dinar', 'BHD ', 2],
  USD: ['US Dollar', '$', 2],
  GBP: ['Great Britain Pound', '£', 2],
  EUR: ['Euro', '€', 2],
  AUD: ['Australian Dollar', 'AUD ', 2],
  BRL: ['Brazilian Real', 'BRL ', 2],
  CAD: ['Canadian Dollar', 'CAD ', 2],
  CHF: ['Swiss Franc', 'CHF ', 2],
  CNY: ['Chinese Yuan', 'CNY ', 2],
  DJF: ['Djiboutian Franc', 'DJF ', 0],
  DKK: ['Danish Krone', 'DKK ', 2],
  EGP: ['Egyptian Pound', 'EGP ', 2],
  ILS: ['Israeli Shekel', 'ILS ', 2],
  INR: ['Indian Rupee', '₹', 2],
  JPY: ['Japanese Yen', 'JPY ', 0],
  KES: ['Kenyan Shilling', 'KES ', 2],
  MYR: ['Malaysian Ringgit', 'MYR ', 2],
  NOK: ['Norwegian Krone', 'NOK ', 2],
  NZD: ['New Zealand Dollar', 'NZD ', 2],
  SEK: ['Swedish Krona', 'SEK ', 2],
  SGD: ['Singapore Dollar', 'SGD ', 2],
  TRY: ['Turkish Lira', 'TRY ', 2],
  TZS: ['Tanzanian Shilling', 'TZS ', 0],
  UGX: ['Ugandan Shilling', 'UGX ', 0],
  ZAR: ['South African Rand', 'ZAR ', 2],
};

const POPULAR = 9;

export const DISPLAY_CURRENCIES: readonly CurrencyInfo[] = DISPLAY_CURRENCY_CODES.map((code, i) => ({
  code, name: NAMES[code][0], symbol: NAMES[code][1], decimals: NAMES[code][2], popular: i < POPULAR,
}));

export function isDisplayCurrency(code: string): code is DisplayCurrency {
  return (DISPLAY_CURRENCY_CODES as readonly string[]).includes(code);
}

export function currencyInfo(code: DisplayCurrency): CurrencyInfo {
  return DISPLAY_CURRENCIES.find((c) => c.code === code) as CurrencyInfo;
}

/**
 * An amount in hundredths of a display currency (what convertEtb returns) as text: "$12.50", "€80", "JPY 1,240". Groups thousands by hand so the output is
 * the same on every JavaScript engine, drops ".00", and shows no fraction for the currencies written without one.
 */
export function formatDisplayAmount(hundredths: number, code: DisplayCurrency): string {
  const info = currencyInfo(code);
  const negative = hundredths < 0;
  const abs = Math.abs(hundredths);
  let whole = Math.floor(abs / 100);
  let cents = abs % 100;
  if (info.decimals === 0) {
    whole = Math.round(abs / 100);
    cents = 0;
  }
  const grouped = String(whole).replace(/\B(?=(\d{3})+(?!\d))/g, ',');
  const body = cents === 0 ? grouped : `${grouped}.${String(cents).padStart(2, '0')}`;
  const text = `${info.symbol}${body}`;
  return negative ? `-${text}` : text;
}
