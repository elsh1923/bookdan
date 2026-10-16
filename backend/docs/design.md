# Backend design (draft for review)

Status: **proposal, not yet implemented.** Derived from the mobile spec (React Native + Go) because the
Implementation Guide is not available. Anything marked **DECISION** changes money, security or guest-visible
behaviour and needs a yes/no from the product owner before its migration is written.

## 1. Principles

1. **PostgreSQL is the source of truth.** Rules that protect money and inventory live in the database
   (constraints, transactions, triggers), not only in Go code.
2. **Money** is `bigint` santim plus a `currency` column. Never floats.
3. **The ledger is append-only.** Rows are never updated or deleted. Corrections are new rows.
4. **Every mutating endpoint is idempotent** (`Idempotency-Key`), and so is the payment webhook.
5. **One transaction per state change.** Booking status, inventory, ledger rows and the event log change together or not at all.
6. **Time:** timestamps are `timestamptz` in UTC. Stay nights are `date` in Africa/Addis_Ababa. Every response carries `server_time`.
7. **Nothing sensitive in logs**: no phone numbers, OTPs, tokens, PINs, ID numbers.

## 2. Tables (first migrations)

### Identity
| Table | Purpose |
|---|---|
| `users` | id uuid, phone (unique when set, E.164), email (unique when set, stored lower-case), role (`guest`,`owner`,`receptionist`,`admin`), status, created_at. At least one of phone or email is required (CHECK) |
| `otp_challenges` | id, channel (`sms`,`email`), identifier (phone or lower-cased email), code_hash (HMAC), expires_at, attempts, consumed_at. Max 5 attempts, 5-minute life |
| `devices` | id, user_id, device_id (from `X-Device-Id`), platform, app_version, push_token, revoked_at, last_seen_at |
| `refresh_tokens` | id, user_id, device_id, family_id, token_hash, expires_at, rotated_at, revoked_at. Rotating; reuse of a rotated token revokes the whole family |

### Catalogue
| Table | Purpose |
|---|---|
| `properties` | id, name, type (`hotel`,`guesthouse`,`lodge`,`resort`), city, area, address, `location geography(Point,4326)`, phone, verified (bool), pay_at_hotel (bool), freshness_limit_minutes, last_synced_at, house_rules jsonb, amenities text[], status |
| `property_staff` | property_id, user_id, role (`owner`,`receptionist`) |
| `landmarks` | id, name, name_am, city, `location geography(Point)`. Search "near Meskel Square" = properties within a radius of the landmark (`ST_DWithin`) |
| `room_types` | id, property_id, name, capacity, amenities text[], total_rooms |
| `rooms` | id, property_id, room_type_id, number, status (`vacant`,`occupied`,`dirty`,`out_of_service`), updated_at |
| `rate_plans` | id, room_type_id, currency, nightly_price_santim, valid_from, valid_to |
| `rate_boosts` | id, property_id, starts_on, ends_on, percent_bp (holiday boosts) |

### Inventory (what makes double booking impossible)
`inventory(room_type_id, night date, capacity int, held int, booked int)` with
`CHECK (held >= 0 AND booked >= 0 AND held + booked <= capacity)`.
A hold increments `held` for each night in one transaction (`UPDATE ... WHERE held + booked < capacity`; zero rows
updated means sold out, mapped to `HOLD_CONFLICT`). Expiry or confirmation moves the count. The database rejects
any state where more rooms are promised than exist.

### Booking
| Table | Purpose |
|---|---|
| `quotes` | id, user_id, property_id, room_type_id, check_in, check_out, guests, rooms, currency, base/service/vat/total/deposit/balance_due `_santim`, price_snapshot jsonb, expires_at |
| `bookings` | id, code (short, unique), user_id, property_id, room_type_id, room_id (nullable until assigned), check_in, check_out, guests, rooms, guest_name, status, amounts copied from the quote, hold_expires_at, hotel_deadline_at, version, created_at |
| `booking_events` | append-only status history: booking_id, from_status, to_status, actor, reason, at |
| `idempotency_keys` | user_id, key, endpoint, request_hash, response_status, response_body, created_at. Same key with a different body = `409 IDEMPOTENCY_MISMATCH` |

**Status machine** (enforced in one Go function and a trigger that rejects anything else):

```
HELD -> PAID_AWAITING_HOTEL -> CONFIRMED -> CHECKED_IN -> COMPLETED
HELD -> EXPIRED
PAID_AWAITING_HOTEL -> DECLINED_REFUNDED            (hotel declines or misses the deadline)
CONFIRMED -> CANCELLED_REFUNDED | NO_SHOW | NEEDS_RELOCATION
any paid state -> DISPUTED
```

### Payments and ledger
| Table | Purpose |
|---|---|
| `payments` | id, booking_id, provider (`mock`,`telebirr`,`chapa`), provider_ref (unique), amount_santim, currency, status, payment_url |
| `payment_events` | provider, event_id (unique), payload jsonb, processed_at. A duplicate callback hits the unique key and is ignored (acceptance test M4) |
| `ledger_accounts` | code, kind. e.g. `gateway_clearing`, `guest_deposits_held`, `property_payable`, `commission_revenue`, `vat_on_commission_payable`, `refunds_payable` |
| `ledger_transactions` | id, booking_id, kind, created_at |
| `ledger_entries` | transaction_id, account, `amount_santim` (signed), currency. A deferred trigger requires each transaction to sum to zero. A trigger blocks `UPDATE` and `DELETE` |

Partner statements (deposit, commission, VAT on commission, net payout, paid/pending) are **queries over the
ledger**. The app never calculates them.

### Voucher, partner sync, audit
| Table | Purpose |
|---|---|
| `vouchers` | booking_id, kid, issued_at, pin_hash (HMAC), pin_attempts, locked_until |
| `sync_actions` | action_id (pk, from the device), device_id, property_id, seq, type, payload, status, result, error, processed_at. Sending the same action twice returns the saved result |
| `change_log` | id bigserial, property_id, entity, entity_id, op. The `sync/pull` cursor is the last id seen |
| `audit_log` | append-only record of security-relevant actions (login, refresh reuse, PIN lockout, device revoke) |

### Guest identity and the hotel's copy (national ID)
Tables `guest_profiles` and `booking_guests` (migration 00008). The guest enters their name, ID type, ID number and
nationality **once, in the guest app**. A booking cannot be made without a complete profile (`PROFILE_INCOMPLETE`).
When the guest requests a room, the server freezes a copy of the profile into `booking_guests`, and the partner app
shows it with the booking: the hotel sees the guest as they were when they asked.

- **Stored encrypted:** the full ID number is AES-GCM ciphertext (`id_number_enc`); the key lives in the server's
  secret store (or a KMS), never in the database or the apps. A database leak alone does not expose numbers.
- **Findable but not readable:** `id_number_hash` is a keyed hash, so a hotel can recognise a returning guest.
- **Masked in lists and caches:** both apps hold only `id_last4`. A hotel reads the full number with an explicit
  `reveal` call (`POST /partner/bookings/{id}/guest/reveal`) that writes an `audit_log` row first. Full numbers are
  never logged. The guest can see only the last four of what they saved; changing it means typing it again.
- **The guest owns their profile:** they can change or delete it any time (`/me/profile`). Deleting it does not
  remove a hotel's copy for stays already requested; that copy is erased by the retention job.
- **The hotel's copy is a record, not a draft:** never edited or deleted. The only change allowed is the one-way
  erasure of the ID number after the retention period (`purge_id_numbers(date)`, run by a job).
- **Hotel isolation:** the copy is tied to the booking's own hotel by a composite foreign key, and every partner
  endpoint checks `property_staff`, so a hotel only sees its own guests.
- **Consent:** the guest app tells the guest, before they save, that the ID is stored encrypted and shared with a hotel
  only when they request a room there.

**DECISION 5 (retention and law):** how long must a hotel keep the full ID number, and when must it be erased?
Ethiopian hotel-register and data-protection rules decide this; take legal advice. The erasure function has no default
on purpose. Also decide: which ID formats to accept (kebele ID, Fayda, passport, licence), the wording of the privacy
notice, and **when the hotel first sees the full ID**: today it is visible, behind a tap, from the moment of the
request. Showing it only after the hotel accepts the booking would share less with hotels that decline.

## 3. Pricing

Computed once, on the server, inside `POST /quotes`, then frozen on the quote and copied to the booking.

```
base        = sum over nights of (nightly price + holiday boost)       integer santim, rounded per night
service     = base * service_bps / 10000
vat         = (base + service) * vat_bps / 10000                        15% = 1500 bps
total       = base + service + vat
deposit     = total * deposit_bps / 10000                               paid online
balance_due = total - deposit                                           paid at the hotel
```
Rounding is "half up" at each line, covered by table-driven tests. Rates live in a `pricing_config` row so a rate
change never needs a deploy.

**DECISION 1 (money):** please confirm or give the real numbers: `service_bps`, `vat_bps` (15%?), `deposit_bps`,
platform `commission_bps`, and whether VAT applies to the guest price, the commission, or both.
The mobile mock today charges the full amount online with 15% VAT and no deposit; the spec shows a deposit
and balance at the hotel.

**DECISION 2 (refunds):** cancellation policy: which refund is allowed at which time before check-in.

## 4. Authentication and security

- OTP: 6 digits, stored as HMAC, 5-minute life, 5 attempts, per-phone and per-IP rate limits. Demo mode returns the
  code in the response, only when `DEMO_MODE=true`; the server refuses to start with it on in production.
- Access token: signed JWT (EdDSA or HS256 with a rotated key), 15 minutes. Refresh token: random 256-bit,
  stored hashed, 30 days, rotating with reuse detection, bound to a device. Device revoke invalidates its family.
- Voucher: `base64url(payload).base64url(ed25519 signature)`, payload `{v, kid, pid, code, in, out, exp}`.
  Private key only on the server (env or KMS). Partner apps embed the public keys with their `kid`.
  The PIN is stored hashed and returned once, when the voucher is issued; a re-issue rotates it.
- PIN check-in: 5 wrong attempts lock the voucher, enforced on the server.
- Per-role authorisation on every route. Partner routes only see their own `property_id`.

**DECISION 3:** OTP delivery provider for launch: an SMS aggregator vs direct for phone, and a transactional email
service for email (for example Resend, Postmark or SES). The demo uses on-screen codes for both.

**DECISION 4 (accounts):** a guest may sign in with a phone on one visit and an email on another. Proposed: they are
separate accounts until the guest verifies the second identifier while signed in (Profile, "Add email" or "Add phone"),
which links it to the same user. Bookings belong to the user, so linking merges nothing silently. Never auto-merge
accounts on a matching email alone.

Email OTP specifics: codes expire in 5 minutes with 5 attempts, resend is rate-limited per address and per IP, the
email says only that a sign-in code was requested (no booking details), and addresses are lower-cased before storage.

## 5. Background work

Hold expiry (every 30 s), hotel-deadline expiry (15 minutes after payment), voucher issue after confirmation,
alert escalation. Built on River (Postgres-backed queue) so jobs commit in the same transaction as the state
change that creates them.

## 6. Build order and tests

1. Foundation: config, logging, DB pool, migrations runner, readiness check, CI.
2. Identity: users, OTP, devices, tokens + tests for expiry, lockout, refresh reuse.
3. Catalogue and search (PostGIS landmarks), seeded with the same demo properties as the app.
4. Quotes and pricing + table-driven money tests.
5. Holds and inventory + a concurrency test (N parallel holds on the last room: exactly one wins).
6. Payments (mock gateway, demo controls), webhook idempotency, ledger + a "ledger always balances" property test.
7. Voucher, partner inbox, deadline worker.
8. Offline sync and check-in.

Every phase updates `api/openapi.yaml` first and ships with integration tests against real PostgreSQL.

## More than one room

A quote and a booking are for `rooms` (1 to 9) units of one room type. The amounts cover all of them, `rooms` can never change
after the booking exists, and `guests >= rooms`. When the booking is held, paid, cancelled or expired the server moves
`inventory.held`/`booked` by `rooms` in one statement per night (`held + booked + rooms <= capacity`), so the
"never oversold" rule still holds for multi-room bookings: either all the rooms are taken or none. Different room types
in one stay are separate bookings for now (decision for later: a basket with one payment).
