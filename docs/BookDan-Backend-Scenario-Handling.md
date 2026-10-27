# BookDan backend: scenario handling guide for senior backend review

_Repository state: 27 October 2026. Go 1.26, PostgreSQL (Supabase), 33 migrations, 107 HTTP routes, 229 test functions._

Purpose: to let a reviewer judge the design without reading 22,000 lines first. Section 1 to 4 explain the architecture and the invariants the database enforces. Section 5 is the core: for every business flow, each scenario, the mechanism that handles it, and what happens when two requests race or a dependency fails. Sections 6 to 10 are inventories (endpoints, jobs, integrations, configuration, tests). Section 11 lists the weak spots we already know about and the questions we would most like challenged.

Conventions: money is always integer santim (1 birr = 100 santim) in bigint columns, never floats. Times are stored UTC; business days use Africa/Addis_Ababa (UTC+3, no DST). "Tx" means one PostgreSQL transaction opened by s.tx(ctx, fn). File names refer to backend/internal/api unless stated.


# 1. Architecture


| Aspect | Choice |
|---|---|
| Language / HTTP | Go 1.26, chi router, net/http. One binary: JSON API under /v1, the admin web page embedded and served at /admin (plain HTML/JS, embed.FS), the public hotel-application page at /apply, and background workers in the same process. |
| Database | PostgreSQL on Supabase with PostGIS and btree_gist. Everything lives in schema "app" (Supabase's public API roles are revoked), extensions in schema "extensions". pgx v5 connection pool. Migrations: goose, 33 files. Tests run against an embedded Postgres (no PostGIS there, so a plain-text coordinate fallback path exists: Server.postgis). |
| Hosting | Render (web service) + Supabase (database). Config entirely from environment variables; config.Validate refuses to start in production with stand-ins (fake gateway, log-only OTP/push, test Chapa key, short secrets, http PUBLIC_URL, demo mode). |
| Request pipeline | RequestID, RealIP, requestLog (method, path, status, ms only; no headers/query/body), recoverAndAlert (panic to 500 in the stable error shape, plus operator alert), securityHeaders (HSTS only on https), cors (open only outside production), rateLimit. Then per-route authenticate(app) for guest, partner and admin groups. |
| Handler shape | type handler func(w, r) error wrapped by s.h(): an *apiError becomes {status, code, message}; anything else is logged, reported to ops.Report and returned as a generic 500. Stable error codes (SOLD_OUT, QUOTE_EXPIRED, HOLD_EXPIRED, DEADLINE_PASSED, INVALID_STATE, CANCEL_TOO_LATE, ...) are what the apps translate. |
| Identity | Passwordless OTP (email, SMS) for guests and hotel staff. Access token HS256 JWT, 15 min, carries app ("guest"/"partner"/"admin") and device id. Refresh token: random, only a hash stored, 30 days, rotates on every use, family revoked on reuse. Admin: ADMIN_EMAILS + one shared ADMIN_PASSWORD, 8 h token, no refresh. See 5.1. |
| Secrets | Keys struct derives all keys from env: JWT secret, 32-byte ID encryption key (AES-256-GCM via Seal/Open), HMAC for hashing OTP/refresh/PIN, Ed25519 seed for voucher signatures. Development falls back to fixed public values; production refuses. |
| External services (all behind interfaces in providers.go) | OTPSender (SMTP / Brevo email, Africa's Talking / Twilio SMS, or off), Gateway (Chapa; dev stand-in), Pusher (FCM), Mailer. Failures of notification channels never undo the business action that caused them. |
| Side effects | Pushes and emails are sent after the transaction commits, best effort, errors logged. There is no outbox table (see 11). |


## Request flow in one picture


```
client -> RequestID/RealIP -> log -> recover -> secHeaders -> cors -> rateLimit
       -> authenticate(app)  [JWT verify + live check: user active AND device not revoked]
       -> s.h(handler)       [handler opens s.tx, locks rows FOR UPDATE, writes ledger/events]
       -> after commit: push / email / runRefunds (best effort)
background: RunWorkers ticker 20 s -> sweep() ; on Addis date change -> RunMaintenance()
```


# 2. Design principles that explain most decisions

- The database is the last line of defence. Overselling, illegal status changes, unbalanced money and tampering with history are prevented by constraints and triggers, not only by application code (section 3).
- Every state change that matters happens in one transaction that first locks the row (SELECT ... FOR UPDATE), re-reads the state, and only then changes it. Handlers therefore tolerate retries and races by re-checking state under the lock.
- Idempotency at every boundary: client retries (Idempotency-Key on booking create), gateway callbacks (unique payment_events key), refunds (gateway Idempotency-Key per payment or per partial refund), emails/alerts (claim timestamp before sending, unique marker rows).
- Time is derived as well as stored: a hold that has run out reads EXPIRED even before the sweeper has run, and read endpoints for hotels run sweep() first, so no screen shows a stale deadline.
- Money is an append-only double-entry ledger. Balances (what the hotel is owed, what a hotel owes BookDan) are computed from it; there is no mutable balance column.
- Side effects are best effort and never block or undo the action. The corresponding trade-off is the lack of an outbox (11).
- Personal data is minimised: ID numbers are encrypted and hashed, shown masked, revealed only with an audit entry, and erased after a configurable retention period; logs carry no bodies, tokens or codes.

# 3. Invariants enforced in the database


| Invariant | Mechanism (migration) | What it prevents |
|---|---|---|
| Never oversold | inventory(room_type_id, night, capacity, held, booked) with CHECK held>=0 AND booked>=0 AND held+booked<=capacity (00004). Hold is one UPDATE ... SET held=held+n WHERE held+booked+n<=capacity over all nights; affected rows must equal the number of nights, else SOLD_OUT and the tx rolls back. | Double booking under any concurrency, including walk-ins and blocks (which also use inventory.booked). |
| Legal status changes only | booking_transition_allowed(from,to) IMMUTABLE function + bookings_guard BEFORE UPDATE trigger (00004, redefined in 00010, 00020, 00024). Raises check_violation for anything else. Every status change is also appended to booking_events by trigger (with app.actor / app.reason set via set_config in the tx). | Application bugs that skip states; unaudited changes. |
| Price frozen | bookings_guard also rejects any change to currency, base, service, vat, total, deposit, balance or rooms. | Price drift after the hold. |
| Quote arithmetic | quotes CHECK total=base+service+vat and deposit+balance=total; guests>=rooms; rooms 1..9. bookings.quote_id UNIQUE (one booking per quote). | Inconsistent amounts; reuse of a quote. |
| Append-only history | forbid_change() triggers (UPDATE/DELETE/TRUNCATE) on booking_events, ledger_entries, ledger_transactions, audit_log, hotel_payouts, hotel_receipts; payment_events guard (00004-00031). | Rewriting money or audit history. |
| Balanced ledger | Deferred constraint triggers ledger_check_balanced (entries of a transaction sum to zero) and ledger_check_has_entries (00005). | Money created or lost. |
| One payment at a time | UNIQUE index payments_one_pending_per_booking and payments_one_success_per_booking (partial indexes); UNIQUE(provider, provider_ref) (00005). | Double charge; two successes for one booking. |
| Each gateway event applied once | payment_events UNIQUE(provider, event_id); INSERT ... ON CONFLICT DO NOTHING; zero rows means a repeat (00005). | Duplicate/out-of-order webhooks. |
| Rate plans do not overlap | EXCLUDE USING gist (room_type_id =, daterange(valid_from, valid_to, [])  &&) on rate_plans (00003, needs btree_gist). | Two prices for one night. |
| One open dispute / relocation per booking | Partial unique indexes support_one_open_dispute and support_one_open_relocation on support_tickets(booking_id) WHERE kind=... AND status<>closed (00024, 00028). | Parallel disputes; double relocation. |
| Idempotent booking creation | idempotency_keys(user_id, key) with request_hash; same key and body returns the same booking, same key and different body is IDEMPOTENCY_MISMATCH (00004). | Duplicate bookings on retry. |
| Guest copy of identity is protected | booking_guests_guard trigger: only the ID-number columns can be erased (purge_id_numbers function); name and rest are immutable (00008). | Altering what the hotel was shown. |
| Identity data shape | users CHECK phone ~ ^\+[1-9][0-9]{7,14}$, email lower-case and well-formed, at least one identifier; role and status enumerations; bookings.code ~ ^BD-[A-Z0-9]{6}$ from an alphabet without 0/O/1/I. | Malformed identities; ambiguous codes. |
| API cannot reach the data | Schema "app" with REVOKE for Supabase anon/authenticated roles (00001, 00007). | Supabase's auto REST API exposing tables. |


## Booking status machine


```
HELD ----pay--------------> PAID_AWAITING_HOTEL --accept--> CONFIRMED --check-in--> CHECKED_IN --> COMPLETED
 |  \--(hold ends)--> EXPIRED      |  \--decline / 15 min--> DECLINED_REFUNDED          |  \--stay ends--> COMPLETED
 |                                 \--guest cancel--> CANCELLED_REFUNDED               |  \--no-show--> NO_SHOW
 |                                                                                       \--cancel--> CANCELLED_REFUNDED
CONFIRMED --hotel cannot host--> NEEDS_RELOCATION --restored--> CONFIRMED | --cancel/refund--> CANCELLED_REFUNDED
PAID_AWAITING_HOTEL | CONFIRMED | CHECKED_IN | COMPLETED | NO_SHOW | NEEDS_RELOCATION --dispute--> DISPUTED
DISPUTED --> CONFIRMED | COMPLETED | CANCELLED_REFUNDED     (returns to bookings.disputed_from or is refunded)
```

DISPUTED stops money being released: earnings treat only CHECKED_IN, COMPLETED and NO_SHOW stays as payable. A pay-at-hotel booking passes HELD to PAID_AWAITING_HOTEL without a payment row.


# 4. Data model by domain


| Domain | Tables (migration) |
|---|---|
| Identity | users, otp_challenges, devices (push token, revoked_at, app), refresh_tokens (family_id, rotated_at, revoked_at) (00002) |
| Catalogue | properties (PostGIS location, status, pay_at_hotel), property_staff (role owner/receptionist, shift, alerts), landmarks, room_types, rooms, rate_plans (exclusion constraint), rate_boosts, pricing_config (commission_bps, commission_vat_bps, VAT, service) (00003, 00015), property_photos incl. room photos (00009, 00018), hotel_business (encrypted payout details) (00011) |
| Inventory and booking | inventory, quotes, bookings, booking_events, idempotency_keys, reviews (00004, 00010, 00016), room_blocks (walk-ins/blocks) (00020), vouchers/sync_actions/change_log (00006) |
| Guest identity | guest_profiles, booking_guests (frozen copy per booking) (00008, 00019); guest_id_photos, booking_id_photos (00021, feature removed, tables empty) |
| Money | payments, payment_events, ledger_accounts, ledger_transactions, ledger_entries (00005); hotel_payouts (00014); partial_refunds (00027); hotel_receipts, statement_emails (00031); payment_issues (00022); coupons, coupon_redemptions (00017); fx_rates (00026) |
| Support and disputes | support_tickets (kind question/problem/dispute/relocation, status open/answered/closed, property_id for hotel staff, outcome), support_messages (00023, 00024, 00028, 00029); bookings.disputed_from, original_check_in/out (00024, 00025) |
| Operations | audit_log (append-only), alert rules on properties/property_staff (00012), hotel_calls (00032), hotel_applications (00033), review replies (00030) |

Ledger accounts (00005 plus later additions): gateway_clearing (asset), guest_deposits_held, property_payable, vat_on_commission_payable, refunds_payable (liabilities), commission_revenue (revenue), coupon_expense and dispute_expense (expenses). Transaction kinds: deposit_received, booking_confirmed, refund, payout, adjustment.


# 5. Scenario handling by flow


## 5.1 Authentication and sessions (auth.go, keys.go, admin.go, accounts.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Request a sign-in code | Identity normalised (phone to +251..., email lower-case). Counts otp_challenges for the identifier in the last 10 min; 5 or more returns 429. Random 6-digit code from crypto/rand; only HMAC hash stored (label "otp:"+identifier), 5 min expiry, client IP stored. Sent via the channel-routed OTPSender. | Send failure returns 502 SEND_FAILED (the challenge row exists but is never usable without the code). Phone channel with OTP_PROVIDER=off returns 503 PHONE_SIGNIN_OFF. Demo mode keeps fixed code 123456 only for addresses/numbers that cannot receive mail/SMS. |
| Verify the code | checkOTP locks the newest live challenge FOR UPDATE, compares hash in constant time, counts every attempt against max_attempts (5). Success consumes the challenge, finds or creates the user (guests created on first sign-in; partner app only admits users in property_staff, else NOT_STAFF), registers the device (X-Device-Id, X-App), issues access + refresh. | A wrong code increments attempts outside the main transaction so the failed attempt is recorded even though the tx errors. Expired or exhausted challenges return the same INVALID_OTP (no oracle). |
| Phone and email are different accounts | users.phone and users.email are separate UNIQUE columns; no merge logic. | Known limitation (11). |
| Authenticate each request | Verify JWT signature/expiry and that claims.App equals the route group's app; then one query: user.status = active AND (device is null OR device.revoked_at IS NULL). | Logout, suspension and account deletion take effect immediately rather than at token expiry. Cost: one extra indexed query per authenticated request. |
| Refresh | Refresh token looked up by hash FOR UPDATE; rotated_at set, new pair issued in the same family. Reuse of a rotated token returns 401 and revokes every token of that family (revocation runs outside the failed tx so it persists). | Concurrent double-refresh: the second sees rotated_at and is treated as reuse, signing the device out (acceptable, documented). |
| Logout | Revokes the refresh family and the device (push token stops). Answers identically for unknown/stale tokens. | An already-issued access token still passes signature but fails the live device check. |
| Admin sign-in | POST /admin/login: email must be in ADMIN_EMAILS and password equal to ADMIN_PASSWORD (SHA-256 then constant-time compare; both checks always run). In-memory limiter: 8 tries / 10 min per ip+email. Upserts a users row with role admin, audit entry admin_login, 8 h token with no refresh. Optional owner portal via OTP when ADMIN_OWNERS=true and real codes are on. | Single shared password and per-process limiter are called out in 11. |
| Account export / delete | GET /me/export returns the guest's data. DELETE /me/account refuses with active bookings (stay to come or money in play). Otherwise: erase profile and ID, phone numbers/notes on bookings, review text, support message text, revoke sessions/devices, delete OTP rows, close account and free the email. | Booking rows, ledger and the hotel's booking_guests copy remain (legal records). One tx. |


## 5.2 Catalogue and search (catalog.go, suggest.go, alternatives.go, search_window.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Search | Free text over name/area/city plus filters; "near a landmark" uses ST_DWithin(landmark.location, property.location, 3000). Availability computed from inventory for the whole stay (missing inventory rows mean all rooms free). Sort by recommended/price/rating/distance, paging 12. Input validation: past dates, > max nights, party/rooms bounds rejected 400. | Read-only; no locks. Non-Latin search needs a UTF-8 database (the local test DB is not, one test is skipped). |
| Sold-out hotels | Still listed with sold-out flag. "Other dates that work" (alternatives.go): from the same stock the booking uses, computes shorter stays inside the request and same-length shifts of up to altShiftDays, never past nights, never beyond party capacity, never nights without a rate; deduplicated; at most 10 hotels x 3 stays. | Suggestions hold nothing; the quote and hold re-check each night. |
| Suggestions | GET /suggest: landmarks, areas, cities, hotel names, public. | - |
| Hotel page | GET /properties/{id}: rooms with live rooms-left, options, house rules, how-to-find-us, review summary and replies, typical reply minutes (over 60 days, shown from 3 answers), photos. Hotel phone is blank on the public page (calls.go). | - |
| Pricing options | options.go: up to four sellable options per room type (room only/breakfast x flexible/non-refundable). Hotel configures breakfast supplement and non-refundable discount per room type. | Pure functions of the base price. |


## 5.3 Quote (booking.go priceStay / createQuote)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Price a stay | priceStay prices every night (rate plan + any boost, then option), multiplies by rooms, subtracts a coupon discount from room charges, then adds service charge and VAT from pricing_config. Whole santim, half-up rounding, no floats. The result and per-night snapshot are stored in quotes with expires_at = now+10 min. | In production PRICING_NOT_CONFIRMED refuses quotes until pricing figures are confirmed. |
| Validation | Dates (check_out > check_in, not in the past in Addis time, <= 30 nights), 1..9 rooms, guests >= rooms and within capacity, room type belongs to the property, property active. | Rate-limited in the "write" group. |
| Coupon | loadCoupon checks active, dates, hotel scope, total uses, uses per guest, first-booking rule, minimum booking; case-insensitive code. Quote stores coupon_id and discount_santim. | The coupon row is locked FOR UPDATE only when a booking is made (next section), so two guests cannot take the last use. |


## 5.4 Hold = create booking (booking.go createBooking)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Retry safety | Step 1 inserts (user, Idempotency-Key, request_hash) into idempotency_keys ON CONFLICT DO NOTHING. If it already existed: same hash returns the stored booking id; different hash gives 409 IDEMPOTENCY_MISMATCH; no response yet gives 409 "still being processed". | Same-key concurrent requests serialise on the unique key insert. |
| Quote validity and ownership | SELECT ... FROM quotes WHERE id AND user_id FOR UPDATE; expired gives QUOTE_EXPIRED; another user's quote looks like not found. | Quote row lock also prevents two bookings from one quote (plus the UNIQUE). |
| Profile required | Guest profile (name, ID type/number, nationality) must be complete (PROFILE_INCOMPLETE). A frozen copy goes to booking_guests with ID encrypted (AES-GCM) and an HMAC hash; guest_name, special requests (<= 400), booked_for_other and traveller phone validated. | - |
| Take the rooms | ensureInventory inserts missing night rows from room_types.total_rooms ON CONFLICT DO NOTHING, then holdRooms (single guarded UPDATE). | Fails whole with 409 SOLD_OUT and rolls back everything; the DB CHECK is the backstop. Tested: 60 concurrent guests, no oversell. |
| Coupon use | Coupon row FOR UPDATE; coupon_redemptions row inserted; counts re-checked under the lock. | Redemption released when the booking is refunded or expires. |
| Booking row | status HELD, hold_expires_at = now+15 min, code from the unambiguous alphabet (retry on UNIQUE violation), amounts copied from the quote, booking_events row written by trigger. | bookings_guard freezes the amounts thereafter. |
| Hold expiry | sweep() sets HELD past hold_expires_at to EXPIRED and releases held rooms (actor "system"). Reads also derive EXPIRED from time. | Sweeper uses FOR UPDATE SKIP LOCKED, so multiple instances or concurrent requests do not collide. |


## 5.5 Payment (payments.go, chapa.go, payment_issues.go, devgateway.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Start payment | POST /bookings/{id}/pay (method chapa\|telebirr\|pay_at_hotel). Locks the booking; requires status HELD and hold not expired (else HOLD_EXPIRED). Online: if a pending payment of the same method exists return the same session; if a different method, mark the old one failed and create a new payments row (provider_ref temporarily "tmp_..." then replaced by the gateway ref), call Gateway.CreateSession, store payment_url. Gateway failure returns 502 PAYMENT_FAILED and the tx rolls back. | Pending-per-booking unique index makes a concurrent second /pay either wait on the row lock or conflict; never two live payments. |
| Chapa specifics | Hosted checkout; merchant reference "bd"+16 hex of the payment id (20 char limit); amounts are in the same unit as santim; result is never trusted from the callback body: chapaSettle calls Chapa's verify API with the secret key and requires status success and matching amount and currency. | Idempotency-Key sent on create and refund calls. |
| Webhook | POST /webhooks/payments/chapa verifies x-chapa-signature (hex HMAC-SHA256 of the raw body with the webhook secret; with no secret nothing is accepted), then chapaSettle(ref). Generic POST /webhooks/payments/{provider} uses an HMAC secret for other gateways and the dev stand-in. | Webhook, return URL and app polling may all call chapaSettle; processPaymentEvent applies each event once. |
| Return URL and polling | GET /payments/chapa/return confirms with Chapa so the booking moves on immediately; getBooking also calls settleChapaPending for the guest's own pending payment, so a lost webhook self-heals when the app opens the booking. | Errors logged only. |
| processPaymentEvent | One tx: INSERT payment_events ON CONFLICT DO NOTHING (0 rows = repeat, return). Lock payment + booking FOR UPDATE. If payment is pending (or failed and result is success) apply: failed -> status failed; succeeded -> status succeeded, ledger deposit_received (gateway_clearing +, guest_deposits_held -), and if booking is HELD move to PAID_AWAITING_HOTEL with hotel_deadline_at = now+15 min (even if the hold clock ran out a moment earlier, because the rooms are still reserved). | If the booking is no longer HELD (rooms already released) the money is not kept: accrueRefund records refunds_payable and runRefunds returns it. Money received is never ignored. |
| Late or out-of-order results | A payment recorded failed (e.g. method switched) that later succeeds is accepted by the rule above and logged as a warning. | Tested. |
| Amount or currency mismatch | Booking is NOT confirmed; payment_issues row created once per (payment, kind); admins and SUPPORT_EMAIL emailed; shown on the admin Payment issues page; a person decides (refund, ask for difference) and marks it resolved with a note. | No automatic refund or retention: deliberate. |
| Pay at hotel | Allowed only if properties.pay_at_hotel and no coupon discount (COUPON_ONLINE_ONLY). Pending online payments are failed; booking goes straight to PAID_AWAITING_HOTEL (payment_method pay_at_hotel, deadline +15 min). No payment row, no ledger entries now (commission accrues at completion, 5.14). | - |
| Deposit | quotes carry deposit_santim and balance_due_santim; only the deposit is charged online (deposit = total unless the hotel sets one). | - |


## 5.6 Hotel answer, deadline, alerts (partner.go, alerts.go, fcm.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Accept | answer(): sweep() first; lock booking by id AND property in the caller's hotels (FOR UPDATE); state must be PAID_AWAITING_HOTEL and deadline in the future. confirm(): status CONFIRMED, deadline cleared, confirmRooms (held -> booked), and for online payments one ledger transaction booking_confirmed: guest_deposits_held debit, coupon_expense, property_payable credit = full - commission - VAT, commission_revenue, vat_on_commission_payable. Commission = round(full * commission_bps / 10000), VAT on commission likewise, both from pricing_config; full = paid + coupon discount. | Accept racing a guest cancel or the deadline sweep: whichever takes the row lock first wins; the other sees a different state and gets INVALID_STATE / DEADLINE_PASSED. Tested. |
| Decline | Requires reason in the fixed set; status DECLINED_REFUNDED, releaseRooms, accrueRefund for the whole payment. After commit runRefunds and notifyGuest. | - |
| No answer in 15 min | sweep() (every 20 s and on every partner read/answer) selects PAID_AWAITING_HOTEL with hotel_deadline_at <= now FOR UPDATE SKIP LOCKED LIMIT 100, calls decline(reason no_response, actor system); guests are notified after commit. | A late accept returns DEADLINE_PASSED (the guest has already been refunded). |
| Alert fan-out | notifyHotel on payment: recipients = staff of the hotel with a push token and alerts on. Mode "everyone": all on duty; "front_desk_first": receptionists on duty, falling back to everyone on duty then owners if nobody can be reached; owners alerted after escalate_after_min. Shift evaluated in Addis time and may cross midnight; no shift means always on duty. Reminder 5 minutes before the deadline to everyone on duty plus owners, plus email to owners and the business contact. | stepAlerts claims each escalation/reminder by setting a timestamp BEFORE sending, so a slow send or two instances never send twice. |
| Dead push tokens | Pusher returns tokens FCM no longer knows; they are cleared. | Push failure is logged only. |
| Unpaid holds | Partner queries exclude HELD bookings entirely. | - |


## 5.7 Guest cancellation (cancel_review.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Rules | Allowed from PAID_AWAITING_HOTEL, CONFIRMED, NEEDS_RELOCATION. Not after check-in time (12:00 Addis on check-in date) except from NEEDS_RELOCATION. Refund = everything left (payment minus partial refunds) when: awaiting hotel, relocation, or (refundable AND now <= check-in - 48 h); otherwise zero (non-refundable or late). Pay-at-hotel: nothing to refund. | Decision inputs read under FOR UPDATE on the booking. |
| Effects | status CANCELLED_REFUNDED with refund_santim, cancelled_at, cancelled_by guest; rooms: unbookRooms if it had been confirmed, releaseRooms if only held; relocation ticket auto-closed with outcome relocation_cancelled. Refund > 0: awaiting hotel -> accrueRefund; confirmed -> reverseConfirmedAndOweRefund (reverses exactly the booked hotel share, commission and VAT and owes the guest, one balanced ledger tx). Late cancel: the hotel keeps its share, no money moves. | Tested: 20 simultaneous cancels produce one refund; accept racing cancel is clean. |
| After commit | Hotel push, guest email, runRefunds. | Best effort. |


## 5.8 Refund engine and partial refunds (payments.go runRefunds, partialrefund.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Full refund execution | accrueRefund records refunds_payable (ledger "refund owed"). runRefunds selects up to 50 payments with status succeeded whose booking is DECLINED_REFUNDED, EXPIRED, or CANCELLED_REFUNDED with refund_santim > 0, locks each FOR UPDATE SKIP LOCKED, computes amount = payment - partial refunds, calls Gateway.Refund(ref, "", amount), marks payment refunded and writes ledger "refund paid" (refunds_payable debit, gateway_clearing credit) in the same tx. | Called after every state change that can owe money and by the 20 s sweeper, so a failure is retried until accepted. Failure: slog + ops.Report("refund-failed") (throttled). Chapa Idempotency-Key "refund-"+providerRef makes a retry after a commit failure safe. Note the gateway HTTP call happens while the row lock/tx is open (11). |
| Partial refund rules | Only for bookings whose hotel share is booked (CONFIRMED, CHECKED_IN, COMPLETED, NO_SHOW, DISPUTED while settling). 0 < amount < payment and <= payment minus earlier partial refunds. lockRefundable locks the payment row, so concurrent refunds cannot spend the same money. Who bears it: "hotel" -> reverseShareOfConfirmed reverses the proportional part of hotel share, commission and VAT (rounding difference absorbed by the hotel share so the tx balances exactly) and owes the guest; "platform" -> expense dispute_expense, hotel keeps its share. | partial_refunds rows carry their own refund key; runPartialRefunds sends one gateway refund per row (key = refund id) and books the ledger once accepted; retried until accepted. |
| Cancel after partial refund | lockRefundable returns Left; cancellation/dispute full refund refunds only what is left and reverses only the matching share. | - |
| Hotel already paid out | Hotel-pays outcomes still reverse property_payable, driving the hotel's balance negative (owes) against its next payout. | See 11. |


## 5.9 Voucher, check-in and offline check-in (checkin.go, checkinsync.go, keys.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Voucher | GET /bookings/{id}/voucher (CONFIRMED/CHECKED_IN only, else VOUCHER_NOT_READY): token = base64url(payload) + "." + base64url(Ed25519 signature) with payload v, kid, pid (property), code, in, out, exp (day after check-out). PIN derived from booking id (HMAC) so it is stable; only its hash is stored. Public key at GET /keys/voucher. | Nothing about the voucher state is stored on the phone that the server cannot re-verify. |
| Lookup | POST /partner/checkin/lookup by token or by reference + PIN. Token: signature and expiry verified; must belong to one of the caller's hotels. PIN: constant-time compare; 5 wrong tries lock the PIN path for 30 min per booking (the QR path still works). checkinState: ready, checked_in, too_early, ended, cancelled, waiting, disputed, not_ready. | Wrong hotel gives not found (no existence oracle). |
| Confirm check-in | POST /partner/bookings/{id}/check-in: lock booking (own hotels), state CONFIRMED, window = check-in date to 06:00 on check-out date (Addis); set CHECKED_IN, checked_in_at, who. Twice is harmless (returns current). | Audit entry; concurrent double scan resolved by the row lock. |
| Offline sync | POST /partner/checkin/sync: 1..100 items {token, at}. For each: verify voucher again, find booking, hotel check, reject cancelled/declined/disputed, apply only if the claimed time is inside the stay and not in the future or older than 30 days. Returns per item {reference, result} with result checked_in, already, cancelled, wrong_hotel, invalid, not_found, too_early, ended, not_ready, bad_time. Recorded check-in time is the phone's time; audit action guest_checked_in_offline. | The server never trusts the phone. Items are independent; a repeat is "already". Tested with ten mixed items. |
| Show full ID | POST /partner/bookings/{id}/guest/reveal decrypts the stored number for a booking of the caller's hotels; every call writes audit_log; erased numbers return 410 GONE. | - |


## 5.10 Stay lifecycle, walk-ins, blocks, no-show (rooms.go, partner.go, maintenance.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Rooms chart | GET /partner/rooms: free rooms per room type for the next 14 nights from inventory. | Read-only. |
| Walk-in / block | POST /partner/rooms/blocks: takes rooms on every night via the same guarded UPDATE (inventory.booked), writes room_blocks; SOLD_OUT if any night is short. DELETE releases (second release harmless). | Same CHECK constraint protects against selling a blocked room. |
| No-show | POST /partner/bookings/{id}/no-show: CONFIRMED and check-in day reached; status NO_SHOW; unused nights unbooked; nothing refunded; hotel share becomes payable. | Row lock; illegal states rejected by trigger. |
| Stay ends | Daily maintenance: CONFIRMED/CHECKED_IN bookings past check-out become COMPLETED (so hotels that never use the app still get paid). | Idempotent. |


## 5.11 Hotel cancels / cannot host (hotelaction.go, hotelsupport via support.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Hotel cancels (owners only) | POST /partner/bookings/{id}/cancel with reason from a fixed set. hotelSideCancel (also used by admin with by=admin): locks booking, requires a confirmed-like state, unbooks future rooms, sets CANCELLED_REFUNDED with cancelled_by hotel/admin and cancel_reason, refunds in full (what is left) via reverseConfirmedAndOweRefund so hotel share, commission and VAT are reversed (hotel bears it). Guest push/email and a support notification. | Owner check via property_staff.role=owner; receptionists get 403. |
| Cannot host (any staff) | POST /partner/bookings/{id}/relocate: CONFIRMED -> NEEDS_RELOCATION; opens a support ticket kind relocation (unique open one per booking). Guest can cancel for a full refund at once (5.7). Admin settles with outcome restored (back to CONFIRMED) or refunded (hotelSideCancel). | Partial unique index prevents a second open relocation. |


## 5.12 Disputes (dispute.go, partialrefund.go, admin.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Open | POST /bookings/{id}/dispute: allowed once check-in time has passed and until 14 days after check-out (earlier -> cancel instead); one per booking (unique open dispute). Sets DISPUTED with disputed_from = previous status, opens ticket kind dispute with the guest's reason, alerts the hotel's phones, mails support. | While DISPUTED earnings exclude the stay, so money is not released. |
| Hotel side | GET /partner/bookings/{id}/dispute shows the guest's account; POST .../dispute/statement adds the hotel's statement (role hotel in support_messages). | Only own-hotel bookings. |
| Withdraw | Guest closes the ticket: booking returns to disputed_from. | - |
| Decision (admin) | POST /admin/support/tickets/{id}/resolve with outcome: rejected (back to disputed_from); refunded_hotel_pays; refunded_platform_pays (expense dispute_expense); refunded_partial_hotel_pays / refunded_partial_platform_pays (amount_santim, via partialRefund, booking returns to its state); each with a note shown to the guest. Ticket closed with outcome. | One tx with the booking locked; invalid outcome/amount rejected before any change. |


## 5.13 Change of dates (datechange.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Eligibility | Booking CONFIRMED, refundable, no coupon, never moved before; old and new check-in both more than 48 h away; same room type; new price identical in every component (base, service, VAT, total, deposit) else PRICE_DIFFERS with the new price. | All checked under the booking lock. |
| Execution | bookRooms takes confirmed rooms on every new night (all or none, SOLD_OUT) then unbookRooms on the old nights; dates updated through a guarded path that keeps amounts; original_check_in/out stored; audit entry; hotel push and guest email. | If the new nights are short the whole tx rolls back and the old booking is untouched. |
| Not built | Different price (would need a second payment or partial refund). | - |


## 5.14 Hotel money: earnings, payouts, receipts, statements, pay-at-hotel commission (earnings.go, statements.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Earnings states | From the ledger per booking: upcoming (confirmed, guest can still cancel), ready (CHECKED_IN, COMPLETED, NO_SHOW), paid (hotel_payouts). Owed now = ready - paid. Disputed and cancelled stays contribute nothing. Receptionists get no money screens. | Computed on read from append-only data; no mutable balance. |
| Record payout (admin) | POST /admin/hotels/{id}/payouts: amount must be <= ready minus paid (checked after locking the hotel's properties row FOR UPDATE), writes hotel_payouts (append-only) with transfer reference, ledger kind payout, audit entry, email to owners. | Two admins recording simultaneously cannot exceed the balance. |
| Pay-at-hotel commission | accrueHotelCommissions (daily and on startup, also catches older stays): for COMPLETED pay-at-hotel bookings without hotel_commission_booked_at, books commission + VAT as a debit on property_payable (so it reduces what BookDan owes that hotel; if negative the hotel owes BookDan) and sets the marker. Commission computed on the booking total as for online bookings. | Marker column makes it exactly-once. |
| Receipt (admin) | POST /admin/hotels/{id}/receipts records money a hotel paid BookDan, capped at owes_santim; append-only hotel_receipts, ledger entry. | - |
| Statement | Calendar month (Addis): stays that ended, online (share, commission, VAT) and pay-at-hotel (commission owed), payouts and receipts, opening and closing balance; JSON, CSV (csvCell defuses leading = + - @ before quoting), and a monthly email to owners + business contact sent once (statement_emails unique per hotel/month). | A restart never resends. |


## 5.15 Support (support.go, hotel_support in support.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Ticket lifecycle | POST /support/tickets (guest) or /partner/support/tickets (hotel staff, marked with property_id; booking must be theirs). Status open -> answered (staff reply) -> open (guest reply) -> closed. Messages have role guest/hotel/support derived from author. Text cleaned and length-limited (cleanText). | addMessage requires status open or answered; closing twice is fine. |
| Notifications | mailSupport emails admins and SUPPORT_EMAIL on new tickets and replies; mailGuestAnswer emails the guest (phone-only guests see it in the app). | Never fails the action. |
| Admin actions | List queue, read thread, reply, close, resolve dispute, goodwill partial refund (/refund), settle relocation. | Refund and resolve paths take the booking lock. |
| Erasure | Account deletion overwrites the guest's ticket and message text. | - |


## 5.16 Reviews and replies (cancel_review.go, reviews.go, reviewreplies.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Submit review | Only the booking's guest, only after the check-out date has come (REVIEW_NOT_READY), one per booking (ALREADY_REVIEWED, UNIQUE), rating 1..5 and category ratings, comment <= 1000. | - |
| Hotel reply | Owners only; PUT /partner/reviews/{id}/reply creates or replaces, DELETE removes; shown on hotel page and on the guest booking. GET /partner/reviews lists latest reviews, unanswered first. | Ownership checked against property_staff. |


## 5.17 Coupons, options and FX (coupons.go, options.go, fxrates.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Coupon admin | Create/update with percent or fixed, cap, minimum, dates, total and per-guest limits, first-booking-only, hotel scope. Code never changes after creation. | - |
| Coupon accounting | At confirm the hotel is paid on paid+discount and the discount is booked to coupon_expense; use returned when refunded/expired. | Tested. |
| FX display rates | fx_rates(currency, birr_per_unit_e4 integer, updated_at). PUT /admin/rates/{currency} (USD, EUR); a change over 30% needs explicit confirmation; GET /rates is public and marks a rate stale after 30 days so apps show ETB only. Charging and refunds are ETB only. | Display only; no money path reads fx_rates. |


## 5.18 Hotels, onboarding, applications, photos (admin_hotels.go, admin_photos.go, applications.go)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Create/edit hotel | Admin CRUD for hotel, rooms, rate plans (exclusion constraint on overlap), options, features, map pin, texts (en/am), pay_at_hotel, deposit. Business details and payout account stored encrypted; GET shows last four; POST .../payout/reveal decrypts and audits. | All writes audited. |
| Publish / hide / restore | Publishing requires rooms, prices and business details. Hiding with upcoming bookings returns a warning and needs confirm_hide. Restoring needs complete business details. | Existing bookings are not cancelled by hiding. |
| Staff and invitations | Add owner/receptionist (invitation email), alert rules per hotel and shifts per person; removing staff deletes the property_staff row in a tx with an audit entry, so alert recipients (derived from property_staff) exclude them at once; only a platform admin can remove an owner. | Owners may manage only their own receptionists/hotels. |
| Photos | Upload (admin; JPEG, PNG or WebP), camera rotation applied, resized to a main and a thumbnail JPEG, cover selection, per-room photos (max 10), served from /photos/{id}/{size}; a hotel without photos still renders. | Size and type validated before processing. |
| Applications | Public POST /v1/hotel-applications: honeypot field (bots silently succeed), 5 per IP per hour, 2 pending per email, stored with IP. Admin approve creates a DRAFT hotel at the Addis centre and an owner invitation in one tx; reject emails the reason. | Draft cannot be published without the usual checks. |


## 5.19 Admin insight and operations (analytics.go, calls.go, alerts in ops)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Analytics | GET /admin/analytics?days=N: funnel (searches/quotes/holds/bookings), money, hotel answer speed, cancellations by who, items waiting for a person. SQL over live tables on each read. | No caching; fine at current volume (11). |
| Call log | POST /bookings/{id}/call (guest, own booking, only if the phone is visible) inserts hotel_calls (best effort). Phone shown only for CONFIRMED/CHECKED_IN on the booking; blank on the public page. GET /admin/calls aggregates per hotel. | Visibility, not a lock. |
| Operator alerts | internal/ops: Report(kind, title, detail) throttled to one per kind per 15 min and 20 per hour, sent to a webhook (Slack/Discord/Teams compatible) and/or email. Triggers: panic, any 500 from s.h, 5xx burst (5 within 5 min), refund failure, daily job failure. | Alerts carry route and request id, never bodies or tokens. |
| Payment issues | 5.5. | - |


## 5.20 Paging, limits and hardening (paging.go, protect.go, config)


| Scenario | Handling (mechanism) | Failure / concurrency semantics |
|---|---|---|
| Keyset paging | ?limit=&cursor=: cursor is base64(created_at, id); query WHERE (created_at,id) < cursor ORDER BY created_at DESC, id DESC LIMIT n+1; next_cursor when more. Defaults keep old behaviour (100 guest, 200 hotel). | Stable under inserts. |
| Rate limiting | Fixed one-minute window per client IP in memory, two groups: reads (search, suggest, property) and writes (POST quotes/bookings...). Limit 0 disables a group. | Default 0 (off) unless RATE_LIMIT_*_PER_MIN is set; per instance; trusts RealIP headers (11). |
| Body and input limits | decode() caps bodies at 64 KiB and rejects invalid JSON; UUID parse errors map to 404; strict field validation per endpoint. | - |
| Security headers / CORS | Standard headers; HSTS when PUBLIC_URL is https; CORS * only outside production. | - |
| Health | GET /v1/healthz liveness (no DB); GET /v1/readyz pings DB with 2 s timeout. | - |


# 6. Endpoint inventory (107 routes)


| Group | Routes |
|---|---|
| Public | GET /healthz, /readyz, /search, /suggest, /properties/{id}, /rates, /keys/voucher, /photos/{id}/{size}; POST /auth/otp/request, /auth/otp/verify, /auth/refresh, /auth/logout, /hotel-applications; POST /webhooks/payments/chapa and /webhooks/payments/{provider}; GET /payments/chapa/return; dev only: GET/POST /dev/gateway/{id} |
| Guest (auth app=guest) | POST /quotes, /bookings; GET /bookings, /bookings/{id}; POST /bookings/{id}/pay, /cancel, /review, /dispute, /change-dates, /call; GET /bookings/{id}/voucher; PUT /me/devices/push-token; GET/PUT/DELETE /me/profile; DELETE /me/account; GET /me/export; POST/GET /support/tickets, GET /support/tickets/{id}, POST /support/tickets/{id}/messages, /close |
| Partner (auth app=partner) | PUT /partner/devices/push-token; GET /partner/bookings; POST /partner/bookings/{id}/accept, /decline, /cancel, /relocate, /check-in, /no-show, /guest/reveal, /dispute/statement; GET /partner/bookings/{id}/dispute; GET /partner/statements, /reviews, /rooms, /rooms/blocks, /earnings; PUT/DELETE /partner/reviews/{id}/reply; POST/GET /partner/support/tickets (+ {id}, messages, close); POST/DELETE /partner/rooms/blocks; POST /partner/checkin/lookup, /checkin/sync |
| Admin (auth app=admin) | POST /admin/login, /admin/otp/request, /admin/otp/verify; GET /admin/config, /me, /hotels, /hotels/{id}, /coupons, /payouts, /support/tickets, /support/tickets/{id}, /applications, /analytics, /calls, /rates, /payment-issues, /hotels/{id}/statements, /hotels/{id}/payouts; POST/PUT hotels, rooms, prices, photos, business, payout reveal, coupons, ticket reply/close/resolve/refund/relocation, application approve/reject, rates, payment-issue resolve, receipts, payouts, alerts, staff add/remove |

Authoritative contract: backend/api/openapi.yaml (kept in sync as features were added).


# 7. Background work


| Job | Trigger | What it does | Idempotency / safety |
|---|---|---|---|
| sweep() | Ticker every 20 s; also at the start of partner booking list and answer | stepAlerts (escalations and reminders); expire holds; decline unanswered requests; runRefunds; notify guests of no-response | FOR UPDATE SKIP LOCKED, LIMIT 100; claim-before-send timestamps |
| runRefunds / runPartialRefunds | After state changes and from sweep() | Send owed refunds to the gateway; ledger on acceptance | SKIP LOCKED per payment; gateway idempotency keys; retried until accepted |
| RunMaintenance | On server start and when the Addis date changes | Complete finished stays; accrue pay-at-hotel commission; erase old ID numbers (ID_RETENTION_DAYS, off by default; unfinished bookings after 7 days); email last month's statements once | Marker columns / unique rows; failure reported via ops.Report |
| Offline sync (client side) | Partner phone every minute and on app foreground | POST /partner/checkin/sync | Server idempotent per booking |

There is no distributed lock: every instance runs the same loops. Correctness relies on row locks, SKIP LOCKED and markers rather than on a leader (see 11).


# 8. External integrations and their failure semantics


| Integration | Use | Failure behaviour |
|---|---|---|
| Chapa (payments + refunds) | Create hosted payment, verify transaction, refund (v2 /refunds with Idempotency-Key) | Create failure: /pay returns 502 PAYMENT_FAILED, nothing persisted. Verify failure: booking stays pending, retried on next webhook/return/poll. Refund failure: stays owed, retried every 20 s with alert. |
| SMTP / Brevo email | Sign-in codes, booking emails, support, statements, alerts | Sign-in code failure returns SEND_FAILED; every other email is logged and swallowed. |
| Africa's Talking / Twilio SMS | Sign-in codes (+251 mobile numbers only; message and code never logged) | Tested against fakes only; first real SMS pending. OTP_PROVIDER=off turns phone sign-in off cleanly. |
| Firebase Cloud Messaging | Hotel and guest push | Failures logged; unknown tokens cleared; no retry queue. |
| Operator webhook/email | ops.Report | Best effort; throttled. |
| OpenStreetMap tiles | Client-side map only | Not a backend dependency. |


# 9. Configuration surface

Environment variables (see backend/.env.example): APP_ENV, HTTP_ADDR, PUBLIC_URL, DATABASE_URL, DEMO_MODE, DEMO_ACCOUNT_DOMAINS, JWT_SECRET, ID_ENCRYPTION_KEY, VOUCHER_SIGNING_SEED, PAYMENT_PROVIDER, PAYMENT_WEBHOOK_SECRET, CHAPA_SECRET_KEY, CHAPA_WEBHOOK_SECRET, OTP_PROVIDER, SMS_FROM, AT_USERNAME, AT_API_KEY, AT_SANDBOX, TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN, EMAIL_PROVIDER, SMTP_HOST/PORT/USER/PASSWORD, BREVO_API_KEY, EMAIL_FROM, PUSH_PROVIDER, FCM_SERVICE_ACCOUNT_B64, ADMIN_EMAILS, ADMIN_PASSWORD, ADMIN_OWNERS, PARTNER_APP_URL, ID_RETENTION_DAYS, RATE_LIMIT_READ_PER_MIN, RATE_LIMIT_WRITE_PER_MIN, ALERT_WEBHOOK_URL, ALERT_EMAIL, SUPPORT_EMAIL.

Production guard (config.Validate) rejects: demo mode on; missing DATABASE_URL, JWT_SECRET, ID_ENCRYPTION_KEY, VOUCHER_SIGNING_SEED; JWT secret under 32 chars; PUBLIC_URL not https; PAYMENT_PROVIDER=dev or a Chapa key containing TEST; OTP_PROVIDER log or an AT sandbox; EMAIL_PROVIDER not smtp/brevo; PUSH_PROVIDER=log; admin password under 16 chars or without ADMIN_EMAILS; ADMIN_OWNERS with demo codes. Business constants in code: quote 10 min, hold 15 min, hotel answer 15 min, 1..9 rooms, 30 nights, access token 15 min, refresh 30 days, free cancel 48 h, check-in 12:00. Money rates in pricing_config (commission_bps 1500, commission_vat_bps 1500 after migration 00015).


# 10. Testing

- 229 Go test functions across internal/api, config, server, ops and dbtest. Default suite: go test ./... against an embedded PostgreSQL started by dbtest with all migrations applied (no PostGIS there; the Server.postgis flag switches to a plain-text coordinate path in tests).
- Audit suite behind build tag audit (go test -tags audit ./internal/api -run Audit_): 55 tests - search validation and load (1,000 searches by 100 workers), 60 simultaneous holds, 20 simultaneous cancels, accept racing cancel, duplicate/late/out-of-order payment events, amount mismatch, refund retry, authorization across guest/partner/admin, deleted or revoked accounts, notifications sent once, mail outage not undoing a booking, review bounds, logs free of tokens, rate limits and headers. Result: 54 pass, 1 skipped (non-Latin names need a UTF-8 database).
- Feature tests per area: support, disputes, date change, FX, SMS (against fakes of both providers), partial refunds (including a Chapa fake), hotel cancel/relocation, hotel support, review replies, statements and CSV defusing, calls, analytics, paging, applications, offline sync, ops throttling and alerting.
- Not covered by automated tests: real Chapa live payments/refunds, real SMS, real FCM delivery to devices, non-Latin search on the real database, load against Supabase, migration down/up with real data, backup restore.

# 11. Known weak spots and questions for the reviewer

We list these ourselves so the review can start with the hard questions. Each has a short note on why it is acceptable today or what we would change.


| # | Topic | Observation | Question / likely change |
|---|---|---|---|
| 1 | Gateway call inside a DB transaction | runRefunds holds a row lock (FOR UPDATE SKIP LOCKED) and a pooled connection across the Chapa HTTP call. If Chapa accepts and the commit fails, the refund is retried; it is safe only because of the Idempotency-Key. | Move to a refund-intent row (status pending/sent/confirmed) processed outside the tx, or an outbox. Is the current approach acceptable at our volume? |
| 2 | No outbox for push/email | Notifications are sent after commit in the request goroutine; a crash between commit and send loses them (escalations/reminders are protected by claim timestamps, other messages are not). | Transactional outbox + worker? |
| 3 | runRefunds runs inline in request paths | Up to 50 refunds processed synchronously after accept/decline/cancel/payment events, adding latency and gateway coupling to user requests. | Make it worker-only and just signal it. |
| 4 | Writes on read paths | sweep() runs on GET /partner/bookings and on answer so hotels never see stale deadlines. Safe (SKIP LOCKED) but a GET mutates state. | Keep for UX, or rely on the 20 s worker only? |
| 5 | Multi-instance behaviour | Every instance runs the sweeper and the daily maintenance. Idempotent by locks and markers, but there is no advisory lock/leader, so work is duplicated and the 20 s cadence multiplies with instances. | pg_advisory_lock leader for RunMaintenance and maybe the sweeper? |
| 6 | Rate limiting | In-memory fixed window per process, per client IP from RealIP headers; default 0 (off) unless env is set; sign-in code requests are limited per identifier only, not per IP. | Edge/shared limiter (Redis or provider) and per-IP OTP limits before launch. |
| 7 | Admin authentication | One shared ADMIN_PASSWORD for all platform admins, in-memory login limiter, 8 h token with no refresh/revocation list, no MFA, no separate roles (support/finance). | Per-admin credentials with MFA, scoped roles, revocable sessions. |
| 8 | Per-request liveness query | authenticate() queries users/devices on every authenticated request to make logout and suspension immediate. | Cache with short TTL, or accept the cost? |
| 9 | JWT algorithm | HS256 with one symmetric secret shared by all instances. | Asymmetric (Ed25519) or key rotation scheme (kid). |
| 10 | Money model gaps | Currency columns allow USD/EUR but all paths are ETB only; fx_rates is display only. A hotel paid out earlier can go negative after a hotel-pays dispute (it owes BookDan against future payouts); no mechanism if it has no future payouts. | Policy plus a collections flow; drop unused currency values? |
| 11 | Date change with a price difference | Refused; would need a second payment or partial refund. | Design the supplement payment as a child of the booking? |
| 12 | Partial-refund on Chapa | Tested only against a fake of Chapa's v2 refunds endpoint. | One real small partial refund before relying on it. |
| 13 | Test/production divergence | Tests run without PostGIS and on a WIN1252 database (non-Latin search test skipped); production uses PostGIS and UTF-8. | Run the suite against a Supabase-like Postgres image with PostGIS, UTF-8. |
| 14 | Analytics | Computed live from OLTP tables on each request; no caching or read replica. | Materialised/rollup tables when volume grows. |
| 15 | ID number handling | AES-GCM with a key from env, HMAC hash for matching, retention erasure job off until a legal period is chosen; booking_guests copy retains name after account deletion. | Key rotation strategy (envelope keys, key id in ciphertext)? |
| 16 | Phone and email identities not mergeable | Same person can hold two accounts. | Merge flow or phone-only/email-only policy? |
| 17 | Observability | Structured request logs and operator alerts exist; no metrics, tracing or error tracking inside the phone apps; no backups restore test yet. | Prometheus/OpenTelemetry, Sentry, PITR drill. |
| 18 | Pagination coverage | Guest and hotel booking lists are keyset-paged; several admin lists (tickets, applications, payment issues, hotels) are bounded but not paged. | Add paging before data grows. |


## Suggested review order

- Section 3 and the migrations 00004, 00005, 00024: the invariants. If these are sound, most races are already harmless.
- payments.go (processPaymentEvent, runRefunds), partner.go (answer, confirm, decline, sweep) and cancel_review.go: the money-moving transactions.
- partialrefund.go (reverseShareOfConfirmed rounding) and statements.go (balances, CSV): the arithmetic.
- auth.go and keys.go: token and secret handling; admin.go for the admin gap.
- checkin.go and checkinsync.go: untrusted input from phones.
- The audit tests (audit_*_test.go) to see which concurrency claims are actually exercised.