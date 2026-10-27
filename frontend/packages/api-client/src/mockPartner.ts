import {
  DeclineReasonSchema,
  HotelProblemReasonSchema,
  PartnerBookingListSchema,
  PartnerBookingSchema,
  addDays,
  decodeVoucherPayload,
  EarningsSchema,
  RoomBlockListSchema,
  RoomsChartSchema,
  idLast4,
  priceStay,
  todayAddis,
  type CheckinState,
  type IdType,
  type NewSupportTicket,
  type PartnerBooking,
  type SupportTicket,
} from '@bookdan/domain';
import type { MockStore } from './mock';
import { createMockAuth } from './mockAuth';
import { ApiError, type PartnerApi, type PushRegistration } from './types';

const HOTEL = { id: 'p-addis-1', name: 'Meskel Square Palace Hotel' };
const ANSWER_WINDOW_MS = 15 * 60_000; // the hotel has 15 minutes after the guest pays
const ROOMS: [string, number][] = [['Standard Double', 3200], ['Deluxe King', 4800], ['Family Suite', 7200]];
const TOTAL_ROOMS: Record<string, number> = { 'Standard Double': 5, 'Deluxe King': 2, 'Family Suite': 1 };
interface Block { id: string; kind: 'walk_in' | 'block'; room_type_id: string; check_in: string; check_out: string; rooms: number; guest_name: string; guest_phone: string; note: string; released: boolean }

// Demo guests with the profile they would have saved in the guest app. The numbers are made up.
interface DemoGuest {
  name: string;
  contact: string;
  nationality: string;
  id_type: IdType;
  id_number: string;
}
const GUESTS: DemoGuest[] = [
  { name: 'Hanna Tesfaye', contact: '+251911234567', nationality: 'Ethiopian', id_type: 'national_id', id_number: '482917305561' },
  { name: 'Daniel Mekonnen', contact: 'daniel.m@example.com', nationality: 'Ethiopian', id_type: 'national_id', id_number: '739104286617' },
  { name: 'Selam Bekele', contact: '+251922334455', nationality: 'Ethiopian', id_type: 'driving_licence', id_number: 'DL/0048213' },
  { name: 'Yonas Alemu', contact: '+251933221100', nationality: 'Ethiopian', id_type: 'national_id', id_number: '215604893372' },
  { name: 'Martha Girma', contact: 'martha.g@example.com', nationality: 'Ethiopian', id_type: 'national_id', id_number: '860213547790' },
  { name: 'Sarah Mitchell', contact: 'sarah.mitchell@example.com', nationality: 'British', id_type: 'passport', id_number: 'PA4829173' },
  { name: 'Kenji Watanabe', contact: 'kenji.w@example.com', nationality: 'Japanese', id_type: 'passport', id_number: 'TK8830214' },
  { name: 'Abel Woldemichael', contact: '+251944556677', nationality: 'Ethiopian', id_type: 'residence_permit', id_number: 'RP-2291847' },
];

export interface MockPartnerOptions {
  accessTtlMs?: number;
  latencyMs?: number;
  failWith?: 'network' | 'server' | null;
  /** Keeps the demo bookings across app restarts. */
  store?: MockStore;
}

interface Row {
  id: string;
  reference: string;
  status: PartnerBooking['status'];
  /** DEMO ONLY: the real server keeps the number encrypted and never returns it in lists. */
  guest: DemoGuest & { purged?: boolean };
  room_name: string;
  check_in: string;
  nights: number;
  guests: number;
  /** Rooms of this type (older saved demo rows have none: one). */
  rooms?: number;
  total_santim: number;
  paid_at: number | null;
  deadline: number | null;
  decline_reason: string | null;
  checked_in_at?: number | null;
  pay_at_hotel?: boolean;
}

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));
const pick = <T>(list: T[]): T => list[Math.floor(Math.random() * list.length)];
const guestNamed = (name: string): DemoGuest => GUESTS.find((g) => g.name === name) ?? GUESTS[0];

export function createMockPartnerApi(opts: MockPartnerOptions = {}): PartnerApi {
  const latency = opts.latencyMs ?? 450;
  const rows = new Map<string, Row>();
  const tickets: SupportTicket[] = [];
  const blocks = new Map<string, Block>(); // walk-ins and blocks the hotel recorded (kept for the session)
  const pushTokens = new Map<string, PushRegistration>(); // by device id
  let seq = 0;

  const newRow = (over: Partial<Row> & Pick<Row, 'guest' | 'room_name' | 'check_in' | 'nights'>): Row => {
    const price = ROOMS.find(([n]) => n === over.room_name)?.[1] ?? 3200;
    const id = `pb_${++seq}_${Date.now().toString(36)}`;
    return {
      id,
      reference: `BD-${Math.random().toString(36).slice(2, 8).toUpperCase()}`,
      status: 'CONFIRMED',
      guests: 1 + Math.floor(Math.random() * 2),
      total_santim: priceStay(price * 100 * (over.rooms ?? 1), over.nights).total_santim,
      paid_at: Date.now(),
      deadline: null,
      decline_reason: null,
      ...over,
    };
  };

  function seed() {
    const today = todayAddis();
    const now = Date.now();
    const min = 60_000;
    const rowsToAdd: Row[] = [
      newRow({ guest: guestNamed('Hanna Tesfaye'), room_name: 'Standard Double', check_in: addDays(today, 1), nights: 1, status: 'PAID_AWAITING_HOTEL', paid_at: now - 4 * min, deadline: now - 4 * min + ANSWER_WINDOW_MS }),
      newRow({ guest: guestNamed('Sarah Mitchell'), room_name: 'Deluxe King', check_in: addDays(today, 2), nights: 2, status: 'PAID_AWAITING_HOTEL', paid_at: now - 10 * min, deadline: now - 10 * min + ANSWER_WINDOW_MS }),
      newRow({ guest: guestNamed('Selam Bekele'), room_name: 'Family Suite', check_in: addDays(today, 3), nights: 3, guests: 4, rooms: 2 }),
      newRow({ guest: guestNamed('Yonas Alemu'), room_name: 'Standard Double', check_in: today, nights: 1 }),
      newRow({ guest: guestNamed('Martha Girma'), room_name: 'Standard Double', check_in: addDays(today, 5), nights: 2, status: 'DECLINED_REFUNDED', decline_reason: 'fully_booked' }),
    ];
    for (const r of rowsToAdd) rows.set(r.id, r);
  }

  // ---- optional persistence (demo only)
  let hydration: Promise<void> | null = null;
  const ready = () =>
    (hydration ??= (async () => {
      const raw = await opts.store?.load();
      if (raw) {
        try {
          const s = JSON.parse(raw);
          seq = s.seq ?? 0;
          // Older demo data had no guest profile: start fresh rather than show half a booking.
          const loaded: [string, Row][] = (s.rows ?? []).filter(([, r]: [string, Row]) => !!r.guest);
          for (const [k, v] of loaded) rows.set(k, v);
          for (const [k, v] of s.pushTokens ?? []) pushTokens.set(k, v);
        } catch {
          // a damaged demo store is simply replaced
        }
      }
      if (rows.size === 0) seed();
    })());
  const persist = async () => {
    await opts.store?.save(JSON.stringify({ seq, rows: [...rows], pushTokens: [...pushTokens] }));
  };

  async function gate() {
    await ready();
    await wait(latency);
    if (opts.failWith === 'network') throw new ApiError('NETWORK', 'No connection');
    if (opts.failWith === 'server') throw new ApiError('SERVER', 'Server error', 'req_mock');
  }

  const auth = createMockAuth({ accessTtlMs: opts.accessTtlMs, gate });

  // The deadline is enforced by the server clock: once it passes, the guest is refunded on its own.
  function view(r: Row): PartnerBooking {
    const now = Date.now();
    if (r.status === 'PAID_AWAITING_HOTEL' && r.deadline !== null && now >= r.deadline) {
      r.status = 'DECLINED_REFUNDED';
      r.decline_reason = 'no_response';
    }
    return PartnerBookingSchema.parse({
      id: r.id, reference: r.reference, status: r.status, guest_name: r.guest.name,
      // The profile the guest saved in the guest app, as the hotel sees it: the ID is masked to its last four.
      guest: {
        full_name: r.guest.name, contact: r.guest.contact, nationality: r.guest.nationality,
        id_type: r.guest.id_type, id_last4: idLast4(r.guest.id_number), id_purged: !!r.guest.purged,
      },
      room_name: r.room_name,
      check_in: r.check_in, check_out: addDays(r.check_in, r.nights), nights: r.nights, guests: r.guests, rooms: r.rooms ?? 1,
      total_santim: r.total_santim, currency: 'ETB',
      paid_at: r.paid_at ? new Date(r.paid_at).toISOString() : null,
      hotel_deadline_at: r.deadline ? new Date(r.deadline).toISOString() : null,
      decline_reason: r.decline_reason,
      checked_in_at: r.checked_in_at ? new Date(r.checked_in_at).toISOString() : null,
      balance_due_santim: r.pay_at_hotel ? r.total_santim : 0,
      server_time: new Date(now).toISOString(),
    });
  }

  function checkinState(r: Row): CheckinState {
    const v = view(r);
    if (v.status === 'CHECKED_IN' || v.status === 'COMPLETED') return 'checked_in';
    if (v.status === 'PAID_AWAITING_HOTEL') return 'waiting';
    if (v.status !== 'CONFIRMED') return 'cancelled';
    const today = todayAddis();
    if (today < v.check_in) return 'too_early';
    if (today >= v.check_out) return 'ended';
    return 'ready';
  }

  const find = (id: string): Row => {
    auth.identityOf();
    const r = rows.get(id);
    if (!r) throw new ApiError('NOT_FOUND', 'Booking not found');
    return r;
  };

  async function roomsChart(o?: { property_id?: string; from?: string; days?: number }) {
      await gate();
      auth.identityOf();
      const from = o?.from ?? todayAddis();
      const days = o?.days ?? 14;
      const rooms = ROOMS.map(([name]) => {
        const total = TOTAL_ROOMS[name] ?? 1;
        const nights = Array.from({ length: days }, (_, d) => {
          const date = addDays(from, d);
          const booked = [...rows.values()].filter((r) => r.room_name === name && ['CONFIRMED', 'CHECKED_IN'].includes(view(r).status) && r.check_in <= date && date < addDays(r.check_in, r.nights)).reduce((a, r) => a + (r.rooms ?? 1), 0);
          const held = [...rows.values()].filter((r) => r.room_name === name && view(r).status === 'PAID_AWAITING_HOTEL' && r.check_in <= date && date < addDays(r.check_in, r.nights)).reduce((a, r) => a + (r.rooms ?? 1), 0);
          const blocked = [...blocks.values()].filter((b) => !b.released && b.room_type_id === name && b.check_in <= date && date < b.check_out).reduce((a, b) => a + b.rooms, 0);
          return { date, capacity: total, booked, held, blocked, free: Math.max(0, total - booked - held - blocked) };
        });
        return { id: name, name, total_rooms: total, nights };
      });
      return RoomsChartSchema.parse({ hotels: [HOTEL], property: HOTEL, from, days, rooms, server_time: new Date().toISOString() });
  }

  return {
    ...auth.api,

    async registerPushToken(registration) {
      await gate();
      auth.identityOf();
      // One token per install; a new token for the same device replaces the old one.
      pushTokens.set(registration.device_id, registration);
      await persist();
    },

    async listBookings() {
      await gate();
      auth.identityOf();
      const items = [...rows.values()].map(view);
      await persist();
      return PartnerBookingListSchema.parse({ property: HOTEL, items, server_time: new Date().toISOString() });
    },

    async revealGuestId(bookingId) {
      await gate();
      const r = find(bookingId);
      if (r.guest.purged) throw new ApiError('GONE', 'The ID number was erased after the retention period');
      return { id_number: r.guest.id_number };
    },

    async acceptBooking(id) {
      await gate();
      const r = find(id);
      const v = view(r);
      if (v.status === 'DECLINED_REFUNDED' && r.decline_reason === 'no_response') throw new ApiError('DEADLINE_PASSED', 'The time to answer has passed');
      if (v.status !== 'PAID_AWAITING_HOTEL') throw new ApiError('INVALID_STATE', 'This booking is not waiting for an answer');
      r.status = 'CONFIRMED';
      await persist();
      return view(r);
    },

    async declineBooking(id, reason) {
      await gate();
      const parsed = DeclineReasonSchema.safeParse(reason);
      if (!parsed.success) throw new ApiError('INVALID_STATE', 'A valid reason is required');
      const r = find(id);
      const v = view(r);
      if (v.status === 'DECLINED_REFUNDED' && r.decline_reason === 'no_response') throw new ApiError('DEADLINE_PASSED', 'The time to answer has passed');
      if (v.status !== 'PAID_AWAITING_HOTEL') throw new ApiError('INVALID_STATE', 'This booking is not waiting for an answer');
      r.status = 'DECLINED_REFUNDED';
      r.decline_reason = parsed.data;
      await persist();
      return view(r);
    },

    async cancelBooking(id, reason) {
      await gate();
      if (!HotelProblemReasonSchema.safeParse(reason).success) throw new ApiError('INVALID_REQUEST', 'A valid reason is required');
      const r = find(id);
      const v = view(r);
      if (v.status !== 'CONFIRMED' && v.status !== 'NEEDS_RELOCATION') throw new ApiError('INVALID_STATE', 'Only a confirmed booking can be cancelled by the hotel');
      r.status = 'CANCELLED_REFUNDED';
      await persist();
      return view(r);
    },

    async relocateBooking(id, reason) {
      await gate();
      if (!HotelProblemReasonSchema.safeParse(reason).success) throw new ApiError('INVALID_REQUEST', 'A valid reason is required');
      const r = find(id);
      if (view(r).status !== 'CONFIRMED') throw new ApiError('INVALID_STATE', 'Only a confirmed booking can be moved to another hotel');
      r.status = 'NEEDS_RELOCATION';
      await persist();
      return view(r);
    },

    // Offline check-in (simulated): the demo has no signing key, so nothing can be verified offline.
    async getVoucherKey() {
      await gate();
      return { kid: 'mock', public_key: '' };
    },
    async syncCheckins(items) {
      await gate();
      return items.map(() => ({ reference: '', result: 'checked_in' }));
    },

    // Statements (simulated): an empty month.
    async getStatements(month) {
      await gate();
      const m = month ?? todayAddis().slice(0, 7);
      return [{
        property_id: 'demo', hotel: 'Demo hotel', month: m, from: m + '-01', to: m + '-28', currency: 'ETB' as const, opening_santim: 0, closing_santim: 0,
        online_gross_santim: 0, online_commission_santim: 0, online_vat_santim: 0, online_hotel_share_santim: 0, pay_at_hotel_collected_santim: 0,
        pay_at_hotel_commission_santim: 0, pay_at_hotel_vat_santim: 0, paid_santim: 0, received_santim: 0, lines: [], payouts: [], receipts: [],
      }];
    },

    // Reviews (simulated): none in the demo.
    async listReviews() {
      await gate();
      return [];
    },
    async replyToReview() {
      await gate();
      throw new ApiError('NOT_FOUND', 'Review not found');
    },
    async removeReviewReply() {
      await gate();
      throw new ApiError('NOT_FOUND', 'Review not found');
    },

    // Support for hotel staff (simulated, in memory).
    async listSupportTickets() {
      await gate();
      return tickets.map((t) => ({ ...t, messages: undefined, preview: t.messages?.[t.messages.length - 1]?.body.slice(0, 140) ?? '' }));
    },
    async getSupportTicket(id) {
      await gate();
      const t = tickets.find((x) => x.id === id);
      if (!t) throw new ApiError('NOT_FOUND', 'Not found');
      return t;
    },
    async createSupportTicket(n) {
      await gate();
      if (tickets.filter((t) => t.status !== 'closed').length >= 5) throw new ApiError('TOO_MANY_TICKETS', 'Too many requests waiting');
      const now = new Date().toISOString();
      const id = `t${tickets.length + 1}`;
      tickets.unshift({
        id, kind: n.kind, subject: n.subject, status: 'open', booking_id: n.booking_id ?? null, booking_code: null, created_at: now, updated_at: now,
        messages: [{ from_staff: false, role: 'guest' as const, author: 'guest', body: n.message, created_at: now }],
      });
      return { id };
    },
    async replySupportTicket(id, message) {
      await gate();
      const t = tickets.find((x) => x.id === id);
      if (!t) throw new ApiError('NOT_FOUND', 'Not found');
      if (t.status === 'closed') throw new ApiError('INVALID_STATE', 'Closed');
      const now = new Date().toISOString();
      t.messages = [...(t.messages ?? []), { from_staff: false, role: 'guest' as const, author: 'guest', body: message, created_at: now }];
      t.status = 'open';
      t.updated_at = now;
    },
    async closeSupportTicket(id) {
      await gate();
      const t = tickets.find((x) => x.id === id);
      if (!t) throw new ApiError('NOT_FOUND', 'Not found');
      t.status = 'closed';
      t.updated_at = new Date().toISOString();
    },

    async getDispute() {
      await gate();
      throw new ApiError('NOT_FOUND', 'There is no dispute about this booking');
    },

    async sendDisputeStatement() {
      await gate();
      throw new ApiError('INVALID_STATE', 'There is no open dispute about this booking');
    },

    async lookupCheckin(query) {
      await gate();
      // Demo: the PIN of every demo booking is 1234. A voucher QR from the mock guest app carries the booking reference.
      let ref: string | undefined;
      if ('token' in query) {
        ref = decodeVoucherPayload(query.token)?.code;
        if (!ref) throw new ApiError('INVALID_VOUCHER', 'This is not a BookDan voucher, or it was changed.');
      } else {
        if (!/^[0-9]{4}$/.test(query.pin.trim())) throw new ApiError('INVALID_PIN', 'The PIN is 4 digits.');
        ref = query.reference.trim().toUpperCase();
        if (query.pin.trim() !== '1234') throw new ApiError('INVALID_PIN', 'Wrong PIN.');
      }
      const r = [...rows.values()].find((x) => x.reference === ref);
      if (!r) throw new ApiError('INVALID_PIN', 'Check the reference and the PIN.');
      return { state: checkinState(r), booking: view(r), server_time: new Date().toISOString() };
    },

    async getEarnings() {
      await gate();
      auth.identityOf();
      // Demo: a 10% BookDan fee plus 15% VAT on the fee, on every booking the hotel confirmed.
      const today = todayAddis();
      const items = [...rows.values()]
        .filter((r) => r.status === 'CONFIRMED' || r.status === 'CHECKED_IN' || r.status === 'COMPLETED')
        .map((r) => {
          const gross = r.total_santim;
          const commission = Math.round(gross * 0.1);
          const vat = Math.round(commission * 0.15);
          const checkOut = addDays(r.check_in, r.nights);
          const ready = r.status !== 'CONFIRMED' || checkOut <= today;
          return {
            booking_id: r.id, reference: r.reference, guest_name: r.guest.name, check_in: r.check_in, check_out: checkOut, status: r.status,
            gross_santim: gross, commission_santim: commission, vat_santim: vat, net_santim: gross - commission - vat, ready,
          };
        });
      const sum = (ready: boolean) => items.filter((i) => i.ready === ready).reduce((a, i) => a + i.net_santim, 0);
      return EarningsSchema.parse({
        hotels: [{ id: HOTEL.id, name: HOTEL.name, currency: 'ETB', ready_santim: sum(true), upcoming_santim: sum(false), paid_santim: 0, to_pay_santim: sum(true), last_paid_at: null, items, payouts: [] }],
        server_time: new Date().toISOString(),
      });
    },

    getRoomsChart: (o) => roomsChart(o),

    async listRoomBlocks() {
      await gate();
      auth.identityOf();
      const today = todayAddis();
      return RoomBlockListSchema.parse({
        blocks: [...blocks.values()].filter((b) => !b.released && b.check_out >= today).map((b) => ({ ...b, room_name: b.room_type_id })),
        server_time: new Date().toISOString(),
      }).blocks;
    },

    async createRoomBlock(input) {
      await gate();
      auth.identityOf();
      if (input.check_in < todayAddis()) throw new ApiError('INVALID_REQUEST', 'The first night cannot be in the past');
      if (input.kind === 'walk_in' && (input.guest_name ?? '').trim().length < 2) throw new ApiError('INVALID_REQUEST', "Give the guest's name");
      const chart = await roomsChart({ from: input.check_in, days: Math.max(1, Math.round((Date.parse(input.check_out) - Date.parse(input.check_in)) / 86_400_000)) });
      const room = chart.rooms.find((r) => r.id === input.room_type_id);
      if (!room) throw new ApiError('NOT_FOUND', 'Room not found');
      if (room.nights.some((n) => n.free < input.rooms)) throw new ApiError('SOLD_OUT', 'There are not enough free rooms on every one of those nights');
      const id = `blk_${++seq}_${Date.now().toString(36)}`;
      blocks.set(id, { id, kind: input.kind, room_type_id: input.room_type_id, check_in: input.check_in, check_out: input.check_out, rooms: input.rooms, guest_name: input.guest_name?.trim() ?? '', guest_phone: input.guest_phone?.trim() ?? '', note: input.note?.trim() ?? '', released: false });
      await persist();
    },

    async releaseRoomBlock(id) {
      await gate();
      auth.identityOf();
      const b = blocks.get(id);
      if (!b) throw new ApiError('NOT_FOUND', 'Not found');
      b.released = true;
      await persist();
    },

    async markNoShow(id) {
      await gate();
      const r = find(id);
      if (r.status === 'NO_SHOW') return view(r);
      if (view(r).status !== 'CONFIRMED') throw new ApiError('INVALID_STATE', 'Only a confirmed booking can be marked as a no-show');
      if (r.check_in > todayAddis()) throw new ApiError('INVALID_STATE', "The guest's check-in day has not come yet");
      r.status = 'NO_SHOW';
      await persist();
      return view(r);
    },

    async checkInGuest(id) {
      await gate();
      const r = find(id);
      const state = checkinState(r);
      if (state === 'ready') {
        r.status = 'CHECKED_IN';
        r.checked_in_at = Date.now();
        await persist();
      } else if (state !== 'checked_in') {
        throw new ApiError('INVALID_STATE', 'This booking cannot be checked in.');
      }
      return view(r);
    },

    demo: {
      async addIncomingBooking() {
        await gate();
        const [room_name] = pick(ROOMS);
        const now = Date.now();
        const r = newRow({
          guest: pick(GUESTS),
          room_name,
          check_in: addDays(todayAddis(), 1 + Math.floor(Math.random() * 6)),
          nights: 1 + Math.floor(Math.random() * 3),
          status: 'PAID_AWAITING_HOTEL',
          paid_at: now,
          deadline: now + ANSWER_WINDOW_MS,
        });
        rows.set(r.id, r);
        await persist();
      },
      expireAccessToken: auth.expireAccessToken,
      revokeRefreshToken: auth.revokeRefreshToken,
    },
  };
}
