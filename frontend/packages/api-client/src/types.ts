import type {
  Amenity,
  Booking,
  DateChangeQuote,
  CheckinLookup,
  CheckinQuery,
  Earnings,
  NewRoomBlock,
  RoomBlock,
  RoomsChart,
  DeclineReason,
  HotelDispute,
  HotelProblemReason,
  PartnerReview,
  Statement,
  GuestProfile,
  GuestProfileInput,
  DisputeReason,
  Identity,
  NewSupportTicket,
  PaymentMethod,
  PartnerBooking,
  PartnerBookingList,
  PaymentSession,
  PropertyDetail,
  PropertyType,
  Quote,
  ReviewInput,
  SearchSort,
  Suggestion,
  Rates,
  SearchResponse,
  SupportTicket,
  Voucher,
} from '@bookdan/domain';

export interface SearchParams {
  q?: string;
  lat?: number;
  lng?: number;
  check_in: string;
  check_out: string;
  guests: number;
  /** Rooms wanted (default 1). Hotels without that many free rooms of one type are not offered. */
  rooms?: number;
  types?: PropertyType[];
  amenities?: Amenity[];
  /** Order of the results. Hotels with no room for the party always come last. */
  sort?: SearchSort;
  /** Price per night in santim (lowest room that has a rate for check-in). */
  min_price?: number;
  max_price?: number;
  /** Only hotels rated at least this (0 to 5). */
  min_rating?: number;
  /** Only hotels that offer breakfast / a cheaper non-refundable rate. */
  breakfast?: boolean;
  deals?: boolean;
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
  /** Rooms of this type (default 1, at most 9). */
  rooms?: number;
  /** The room option: room only (default) or with breakfast, and whether it is refundable (default true). A room only offers what its hotel set. */
  meal?: 'room_only' | 'breakfast';
  refundable?: boolean;
  /** A coupon code. Fails with COUPON_INVALID when it does not apply. Coupons work with online payment only. */
  coupon_code?: string;
  /** How many of the guests are children (each room still needs an adult). */
  children?: number;
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
  /** Ends the session on the server (all its refresh tokens, and alerts to this phone). Best effort: the app signs out locally anyway. */
  logout(refreshToken: string): Promise<void>;
}

/**
 * The guest app's contract. Today it is implemented by the mock;
 * once the Go backend publishes its OpenAPI file, the HTTP implementation is
 * generated from it and swapped in with no screen changes.
 */
export interface Api extends AuthApi {
  search(params: SearchParams): Promise<SearchResponse>;
  getProperty(params: GetPropertyParams): Promise<PropertyDetail>;
  /** Landmarks, areas, cities and hotels matching what the guest has typed (with nothing typed: popular landmarks). */
  suggest(q: string): Promise<Suggestion[]>;
  /** The display exchange rates (ETB per dollar and euro), set by BookDan. Empty or stale means the apps show ETB only. Public. */
  getRates(): Promise<Rates>;

  createQuote(params: CreateQuoteParams): Promise<Quote>;
  /**
   * Places a time-limited hold on the room. Fails with SOLD_OUT, QUOTE_EXPIRED, or PROFILE_INCOMPLETE when the guest has
   * not saved their name and ID yet. The hotel receives a copy of the guest's profile with the request.
   * idempotency_key is created once per checkout attempt and reused on retries: the same key
   * returns the same booking, the same key with different details fails with IDEMPOTENCY_MISMATCH.
   */
  createHold(params: {
    quote_id: string;
    /** The name of the person who will stay. */
    guest_name: string;
    idempotency_key: string;
    /** Up to 400 characters for the hotel ("late arrival", "high floor"). */
    special_requests?: string;
    /** Booking for someone else: the hotel is told, and gets the guest's phone number to reach them. */
    booked_for_other?: boolean;
    guest_phone?: string;
  }): Promise<Booking>;
  /**
   * Starts payment for a held booking. Pay-at-hotel completes immediately (session is null).
   * Otherwise returns a session whose payment_url the app opens in an in-app browser; the result
   * is learned by polling getBooking, never from the browser. Calling it again while a payment
   * is pending returns the same session.
   */
  startPayment(params: { booking_id: string; method: PaymentMethod }): Promise<{ booking: Booking; session: PaymentSession | null }>;
  getBooking(id: string): Promise<Booking>;
  listBookings(): Promise<Booking[]>;
  /** The guest's own saved details, or null if none yet. The ID number comes back masked (last four only). */
  getProfile(): Promise<GuestProfile | null>;
  /** Saves the guest's name, ID and nationality. Fails with INVALID_ID for a number that cannot be an ID. */
  saveProfile(input: GuestProfileInput): Promise<GuestProfile>;
  /** Removes the guest's saved ID details. */
  deleteProfile(): Promise<void>;
  /**
   * Closes the guest's account for good (the profile and ID, phone numbers and notes on bookings, sessions). Fails with ACTIVE_BOOKINGS while a booking
   * is held, paid, confirmed or checked in: cancel it first. Bookings and payment records are kept.
   */
  deleteAccount(): Promise<void>;
  /**
   * Help and support. A guest asks a question or reports a problem (optionally about their own booking) and support answers; the guest
   * is emailed the answer. Starting one fails with TOO_MANY_TICKETS at five requests still waiting. A closed request takes no more messages.
   */
  listSupportTickets(): Promise<SupportTicket[]>;
  /** One request with its messages (support's staff shown only as "Support"). */
  getSupportTicket(id: string): Promise<SupportTicket>;
  createSupportTicket(ticket: NewSupportTicket): Promise<{ id: string }>;
  replySupportTicket(id: string, message: string): Promise<void>;
  closeSupportTicket(id: string): Promise<void>;
  /**
   * Tells BookDan the guest tapped "Call the hotel" on a confirmed booking (so it can see how often guests and hotels talk outside the app).
   * The hotel's number is not on its page: it comes with the booking once the hotel has confirmed (booking.property_phone, empty before).
   */
  logHotelCall(bookingId: string): Promise<void>;
  /**
   * Challenges a charge or a stay. The booking becomes DISPUTED (its money is held back from the hotel) and a support request carries the
   * conversation (booking.dispute_ticket_id). Fails with INVALID_STATE before the stay has started, DISPUTE_WINDOW_CLOSED after
   * 14 days past check-out, and DISPUTE_EXISTS when one was opened before. Closing the support request withdraws the dispute.
   */
  raiseDispute(bookingId: string, dispute: { reason: DisputeReason; message: string }): Promise<Booking>;
  /**
   * What moving a confirmed booking to other dates would do: nothing changes (same_price), the difference goes back (refund), the guest pays the
   * difference (pay), or the difference is settled at the hotel (reprice). Changes nothing on the server.
   */
  quoteDateChange(bookingId: string, dates: { check_in: string; check_out: string }): Promise<DateChangeQuote>;
  /**
   * Moves a confirmed booking to other dates, once (booking.can_change_dates says when). When the price differs, confirm_total_santim must be the new total
   * from the quote (else PRICE_DIFFERS). A dearer stay does not move yet: the booking comes back with pending_date_change and the page to pay the difference
   * on, and moves when that payment arrives. Fails with SOLD_OUT, CHANGE_LIMIT, CHANGE_PENDING, CHANGE_TOO_LATE, NEW_DATES_TOO_SOON (both check-ins must be
   * more than 48 hours away), NON_REFUNDABLE and COUPON_NO_CHANGE. Returns the updated booking.
   */
  changeBookingDates(bookingId: string, dates: { check_in: string; check_out: string; confirm_total_santim?: number; method?: PaymentMethod }): Promise<Booking>;
  /** Starts (or resumes) the payment of a change of dates that waits for it. Returns the booking. */
  payDateChange(bookingId: string, method?: PaymentMethod): Promise<Booking>;
  /** Drops a change of dates that waits for its payment; the booking stays as it was. Returns the booking. */
  cancelDateChange(bookingId: string): Promise<Booking>;
  /**
   * Syncs the saved hotels (wishlist) with the server: send the hotel ids saved on the phone and the ones removed since the last sync; the answer is the whole
   * list the server now holds (newest first), which the phone keeps. Needs a signed-in guest.
   */
  syncSaved(change: { add: string[]; remove: string[] }): Promise<string[]>;
  /** Registers this phone for booking updates (the hotel confirmed or declined). Re-sent whenever the token changes. */
  registerPushToken(registration: PushRegistration): Promise<void>;
  /**
   * Cancels a booking that is waiting for the hotel or confirmed. The server applies the refund rule: everything back while the hotel
   * has not answered, and after it confirmed everything back until 48 hours before check-in, nothing after. Fails with CANCEL_TOO_LATE
   * once the stay has started and INVALID_STATE for a booking that cannot be cancelled. Returns the updated booking.
   */
  cancelBooking(id: string): Promise<Booking>;
  /** Reviews a finished stay (1 to 5 stars, optional text). Fails with REVIEW_NOT_READY before check-out and ALREADY_REVIEWED. */
  submitReview(id: string, review: ReviewInput): Promise<Booking>;
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
  /**
   * Shows the full ID number of the guest on one booking. Audited on the server; only staff of the hotel that holds the
   * booking. Fails with GONE once the number was erased after the retention period.
   */
  revealGuestId(bookingId: string): Promise<{ id_number: string }>;
  /** The hotel's bookings: those waiting for an answer, upcoming stays, and recent ones. Each carries the guest's profile (masked ID). */
  listBookings(): Promise<PartnerBookingList>;
  /** Accept a booking. Fails with DEADLINE_PASSED once the time to answer is over (the guest is refunded). */
  acceptBooking(id: string): Promise<PartnerBooking>;
  /** Decline a booking, with a reason. The guest is refunded in full. */
  declineBooking(id: string, reason: DeclineReason): Promise<PartnerBooking>;
  /**
   * Cancels a confirmed booking the hotel cannot honour. Hotel owners only (FORBIDDEN for others). The guest is refunded in full and the
   * hotel bears the refund. Fails with INVALID_STATE for a booking that is not confirmed or whose stay is over.
   */
  cancelBooking(id: string, reason: HotelProblemReason): Promise<PartnerBooking>;
  /** Reports that the hotel cannot host the guest: BookDan support finds another place, and the guest may cancel for a full refund. */
  relocateBooking(id: string, reason: HotelProblemReason): Promise<PartnerBooking>;
  /** The guest's dispute about this booking and the hotel's statements. Fails with NOT_FOUND when there is none. */
  getDispute(bookingId: string): Promise<HotelDispute>;
  /** The hotel's account of a dispute, shown to support and to the guest. Fails with INVALID_STATE when no dispute is open. */
  sendDisputeStatement(bookingId: string, message: string): Promise<void>;
  /** The statement of one month (YYYY-MM; default last month) for each hotel the owner owns, or one hotel with propertyId. Owners only. */
  getStatements(month?: string, propertyId?: string): Promise<Statement[]>;
  /** The latest reviews of the owner's hotels, those without a reply first. Owners only (FORBIDDEN for others). */
  listReviews(): Promise<PartnerReview[]>;
  /** Writes (or replaces) the hotel's public reply to a review. Owners only. */
  replyToReview(reviewId: string, text: string): Promise<void>;
  /** Takes the hotel's reply away. Owners only. */
  removeReviewReply(reviewId: string): Promise<void>;
  /** The server's public key for checking voucher QR codes (cached by the app so check-in can work offline). Public. */
  getVoucherKey(): Promise<{ kid: string; public_key: string }>;
  /** Sends check-ins that were made offline (POST /partner/checkin/sync). One answer per item, in order. */
  syncCheckins(items: { token: string; at: string }[]): Promise<{ reference: string; result: string }[]>;
  /** Help and support for hotel staff: the same conversations as in the guest app, about the hotel or one of its bookings. */
  listSupportTickets(): Promise<SupportTicket[]>;
  getSupportTicket(id: string): Promise<SupportTicket>;
  createSupportTicket(ticket: NewSupportTicket): Promise<{ id: string }>;
  replySupportTicket(id: string, message: string): Promise<void>;
  closeSupportTicket(id: string): Promise<void>;

  /**
   * Finds the booking behind a guest's voucher (the text in their QR code, or reference + PIN) and says whether it can be checked
   * in. Changes nothing. Fails with INVALID_VOUCHER, VOUCHER_EXPIRED, WRONG_HOTEL, INVALID_PIN or PIN_LOCKED.
   */
  lookupCheckin(query: CheckinQuery): Promise<CheckinLookup>;
  /** What the owner's hotels have earned and been paid. Fails with FORBIDDEN for anyone who is not an owner. */
  getEarnings(): Promise<Earnings>;
  /** The rooms chart: free rooms per room type per night, from the same inventory the guest app sells from. */
  getRoomsChart(opts?: { property_id?: string; from?: string; days?: number }): Promise<RoomsChart>;
  /** The walk-ins and blocks that are still on. */
  listRoomBlocks(property_id?: string): Promise<RoomBlock[]>;
  /** Records a walk-in guest or blocks rooms. Fails with SOLD_OUT when the rooms are not free on every night. */
  createRoomBlock(input: NewRoomBlock): Promise<void>;
  /** Gives the rooms back. Harmless to repeat. */
  releaseRoomBlock(id: string): Promise<void>;
  /** The guest never came: the hotel keeps its share and the nights still to come are free again. Only from the check-in day. */
  markNoShow(bookingId: string): Promise<PartnerBooking>;
  /** Marks the guest as arrived. Harmless to repeat. Fails with INVALID_STATE when the booking cannot be checked in. */
  checkInGuest(id: string): Promise<PartnerBooking>;

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
      | 'SOLD_OUT' | 'QUOTE_EXPIRED' | 'HOLD_EXPIRED' | 'IDEMPOTENCY_MISMATCH' | 'VOUCHER_NOT_READY' | 'PAYMENT_FAILED' | 'DEADLINE_PASSED' | 'INVALID_STATE' | 'INVALID_ID' | 'GONE' | 'PROFILE_INCOMPLETE'
      | (string & {}),
    message: string,
    public request_id?: string,
  ) {
    super(message);
  }
}
