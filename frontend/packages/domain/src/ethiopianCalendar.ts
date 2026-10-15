// Gregorian -> Ethiopian calendar conversion via Julian Day Number.
// Display only: the API always speaks Gregorian ISO dates.

const ETHIOPIC_EPOCH_JDN = 1723856;

export interface EthiopianDate {
  year: number;
  month: number; // 1..13 (13 = Pagume)
  day: number; // 1..30 (Pagume: 1..5, or 6 in a leap year)
}

export const ETHIOPIAN_MONTHS_AM = [
  'መስከረም', 'ጥቅምት', 'ኅዳር', 'ታኅሣሥ', 'ጥር', 'የካቲት', 'መጋቢት',
  'ሚያዝያ', 'ግንቦት', 'ሰኔ', 'ሐምሌ', 'ነሐሴ', 'ጳጉሜን',
];

export const ETHIOPIAN_MONTHS_EN = [
  'Meskerem', 'Tikimt', 'Hidar', 'Tahsas', 'Tir', 'Yekatit', 'Megabit',
  'Miazia', 'Ginbot', 'Sene', 'Hamle', 'Nehase', 'Pagume',
];

function gregorianToJdn(y: number, m: number, d: number): number {
  const a = Math.floor((14 - m) / 12);
  const yy = y + 4800 - a;
  const mm = m + 12 * a - 3;
  return (
    d +
    Math.floor((153 * mm + 2) / 5) +
    365 * yy +
    Math.floor(yy / 4) -
    Math.floor(yy / 100) +
    Math.floor(yy / 400) -
    32045
  );
}

function jdnToEthiopian(jdn: number): EthiopianDate {
  const r = (jdn - ETHIOPIC_EPOCH_JDN) % 1461;
  const n = (r % 365) + 365 * Math.floor(r / 1460);
  const year =
    4 * Math.floor((jdn - ETHIOPIC_EPOCH_JDN) / 1461) +
    Math.floor(r / 365) -
    Math.floor(r / 1460);
  return { year, month: Math.floor(n / 30) + 1, day: (n % 30) + 1 };
}

export function gregorianToEthiopian(year: number, month: number, day: number): EthiopianDate {
  return jdnToEthiopian(gregorianToJdn(year, month, day));
}

/** Accepts a plain stay date, YYYY-MM-DD. */
export function isoToEthiopian(iso: string): EthiopianDate {
  const [y, m, d] = iso.split('-').map(Number);
  return gregorianToEthiopian(y, m, d);
}

export function formatEthiopian(e: EthiopianDate, lang: 'en' | 'am' = 'am'): string {
  const names = lang === 'am' ? ETHIOPIAN_MONTHS_AM : ETHIOPIAN_MONTHS_EN;
  return `${names[e.month - 1]} ${e.day}, ${e.year}`;
}
