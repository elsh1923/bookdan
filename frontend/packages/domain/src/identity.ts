// How a guest proves who they are: a phone number or an email address, both with a one-time code.

export type IdentityType = 'phone' | 'email';

export interface Identity {
  type: IdentityType;
  /** Normalised: phone as +251…, email lower-cased and trimmed. */
  value: string;
}

/**
 * Returns the lower-cased address, or null if it is not a plausible email.
 * Deliberately simple: the real proof is the code that arrives in the inbox.
 */
export function normalizeEmail(input: string): string | null {
  const email = input.trim().toLowerCase();
  if (email.length > 254) return null;
  const at = email.indexOf('@');
  if (at < 1 || at !== email.lastIndexOf('@')) return null;
  const local = email.slice(0, at);
  const domain = email.slice(at + 1);
  if (local.length > 64 || /\s/.test(email)) return null;
  // domain: labels of letters, digits and inner hyphens, at least one dot, no empty label
  const labels = domain.split('.');
  if (labels.length < 2) return null;
  const labelOk = (l: string) => /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/.test(l);
  if (!labels.every(labelOk)) return null;
  if (!/^[a-z]{2,}$/.test(labels[labels.length - 1])) return null;
  return email;
}

