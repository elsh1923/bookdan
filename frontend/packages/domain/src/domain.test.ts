// Run with: npm test -w @bookdan/domain  (Node built-in runner, no extra deps)
import assert from 'node:assert/strict';
import { test } from 'node:test';
import { addDays, nightsBetween, holdRemainingMs } from './dates.ts';
import { isoToEthiopian } from './ethiopianCalendar.ts';
import { etb, formatMoney } from './money.ts';
import { b64urlDecode, b64urlEncode, decodeVoucherPayload, encodeVoucherToken, newIdempotencyKey } from './voucher.ts';
import { VoucherSchema } from './schemas.ts';
import { normalizeEmail } from './identity.ts';
import { PartnerBookingSchema, DECLINE_REASONS, isAwaitingAnswer, sortInbox } from './partner.ts';
import { alertReadiness, parseBookingAlert, parseBookingUpdate } from './alerts.ts';
import { BookingGuestSchema, GuestProfileInputSchema, GuestProfileSchema, idLast4, isProfileComplete, maskId, normalizeIdNumber } from './guestIdentity.ts';
import { isPaymentInFlight, isStillChanging, shouldResumeOnLaunch } from './booking.ts';
import { formatCountdown, normalizeEthiopianPhone, priceStay } from './pricing.ts';

test('Ethiopian new year 2019 starts on 2026-09-11', () => {
  assert.deepEqual(isoToEthiopian('2026-09-11'), { year: 2019, month: 1, day: 1 });
});

test('2026-10-09 is Meskerem 29, 2019', () => {
  assert.deepEqual(isoToEthiopian('2026-10-09'), { year: 2019, month: 1, day: 29 });
});

test('13th month and leap year: 2023-09-11 is Pagume 6, 2015 and 2023-09-12 is new year 2016', () => {
  assert.deepEqual(isoToEthiopian('2023-09-11'), { year: 2015, month: 13, day: 6 });
  assert.deepEqual(isoToEthiopian('2023-09-12'), { year: 2016, month: 1, day: 1 });
});

test('Pagume has 5 days in a normal year', () => {
  assert.deepEqual(isoToEthiopian('2026-09-10'), { year: 2018, month: 13, day: 5 });
});

test('Ethiopian Christmas (Genna) is Tahsas 29 on 2026-01-07', () => {
  assert.deepEqual(isoToEthiopian('2026-01-07'), { year: 2018, month: 4, day: 29 });
});

test('money formats from integer santim', () => {
  assert.equal(formatMoney(etb(1500)), '1,500 ETB');
  assert.equal(formatMoney({ amount_santim: 123456789, currency: 'ETB' }), '1,234,567.89 ETB');
  assert.equal(formatMoney({ amount_santim: 5000, currency: 'USD' }), '$50');
  assert.equal(formatMoney({ amount_santim: 5, currency: 'ETB' }), '0.05 ETB');
});

test('date arithmetic crosses month and year boundaries', () => {
  assert.equal(addDays('2026-12-31', 1), '2027-01-01');
  assert.equal(addDays('2028-02-28', 1), '2028-02-29');
  assert.equal(nightsBetween('2026-10-09', '2026-10-12'), 3);
});

test('hold countdown ignores the phone clock', () => {
  const left = holdRemainingMs('2026-10-09T10:15:00Z', '2026-10-09T10:00:00Z', 60_000);
  assert.equal(left, 14 * 60_000);
  assert.equal(holdRemainingMs('2026-10-09T10:15:00Z', '2026-10-09T10:00:00Z', 20 * 60_000), 0);
});

test('priceStay adds 15% VAT in whole santim', () => {
  assert.deepEqual(priceStay(250_000, 2), { subtotal_santim: 500_000, vat_santim: 75_000, total_santim: 575_000 });
  assert.equal(priceStay(33_333, 1).vat_santim, 5_000); // 4999.95 rounds to 5000
});

test('formatCountdown pads and never goes negative', () => {
  assert.equal(formatCountdown(14 * 60_000 + 5_000), '14:05');
  assert.equal(formatCountdown(-10), '00:00');
});

test('normalizeEthiopianPhone accepts common local forms', () => {
  for (const v of ['0911223344', '911223344', '+251911223344', '251 91 122 3344']) {
    assert.equal(normalizeEthiopianPhone(v), '+251911223344');
  }
  assert.equal(normalizeEthiopianPhone('0111223344'), null);
  assert.equal(normalizeEthiopianPhone('12345'), null);
});

test('base64url round-trips every length and rejects bad input', () => {
  for (let n = 0; n < 40; n++) {
    const bytes = Uint8Array.from({ length: n }, (_, i) => (i * 37 + 11) & 255);
    assert.deepEqual(b64urlDecode(b64urlEncode(bytes)), bytes);
  }
  assert.equal(b64urlEncode(Uint8Array.from([251, 255, 254])), '-__-'); // url-safe, no padding
  assert.equal(b64urlDecode('a+b'), null); // standard-base64 characters are not allowed
  assert.equal(b64urlDecode('abcde'), null); // impossible length
});

test('voucher token carries the payload and survives a round trip', () => {
  const payload = { v: 1 as const, kid: 'k1', pid: 'p-addis-1', code: 'BD-AB12CD', in: '2026-10-12', out: '2026-10-13', exp: 1789000000 };
  const token = encodeVoucherToken(payload, new Uint8Array(64).fill(7));
  assert.match(token, /^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/);
  assert.deepEqual(decodeVoucherPayload(token), payload);
});

test('malformed or altered voucher tokens do not decode to a payload', () => {
  const payload = { v: 1 as const, kid: 'k1', pid: 'p1', code: 'BD-1', in: '2026-10-12', out: '2026-10-13', exp: 1789000000 };
  const token = encodeVoucherToken(payload, new Uint8Array(64));
  for (const bad of ['', 'abc', 'a.b.c', '.sig', 'payload.', 'not base64!.sig']) assert.equal(decodeVoucherPayload(bad), null, bad);
  // One changed character inside the payload either fails to parse or changes the content,
  // which is why the partner app must check the signature before trusting it.
  const i = 10;
  const flipped = token.slice(0, i) + (token[i] === 'A' ? 'B' : 'A') + token.slice(i + 1);
  assert.notDeepEqual(decodeVoucherPayload(flipped), payload);
});

test('idempotency keys are unique and well formed', () => {
  const keys = new Set(Array.from({ length: 200 }, () => newIdempotencyKey()));
  assert.equal(keys.size, 200);
  for (const k of keys) assert.match(k, /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/);
});

test('VoucherSchema accepts a real voucher and rejects a bad PIN', () => {
  const base = {
    booking_id: 'b1', reference: 'BD-AB12CD', token: 'a.b', pin: '4821', expires_at: '2026-10-18T20:59:59.000Z',
    property_id: 'p1', property_name: 'Hotel', property_phone: '+251 11 000 0000', address: 'Bole, Addis Ababa',
    lat: 9.01, lng: 38.76, check_in: '2026-10-16', check_out: '2026-10-17', nights: 1, guests: 1, guest_name: 'Hanna',
    balance_due_santim: 0, currency: 'ETB', server_time: '2026-10-15T08:00:00.000Z',
  };
  assert.equal(VoucherSchema.safeParse(base).success, true);
  for (const pin of ['abcd', '123', '12345', '12 4', '']) assert.equal(VoucherSchema.safeParse({ ...base, pin }).success, false, pin);
  assert.equal(VoucherSchema.safeParse({ ...base, balance_due_santim: 12.5 }).success, false); // money is whole santim
});

test('normalizeEmail trims, lower-cases and accepts ordinary addresses', () => {
  assert.equal(normalizeEmail('  Hanna.T@Example.COM '), 'hanna.t@example.com');
  assert.equal(normalizeEmail('visitor+trip@mail.example.co.uk'), 'visitor+trip@mail.example.co.uk');
  assert.equal(normalizeEmail('a@b-c.io'), 'a@b-c.io');
});

test('normalizeEmail rejects things that cannot be an address', () => {
  const bad = ['', 'plain', 'a@b', '@example.com', 'a@@example.com', 'a b@example.com', 'a@example', 'a@.com', 'a@example..com',
    'a@-example.com', 'a@example-.com', 'a@example.c', 'a@example.123', 'a@exa mple.com', `${'x'.repeat(65)}@example.com`, `a@${'d'.repeat(250)}.com`];
  for (const b of bad) assert.equal(normalizeEmail(b), null, JSON.stringify(b));
});

test('a payment that was started but not finished is resumed on the next launch', () => {
  assert.equal(isPaymentInFlight({ status: 'HELD', payment: 'pending' }), true);
  assert.equal(shouldResumeOnLaunch({ status: 'HELD', payment: 'pending' }), true); // app killed mid-payment (M3)
  assert.equal(shouldResumeOnLaunch({ status: 'PAID_AWAITING_HOTEL', payment: 'none' }), true);
});

test('settled or untouched bookings are not resumed', () => {
  for (const status of ['CONFIRMED', 'CHECKED_IN', 'COMPLETED', 'EXPIRED', 'DECLINED_REFUNDED', 'CANCELLED_REFUNDED'] as const) {
    assert.equal(shouldResumeOnLaunch({ status, payment: 'none' }), false, status);
  }
  assert.equal(shouldResumeOnLaunch({ status: 'HELD', payment: 'none' }), false); // never started paying
  assert.equal(shouldResumeOnLaunch({ status: 'HELD', payment: 'failed' }), false); // guest sees the retry on the booking screen
});

test('polling stops once a booking can no longer change by itself', () => {
  assert.equal(isStillChanging({ status: 'HELD', payment: 'none' }), true);
  assert.equal(isStillChanging({ status: 'PAID_AWAITING_HOTEL', payment: 'none' }), true);
  assert.equal(isStillChanging({ status: 'CONFIRMED', payment: 'none' }), false);
  assert.equal(isStillChanging({ status: 'EXPIRED', payment: 'none' }), false);
});

const pb = (over: Record<string, unknown>) => ({
  id: 'b', reference: 'BD-1', status: 'CONFIRMED', guest_name: 'G', room_name: 'R', check_in: '2026-10-20', check_out: '2026-10-21',
  nights: 1, guests: 1, total_santim: 368000, currency: 'ETB', paid_at: null, hotel_deadline_at: null, decline_reason: null,
  server_time: '2026-10-15T08:00:00.000Z',
  guest: { full_name: 'G', contact: '+251911234567', nationality: 'Ethiopian', id_type: 'national_id', id_last4: '9012', id_purged: false },
  ...over,
});

test('inbox puts bookings awaiting an answer first, nearest deadline on top', () => {
  const items = [
    pb({ id: 'later', status: 'CONFIRMED', check_in: '2026-10-25' }),
    pb({ id: 'late-deadline', status: 'PAID_AWAITING_HOTEL', hotel_deadline_at: '2026-10-15T08:14:00.000Z' }),
    pb({ id: 'soon', status: 'CONFIRMED', check_in: '2026-10-16' }),
    pb({ id: 'urgent', status: 'PAID_AWAITING_HOTEL', hotel_deadline_at: '2026-10-15T08:04:00.000Z' }),
  ];
  assert.deepEqual(sortInbox(items).map((x) => x.id), ['urgent', 'late-deadline', 'soon', 'later']);
  assert.equal(items[0].id, 'later'); // the input is not mutated
});

test('only PAID_AWAITING_HOTEL needs an answer', () => {
  assert.equal(isAwaitingAnswer({ status: 'PAID_AWAITING_HOTEL' }), true);
  for (const status of ['CONFIRMED', 'DECLINED_REFUNDED', 'HELD', 'EXPIRED'] as const) assert.equal(isAwaitingAnswer({ status }), false, status);
});

test('PartnerBookingSchema carries the guest profile but never a full ID number, card data, PIN or token', () => {
  const ok = PartnerBookingSchema.safeParse(pb({}));
  assert.equal(ok.success, true);
  if (ok.success) {
    const all = JSON.stringify(Object.keys(ok.data)) + JSON.stringify(Object.keys(ok.data.guest));
    assert.equal(/card|pin|token|id_number/i.test(all), false, all);
    assert.equal(ok.data.guest.id_last4, '9012');
  }
  assert.equal(PartnerBookingSchema.safeParse(pb({ total_santim: 12.5 })).success, false);
  assert.equal(PartnerBookingSchema.safeParse(pb({ status: 'NOPE' })).success, false);
  assert.equal(PartnerBookingSchema.safeParse(pb({ guest: undefined })).success, false); // a booking always shows who the guest is
  assert.deepEqual([...DECLINE_REASONS], ['fully_booked', 'room_unavailable', 'maintenance', 'other']);
});

const phoneOk = { supported: true, permission: 'granted' as const, channel: { importance: 5, hasSound: true }, pushToken: 'fcm-token' };

test('alert checklist: a fully set up phone is ready, but battery is never marked done', () => {
  const r = alertReadiness(phoneOk);
  assert.equal(r.ready, true);
  assert.deepEqual(r.items.map((i) => i.status), ['ok', 'ok', 'ok', 'unknown']);
});

test('alert checklist: each missing piece is reported and blocks readiness', () => {
  assert.equal(alertReadiness({ ...phoneOk, permission: 'denied' }).items[0].status, 'action');
  assert.equal(alertReadiness({ ...phoneOk, permission: 'undetermined' }).ready, false);
  assert.equal(alertReadiness({ ...phoneOk, channel: { importance: 3, hasSound: true } }).items[1].status, 'action'); // not loud enough
  assert.equal(alertReadiness({ ...phoneOk, channel: { importance: 5, hasSound: false } }).items[1].status, 'action'); // silent channel
  assert.equal(alertReadiness({ ...phoneOk, pushToken: null }).items[2].status, 'action');
});

test('alert checklist: an iPhone has no channels, and a browser is never ready', () => {
  assert.equal(alertReadiness({ ...phoneOk, channel: null }).ready, true);
  const web = alertReadiness({ ...phoneOk, supported: false });
  assert.equal(web.ready, false);
  assert.deepEqual(web.items.map((i) => i.status), ['action', 'action', 'action', 'unknown']);
});

test('only a well-formed new-booking push is accepted', () => {
  assert.deepEqual(parseBookingAlert({ type: 'new_booking', booking_id: 'pb_1' }), { type: 'new_booking', booking_id: 'pb_1' });
  for (const bad of [null, undefined, {}, { type: 'new_booking' }, { type: 'new_booking', booking_id: '' }, { type: 'other', booking_id: 'x' }, { type: 'new_booking', booking_id: 5 }, 'text']) {
    assert.equal(parseBookingAlert(bad), null, JSON.stringify(bad));
  }
});

test('only a well-formed booking-update push is accepted', () => {
  assert.deepEqual(parseBookingUpdate({ type: 'booking_update', booking_id: 'b1', status: 'confirmed' }), { type: 'booking_update', booking_id: 'b1', status: 'confirmed' });
  for (const bad of [null, {}, { type: 'booking_update', booking_id: 'b1' }, { type: 'booking_update', booking_id: 'b1', status: 'paid' }, { type: 'new_booking', booking_id: 'b1', status: 'confirmed' }, { type: 'booking_update', booking_id: '', status: 'declined' }]) {
    assert.equal(parseBookingUpdate(bad), null, JSON.stringify(bad));
  }
});

test('ID numbers are cleaned up and anything that cannot be an ID is refused', () => {
  assert.equal(normalizeIdNumber(' ab 1234-56 '), 'AB1234-56');
  assert.equal(normalizeIdNumber('1234 5678 9012'), '123456789012');
  assert.equal(normalizeIdNumber('EP/0012345'), 'EP/0012345');
  for (const bad of ['', '123', 'ab', 'has_underscore1', 'bad!char1', 'x'.repeat(26), '١٢٣٤٥٦']) assert.equal(normalizeIdNumber(bad), null, JSON.stringify(bad));
});

test('only the last four characters of an ID are ever shown', () => {
  assert.equal(idLast4('123456789012'), '9012');
  assert.equal(idLast4('AB12'), 'AB12');
  assert.equal(maskId('9012'), '•••• 9012');
  assert.equal(maskId(idLast4('EP/0012345')).includes('0012'), false);
});

test('a guest profile needs a name, a valid ID type and a nationality', () => {
  const ok = { full_name: 'Hanna Tesfaye', id_type: 'national_id', id_number: '123456789012', nationality: 'Ethiopian' };
  assert.equal(GuestProfileInputSchema.safeParse(ok).success, true);
  assert.equal(GuestProfileInputSchema.safeParse({ ...ok, full_name: 'H' }).success, false);
  assert.equal(GuestProfileInputSchema.safeParse({ ...ok, id_type: 'library_card' }).success, false);
  assert.equal(GuestProfileInputSchema.safeParse({ ...ok, nationality: '' }).success, false);
});

test('what the guest app and the hotel receive can never contain a full ID number', () => {
  for (const shape of [GuestProfileSchema.shape, BookingGuestSchema.shape]) {
    assert.deepEqual(Object.keys(shape).filter((k) => k.includes('id_number')), []);
    assert.equal(Object.keys(shape).includes('id_last4'), true);
  }
});

test('booking needs a complete profile', () => {
  const p = { full_name: 'Hanna Tesfaye', id_last4: '9012', nationality: 'Ethiopian' };
  assert.equal(isProfileComplete(p), true);
  assert.equal(isProfileComplete(null), false);
  assert.equal(isProfileComplete(undefined), false);
  assert.equal(isProfileComplete({ ...p, id_last4: '' }), false);
  assert.equal(isProfileComplete({ ...p, nationality: ' ' }), false);
  assert.equal(isProfileComplete({ ...p, full_name: 'H' }), false);
});
