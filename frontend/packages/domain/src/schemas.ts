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

export const AmenitySchema = z.enum(['generator', 'hot_water', 'wifi', 'parking', 'pay_at_hotel']);

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
});

export const SearchResponseSchema = z.object({
  items: z.array(SearchItemSchema),
  next_cursor: z.string().nullable(),
  server_time: z.string(),
});

export const RoomTypeSchema = z.object({
  id: z.string(),
  name: z.string(),
  capacity: z.number().int(),
  price_santim: z.number().int(),
  currency: CurrencySchema,
  amenities: z.array(z.string()),
  rooms_left: z.number().int(),
});

export const ReviewSchema = z.object({
  id: z.string(),
  author: z.string(),
  rating: z.number(),
  text: z.string(),
  date: z.string(),
});

export const PropertyDetailSchema = z.object({
  property: SearchItemSchema.extend({
    description: z.string(),
    phone: z.string(),
    address: z.string(),
    house_rules: z.array(z.string()),
    photos: z.array(z.string()),
  }),
  room_types: z.array(RoomTypeSchema),
  reviews: z.array(ReviewSchema),
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
  server_time: z.string(),
});

export const PaymentProgressSchema = z.enum(['none', 'pending', 'failed']);

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
  review: z.object({ rating: z.number().int(), text: z.string() }).nullable().default(null),
  server_time: z.string(),
});

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
