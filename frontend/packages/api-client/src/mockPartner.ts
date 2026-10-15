import {
  DeclineReasonSchema,
  PartnerBookingListSchema,
  PartnerBookingSchema,
  addDays,
  priceStay,
  todayAddis,
  type PartnerBooking,
} from '@bookdan/domain';
import type { MockStore } from './mock';
import { createMockAuth } from './mockAuth';
import { ApiError, type PartnerApi, type PushRegistration } from './types';

const HOTEL = { id: 'p-addis-1', name: 'Meskel Square Palace Hotel' };
const ANSWER_WINDOW_MS = 15 * 60_000; // the hotel has 15 minutes after the guest pays
const GUESTS = ['Hanna Tesfaye', 'Daniel Mekonnen', 'Selam Bekele', 'Yonas Alemu', 'Martha Girma', 'Samuel Haile', 'Rahel Tadesse', 'Abel Woldemichael'];
const ROOMS: [string, number][] = [['Standard Double', 3200], ['Deluxe King', 4800], ['Family Suite', 7200]];

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
  guest_name: string;
  room_name: string;
  check_in: string;
  nights: number;
  guests: number;
  total_santim: number;
  paid_at: number | null;
  deadline: number | null;
  decline_reason: string | null;
}

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));
const pick = <T>(list: T[]): T => list[Math.floor(Math.random() * list.length)];

export function createMockPartnerApi(opts: MockPartnerOptions = {}): PartnerApi {
  const latency = opts.latencyMs ?? 450;
  const rows = new Map<string, Row>();
  const pushTokens = new Map<string, PushRegistration>(); // by device id
  let seq = 0;

  const newRow = (over: Partial<Row> & Pick<Row, 'guest_name' | 'room_name' | 'check_in' | 'nights'>): Row => {
    const price = ROOMS.find(([n]) => n === over.room_name)?.[1] ?? 3200;
    const id = `pb_${++seq}_${Date.now().toString(36)}`;
    return {
      id,
      reference: `BD-${Math.random().toString(36).slice(2, 8).toUpperCase()}`,
      status: 'CONFIRMED',
      guests: 1 + Math.floor(Math.random() * 2),
      total_santim: priceStay(price * 100, over.nights).total_santim,
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
      newRow({ guest_name: 'Hanna Tesfaye', room_name: 'Standard Double', check_in: addDays(today, 1), nights: 1, status: 'PAID_AWAITING_HOTEL', paid_at: now - 4 * min, deadline: now - 4 * min + ANSWER_WINDOW_MS }),
      newRow({ guest_name: 'Daniel Mekonnen', room_name: 'Deluxe King', check_in: addDays(today, 2), nights: 2, status: 'PAID_AWAITING_HOTEL', paid_at: now - 10 * min, deadline: now - 10 * min + ANSWER_WINDOW_MS }),
      newRow({ guest_name: 'Selam Bekele', room_name: 'Family Suite', check_in: addDays(today, 3), nights: 3, guests: 4 }),
      newRow({ guest_name: 'Yonas Alemu', room_name: 'Standard Double', check_in: today, nights: 1 }),
      newRow({ guest_name: 'Martha Girma', room_name: 'Standard Double', check_in: addDays(today, 5), nights: 2, status: 'DECLINED_REFUNDED', decline_reason: 'fully_booked' }),
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
          for (const [k, v] of s.rows ?? []) rows.set(k, v);
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
      id: r.id, reference: r.reference, status: r.status, guest_name: r.guest_name, room_name: r.room_name,
      check_in: r.check_in, check_out: addDays(r.check_in, r.nights), nights: r.nights, guests: r.guests,
      total_santim: r.total_santim, currency: 'ETB',
      paid_at: r.paid_at ? new Date(r.paid_at).toISOString() : null,
      hotel_deadline_at: r.deadline ? new Date(r.deadline).toISOString() : null,
      decline_reason: r.decline_reason,
      server_time: new Date(now).toISOString(),
    });
  }

  const find = (id: string): Row => {
    auth.identityOf();
    const r = rows.get(id);
    if (!r) throw new ApiError('NOT_FOUND', 'Booking not found');
    return r;
  };

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

    demo: {
      async addIncomingBooking() {
        await gate();
        const [room_name] = pick(ROOMS);
        const now = Date.now();
        const r = newRow({
          guest_name: pick(GUESTS),
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
