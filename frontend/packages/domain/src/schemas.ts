import { z } from 'zod';

export const CurrencySchema = z.enum(['ETB', 'USD', 'EUR']);

export const MoneySchema = z.object({
  amount_santim: z.number().int(),
  currency: CurrencySchema,
});

export const BookingStatusSchema = z.enum([
  'HELD', 'PAID_AWAITING_HOTEL', 'CONFIRMED', 'CHECKED_IN',
  'COMPLETED', 'EXPIRED', 'DECLINED_REFUNDED', 'CANCELLED_REFUNDED',
  'NO_SHOW', 'NEEDS_RELOCATION', 'DISPUTED',
]);

export const PropertyTypeSchema = z.enum(['hotel', 'guesthouse', 'lodge', 'resort']);

export const AmenitySchema = z.enum(['generator', 'hot_water', 'wifi', 'parking', 'pay_at_hotel', 'ac', 'pool', 'restaurant', 'gym', 'spa', 'airport_shuttle', 'laundry', 'front_desk_24h', 'water_tank']);
export const ALL_AMENITIES = AmenitySchema.options;

/** A well-known place and how far it is from a hotel, in metres. */
export const PlaceRefSchema = z.object({ name: z.string(), name_am: z.string().optional(), distance_m: z.number().int() });

/** A stay a sold-out hotel can still offer: nearby dates, or part of the stay. The price is per night for the first night; the quote gives the exact total. */
export const AltWindowSchema = z.object({
  check_in: z.string(),
  check_out: z.string(),
  nights: z.number().int(),
  from_price_santim: z.number().int(),
  currency: CurrencySchema,
});
export type AltWindow = z.infer<typeof AltWindowSchema>;

export const SearchItemSchema = z.object({
  id: z.string(),
  name: z.string(),
  area: z.string(),
  city: z.string(),
  type: PropertyTypeSchema,
  badge: z.enum(['verified', 'none']),
  from_price_santim: z.number().int(),
  currency: CurrencySchema,
  available: z.boolean(),
  amenities: z.array(AmenitySchema),
  rating: z.number().nullable(),
  review_count: z.number().int(),
  photo_url: z.string().nullable(),
  accent: z.string().optional(), // placeholder colour while photos load
  lat: z.number(),
  lng: z.number(),
  /** Rooms still free of the cheapest room type that fits the party (0 when none): "Only 2 left". */
  rooms_left: z.number().int().default(0),
  /** Distance from the point the guest searched around, when the search had one. */
  distance_m: z.number().int().nullable().default(null),
  /** The closest well-known place, for the card ("1.4 km to Bole Airport"). */
  landmark: PlaceRefSchema.nullable().default(null),
  /** Some room here can be booked with breakfast / a cheaper non-refundable rate is offered. */
  breakfast: z.boolean().default(false),
  nonrefundable: z.boolean().default(false),
  /** How many minutes the hotel usually takes to answer a request (null until it has answered a few). */
  reply_minutes: z.number().int().nullable().default(null),
  /** Only in the "other dates" list: stays this hotel can still sell when it is full for the whole stay asked for. */
  windows: z.array(AltWindowSchema).default([]),
});

/** One place suggestion as the guest types: a landmark, area or city to search for, or a hotel to open. */
export const SuggestionSchema = z.object({
  kind: z.enum(['landmark', 'area', 'city', 'hotel']),
  text: z.string(),
  text_am: z.string().optional(),
  subtitle: z.string().default(''),
  property_id: z.string().optional(),
});
export type Suggestion = z.infer<typeof SuggestionSchema>;
export const SuggestResponseSchema = z.object({ items: z.array(SuggestionSchema), server_time: z.string() });

export const SearchSortSchema = z.enum(['recommended', 'price_asc', 'price_desc', 'rating', 'distance']);
export type SearchSort = z.infer<typeof SearchSortSchema>;

export const SearchResponseSchema = z.object({
  items: z.array(SearchItemSchema),
  next_cursor: z.string().nullable(),
  /** How many hotels match the filters (all pages), the cheapest and dearest night among hotels with a room, and the count behind each filter. */
  total: z.number().int().optional(),
  price_min_santim: z.number().int().default(0),
  price_max_santim: z.number().int().default(0),
  facets: z.object({ types: z.record(z.number().int()).default({}), amenities: z.record(z.number().int()).default({}) }).default({ types: {}, amenities: {} }),
  /** Hotels that are full for the dates asked but have rooms on nearby dates, each with the stays it can sell (first page only). */
  other_dates: z.array(SearchItemSchema).default([]),
  server_time: z.string(),
});

/** The ways to book a room: room only or with breakfast, with free cancellation or non-refundable (cheaper). */
export const MealSchema = z.enum(['room_only', 'breakfast']);
export type Meal = z.infer<typeof MealSchema>;
export const RoomOptionSchema = z.object({ meal: MealSchema, refundable: z.boolean(), price_santim: z.number().int() });
export type RoomOption = z.infer<typeof RoomOptionSchema>;

export const RoomTypeSchema = z.object({
  id: z.string(),
  name: z.string(),
  capacity: z.number().int(),
  price_santim: z.number().int(),
  currency: CurrencySchema,
  amenities: z.array(z.string()),
  rooms_left: z.number().int(),
  /** What this room offers, each with its price per night. Always has the plain room first. */
  options: z.array(RoomOptionSchema).default([]),
  /** This room's own pictures (full size), cover first, and the first one small for the card. */
  photos: z.array(z.string()).default([]),
  photo_thumb: z.string().nullable().default(null),
});

export const ReviewSchema = z.object({
  id: z.string(),
  author: z.string(),
  rating: z.number(),
  text: z.string(),
  date: z.string(),
  /** The review comes from a guest whose stay was booked and finished on BookDan. */
  verified: z.boolean().default(true),
  /** The hotel's public answer, if it wrote one. */
  reply: z.object({ text: z.string(), date: z.string() }).nullable().default(null),
});

/** The numbers above a hotel's reviews. distribution[0] counts one-star reviews and distribution[4] five-star reviews. */
export const RatingSummarySchema = z.object({
  average: z.number().nullable(),
  count: z.number().int(),
  distribution: z.array(z.number().int()).length(5),
  categories: z.object({
    cleanliness: z.number().nullable(),
    location: z.number().nullable(),
    staff: z.number().nullable(),
    value: z.number().nullable(),
  }),
});
export type RatingSummary = z.infer<typeof RatingSummarySchema>;

export const PropertyDetailSchema = z.object({
  property: SearchItemSchema.extend({
    description: z.string(),
    phone: z.string(),
    address: z.string(),
    house_rules: z.array(z.string()),
    photos: z.array(z.string()),
    /** When guests may arrive and must leave (HH:MM, Addis Ababa time). */
    check_in_time: z.string().default('14:00'),
    check_out_time: z.string().default('12:00'),
    /** The description in Amharic, and how to find the hotel in words (English and Amharic). Empty when the hotel wrote none. */
    description_am: z.string().default(''),
    directions: z.string().default(''),
    directions_am: z.string().default(''),
  }),
  room_types: z.array(RoomTypeSchema),
  reviews: z.array(ReviewSchema),
  rating_summary: RatingSummarySchema.default({ average: null, count: 0, distribution: [0, 0, 0, 0, 0], categories: { cleanliness: null, location: null, staff: null, value: null } }),
  /** Landmarks within 5 km, closest first. */
  nearby: z.array(PlaceRefSchema).default([]),
  server_time: z.string(),
});

export type BookingStatus = z.infer<typeof BookingStatusSchema>;
export type SearchItem = z.infer<typeof SearchItemSchema>;
export type SearchResponse = z.infer<typeof SearchResponseSchema>;
export type RoomType = z.infer<typeof RoomTypeSchema>;
export type Review = z.infer<typeof ReviewSchema>;
export type PropertyDetail = z.infer<typeof PropertyDetailSchema>;
export type Amenity = z.infer<typeof AmenitySchema>;
export type PropertyType = z.infer<typeof PropertyTypeSchema>;

export const PaymentMethodSchema = z.enum(['telebirr', 'chapa', 'pay_at_hotel']);

export const QuoteSchema = z.object({
  id: z.string(),
  property_id: z.string(),
  property_name: z.string(),
  room_type_id: z.string(),
  room_name: z.string(),
  check_in: z.string(),
  check_out: z.string(),
  nights: z.number().int(),
  guests: z.number().int(),
  /** How many rooms of this type the guest is booking. Prices below are for all of them. */
  rooms: z.number().int().min(1).default(1),
  /** How many of this room type could still be booked for these dates when the quote was made. */
  rooms_available: z.number().int().default(1),
  /** The price of one room for one night. */
  nightly_santim: z.number().int(),
  subtotal_santim: z.number().int(),
  vat_santim: z.number().int(),
  total_santim: z.number().int(),
  currency: CurrencySchema,
  pay_at_hotel: z.boolean(),
  /** The room option chosen, the coupon applied, how much it took off the total (VAT included), and the total without it. */
  meal: MealSchema.default('room_only'),
  refundable: z.boolean().default(true),
  coupon_code: z.string().nullable().default(null),
  discount_santim: z.number().int().default(0),
  list_total_santim: z.number().int().default(0),
  /** How many of the guests are children (the rest are adults). */
  children: z.number().int().default(0),
  /** The hotel's first photo (small), null when it has none; and the check-in and check-out times (Addis Ababa time). */
  photo_url: z.string().nullable().default(null),
  check_in_time: z.string().default('14:00'),
  check_out_time: z.string().default('12:00'),
  server_time: z.string(),
});

export const PaymentProgressSchema = z.enum(['none', 'pending', 'failed']);

/** A change of dates that waits for the payment of the difference: the new dates, what to pay, until when, and the page to pay on. */
export const PendingDateChangeSchema = z.object({
  check_in: z.string(),
  check_out: z.string(),
  new_total_santim: z.number().int(),
  pay_now_santim: z.number().int(),
  expires_at: z.string(),
  payment: z.object({ id: z.string(), status: z.string(), payment_url: z.string() }).nullable().default(null),
});
export type PendingDateChange = z.infer<typeof PendingDateChangeSchema>;

/** What moving a booking to other dates would do (the answer of POST /bookings/{id}/change-dates/quote). */
export const DateChangeQuoteSchema = z.object({
  kind: z.enum(['same_price', 'refund', 'pay', 'reprice']),
  check_in: z.string(),
  check_out: z.string(),
  nights: z.number().int(),
  currency: CurrencySchema,
  old_total_santim: z.number().int(),
  new_total_santim: z.number().int(),
  difference_santim: z.number().int(),
  pay_now_santim: z.number().int().default(0),
  refund_now_santim: z.number().int().default(0),
  old_balance_due_santim: z.number().int().default(0),
  new_balance_due_santim: z.number().int().default(0),
  difference_at_hotel: z.boolean().default(false),
});
export type DateChangeQuote = z.infer<typeof DateChangeQuoteSchema>;

export const BookingSchema = z.object({
  id: z.string(),
  reference: z.string(),
  status: BookingStatusSchema,
  property_id: z.string(),
  property_name: z.string(),
  property_phone: z.string(),
  address: z.string(),
  room_name: z.string(),
  check_in: z.string(),
  check_out: z.string(),
  nights: z.number().int(),
  guests: z.number().int(),
  rooms: z.number().int().min(1).default(1),
  guest_name: z.string(),
  total_santim: z.number().int(),
  currency: CurrencySchema,
  payment_method: PaymentMethodSchema.nullable(),
  pay_at_hotel: z.boolean(),
  payment: PaymentProgressSchema, // none, pending (waiting for the gateway), failed (try again while held)
  hold_expires_at: z.string().nullable(),
  /** The server says whether the guest can cancel now, what they would get back, and until when a confirmed booking is refunded in full. */
  can_cancel: z.boolean().default(false),
  refund_if_cancelled_santim: z.number().int().default(0),
  full_refund_until: z.string().nullable().default(null),
  /** What a cancellation refunded (0 when it came too late for a refund); null if the booking was not cancelled. */
  refund_santim: z.number().int().nullable().default(null),
  /** Whether the guest may review this stay now (after check-out), and the review they wrote. */
  can_review: z.boolean().default(false),
  review: z.object({ rating: z.number().int(), text: z.string(), hotel_reply: z.object({ text: z.string(), date: z.string() }).nullable().default(null) }).nullable().default(null),
  /** Whether the guest may open a dispute now (once per booking; from the start of the stay to 14 days after check-out), and their dispute, if any. */
  can_dispute: z.boolean().default(false),
  dispute_ticket_id: z.string().nullable().default(null),
  /** Whether the guest may move the booking to other dates now (once, at the same price), and the dates it first had. */
  can_change_dates: z.boolean().default(false),
  /** A change of dates waiting for the payment of the difference (single-booking view only). */
  pending_date_change: PendingDateChangeSchema.nullable().default(null),
  /** The part of the payment already returned while the booking stands (a dispute settled in part, or a goodwill credit). */
  partially_refunded_santim: z.number().int().default(0),
  original_check_in: z.string().nullable().default(null),
  original_check_out: z.string().nullable().default(null),
  /** The hotel's check-in and check-out times (HH:MM). */
  check_in_time: z.string().default('14:00'),
  check_out_time: z.string().default('12:00'),
  /** What happened to the booking and when (single-booking view only). */
  timeline: z.array(z.object({ status: BookingStatusSchema, at: z.string() })).default([]),
  /** The room option booked, and the coupon used (the discount is paid online only). */
  meal: MealSchema.default('room_only'),
  refundable: z.boolean().default(true),
  coupon_code: z.string().nullable().default(null),
  discount_santim: z.number().int().default(0),
  children: z.number().int().default(0),
  /** What the guest asked the hotel for, and whether the person staying booked it for someone else (with a phone number to reach them). */
  special_requests: z.string().nullable().default(null),
  booked_for_other: z.boolean().default(false),
  guest_phone: z.string().nullable().default(null),
  server_time: z.string(),
});

/** The optional sub-ratings a guest can give with their stars. */
export interface ReviewInput {
  rating: number;
  text: string;
  cleanliness?: number;
  location?: number;
  staff?: number;
  value?: number;
}

/** What the app opens to take the payment: an in-app browser at payment_url, which returns by deep link. */
export const PaymentSessionSchema = z.object({
  id: z.string(),
  booking_id: z.string(),
  method: PaymentMethodSchema,
  payment_url: z.string(),
});

export type PaymentSession = z.infer<typeof PaymentSessionSchema>;
export type PaymentProgress = z.infer<typeof PaymentProgressSchema>;
export type PaymentMethod = z.infer<typeof PaymentMethodSchema>;
export type Quote = z.infer<typeof QuoteSchema>;
export type Booking = z.infer<typeof BookingSchema>;

export const VoucherSchema = z.object({
  booking_id: z.string(),
  reference: z.string(),
  token: z.string(), // signed QR text (see voucher.ts)
  pin: z.string().regex(/^[0-9]{4}$/),
  expires_at: z.string(),
  property_id: z.string(),
  property_name: z.string(),
  property_phone: z.string(),
  address: z.string(),
  lat: z.number(),
  lng: z.number(),
  check_in: z.string(),
  check_out: z.string(),
  nights: z.number().int(),
  guests: z.number().int(),
  rooms: z.number().int().min(1).default(1),
  guest_name: z.string(),
  balance_due_santim: z.number().int(), // what is still owed at the hotel
  currency: CurrencySchema,
  server_time: z.string(),
});

export type Voucher = z.infer<typeof VoucherSchema>;

/** Support: a guest writes to the platform and support answers. One ticket is one conversation, optionally about a booking. */
export const SupportKindSchema = z.enum(['question', 'problem', 'dispute']);
export const SupportStatusSchema = z.enum(['open', 'answered', 'closed']);
export const SupportMessageSchema = z.object({
  from_staff: z.boolean(),
  /** Who wrote it: the guest, the hotel (a statement in a dispute) or BookDan support. */
  role: z.enum(['guest', 'hotel', 'support']).default('guest'),
  author: z.string(),
  body: z.string(),
  created_at: z.string(),
});
export const SupportTicketSchema = z.object({
  id: z.string(),
  kind: SupportKindSchema,
  subject: z.string(),
  status: SupportStatusSchema,
  booking_id: z.string().nullable(),
  booking_code: z.string().nullable(),
  created_at: z.string(),
  updated_at: z.string(),
  preview: z.string().optional(),
  /** Disputes: why it was opened, and how it ended (rejected, refunded_hotel_pays, refunded_platform_pays, withdrawn). */
  reason: z.string().nullable().optional(),
  outcome: z.string().nullable().optional(),
  messages: z.array(SupportMessageSchema).optional(),
});
export const SupportTicketListSchema = z.object({ tickets: z.array(SupportTicketSchema) });
export const SupportTicketDetailSchema = z.object({ ticket: SupportTicketSchema });
export const SupportTicketCreatedSchema = z.object({ id: z.string() });

export const DISPUTE_REASONS = ['not_as_described', 'refused_entry', 'charged_wrong', 'no_show_wrong', 'other'] as const;
export type DisputeReason = (typeof DISPUTE_REASONS)[number];

export type SupportKind = z.infer<typeof SupportKindSchema>;
export type SupportMessage = z.infer<typeof SupportMessageSchema>;
export type SupportTicket = z.infer<typeof SupportTicketSchema>;
/** What a guest can start: dispute is raised from a booking (not here). */
export interface NewSupportTicket {
  kind: 'question' | 'problem';
  subject: string;
  message: string;
  booking_id?: string;
}

/** The display exchange rates: ETB for one dollar or euro. A stale rate is ignored by the apps (ETB only). */
export const RatesSchema = z.object({
  base: z.literal('ETB'),
  rates: z.array(z.object({ currency: z.enum(['USD', 'EUR']), birr_per_unit: z.number().positive(), updated_at: z.string(), stale: z.boolean() })),
  server_time: z.string(),
});
export type Rates = z.infer<typeof RatesSchema>;
