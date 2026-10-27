import { ed25519 } from '@noble/curves/ed25519.js';
import { b64urlDecode, decodeVoucherPayload, type VoucherPayload } from './voucher.ts';

// Offline check-in. The guest's voucher QR code holds a token signed by the server (Ed25519). The partner app caches the server's public key and
// can therefore tell a genuine voucher from a forged one with no internet. It cannot know whether the booking was cancelled meanwhile: the check-in is
// queued and the server decides when it is sent (POST /partner/checkin/sync).

/** Checks the signature of a voucher token against the server's public key (base64url) and returns what it says, or null if it is not genuine. */
export function verifyVoucherToken(token: string, publicKeyB64: string): VoucherPayload | null {
  const parts = token.trim().split('.');
  if (parts.length !== 2) return null;
  const payload = b64urlDecode(parts[0]);
  const sig = b64urlDecode(parts[1]);
  const key = b64urlDecode(publicKeyB64);
  if (!payload || !sig || !key || sig.length !== 64 || key.length !== 32) return null;
  try {
    if (!ed25519.verify(sig, payload, key)) return null;
  } catch {
    return null;
  }
  return decodeVoucherPayload(token.trim());
}

export type OfflineVerdict =
  | { ok: true; payload: VoucherPayload }
  | { ok: false; reason: 'invalid' | 'expired' | 'wrong_hotel' | 'too_early' | 'ended' };

/**
 * Whether a scanned voucher can be accepted for check-in right now, offline: genuine, not expired, for one of this hotel's properties, and today (Addis
 * Ababa date) is inside the stay. hotelIds are the hotels the signed-in staff member works at (cached while online).
 */
export function judgeVoucher(token: string, publicKeyB64: string, hotelIds: string[], nowMs: number, today: string): OfflineVerdict {
  const payload = verifyVoucherToken(token, publicKeyB64);
  if (!payload) return { ok: false, reason: 'invalid' };
  if (nowMs / 1000 > payload.exp) return { ok: false, reason: 'expired' };
  if (!hotelIds.includes(payload.pid)) return { ok: false, reason: 'wrong_hotel' };
  if (today < payload.in) return { ok: false, reason: 'too_early' };
  if (today > payload.out) return { ok: false, reason: 'ended' };
  return { ok: true, payload };
}

/** One check-in made while offline, waiting to be sent. */
export interface QueuedCheckin {
  token: string;
  reference: string;
  /** When the guest was checked in (ISO 8601), by the phone's clock. */
  at: string;
}

/** The server's answer for one item (see POST /partner/checkin/sync). */
export type SyncResult = 'checked_in' | 'already' | 'cancelled' | 'wrong_hotel' | 'invalid' | 'not_found' | 'too_early' | 'ended' | 'not_ready' | 'bad_time';

/** Results that mean the check-in is settled one way or the other and leave the queue; the rest of them were refused and are shown to the hotel. */
export const SYNC_DONE: readonly SyncResult[] = ['checked_in', 'already'];

/** Adds a check-in to the queue unless the same booking is already in it (scanning twice must not queue twice). */
export function enqueueCheckin(queue: QueuedCheckin[], item: QueuedCheckin): QueuedCheckin[] {
  return queue.some((q) => q.reference === item.reference) ? queue : [...queue, item];
}

/** Applies the server's answers to the queue: settled ones leave, refused ones are returned as problems. Answers come in the order of what was sent. */
export function settleQueue(queue: QueuedCheckin[], sent: QueuedCheckin[], results: SyncResult[]): { queue: QueuedCheckin[]; problems: { reference: string; result: SyncResult; at: string }[] } {
  const gone = new Set<string>();
  const problems: { reference: string; result: SyncResult; at: string }[] = [];
  sent.forEach((item, i) => {
    const result = results[i];
    if (!result) return; // no answer for it: it stays queued
    gone.add(item.reference);
    if (!SYNC_DONE.includes(result)) problems.push({ reference: item.reference, result, at: item.at });
  });
  return { queue: queue.filter((q) => !gone.has(q.reference)), problems };
}
