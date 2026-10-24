import { z } from 'zod';
import { BookingGuestSchema } from './guestIdentity.ts';
import { BookingStatusSchema, CurrencySchema, MealSchema } from './schemas.ts';

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
  /** When the guest arrived (set by check-in). */
  checked_in_at: z.string().nullable().default(null),
  /** What the guest still pays at the hotel: the whole total for a pay-at-hotel booking, otherwise 0. */
  balance_due_santim: z.number().int().default(0),
  /** The room option the guest booked: serve breakfast when it is 'breakfast'; a non-refundable booking is not refunded on cancelling. */
  meal: MealSchema.default('room_only'),
  refundable: z.boolean().default(true),
  /** Who is travelling and what they asked for. For a booking made for someone else, guest_name is the person staying and guest_phone reaches them. */
  children: z.number().int().default(0),
  special_requests: z.string().default(''),
  booked_for_other: z.boolean().default(false),
  guest_phone: z.string().default(''),
  server_time: z.string(),
});

export const PartnerBookingListSchema = z.object({
  property: z.object({ id: z.string(), name: z.string() }),
  items: z.array(PartnerBookingSchema),
  server_time: z.string(),
});

/** One booking's share of a hotel's earnings. Amounts are santim: what the guest paid, BookDan's fee, VAT on the fee, and what the hotel receives. */
export const EarningLineSchema = z.object({
  booking_id: z.string(),
  reference: z.string(),
  guest_name: z.string(),
  check_in: z.string(),
  check_out: z.string(),
  status: z.string(),
  gross_santim: z.number().int(),
  /** The part of the price BookDan paid with a coupon (0 when none). */
  discount_santim: z.number().int().default(0),
  commission_santim: z.number().int(),
  vat_santim: z.number().int(),
  net_santim: z.number().int(),
  /** True once the guest has checked in or the stay is over: the money is payable. */
  ready: z.boolean(),
});

export const PayoutLineSchema = z.object({
  id: z.string(),
  amount_santim: z.number().int(),
  method: z.string(),
  reference: z.string(),
  note: z.string().optional(),
  paid_at: z.string(),
});

export const HotelEarningsSchema = z.object({
  id: z.string(),
  name: z.string(),
  currency: CurrencySchema,
  /** Payable now (checked in or stay over), not payable yet (stay still to come), already paid, and what BookDan owes the hotel today. */
  ready_santim: z.number().int(),
  upcoming_santim: z.number().int(),
  paid_santim: z.number().int(),
  to_pay_santim: z.number().int(),
  last_paid_at: z.string().nullable(),
  items: z.array(EarningLineSchema),
  payouts: z.array(PayoutLineSchema),
});

export const EarningsSchema = z.object({ hotels: z.array(HotelEarningsSchema), server_time: z.string() });
export type EarningLine = z.infer<typeof EarningLineSchema>;
export type PayoutLine = z.infer<typeof PayoutLineSchema>;
export type HotelEarnings = z.infer<typeof HotelEarningsSchema>;
export type Earnings = z.infer<typeof EarningsSchema>;

/** One night of one room type on the hotel's chart. booked = rooms of guests who booked in the app, held = being paid for right now, blocked = the hotel's own walk-ins and blocks. */
export const ChartNightSchema = z.object({
  date: z.string(),
  capacity: z.number().int(),
  booked: z.number().int(),
  held: z.number().int(),
  blocked: z.number().int(),
  free: z.number().int(),
});
export const ChartRoomSchema = z.object({ id: z.string(), name: z.string(), total_rooms: z.number().int(), nights: z.array(ChartNightSchema) });
export const HotelRefSchema = z.object({ id: z.string(), name: z.string() });
export const RoomsChartSchema = z.object({
  hotels: z.array(HotelRefSchema),
  property: HotelRefSchema,
  from: z.string(),
  days: z.number().int(),
  rooms: z.array(ChartRoomSchema),
  server_time: z.string(),
});
export type ChartNight = z.infer<typeof ChartNightSchema>;
export type ChartRoom = z.infer<typeof ChartRoomSchema>;
export type RoomsChart = z.infer<typeof RoomsChartSchema>;

/** A walk-in guest the hotel recorded, or rooms it blocked (repairs, an event). Both take rooms out of sale until given back. */
export const RoomBlockSchema = z.object({
  id: z.string(),
  kind: z.enum(['walk_in', 'block']),
  room_type_id: z.string(),
  room_name: z.string(),
  check_in: z.string(),
  check_out: z.string(),
  rooms: z.number().int(),
  guest_name: z.string(),
  guest_phone: z.string(),
  note: z.string(),
});
export const RoomBlockListSchema = z.object({ blocks: z.array(RoomBlockSchema), server_time: z.string() });
export type RoomBlock = z.infer<typeof RoomBlockSchema>;
export interface NewRoomBlock {
  property_id?: string;
  room_type_id: string;
  check_in: string;
  /** The morning the rooms are free again (the last night is the day before). */
  check_out: string;
  rooms: number;
  kind: 'walk_in' | 'block';
  guest_name?: string;
  guest_phone?: string;
  note?: string;
}

/** True when a hotel may mark a confirmed booking as a no-show: the guest's check-in day has come. */
export const canMarkNoShow = (b: Pick<PartnerBooking, 'status' | 'check_in'>, today: string): boolean => b.status === 'CONFIRMED' && b.check_in <= today;

/** What a check-in lookup found: ready to check in, or why not. */
export const CHECKIN_STATES = ['ready', 'checked_in', 'too_early', 'ended', 'cancelled', 'waiting', 'not_ready'] as const;
export const CheckinLookupSchema = z.object({
  state: z.enum(CHECKIN_STATES),
  booking: PartnerBookingSchema,
  server_time: z.string(),
});
export type CheckinState = (typeof CHECKIN_STATES)[number];
export type CheckinLookup = z.infer<typeof CheckinLookupSchema>;
/** Look a booking up from the guest's voucher QR code, or from the booking reference and the 4-digit PIN. */
export type CheckinQuery = { token: string } | { reference: string; pin: string };

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
