import { z } from 'zod';

// A guest's identity details. The guest enters them once in the guest app (Profile, or at their first booking).
// When they request a room, the hotel sees this profile with the booking, including the ID. It is personal data,
// so the rules here keep the full number out of lists, logs and caches.

export const IdTypeSchema = z.enum(['national_id', 'passport', 'driving_licence', 'residence_permit', 'other']);
export type IdType = z.infer<typeof IdTypeSchema>;
export const ID_TYPES = IdTypeSchema.options;

/**
 * Cleans what the guest typed: removes spaces, upper-cases letters. Returns null when it cannot be an ID.
 * Deliberately permissive (letters, digits, dash, slash, 4 to 25 characters): Ethiopian ID formats differ by issuer
 * (kebele ID, Fayda, passport, licence). [verify] the exact national-ID formats with the issuing authority before
 * tightening this.
 */
export function normalizeIdNumber(input: string): string | null {
  const n = input.replace(/\s+/g, '').toUpperCase();
  return /^[A-Z0-9/-]{4,25}$/.test(n) ? n : null;
}

/** The last four characters: all that lists ever show. */
export function idLast4(normalized: string): string {
  return normalized.slice(-4);
}

/** "•••• 4821": enough to recognise an ID, never the whole number. */
export function maskId(last4: string): string {
  return `•••• ${last4}`;
}

/** Which sides of an ID photo exist. A card has a front and a back; a passport only a front. */
export const IdPhotoSidesSchema = z.object({ front: z.boolean(), back: z.boolean() });
export type IdPhotoSides = z.infer<typeof IdPhotoSidesSchema>;
export type IdSide = keyof IdPhotoSides;

/** What the guest types in. The full ID number travels only in this direction, to the server. */
export const GuestProfileInputSchema = z.object({
  full_name: z.string().trim().min(2).max(120),
  id_type: IdTypeSchema,
  id_number: z.string(),
  nationality: z.string().trim().min(2).max(60),
});
export type GuestProfileInput = z.infer<typeof GuestProfileInputSchema>;

/**
 * The guest's own profile as the server returns it. There is NO id_number field on purpose: the guest sees only the
 * last four characters of what they saved, and changing the ID means typing it again.
 */
export const GuestProfileSchema = z.object({
  full_name: z.string(),
  id_type: IdTypeSchema,
  id_last4: z.string(),
  nationality: z.string(),
  updated_at: z.string(),
  /** Which sides of the ID photo the guest added (the pictures are fetched one at a time). */
  id_photos: IdPhotoSidesSchema.default({ front: false, back: false }),
  server_time: z.string(),
});
export type GuestProfile = z.infer<typeof GuestProfileSchema>;

/** Booking needs a profile with a name, an ID and a nationality. */
export function isProfileComplete(p: Pick<GuestProfile, 'full_name' | 'id_last4' | 'nationality'> | null | undefined): boolean {
  return !!p && p.full_name.trim().length >= 2 && p.id_last4.length > 0 && p.nationality.trim().length >= 2;
}

/**
 * What the hotel sees of the guest on a booking: the profile as it was when the guest requested the room, plus how to
 * reach them. The full ID number is not in here; the hotel asks for it with an explicit, audited "reveal".
 */
export const BookingGuestSchema = z.object({
  full_name: z.string(),
  /** Phone or email the guest signed in with. */
  contact: z.string(),
  nationality: z.string(),
  id_type: IdTypeSchema,
  id_last4: z.string(),
  /** True once the full number was erased after the retention period; only the last four remain. */
  id_purged: z.boolean(),
  /** Which sides of the guest's ID photo came with this booking and are not erased yet. */
  id_photos: IdPhotoSidesSchema.default({ front: false, back: false }),
});
export type BookingGuest = z.infer<typeof BookingGuestSchema>;
