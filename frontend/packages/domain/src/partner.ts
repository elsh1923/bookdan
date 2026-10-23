import { z } from 'zod';
import { BookingGuestSchema } from './guestIdentity.ts';
import { BookingStatusSchema, CurrencySchema } from './schemas.ts';

/** Reasons a hotel can give when declining. The server stores the code; the app translates it. */
export const DeclineReasonSchema = z.enum(['fully_booked', 'room_unavailable', 'maintenance', 'other']);
export type DeclineReason = z.infer<typeof DeclineReasonSchema>;
export const DECLINE_REASONS = DeclineReasonSchema.options;

/** A booking as the hotel sees it: the stay, and the guest's profile. No payment details and no full ID number. */
export const PartnerBookingSchema = z.object({
  id: z.string(),
  reference: z.string(),
  status: BookingStatusSchema,
  guest_name: z.string(),
  /** The guest's profile as it was when they requested the room, including the masked ID. */
  guest: BookingGuestSchema,
  /** Present only when the account works for several hotels: which one this booking is for. */
  property_name: z.string().optional(),
  room_name: z.string(),
  check_in: z.string(),
  check_out: z.string(),
  nights: z.number().int(),
  guests: z.number().int(),
  rooms: z.number().int().min(1).default(1),
  total_santim: z.number().int(),
  currency: CurrencySchema,
  paid_at: z.string().nullable(),
  /** Until when the hotel may answer. After this the guest is refunded automatically. */
  hotel_deadline_at: z.string().nullable(),
  decline_reason: z.string().nullable(), // a DeclineReason, or 'no_response' when the deadline was missed
  server_time: z.string(),
});

export const PartnerBookingListSchema = z.object({
  property: z.object({ id: z.string(), name: z.string() }),
  items: z.array(PartnerBookingSchema),
  server_time: z.string(),
});

export type PartnerBooking = z.infer<typeof PartnerBookingSchema>;
export type PartnerBookingList = z.infer<typeof PartnerBookingListSchema>;

/** A booking the hotel still has to answer. */
export const isAwaitingAnswer = (b: Pick<PartnerBooking, 'status'>): boolean => b.status === 'PAID_AWAITING_HOTEL';

/**
 * Inbox order: bookings waiting for an answer first, the one closest to its deadline at the top,
 * then everything else by check-in date.
 */
export function sortInbox<T extends Pick<PartnerBooking, 'status' | 'hotel_deadline_at' | 'check_in'>>(items: T[]): T[] {
  return [...items].sort((a, b) => {
    const aw = isAwaitingAnswer(a);
    const bw = isAwaitingAnswer(b);
    if (aw !== bw) return aw ? -1 : 1;
    if (aw && bw) return (a.hotel_deadline_at ?? '').localeCompare(b.hotel_deadline_at ?? '');
    return a.check_in.localeCompare(b.check_in);
  });
}
