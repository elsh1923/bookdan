import {
  BookingSchema,
  CheckinLookupSchema,
  SuggestResponseSchema,
  EarningsSchema,
  RoomBlockListSchema,
  RoomsChartSchema,
  GuestProfileSchema,
  PartnerBookingListSchema,
  PartnerBookingSchema,
  PaymentSessionSchema,
  PropertyDetailSchema,
  QuoteSchema,
  SearchResponseSchema,
  VoucherSchema,
  type Identity,
} from '@bookdan/domain';
import { z } from 'zod';
import { ApiError, type Api, type AuthApi, type PartnerApi, type SearchParams, type Tokens } from './types';

/**
 * The real client: the same Api and PartnerApi contracts as the mocks, over HTTP to the Go backend (backend/api/openapi.yaml).
 * Every response is checked against its schema, so a server that drifts from the contract fails loudly here instead of
 * showing broken screens. Errors arrive in the backend's one shape and become ApiError with the same stable codes.
 */
export interface HttpOptions {
  /** For example https://api.bookdan.com (no trailing slash, no /v1). */
  baseUrl: string;
  app: 'guest' | 'partner';
  /** A stable id for this install, or a function that finds one. At least 8 characters. */
  deviceId: string | (() => Promise<string>);
  platform: 'android' | 'ios' | 'web';
  timeoutMs?: number;
  fetchImpl?: typeof fetch;
}

const ErrorBody = z.object({ error: z.object({ code: z.string(), message: z.string(), request_id: z.string().optional() }) });

function createCore(o: HttpOptions) {
  let token: string | null = null;
  const doFetch = o.fetchImpl ?? fetch;
  const base = o.baseUrl.replace(/\/+$/, '') + '/v1';

  async function request<S extends z.ZodTypeAny>(
    method: 'GET' | 'POST' | 'PUT' | 'DELETE',
    path: string,
    schema: S | null,
    body?: unknown,
    headers: Record<string, string> = {},
    timeoutMs?: number,
  ): Promise<z.infer<S>> {
    const deviceId = typeof o.deviceId === 'string' ? o.deviceId : await o.deviceId();
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs ?? o.timeoutMs ?? 15_000);
    let res: Response;
    try {
      res = await doFetch(base + path, {
        method,
        signal: controller.signal,
        headers: {
          'Content-Type': 'application/json',
          'X-App': o.app,
          'X-Device-Id': deviceId,
          'X-Platform': o.platform,
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
          ...headers,
        },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch {
      throw new ApiError('NETWORK', 'No connection'); // offline, DNS, timeout
    } finally {
      clearTimeout(timer);
    }

    let json: unknown = null;
    try {
      json = await res.json();
    } catch {
      // not JSON: handled below
    }
    if (!res.ok) {
      const parsed = ErrorBody.safeParse(json);
      if (parsed.success) {
        const { code, message, request_id } = parsed.data.error;
        throw new ApiError(code === 'INTERNAL' ? 'SERVER' : code, message, request_id);
      }
      throw new ApiError(res.status >= 500 ? 'SERVER' : 'INVALID_STATE', `Unexpected response (${res.status})`);
    }
    if (!schema) return undefined as z.infer<S>;
    const ok = schema.safeParse(json);
    if (!ok.success) throw new ApiError('SERVER', 'The server answered in an unexpected shape');
    return ok.data;
  }

  const auth: AuthApi = {
    setToken(t) {
      token = t;
    },
    async requestOtp(identity: Identity) {
      // Sending the email or SMS can take a few seconds: give it 30.
      await request('POST', '/auth/otp/request', null, { type: identity.type, value: identity.value }, {}, 30_000);
    },
    async verifyOtp(identity, code) {
      const r = await request('POST', '/auth/otp/verify', z.object({ token: z.string(), refresh_token: z.string() }), {
        type: identity.type, value: identity.value, code,
      });
      return { token: r.token, refresh_token: r.refresh_token, identity };
    },
    async refresh(refreshToken): Promise<Tokens> {
      const r = await request('POST', '/auth/refresh', z.object({ token: z.string(), refresh_token: z.string() }), { refresh_token: refreshToken });
      return { token: r.token, refresh_token: r.refresh_token };
    },
    async logout(refreshToken) {
      await request('POST', '/auth/logout', null, { refresh_token: refreshToken });
    },
  };
  return { request, auth };
}

function searchQuery(p: SearchParams): string {
  const q = new URLSearchParams({ check_in: p.check_in, check_out: p.check_out, guests: String(p.guests) });
  if (p.q) q.set('q', p.q);
  if (p.rooms && p.rooms > 1) q.set('rooms', String(p.rooms));
  if (p.types?.length) q.set('types', p.types.join(','));
  if (p.amenities?.length) q.set('amenities', p.amenities.join(','));
  if (p.sort && p.sort !== 'recommended') q.set('sort', p.sort);
  if (p.min_price) q.set('min_price', String(p.min_price));
  if (p.max_price) q.set('max_price', String(p.max_price));
  if (p.min_rating) q.set('min_rating', String(p.min_rating));
  if (p.breakfast) q.set('breakfast', '1');
  if (p.deals) q.set('deals', '1');
  if (p.lat != null && p.lng != null) {
    q.set('lat', String(p.lat));
    q.set('lng', String(p.lng));
  }
  if (p.cursor) q.set('cursor', p.cursor);
  return q.toString();
}

const BookingList = z.object({ items: z.array(BookingSchema) });
const ProfileEnvelope = z.object({ profile: GuestProfileSchema.nullable() });
const PayResponse = z.object({ booking: BookingSchema, session: PaymentSessionSchema.nullable() });

export function createHttpApi(options: HttpOptions): Api {
  const { request, auth } = createCore({ ...options, app: 'guest' });
  return {
    ...auth,
    search: (p) => request('GET', `/search?${searchQuery(p)}`, SearchResponseSchema),
    getProperty: (p) =>
      request('GET', `/properties/${encodeURIComponent(p.id)}?${new URLSearchParams({ check_in: p.check_in, check_out: p.check_out })}`, PropertyDetailSchema),
    createQuote: (p) => request('POST', '/quotes', QuoteSchema, p),
    suggest: async (q) => (await request('GET', `/suggest?${new URLSearchParams({ q })}`, SuggestResponseSchema)).items,
    createHold: (p) =>
      request(
        'POST', '/bookings', BookingSchema,
        { quote_id: p.quote_id, guest_name: p.guest_name, special_requests: p.special_requests, booked_for_other: p.booked_for_other, guest_phone: p.guest_phone },
        { 'Idempotency-Key': p.idempotency_key },
      ),
    startPayment: (p) => request('POST', `/bookings/${encodeURIComponent(p.booking_id)}/pay`, PayResponse, { method: p.method }),
    getBooking: (id) => request('GET', `/bookings/${encodeURIComponent(id)}`, BookingSchema),
    listBookings: async () => (await request('GET', '/bookings', BookingList)).items,
    getProfile: async () => (await request('GET', '/me/profile', ProfileEnvelope)).profile,
    saveProfile: async (input) => {
      const r = await request('PUT', '/me/profile', ProfileEnvelope, input);
      if (!r.profile) throw new ApiError('SERVER', 'The server did not return the saved profile');
      return r.profile;
    },
    deleteProfile: async () => {
      await request('DELETE', '/me/profile', null);
    },
    registerPushToken: async (r) => {
      await request('PUT', '/me/devices/push-token', null, r);
    },
    cancelBooking: (id) => request('POST', `/bookings/${encodeURIComponent(id)}/cancel`, BookingSchema),
    submitReview: (id, review) => request('POST', `/bookings/${encodeURIComponent(id)}/review`, BookingSchema, review),
    getVoucher: (id) => request('GET', `/bookings/${encodeURIComponent(id)}/voucher`, VoucherSchema),
    // No demo controls: those exist only on the mock.
  };
}

export function createHttpPartnerApi(options: HttpOptions): PartnerApi {
  const { request, auth } = createCore({ ...options, app: 'partner' });
  return {
    ...auth,
    registerPushToken: async (r) => {
      await request('PUT', '/partner/devices/push-token', null, r);
    },
    revealGuestId: (id) => request('POST', `/partner/bookings/${encodeURIComponent(id)}/guest/reveal`, z.object({ id_number: z.string() })),
    listBookings: () => request('GET', '/partner/bookings', PartnerBookingListSchema),
    acceptBooking: (id) => request('POST', `/partner/bookings/${encodeURIComponent(id)}/accept`, PartnerBookingSchema),
    declineBooking: (id, reason) => request('POST', `/partner/bookings/${encodeURIComponent(id)}/decline`, PartnerBookingSchema, { reason }),
    lookupCheckin: (query) => request('POST', '/partner/checkin/lookup', CheckinLookupSchema, query),
    getEarnings: () => request('GET', '/partner/earnings', EarningsSchema),
    getRoomsChart: (o) => {
      const q = new URLSearchParams();
      if (o?.property_id) q.set('property_id', o.property_id);
      if (o?.from) q.set('from', o.from);
      if (o?.days) q.set('days', String(o.days));
      return request('GET', `/partner/rooms?${q}`, RoomsChartSchema);
    },
    listRoomBlocks: async (property_id) => (await request('GET', `/partner/rooms/blocks?${new URLSearchParams(property_id ? { property_id } : {})}`, RoomBlockListSchema)).blocks,
    createRoomBlock: async (input) => {
      await request('POST', '/partner/rooms/blocks', null, input);
    },
    releaseRoomBlock: async (id) => {
      await request('DELETE', `/partner/rooms/blocks/${encodeURIComponent(id)}`, null);
    },
    markNoShow: (id) => request('POST', `/partner/bookings/${encodeURIComponent(id)}/no-show`, PartnerBookingSchema),
    checkInGuest: (id) => request('POST', `/partner/bookings/${encodeURIComponent(id)}/check-in`, PartnerBookingSchema),
  };
}
