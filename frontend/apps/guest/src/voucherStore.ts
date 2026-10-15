import { VoucherSchema, todayAddis, type Booking, type Voucher } from '@bookdan/domain';
import { api } from './api';
import { secureStorage } from './secureStorage';

// Vouchers live only in secure storage on this phone. The voucher screen reads from here and never
// from the network, so it opens in airplane mode.
const INDEX_KEY = 'voucher-index';
const itemKey = (bookingId: string) => `voucher-${bookingId.replace(/[^A-Za-z0-9._-]/g, '_')}`;

async function readIndex(): Promise<string[]> {
  try {
    const raw = await secureStorage.getItem(INDEX_KEY);
    const parsed = raw ? JSON.parse(raw) : [];
    return Array.isArray(parsed) ? parsed.filter((x): x is string => typeof x === 'string') : [];
  } catch {
    return [];
  }
}

const writeIndex = (ids: string[]) => secureStorage.setItem(INDEX_KEY, JSON.stringify([...new Set(ids)]));

export async function saveVoucher(v: Voucher): Promise<void> {
  await secureStorage.setItem(itemKey(v.booking_id), JSON.stringify(v));
  await writeIndex([...(await readIndex()), v.booking_id]);
}

/** Returns null when nothing is stored or the stored copy is damaged (never throws). */
export async function loadVoucher(bookingId: string): Promise<Voucher | null> {
  try {
    const raw = await secureStorage.getItem(itemKey(bookingId));
    if (!raw) return null;
    const parsed = VoucherSchema.safeParse(JSON.parse(raw));
    return parsed.success ? parsed.data : null;
  } catch {
    return null;
  }
}

export async function removeVoucher(bookingId: string): Promise<void> {
  await secureStorage.removeItem(itemKey(bookingId));
  await writeIndex((await readIndex()).filter((id) => id !== bookingId));
}

/** Sign-out: nothing of the guest's may stay on the phone. */
export async function clearAllVouchers(): Promise<void> {
  for (const id of await readIndex()) await secureStorage.removeItem(itemKey(id));
  await secureStorage.removeItem(INDEX_KEY);
}

const ACTIVE = ['CONFIRMED', 'CHECKED_IN'];

/** Spec: kept until the stay ends, then removed. Called whenever the booking list is fresh. */
export async function pruneVouchers(bookings: Booking[]): Promise<void> {
  const today = todayAddis();
  const keep = new Set(bookings.filter((b) => ACTIVE.includes(b.status) && b.check_out >= today).map((b) => b.id));
  for (const id of await readIndex()) if (!keep.has(id)) await removeVoucher(id);
}

/**
 * Downloads the voucher once and saves it. A copy that is already on the phone is never fetched again.
 * Throws the API error (for example VOUCHER_NOT_READY or NETWORK) when it cannot be downloaded.
 */
export async function ensureVoucher(bookingId: string): Promise<Voucher> {
  const local = await loadVoucher(bookingId);
  if (local) return local;
  const fresh = await api.getVoucher(bookingId);
  await saveVoucher(fresh);
  return fresh;
}
