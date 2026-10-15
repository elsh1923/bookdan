// Voucher token: base64url(payload JSON bytes) + "." + base64url(signature over those bytes).
// The server signs it (Ed25519); the guest app only displays it, the partner app verifies it offline.

const ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_';

export function b64urlEncode(bytes: Uint8Array): string {
  let out = '';
  for (let i = 0; i < bytes.length; i += 3) {
    const n = (bytes[i] << 16) | ((bytes[i + 1] ?? 0) << 8) | (bytes[i + 2] ?? 0);
    out += ALPHABET[(n >> 18) & 63] + ALPHABET[(n >> 12) & 63];
    if (i + 1 < bytes.length) out += ALPHABET[(n >> 6) & 63];
    if (i + 2 < bytes.length) out += ALPHABET[n & 63];
  }
  return out;
}

/** Returns null for anything that is not valid unpadded base64url. */
export function b64urlDecode(text: string): Uint8Array | null {
  if (!/^[A-Za-z0-9_-]*$/.test(text) || text.length % 4 === 1) return null;
  const bytes: number[] = [];
  let buffer = 0;
  let bits = 0;
  for (const ch of text) {
    buffer = (buffer << 6) | ALPHABET.indexOf(ch);
    bits += 6;
    if (bits >= 8) {
      bits -= 8;
      bytes.push((buffer >> bits) & 255);
      buffer &= (1 << bits) - 1;
    }
  }
  return Uint8Array.from(bytes);
}

export interface VoucherPayload {
  v: 1;
  kid: string; // which signing key (for rotation)
  pid: string; // property id
  code: string; // booking reference
  in: string; // check-in, YYYY-MM-DD (Africa/Addis_Ababa)
  out: string; // check-out
  exp: number; // unix seconds
}

export function encodeVoucherToken(payload: VoucherPayload, signature: Uint8Array): string {
  const json = new TextEncoder().encode(JSON.stringify(payload));
  return `${b64urlEncode(json)}.${b64urlEncode(signature)}`;
}

/**
 * Reads the payload WITHOUT verifying the signature. Only for display and tests: trust decisions
 * belong to the partner app, which checks the Ed25519 signature offline.
 */
export function decodeVoucherPayload(token: string): VoucherPayload | null {
  const parts = token.split('.');
  if (parts.length !== 2 || !parts[0] || !parts[1]) return null;
  const bytes = b64urlDecode(parts[0]);
  if (!bytes) return null;
  try {
    const p = JSON.parse(new TextDecoder().decode(bytes));
    if (p && p.v === 1 && typeof p.pid === 'string' && typeof p.code === 'string' && typeof p.exp === 'number') return p as VoucherPayload;
  } catch {
    // fall through
  }
  return null;
}

/** A fresh key per checkout attempt, reused on retries so the server never creates two holds. */
export function newIdempotencyKey(): string {
  const c = (globalThis as { crypto?: { randomUUID?: () => string } }).crypto;
  if (c?.randomUUID) return c.randomUUID();
  const hex = () => Math.floor(Math.random() * 0x100000000).toString(16).padStart(8, '0');
  return `${hex()}-${hex().slice(0, 4)}-${hex().slice(0, 4)}-${hex().slice(0, 4)}-${hex()}${hex().slice(0, 4)}`;
}
