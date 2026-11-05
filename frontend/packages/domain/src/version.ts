/** Reads "1.4.2" (also "1.4", "v1.4.2" and "1.4.2-beta.1") as three numbers, or null when it is not a version. */
export function parseVersion(v: string): [number, number, number] | null {
  const core = v.trim().replace(/^v/i, '').split(/[-+ ]/)[0] ?? '';
  if (!core) return null;
  const parts = core.split('.');
  if (parts.length > 3) return null;
  const out: [number, number, number] = [0, 0, 0];
  for (let i = 0; i < parts.length; i++) {
    const p = parts[i] ?? '';
    if (!/^\d{1,7}$/.test(p)) return null;
    out[i] = Number(p);
  }
  return out;
}

/** True when this build is older than the minimum the server asks for. No minimum, or a version that cannot be read, is never "older": nobody is locked out by a mistake. */
export function isOlderVersion(have: string, min: string): boolean {
  const a = parseVersion(have);
  const b = parseVersion(min);
  if (!a || !b) return false;
  for (let i = 0; i < 3; i++) {
    if (a[i] !== b[i]) return (a[i] as number) < (b[i] as number);
  }
  return false;
}
