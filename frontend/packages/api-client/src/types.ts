import type {
  Amenity,
  Booking,
  DeclineReason,
  Identity,
  PaymentMethod,
  PartnerBooking,
  PartnerBookingList,
  PaymentSession,
  PropertyDetail,
  PropertyType,
  Quote,
  SearchResponse,
  Voucher,
} from '@bookdan/domain';

export interface SearchParams {
  q?: string;
  lat?: number;
  lng?: number;
  check_in: string;
  check_out: string;
  guests: number;
  types?: PropertyType[];
  amenities?: Amenity[];
  cursor?: string | null;
}

export interface GetPropertyParams {
  id: string;
  check_in: string;
  check_out: string;
}

export interface CreateQuoteParams {
  property_id: string;
  room_type_id: string;
  check_in: string;
  check_out: string;
  guests: number;
}

export interface Tokens {
  token: string;
  refresh_token: string;
}

/** Sign-in and session calls, the same for guests and hotel staff. */
export interface AuthApi {
  /** Sets the bearer token used by the calls below (null on sign-out). */
  setToken(token: string | null): void;
  /** Sends a one-time code by SMS (phone) or email. */
  requestOtp(identity: Identity): Promise<void>;
  verifyOtp(identity: Identity, code: string): Promise<Tokens & { identity: Identity }>;
  /** Trades a refresh token for a new access token. Fails with UNAUTHENTICATED when it is no longer valid. */
  refresh(refreshToken: string): Promise<Tokens>;
}

/**
 * The guest app's contract. Today it is implemented by the mock;
 * once the Go backend publishes its OpenAPI file, the HTTP implementation is
 * generated from it and swapped in with no screen changes.
 */
export interface Api extends AuthApi {
  search(params: SearchParams): Promise<SearchResponse>;
  getProperty(params: GetPropertyParams): Promise<PropertyDetail>;

  createQuote(params: CreateQuoteParams): Promise<Quote>;
  /**
   * Places a time-limited hold on the room. Fails with SOLD_OUT or QUOTE_EXPIRED.
   * idempotency_key is created once per checkout attempt and reused on retries: the same key
   * returns the same booking, the same key with different details fails with IDEMPOTENCY_MISMATCH.
   */
  createHold(params: { quote_id: string; guest_name: string; idempotency_key: string }): Promise<Booking>;
  /**
   * Starts payment for a held booking. Pay-at-hotel completes immediately (session is null).
   * Otherwise returns a session whose payment_url the app opens in an in-app browser; the result
   * is learned by polling getBooking, never from the browser. Calling it again while a payment
   * is pending returns the same session.
   */
  startPayment(params: { booking_id: string; method: PaymentMethod }): Promise<{ booking: Booking; session: PaymentSession | null }>;
  getBooking(id: string): Promise<Booking>;
  listBookings(): Promise<Booking[]>;
  /** Signed voucher for a confirmed booking. Fails with VOUCHER_NOT_READY before confirmation. */
  getVoucher(bookingId: string): Promise<Voucher>;

  /** Demo controls. Only the mock implements this; production builds must not have it. */
  demo?: {
    /** Plays the gateway's callback. 'duplicate' sends the success callback twice. */
    completePayment(sessionId: string, outcome: 'success' | 'failed' | 'duplicate'): Promise<void>;
    /** Makes the current access token expire now, to exercise the refresh path. */
    expireAccessToken(): void;
    /** Also invalidates the refresh token, so the guest must sign in again. */
    revokeRefreshToken(): void;
  };
}

/**
 * The partner app's contract (hotel owners and reception). Answering a booking needs a connection:
 * the deadline is short, so an answer cannot be queued for later.
 */
/** Tells the server where to send this phone's alerts. Re-sent whenever the token changes. */
export interface PushRegistration {
  /** The phone's FCM token. */
  token: string;
  token_type: 'fcm';
  /** Stable id of this app install (the X-Device-Id header). */
  device_id: string;
  platform: 'android' | 'ios';
  app_version: string;
}

export interface PartnerApi extends AuthApi {
  /** Registers (or refreshes) this phone for new-booking alerts. */
  registerPushToken(registration: PushRegistration): Promise<void>;
  /** The hotel's bookings: those waiting for an answer, upcoming stays, and recent ones. */
  listBookings(): Promise<PartnerBookingList>;
  /** Accept a booking. Fails with DEADLINE_PASSED once the time to answer is over (the guest is refunded). */
  acceptBooking(id: string): Promise<PartnerBooking>;
  /** Decline a booking, with a reason. The guest is refunded in full. */
  declineBooking(id: string, reason: DeclineReason): Promise<PartnerBooking>;

  /** Demo controls. Only the mock implements this; production builds must not have it. */
  demo?: {
    /** A new booking arrives with a fresh 15-minute deadline, as a real guest payment would cause. */
    addIncomingBooking(): Promise<void>;
    expireAccessToken(): void;
    revokeRefreshToken(): void;
  };
}

/** Stable error shape from the backend: code, message, request_id, details. */
export class ApiError extends Error {
  constructor(
    public code:
      | 'NETWORK' | 'NOT_FOUND' | 'SERVER' | 'UNAUTHENTICATED' | 'INVALID_PHONE' | 'INVALID_EMAIL' | 'INVALID_OTP'
      | 'SOLD_OUT' | 'QUOTE_EXPIRED' | 'HOLD_EXPIRED' | 'IDEMPOTENCY_MISMATCH' | 'VOUCHER_NOT_READY' | 'PAYMENT_FAILED' | 'DEADLINE_PASSED' | 'INVALID_STATE'
      | (string & {}),
    message: string,
    public request_id?: string,
  ) {
    super(message);
  }
}
