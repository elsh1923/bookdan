import {
  BookingSchema,
  PaymentSessionSchema,
  PropertyDetailSchema,
  QuoteSchema,
  SearchResponseSchema,
  VoucherSchema,
  GuestProfileInputSchema,
  GuestProfileSchema,
  encodeVoucherToken,
  idLast4,
  normalizeIdNumber,
  nightsBetween,
  priceStay,
  type Booking,
  type GuestProfile,
  type PaymentMethod,
  type PaymentSession,
  type PropertyDetail,
  type Quote,
  type SearchItem,
  type SearchResponse,
  type Voucher,
} from '@bookdan/domain';
import { createMockAuth } from './mockAuth';
import { SEED } from './seed';
import { ApiError, type Api, type CreateQuoteParams, type GetPropertyParams, type SearchParams } from './types';

const PAGE_SIZE = 5;
const HOLD_MS = 15 * 60_000;
const QUOTE_MS = 10 * 60_000;
const CONFIRM_AFTER_MS = 8_000; // simulated hotel response time
export { MOCK_OTP } from './mockAuth';

// Landmarks guests actually search by, mapped to the property that sits next to them.
const LANDMARKS: Record<string, string[]> = {
  'meskel square': ['p-addis-1'],
  'bole airport': ['p-addis-2', 'p-addis-1'],
  'bole': ['p-addis-2'],
  'piassa': ['p-addis-3'],
  'lake tana': ['p-bahirdar-1'],
  'bete giyorgis': ['p-lalibela-1'],
  'fasil': ['p-gondar-1'],
};

// Well-known places with their coordinates, for "1.4 km to Meskel Square" and the nearby list.
const PLACES = [
  { name: 'Meskel Square', name_am: 'መስቀል አደባባይ', lat: 9.0108, lng: 38.7613 },
  { name: 'Bole Airport', name_am: 'ቦሌ አየር ማረፊያ', lat: 8.9779, lng: 38.7993 },
  { name: 'Piassa', name_am: 'ፒያሳ', lat: 9.0346, lng: 38.7469 },
  { name: 'Lake Tana', name_am: 'ጣና ሐይቅ', lat: 11.6, lng: 37.39 },
  { name: 'Bete Giyorgis', name_am: 'ቤተ ጊዮርጊስ', lat: 12.0319, lng: 39.0473 },
  { name: 'Fasil Ghebbi', name_am: 'ፋሲል ግቢ', lat: 12.6074, lng: 37.4666 },
];

function metres(lat1: number, lng1: number, lat2: number, lng2: number): number {
  const rad = Math.PI / 180;
  const a = Math.sin(((lat2 - lat1) * rad) / 2) ** 2 + Math.cos(lat1 * rad) * Math.cos(lat2 * rad) * Math.sin(((lng2 - lng1) * rad) / 2) ** 2;
  return Math.round(2 * 6371000 * Math.asin(Math.min(1, Math.sqrt(a))));
}

function placesNear(lat: number, lng: number, maxM: number, limit: number) {
  return PLACES.map((p) => ({ name: p.name, name_am: p.name_am, distance_m: metres(lat, lng, p.lat, p.lng) }))
    .filter((p) => p.distance_m <= maxM)
    .sort((a, b) => a.distance_m - b.distance_m)
    .slice(0, limit);
}

// What the demo hotels offer besides the plain room: breakfast at 150 birr a night and 10% off for no refund (not on the Family Suite).
const BREAKFAST_SANTIM = 15_000;
const NONREFUNDABLE_BPS = 1000;
const offersExtras = (roomName: string) => roomName !== 'Family Suite';
function roomOptions(room: { name: string; price_santim: number }) {
  const base = room.price_santim;
  const cheaper = Math.round((base * (10000 - NONREFUNDABLE_BPS)) / 10000);
  const out: { meal: 'room_only' | 'breakfast'; refundable: boolean; price_santim: number }[] = [{ meal: 'room_only', refundable: true, price_santim: base }];
  if (offersExtras(room.name)) {
    out.push({ meal: 'room_only', refundable: false, price_santim: cheaper });
    out.push({ meal: 'breakfast', refundable: true, price_santim: base + BREAKFAST_SANTIM });
    out.push({ meal: 'breakfast', refundable: false, price_santim: cheaper + BREAKFAST_SANTIM });
  }
  return out;
}

// Demo coupons: WELCOME10 is 10% off, SAVE200 takes 200 birr off.
const COUPONS: Record<string, { percentBp?: number; fixed?: number }> = { WELCOME10: { percentBp: 1000 }, SAVE200: { fixed: 20_000 } };

const MAX_ROOMS = 9;
const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));

function toItem(s: (typeof SEED)[number]): SearchItem {
  const { rooms: _r, reviews: _v, description: _d, phone: _p, address: _a, house_rules: _h, photos: _ph, ...item } = s as any;
  const cheapest = [...s.rooms].sort((a, b) => a.price_santim - b.price_santim).find((r) => r.rooms_left > 0);
  return { ...item, rooms_left: cheapest?.rooms_left ?? 0, landmark: placesNear(s.lat, s.lng, 15000, 1)[0] ?? null } as SearchItem;
}

function summarize(reviews: { rating: number }[]) {
  const distribution = [0, 0, 0, 0, 0];
  for (const r of reviews) distribution[Math.min(5, Math.max(1, Math.round(r.rating))) - 1]++;
  const average = reviews.length ? Math.round((reviews.reduce((a, r) => a + r.rating, 0) / reviews.length) * 10) / 10 : null;
  return { average, count: reviews.length, distribution, categories: { cleanliness: null, location: null, staff: null, value: null } };
}

function matches(q: string, s: (typeof SEED)[number]): boolean {
  const needle = q.trim().toLowerCase();
  if (!needle) return true;
  const hit = Object.entries(LANDMARKS).find(([k]) => needle.includes(k) || k.includes(needle));
  if (hit && hit[1].includes(s.id)) return true;
  return [s.name, s.area, s.city].some((f) => f.toLowerCase().includes(needle));
}

export interface MockStore {
  load(): Promise<string | null>;
  save(json: string): Promise<void>;
}

export interface MockOptions {
  /** Override the hold length, e.g. 20_000 to see the expiry state quickly. */
  holdMs?: number;
  /** Access-token lifetime, e.g. 20_000 to watch silent refresh happen. */
  accessTtlMs?: number;
  latencyMs?: number;
  /** 'network' or 'server' makes every call fail, for designing error states. */
  failWith?: 'network' | 'server' | null;
  /** Keeps the mock's bookings across app restarts, so a demo can survive the app being killed. */
  store?: MockStore;
}

interface Row {
  b: Booking;
  owner: string;
  created: number;
  paid_at: number | null;
  holdMs: number;
  pin: string | null;
  session_id: string | null; // gateway session while a payment is pending
}

interface SessionInfo {
  booking_id: string;
  method: PaymentMethod;
}

export function createMockApi(opts: MockOptions = {}): Api {
  const latency = opts.latencyMs ?? 450;
  const holdLength = opts.holdMs ?? HOLD_MS;

  let seq = 0;
  const quotes = new Map<string, Quote & { expires: number }>();
  const rows = new Map<string, Row>();
  // idempotency: owner + key -> what the first request asked for and the booking it created
  const holdKeys = new Map<string, { quote_id: string; guest_name: string; booking_id: string }>();
  const sessions = new Map<string, SessionInfo>();
  // The guest's saved identity details, by who is signed in. DEMO ONLY: the real server stores the number encrypted
  // and the app never gets it back; this mock keeps it so the hotel's copy can be shown in demos.
  interface ProfileRow { full_name: string; id_type: GuestProfile['id_type']; id_number: string; nationality: string; updated_at: string }
  const profiles = new Map<string, ProfileRow>();

  // ---- optional persistence (demo only)
  let hydration: Promise<void> | null = null;
  const ready = () =>
    (hydration ??= (async () => {
      const raw = await opts.store?.load();
      if (!raw) return;
      try {
        const s = JSON.parse(raw);
        seq = s.seq ?? 0;
        for (const [k, v] of s.quotes ?? []) quotes.set(k, v);
        for (const [k, v] of s.rows ?? []) rows.set(k, v);
        for (const [k, v] of s.holdKeys ?? []) holdKeys.set(k, v);
        for (const [k, v] of s.sessions ?? []) sessions.set(k, v);
        for (const [k, v] of s.profiles ?? []) profiles.set(k, v);
      } catch {
        // a damaged demo store is simply ignored
      }
    })());
  const persist = async () => {
    await opts.store?.save(JSON.stringify({ seq, quotes: [...quotes], rows: [...rows], holdKeys: [...holdKeys], sessions: [...sessions], profiles: [...profiles] }));
  };

  async function gate() {
    await ready();
    await wait(latency);
    if (opts.failWith === 'network') throw new ApiError('NETWORK', 'No connection');
    if (opts.failWith === 'server') throw new ApiError('SERVER', 'Server error', 'req_mock');
  }

  // ---- sign-in, expiring tokens and refresh are shared with the partner mock
  const auth = createMockAuth({ accessTtlMs: opts.accessTtlMs, gate });
  const identityOf = auth.identityOf;

  // Status is derived from time on every read, the way the server would report it.
  function view(r: Row): Booking {
    const now = Date.now();
    let status = r.b.status;
    if (status === 'HELD' && now - r.created >= r.holdMs) status = 'EXPIRED';
    if (status === 'PAID_AWAITING_HOTEL' && r.paid_at && now - r.paid_at >= CONFIRM_AFTER_MS) status = 'CONFIRMED';
    r.b = { ...r.b, status };
    // The cancellation rules, as the server applies them: free until 48 hours before check-in (12:00 Addis time), nothing after.
    const paid = r.b.payment_method && r.b.payment_method !== 'pay_at_hotel' && (status === 'PAID_AWAITING_HOTEL' || status === 'CONFIRMED') ? r.b.total_santim : 0;
    const checkInAt = Date.parse(`${r.b.check_in}T12:00:00+03:00`);
    const until = checkInAt - 48 * 3_600_000;
    const cancellable = (status === 'PAID_AWAITING_HOTEL' || status === 'CONFIRMED') && now < checkInAt;
    // A non-refundable booking gets nothing back once the hotel has confirmed it.
    const refund = status === 'PAID_AWAITING_HOTEL' || (r.b.refundable && now <= until) ? paid : 0;
    return BookingSchema.parse({
      ...r.b,
      can_cancel: cancellable,
      refund_if_cancelled_santim: cancellable ? refund : 0,
      full_refund_until: status === 'CONFIRMED' && paid > 0 && r.b.refundable ? new Date(until).toISOString() : null,
      // Demo shortcut: a real review waits for check-out; in the demo a confirmed booking can be reviewed straight away.
      can_review: (status === 'CONFIRMED' || status === 'CHECKED_IN' || status === 'COMPLETED') && !r.b.review,
      server_time: new Date(now).toISOString(),
    });
  }

  const settle = (r: Row, method: PaymentMethod) => {
    r.paid_at = Date.now();
    r.session_id = null;
    r.b = { ...r.b, status: 'PAID_AWAITING_HOTEL', payment_method: method, hold_expires_at: null, payment: 'none' };
  };

  // What the guest gets back about their own profile: never the full number.
  const toProfile = (p: ProfileRow): GuestProfile =>
    GuestProfileSchema.parse({ full_name: p.full_name, id_type: p.id_type, id_last4: idLast4(p.id_number), nationality: p.nationality, updated_at: p.updated_at, server_time: new Date().toISOString() });

  const roomsLeft = (propertyId: string, roomId: string) => {
    const room = SEED.find((x) => x.id === propertyId)?.rooms.find((x) => x.id === roomId);
    if (!room) return 0;
    const live = [...rows.values()].filter((r) => {
      const v = view(r);
      return v.property_id === propertyId && v.room_name === room.name && ['HELD', 'PAID_AWAITING_HOTEL', 'CONFIRMED'].includes(v.status);
    }).reduce((sum, r) => sum + view(r).rooms, 0);
    return room.rooms_left - live;
  };

  const sessionFor = (r: Row, method: PaymentMethod): PaymentSession =>
    PaymentSessionSchema.parse({ id: r.session_id, booking_id: r.b.id, method, payment_url: `mock://gateway/${r.session_id}` });

  const owned = (id: string): Row => {
    const owner = identityOf();
    const r = rows.get(id);
    if (!r || r.owner !== owner) throw new ApiError('NOT_FOUND', 'Booking not found');
    return r;
  };

  return {
    ...auth.api,

    async createQuote(p: CreateQuoteParams): Promise<Quote> {
      await gate();
      identityOf();
      const s = SEED.find((x) => x.id === p.property_id);
      const room = s?.rooms.find((x) => x.id === p.room_type_id);
      if (!s || !room) throw new ApiError('NOT_FOUND', 'Room not found');
      const rooms = p.rooms ?? 1;
      if (!Number.isInteger(rooms) || rooms < 1 || rooms > MAX_ROOMS) throw new ApiError('INVALID_STATE', 'Choose between 1 and 9 rooms');
      if (p.guests < rooms) throw new ApiError('INVALID_STATE', 'Each room needs at least one guest');
      const available = roomsLeft(s.id, room.id);
      if (available < rooms) throw new ApiError('SOLD_OUT', 'Not enough rooms left');
      const nights = nightsBetween(p.check_in, p.check_out);
      const meal = p.meal ?? 'room_only';
      const refundable = p.refundable ?? true;
      const option = roomOptions(room).find((o) => o.meal === meal && o.refundable === refundable);
      if (!option) throw new ApiError('OPTION_UNAVAILABLE', 'This room does not offer that option');
      const list = priceStay(option.price_santim * rooms, nights);
      let priced = list;
      let couponCode: string | null = null;
      if (p.coupon_code?.trim()) {
        const code = p.coupon_code.trim().toUpperCase();
        const c = COUPONS[code];
        if (!c) throw new ApiError('COUPON_INVALID', 'That coupon code is not valid');
        const off = Math.min(list.subtotal_santim, c.percentBp ? Math.round((list.subtotal_santim * c.percentBp) / 10000) : (c.fixed ?? 0));
        priced = priceStay(list.subtotal_santim - off, 1);
        couponCode = code;
      }
      const q = QuoteSchema.parse({
        id: `q_${++seq}_${Date.now().toString(36)}`,
        property_id: s.id, property_name: s.name,
        room_type_id: room.id, room_name: room.name,
        check_in: p.check_in, check_out: p.check_out, nights, guests: p.guests, rooms, rooms_available: available,
        nightly_santim: option.price_santim, ...priced, currency: room.currency,
        pay_at_hotel: s.amenities.includes('pay_at_hotel') && !couponCode, // a coupon is for online payment only
        meal, refundable, coupon_code: couponCode, discount_santim: list.total_santim - priced.total_santim, list_total_santim: list.total_santim,
        server_time: new Date().toISOString(),
      });
      quotes.set(q.id, { ...q, expires: Date.now() + QUOTE_MS });
      await persist();
      return q;
    },

    async getProfile() {
      await gate();
      const p = profiles.get(identityOf());
      return p ? toProfile(p) : null;
    },

    async saveProfile(input) {
      await gate();
      const owner = identityOf();
      const parsed = GuestProfileInputSchema.safeParse(input);
      if (!parsed.success) throw new ApiError('INVALID_STATE', 'Name, ID type and nationality are required');
      const number = normalizeIdNumber(parsed.data.id_number);
      if (!number) throw new ApiError('INVALID_ID', 'That cannot be an ID number');
      const row = { full_name: parsed.data.full_name, id_type: parsed.data.id_type, id_number: number, nationality: parsed.data.nationality, updated_at: new Date().toISOString() };
      profiles.set(owner, row);
      await persist();
      return toProfile(row);
    },

    async cancelBooking(id) {
      await gate();
      const r = owned(id);
      const v = view(r);
      if (!v.can_cancel) throw new ApiError(v.status === 'CONFIRMED' || v.status === 'PAID_AWAITING_HOTEL' ? 'CANCEL_TOO_LATE' : 'INVALID_STATE', 'This booking cannot be cancelled');
      r.b = { ...r.b, status: 'CANCELLED_REFUNDED', refund_santim: v.refund_if_cancelled_santim };
      await persist();
      return view(r);
    },

    async submitReview(id, review) {
      await gate();
      const r = owned(id);
      if (!view(r).can_review) throw new ApiError(r.b.review ? 'ALREADY_REVIEWED' : 'REVIEW_NOT_READY', 'This stay cannot be reviewed now');
      r.b = { ...r.b, review: { rating: review.rating, text: review.text } };
      await persist();
      return view(r);
    },

    async registerPushToken() {
      await gate();
      identityOf(); // the mock has nobody to send to; it only checks the guest is signed in
    },

    async deleteProfile() {
      await gate();
      profiles.delete(identityOf());
      await persist();
    },

    async createHold({ quote_id, guest_name, idempotency_key }) {
      await gate();
      const owner = identityOf();
      // A room request carries the guest's profile to the hotel, so there must be one.
      if (!profiles.has(owner)) throw new ApiError('PROFILE_INCOMPLETE', 'Add your name and ID before booking');
      const keyId = `${owner}:${idempotency_key}`;
      const prior = holdKeys.get(keyId);
      if (prior) {
        // A retry of the same attempt: return the booking that already exists, never a second hold.
        if (prior.quote_id !== quote_id || prior.guest_name !== guest_name) {
          throw new ApiError('IDEMPOTENCY_MISMATCH', 'This key was used for a different booking');
        }
        return view(rows.get(prior.booking_id)!);
      }
      const q = quotes.get(quote_id);
      if (!q || q.expires < Date.now()) throw new ApiError('QUOTE_EXPIRED', 'Price expired');
      if (roomsLeft(q.property_id, q.room_type_id) < q.rooms) throw new ApiError('SOLD_OUT', 'Sold out');
      const s = SEED.find((x) => x.id === q.property_id)!;
      const now = Date.now();
      const id = `b_${++seq}_${now.toString(36)}`;
      const b = BookingSchema.parse({
        id, reference: `BD-${Math.random().toString(36).slice(2, 8).toUpperCase()}`,
        status: 'HELD',
        property_id: s.id, property_name: s.name, property_phone: s.phone, address: s.address,
        room_name: q.room_name, check_in: q.check_in, check_out: q.check_out, nights: q.nights, guests: q.guests, rooms: q.rooms,
        guest_name, total_santim: q.total_santim, currency: q.currency,
        meal: q.meal, refundable: q.refundable, coupon_code: q.coupon_code, discount_santim: q.discount_santim,
        payment_method: null,
        pay_at_hotel: q.pay_at_hotel,
        payment: 'none',
        hold_expires_at: new Date(now + holdLength).toISOString(),
        server_time: new Date(now).toISOString(),
      });
      quotes.delete(quote_id);
      rows.set(id, { b, owner, created: now, paid_at: null, holdMs: holdLength, pin: null, session_id: null });
      holdKeys.set(keyId, { quote_id, guest_name, booking_id: id });
      await persist();
      return b;
    },

    async startPayment({ booking_id, method }) {
      await gate();
      const r = owned(booking_id);
      if (view(r).status !== 'HELD') throw new ApiError('HOLD_EXPIRED', 'Hold expired');

      if (method === 'pay_at_hotel') {
        if (!r.b.pay_at_hotel) throw new ApiError('PAYMENT_FAILED', 'This hotel does not take pay-at-hotel bookings');
        settle(r, method);
        await persist();
        return { booking: view(r), session: null };
      }

      // Retrying while a payment is pending returns the same session, so there is never a second charge.
      if (!r.session_id || sessions.get(r.session_id)?.method !== method) {
        r.session_id = `ps_${++seq}_${Date.now().toString(36)}`;
        sessions.set(r.session_id, { booking_id, method });
      }
      r.b = { ...r.b, payment: 'pending' };
      await persist();
      return { booking: view(r), session: sessionFor(r, method) };
    },

    async getBooking(id) {
      await gate();
      return view(owned(id));
    },

    async getVoucher(id): Promise<Voucher> {
      await gate();
      const r = owned(id);
      const v = view(r);
      if (v.status !== 'CONFIRMED' && v.status !== 'CHECKED_IN') throw new ApiError('VOUCHER_NOT_READY', 'Voucher is not ready yet');
      const s = SEED.find((x) => x.id === v.property_id)!;
      r.pin ??= String(1000 + Math.floor(Math.random() * 9000));
      // Valid until the end of the check-out day (Addis Ababa) plus a day of grace.
      const exp = Math.floor(new Date(`${v.check_out}T23:59:59+03:00`).getTime() / 1000) + 86_400;
      // SIMULATED signature: the real server signs with Ed25519 and the partner app verifies it offline.
      const fakeSignature = Uint8Array.from({ length: 64 }, (_, i) => (i * 7 + 3) & 255);
      const tokenText = encodeVoucherToken(
        { v: 1, kid: 'mock-1', pid: v.property_id, code: v.reference, in: v.check_in, out: v.check_out, exp },
        fakeSignature,
      );
      await persist();
      return VoucherSchema.parse({
        booking_id: v.id, reference: v.reference, token: tokenText, pin: r.pin, expires_at: new Date(exp * 1000).toISOString(),
        property_id: v.property_id, property_name: v.property_name, property_phone: v.property_phone, address: v.address,
        lat: s.lat, lng: s.lng, check_in: v.check_in, check_out: v.check_out, nights: v.nights, guests: v.guests, rooms: v.rooms,
        guest_name: v.guest_name,
        balance_due_santim: v.payment_method === 'pay_at_hotel' ? v.total_santim : 0,
        currency: v.currency, server_time: new Date().toISOString(),
      });
    },

    async listBookings() {
      await gate();
      const owner = identityOf();
      return [...rows.values()].filter((r) => r.owner === owner).map(view).reverse();
    },

    async search(p: SearchParams): Promise<SearchResponse> {
      await gate();
      // Direct name/area/city hits rank above "near this landmark" hits.
      const needle = (p.q ?? '').trim().toLowerCase();
      const direct = (s: (typeof SEED)[number]) =>
        needle !== '' && [s.name, s.area, s.city].some((f) => f.toLowerCase().includes(needle));
      let found = SEED.filter((s) => matches(p.q ?? '', s)).sort((a, b) => Number(direct(b)) - Number(direct(a)));
      if (p.types?.length) found = found.filter((s) => p.types!.includes(s.type));
      if (p.amenities?.length) found = found.filter((s) => p.amenities!.every((a) => s.amenities.includes(a)));
      const wanted = p.rooms ?? 1;
      found = found.filter((s) => s.rooms.some((r) => r.capacity * wanted >= p.guests && r.rooms_left >= wanted) || !s.available);

      // Counts behind each filter describe what the words found; price bounds cover the hotels that have a room.
      const facets = { types: {} as Record<string, number>, amenities: {} as Record<string, number> };
      for (const s of found) {
        facets.types[s.type] = (facets.types[s.type] ?? 0) + 1;
        for (const a of s.amenities) facets.amenities[a] = (facets.amenities[a] ?? 0) + 1;
      }
      const prices = found.filter((s) => s.available).map((s) => s.from_price_santim);

      if (p.types?.length) found = found.filter((s) => p.types!.includes(s.type));
      if (p.amenities?.length) found = found.filter((s) => p.amenities!.every((a) => s.amenities.includes(a)));
      if (p.min_rating) found = found.filter((s) => s.rating != null && s.rating >= p.min_rating!);
      if (p.min_price) found = found.filter((s) => s.from_price_santim >= p.min_price!);
      if (p.max_price) found = found.filter((s) => s.from_price_santim <= p.max_price!);

      const sort = p.sort ?? 'recommended';
      const dist = (s: (typeof SEED)[number]) => (p.lat != null && p.lng != null ? metres(p.lat, p.lng, s.lat, s.lng) : 0);
      found = [...found].sort((a, b) => {
        if (a.available !== b.available) return a.available ? -1 : 1; // no room for the party: last, whatever the sort
        if (sort === 'price_asc' && a.from_price_santim !== b.from_price_santim) return a.from_price_santim - b.from_price_santim;
        if (sort === 'price_desc' && a.from_price_santim !== b.from_price_santim) return b.from_price_santim - a.from_price_santim;
        if (sort === 'rating' && (a.rating ?? 0) !== (b.rating ?? 0)) return (b.rating ?? 0) - (a.rating ?? 0);
        if (sort === 'distance' && dist(a) !== dist(b)) return dist(a) - dist(b);
        if (sort === 'recommended') {
          if (direct(a) !== direct(b)) return Number(direct(b)) - Number(direct(a));
          if ((a.rating ?? 0) !== (b.rating ?? 0)) return (b.rating ?? 0) - (a.rating ?? 0);
        }
        return a.name.localeCompare(b.name);
      });

      const start = p.cursor ? Number(p.cursor) : 0;
      const page = found.slice(start, start + PAGE_SIZE);
      const next = start + PAGE_SIZE < found.length ? String(start + PAGE_SIZE) : null;
      return SearchResponseSchema.parse({
        items: page.map((s) => ({ ...toItem(s), distance_m: p.lat != null && p.lng != null ? dist(s) : null })),
        next_cursor: next,
        total: found.length,
        price_min_santim: prices.length ? Math.min(...prices) : 0,
        price_max_santim: prices.length ? Math.max(...prices) : 0,
        facets,
        server_time: new Date().toISOString(),
      });
    },

    async getProperty({ id }: GetPropertyParams): Promise<PropertyDetail> {
      await gate();
      const s = SEED.find((x) => x.id === id);
      if (!s) throw new ApiError('NOT_FOUND', 'Property not found');
      return PropertyDetailSchema.parse({
        property: { ...toItem(s), description: s.description, phone: s.phone, address: s.address, house_rules: s.house_rules, photos: s.photos, check_in_time: '12:00', check_out_time: '11:00' },
        room_types: s.rooms.map((r) => ({ ...r, options: roomOptions(r) })),
        reviews: s.reviews,
        rating_summary: summarize(s.reviews),
        nearby: placesNear(s.lat, s.lng, 5000, 6),
        server_time: new Date().toISOString(),
      });
    },

    demo: {
      async completePayment(sessionId, outcome) {
        await gate();
        const info = sessions.get(sessionId);
        const r = info && rows.get(info.booking_id);
        if (!info || !r) throw new ApiError('NOT_FOUND', 'Payment session not found');
        const applySuccess = () => {
          // A duplicate callback finds the booking already paid and changes nothing (acceptance test M4).
          if (r.b.status === 'HELD' && view(r).status === 'HELD') settle(r, info.method);
        };
        if (outcome === 'failed') {
          if (r.b.status === 'HELD') r.b = { ...r.b, payment: 'failed' };
          r.session_id = null;
        } else {
          applySuccess();
          if (outcome === 'duplicate') applySuccess();
        }
        await persist();
      },
      expireAccessToken: auth.expireAccessToken,
      revokeRefreshToken: auth.revokeRefreshToken,
    },
  };
}
