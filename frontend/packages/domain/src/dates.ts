// Stay nights are plain dates (YYYY-MM-DD) in Africa/Addis_Ababa (UTC+3, no DST).
// These helpers do pure calendar arithmetic and never read the phone timezone.

const MS_PER_DAY = 86_400_000;

function toUtcMs(iso: string): number {
  const [y, m, d] = iso.split('-').map(Number);
  return Date.UTC(y, m - 1, d);
}

function fromUtcMs(ms: number): string {
  const dt = new Date(ms);
  const y = dt.getUTCFullYear();
  const m = String(dt.getUTCMonth() + 1).padStart(2, '0');
  const d = String(dt.getUTCDate()).padStart(2, '0');
  return `${y}-${m}-${d}`;
}

export function addDays(iso: string, days: number): string {
  return fromUtcMs(toUtcMs(iso) + days * MS_PER_DAY);
}

export function nightsBetween(checkIn: string, checkOut: string): number {
  return Math.round((toUtcMs(checkOut) - toUtcMs(checkIn)) / MS_PER_DAY);
}

/** Today in Addis Ababa from a UTC instant (pass server time when you have it). */
export function todayAddis(nowMs: number = Date.now()): string {
  return fromUtcMs(nowMs + 3 * 3_600_000);
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

export function formatShortDate(iso: string): string {
  const [, m, d] = iso.split('-').map(Number);
  return `${MONTHS[m - 1]} ${d}`;
}

/** Hold countdown from server timestamps only, never the phone clock. */
export function holdRemainingMs(
  holdExpiresAtIso: string,
  serverTimeIso: string,
  elapsedSinceResponseMs: number,
): number {
  const remaining = Date.parse(holdExpiresAtIso) - Date.parse(serverTimeIso) - elapsedSinceResponseMs;
  return Math.max(0, remaining);
}
