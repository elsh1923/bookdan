import {
  DeclineReasonSchema,
  PartnerBookingListSchema,
  PartnerBookingSchema,
  addDays,
  idLast4,
  priceStay,
  todayAddis,
  type IdType,
  type PartnerBooking,
} from '@bookdan/domain';
import type { MockStore } from './mock';
import { createMockAuth } from './mockAuth';
import { ApiError, type PartnerApi, type PushRegistration } from './types';

const HOTEL = { id: 'p-addis-1', name: 'Meskel Square Palace Hotel' };
const ANSWER_WINDOW_MS = 15 * 60_000; // the hotel has 15 minutes after the guest pays
const ROOMS: [string, number][] = [['Standard Double', 3200], ['Deluxe King', 4800], ['Family Suite', 7200]];

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
}

const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));
const pick = <T>(list: T[]): T => list[Math.floor(Math.random() * list.length)];
const guestNamed = (name: string): DemoGuest => GUESTS.find((g) => g.name === name) ?? GUESTS[0];

export function createMockPartnerApi(opts: MockPartnerOptions = {}): PartnerApi {
  const latency = opts.latencyMs ?? 450;
  const rows = new Map<string, Row>();
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
