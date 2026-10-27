# Pre-release audit, Phase A: results

Scope: the BookDan backend. No application code was changed in Phase A; only tests were added
(`backend/internal/api/audit_search_test.go` and `audit_booking_test.go`, behind the `audit` build tag so the normal
`go test ./...` stays green). Run them with `go test -tags audit ./internal/api -run Audit_`.
Database: an embedded PostgreSQL started by the test suite, with the demo seed. No production data and no real payment keys.

## 1. Stack (Phase 0)

| Item | What it is |
|---|---|
| Language / framework | Go 1.26 (module), `chi` router, `net/http` |
| Database / ORM | PostgreSQL (Supabase in production). No ORM: `pgx` with hand-written SQL. Migrations with `goose` (21 so far) |
| Auth | One-time codes by email (SMS not built); short-lived JWT access tokens and rotating refresh tokens with reuse detection; roles by app (guest, partner) and a separate admin sign-in (password, or code for owners) |
| Payments | Chapa (hosted checkout, v2 API) plus a development stand-in; a double-entry ledger in the database |
| Background work | One in-process worker: a 20-second sweep (expired holds, unanswered hotel requests, alerts, refunds) and a daily maintenance job (complete finished stays, erase old ID numbers) |
| Integrations | Chapa, Brevo (email), Firebase (push), OpenStreetMap tiles (admin map) |
| Existing tests | Go `testing`, integration tests against a real embedded PostgreSQL (101 tests across 28 files before this audit), reused here |

**Endpoints:** guest (search, suggest, property, quotes, bookings, pay, cancel, review, voucher, profile, push token), partner
(bookings, accept, decline, check-in, no-show, check-in lookup, rooms chart and blocks, earnings, reveal ID, push token),
admin (hotels, rooms, prices, photos, business details, coupons, payouts, alert rules, staff), payments (Chapa webhook and return,
generic webhook), health (`/healthz`, `/readyz`).

**Tables (37):** users, otp_challenges, refresh_tokens, devices, guest_profiles, properties, room_types, rooms, rate_plans,
rate_boosts, inventory, room_blocks, quotes, bookings, booking_events, booking_guests, idempotency_keys, payments,
payment_events, ledger_accounts, ledger_transactions, ledger_entries, coupons, coupon_redemptions, reviews, vouchers,
hotel_business, hotel_payouts, property_staff, property_photos, landmarks, pricing_config, audit_log, change_log, sync_actions,
and the two empty ID-photo tables from migration 00021.

## 2. Booking state machine as it exists

States allowed by the database: HELD, PAID_AWAITING_HOTEL, CONFIRMED, CHECKED_IN, COMPLETED, EXPIRED, DECLINED_REFUNDED,
CANCELLED_REFUNDED, NO_SHOW, NEEDS_RELOCATION, DISPUTED.

```
HELD ──pay──▶ PAID_AWAITING_HOTEL ──accept──▶ CONFIRMED ──check-in──▶ CHECKED_IN ──stay ends──▶ COMPLETED
 │                    │  │                        │  │                    
 │ 15 min             │  └─decline/no answer─▶ DECLINED_REFUNDED          └─(also CONFIRMED ──stay ends──▶ COMPLETED)
 ▼                    └─guest cancels─▶ CANCELLED_REFUNDED ◀─guest cancels (CONFIRMED)
EXPIRED                                  CONFIRMED ──hotel marks──▶ NO_SHOW
```

The database enforces the transitions with a trigger (`bookings_guard`), so an illegal jump is refused even by a bug in the code.

**Flags:**
- `DISPUTED` and `NEEDS_RELOCATION` are allowed by the database but **no code path or API route ever sets them**. A dispute
  cannot be raised, and a hotel cannot relocate a guest.
- There is no route for a hotel to cancel a booking it already confirmed.

## 3. Results

PASS means a test ran and passed. "(existing)" names a test that already existed in the suite, which passed in this run.
NOT TESTED states why.

### S1 Search
| ID | Scenario | Test | Result | Evidence |
|---|---|---|---|---|
| S1.1 | Free every night, hotel returned | audit_search_test: S1_01 | PASS | |
| S1.2 | Free only some nights, hotel excluded | S1_02 | **FAIL** | The hotel stays in the list with `available=false`. Deliberate design; the brief's rule says excluded. Decision needed |
| S1.3 | Booked only on check-out date, included | S1_03 | PASS | |
| S1.4 | Booked the night before check-in, included | S1_04 | PASS | |
| S1.5 | Missing inventory row treated as unavailable | S1_05 | **FAIL** | A night with no inventory row counts as fully free. Rows are created on demand by a hold, and the database check constraint still stops overselling |
| S1.6 | Check-in after check-out, 400 | S1_06 | PASS | |
| S1.7 | Check-in equals check-out, 400 | S1_07 | PASS | |
| S1.8 | Check-in in the past, 400 | S1_08 | **FAIL** | Returns 200 |
| S1.9 | Stay over the maximum, 400 | S1_09 | PASS | 31 nights refused, 30 accepted |
| S1.10 | Dates too far ahead | S1_10 | **FAIL** | 200 with one hotel and no message (no rates exist two years out) |
| S1.11 | 29 February / DST | S1_11 | PASS | Accepted. DST does not apply: one fixed Addis Ababa timezone (UTC+3) |
| S1.12 | Guests above the largest room | S1_12 | PASS | Hotel listed but not available |
| S1.13 | No match, 200 empty | S1_13 | PASS | |
| S1.14 | Ambiguous place resolved by ID | — | NOT TESTED | Search matches by text; place IDs are not used |
| S1.15 | Hostile input | S1_15 | PASS | SQL strings, emoji, 10,000 characters, NUL: no 5xx, table intact |
| S1.16 | Non-Latin names | — | NOT TESTED | Hotel names in the seed are Latin only; landmark matching needs PostGIS, which the test database lacks |
| S1.17 | Each sort order | S1_17 | PASS | |
| S1.18 | Sort ties deterministic | S1_18 | PASS | Name is the tiebreaker |
| S1.19 | Distance sort without coordinates | S1_19 | PASS | 400 |
| S1.20 | Pagination while stock changes | — | NOT TESTED | Paging is by offset; a stable-cursor test was not written |
| S1.21 | Page size 0 / negative / huge | S1_21 | PASS | The page size is fixed at 12; only the cursor is a parameter, and it is clamped |
| S1.22 | Null rating sorts last | S1_17 | PASS | |
| S1.23 | Mixed currencies converted before sorting | — | NOT TESTED | All prices are ETB; no conversion exists |
| S1.24 | Hidden hotels never returned | S1_24 | PASS | |
| S1.25 | Stale search, availability re-checked | S4_02 | PASS | The hold re-checks and answers 409 SOLD_OUT |
| S1.26 | 1,000 concurrent searches | S1_26 | PASS | p50 67 ms, p95 289 ms, p99 298 ms, 0 failures. One machine with a local database; not representative of production |
| S1.27 | Same search twice, same result | S1_27 | PASS | |
| S1.28 | Bot-like rate gets 429 | S1_28 | **FAIL** | 300 searches in a burst were all answered |
| S1.29 | Place only, no dates | S1_29 | **FAIL** | 400: dates are required |
| S1.30 | Guests/rooms validation | S1_30 | PASS | Children and child ages are not search parameters |

### S2 and S3 Hotel detail, pricing, currency
| ID | Scenario | Test | Result | Evidence |
|---|---|---|---|---|
| S2.1 | Price breakdown adds up | TestBookingPriceIsFrozenAndMustAddUp (existing) | PASS | |
| S2.2 | Rate plans priced correctly | TestRoomOptionsAreOfferedPricedAndRefundedAsAgreed (existing) | PASS | |
| S2.3 | Availability matches search | TestSearchAndPropertyDetail (existing) | PASS | |
| S2.4 | Hotel with zero room types | — | NOT TESTED | |
| S3.1 | Seasonal / weekend prices | — | NOT TESTED | The `rate_boosts` table exists; no admin or test for it |
| S3.2 | Child pricing, extra-guest fees | — | NOT TESTED | Not in the pricing model |
| S3.3 | City tax per person per night | — | NOT TESTED | Only a service charge and VAT exist |
| S3.4 | Currency conversion, rate lock | — | NOT TESTED | Not built. Only ETB is priced |
| S3.5 | Price at booking is the price quoted | TestBookingPriceIsFrozenAndMustAddUp (existing) | PASS | The price is frozen in the quote. A price change between search and quote is not tested |
| S3.6 | Promo codes | TestCouponRulesAreEnforced (existing) | PASS | Not checked: minimum stay, stacking, case sensitivity |

### S4 Booking creation
| ID | Scenario | Test | Result | Evidence |
|---|---|---|---|---|
| S4.1 | Inventory taken only for nights stayed | S4_01 | PASS | |
| S4.2 | Room gone, clean 409 | S4_02 | PASS | `SOLD_OUT` |
| S4.3 | Name validation and any script | S4_03 | PASS | Names in Amharic, Greek, Arabic, Japanese and with apostrophes accepted |
| S4.3b | Email / phone / country validation | S4_06 | PASS | An invalid phone is refused. Email is the sign-in identity. Country is not collected |
| S4.4 | Guests over capacity | S4_04 | PASS | |
| S4.5 | Past, zero and too-long stays | S4_05 | PASS | |
| S4.6 | Booking for someone else | existing childrenRequestsAndBookingForSomeoneElse test | PASS | |
| S4.7 | Logged-out booking | — | NOT TESTED | Not supported: sign-in is required |
| S4.8 | Reference unique and well formed | S4_07 | PASS | `BD-XXXXXX`. Not verified: that it is unguessable (6 characters, about 2 billion values) |
| S4.9 | Hold times out and releases | TestExpiredHoldsAndMissedDeadlinesAreClosedByTheSweeper (existing) | PASS | The job running late or twice was not tested separately |

### S5 Concurrency
| ID | Scenario | Test | Result | Evidence |
|---|---|---|---|---|
| S5.1 | 50 requests for the last unit | S5_01 | PASS | Exactly 1 win, 49 `SOLD_OUT`, count exactly 1 |
| S5.2 | Overlapping ranges | TestOverlappingHoldsNeverOversell (existing) | PASS | |
| S5.3 | Double tap by the same user | S5_03 | PASS | 10 simultaneous taps gave one booking |
| S5.4 | Same key retries / different body | TestFullBookingJourney (existing) | PASS | `IDEMPOTENCY_MISMATCH` |
| S5.5 | Booking and cancelling racing | — | NOT TESTED | |
| S5.6 | Inventory never negative or oversold | S13_01, TestInventoryRefusesToBeOversold | PASS | A database check constraint enforces it |

### S6 Payment
| ID | Scenario | Test | Result | Evidence |
|---|---|---|---|---|
| S6.1 | Success confirms the booking | TestFullBookingJourney, TestChapaPaymentsAreConfirmedWithChapa | PASS | |
| S6.2 | Declined payment | TestFullBookingJourney | PASS | The hold stays so the guest can retry |
| S6.3 | 3-D Secure, expired card, insufficient funds | — | NOT TESTED | Needs the provider's sandbox |
| S6.4 | Lost response, reconciled by the webhook | TestChapaPaymentsAreConfirmedWithChapa | PASS | |
| S6.5 | Webhook duplicate / bad signature | TestPaymentWebhookNeedsASignatureAndDuplicatesChangeNothing | PASS | |
| S6.6 | Webhook out of order or very late | — | NOT TESTED | |
| S6.7 | Payment after the hold expired | S6_01 | PASS | Refunded, the booking never confirms |
| S6.8 | Deposit / pay at hotel | TestPayAtHotelSkipsThePaymentPage (existing) | PASS | Pay at hotel records no commission (see gaps) |
| S6.9 | Currency or amount mismatch | TestChapaPaymentsAreConfirmedWithChapa | PASS | Not confirmed. **No admin screen shows it** |
| S6.10 | Refund API failure retried and visible to admin | — | NOT TESTED | Retried by the sweeper in code; no admin view exists |
| S6.11 | Card data never stored | S6_02 | PASS | No card-like columns. Secrets in the repo: `.env` is git-ignored (not scanned) |

### S7 Cancellation and modification
| ID | Scenario | Test | Result | Evidence |
|---|---|---|---|---|
| S7.1 | Cancel in window, full refund | TestCancellingAConfirmedBookingEarlyRefundsAndReversesTheHotelsShare | PASS | |
| S7.2 | After the deadline | TestCancellingConfirmedBookingTooLateKeepsTheMoneyWithTheHotel | PASS | |
| S7.3 | Non-refundable | TestRoomOptionsAreOfferedPricedAndRefundedAsAgreed | PASS | |
| S7.4 | Cancel twice | S7_01 | PASS | Refused, no second refund |
| S7.5 | Cancel someone else's | S7_01 | PASS | 404 |
| S7.6 | Cancel a completed stay | S7_02 | PASS | |
| S7.7 | No-show | TestAGuestWhoDidNotArriveIsMarkedANoShowAndTheHotelKeepsItsShare | PASS | |
| S7.8 | Modify dates, room or guests | — | NOT TESTED | **Not built** |
| S7.9 | Deadlines in the hotel's timezone | TestCancellationWindow | PASS | One timezone (Addis Ababa) |
| S7.10 | Refund in the original currency | — | NOT TESTED | Only ETB |

### S8 Authorization
| ID | Scenario | Test | Result | Evidence |
|---|---|---|---|---|
| S8.1 | Sign-in, refresh, logout, expiry | TestSignInRefreshAndReuseDetection, TestSigningOutEndsTheSession... | PASS | |
| S8.2 | Code expiry and lockout | TestWrongCodesLockTheChallengeAndBadInputIsRefused | PASS | There are no passwords (codes only) |
| S8.3 | IDOR on bookings | S8_01 | PASS | Five endpoints return 404 |
| S8.4 | Role separation | S8_02, TestOnlyHotelStaffCanUseThePartnerApp | PASS | |
| S8.5 | Staff see only their hotel | TestAnOwnerManagesTheirHotelAndTheirReceptionistsOnly | PASS | |
| S8.6 | Account deletion and data export | — | NOT TESTED | **Not built.** Deleting a profile removes the name and ID details only; the account and bookings remain |

### S9 to S12 Hotel side, notifications, reviews, i18n
| ID | Scenario | Test | Result | Evidence |
|---|---|---|---|---|
| S9.1 | Edit rooms, rates, inventory, blocks | TestAdminBuildsAHotelEndToEnd, TestWalkInsAndBlocks... | PASS | |
| S9.2 | Reduce rooms below booked | TestShrinkingRoomsBelowWhatIsBookedIsRefused | PASS | |
| S9.3 | Close a hotel with future bookings | — | NOT TESTED | Hiding a hotel does not touch its bookings |
| S9.4 | Bulk updates atomic and audited | — | NOT TESTED | |
| S10.1 | Notifications sent once | TestTheGuestIsEmailedARequestAConfirmationAndARefund | NOT TESTED | The test checks content, not "exactly once" |
| S10.2 | A failed notification does not undo a booking | — | NOT TESTED | |
| S11.1 | Review rules | TestReviewsNeedACompletedStayAndComeOncePerBooking | PASS | Rating bounds not checked here |
| S12.1 | Unicode, RTL, long text | S4_03 | PASS | Names only |
| S12.2 | Timezones, locales, country rules | — | NOT TESTED | One timezone; no country rules |

### S13 to S17 Integrity, security, reliability, API, operations
| ID | Scenario | Test | Result | Evidence |
|---|---|---|---|---|
| S13.1 | Constraints refuse bad rows | S13_01 | PASS | Negative and oversold stock, bad room blocks, invalid status, no guests, duplicate booking codes |
| S13.2 | Transactions roll back cleanly | TestSeveralRoomsAreHeldAllOrNothingAndFixedAfterBooking | PASS | |
| S13.3 | Audit trail append-only | TestAuditAndHistoryAreAppendOnly, TestLedgerIsAppendOnly | PASS | |
| S13.4 | Migrations on realistic data | — | NOT TESTED | They run on a fresh database in tests; one was applied to production by hand |
| S14.1 | Security headers, no leaks | S14_01 | **FAIL** | `X-Content-Type-Options: nosniff` and HSTS are missing on API responses (the host may add HSTS). No error leaks found |
| S14.2 | Rate limiting | S14_02 | **FAIL** | Quote and booking requests have no rate limit. Only sign-in codes are limited |
| S14.3 | Dependency scan | `govulncheck`, `npm audit` | **FAIL** | Go: 14 vulnerabilities (12 in the standard library of the local Go 1.27.0, fixed in 1.27.2/1.27.3; 2 in `chi` v5.1.0, fixed in v5.3.0). App packages: 11 (7 moderate, 4 high) |
| S14.4 | Webhook and admin endpoints protected | TestPaymentWebhookNeeds..., TestAdminSignInIsStrict | PASS | |
| S15.1 | Timeouts, retries, circuit breakers | — | NOT TESTED | |
| S15.2 | Dependency outage behaviour | TestEmailFailuresAreClearAndSafe | NOT TESTED | Email only |
| S15.3 | Health and readiness | TestHealthz_IsLivenessOnly, TestReadyz | PASS | |
| S15.4 | Load: booking spike | — | NOT TESTED | Only search was load tested |
| S15.5 | EXPLAIN on hot queries, indexes | — | NOT TESTED | |
| S15.6 | Pagination on every list | — | NOT TESTED | By reading the code: the guest bookings list returns the latest 100 with no paging, and the partner list the latest 200. No test was written |
| S16.1 | Consistent error format | TestUnbuiltRoutes_UseTheStandardErrorShape | PASS | |
| S16.2 | Minimum app version check | — | NOT TESTED | **Not built** |
| S16.3 | Idempotency on money POSTs | S5_03, TestOneOpenPaymentPerBooking... | PASS | |
| S16.4 | Push token cleanup | TestPhonesThatFirebaseForgetsAreRemoved | PASS | |
| S16.5 | OpenAPI matches behaviour | — | NOT TESTED | |
| S17.1 | Structured logs, no secrets | — | NOT TESTED | Logs are JSON with the path only; no test |
| S17.2 | Metrics and alerts | — | NOT TESTED | **None exist** |
| S17.3 | Backups and restore | — | NOT TESTED | A hosting setting (Supabase), not verified |

**Totals this audit added:** 42 new tests: 34 pass, 8 fail (S1.2, S1.5, S1.8, S1.10, S1.28, S1.29, S14.1, S14.2). Plus the dependency scans (fail).
The full existing suite still passes.

## 4. Prioritised gap list (for your approval)

**P0 (money loss, double booking, data leak, security hole)**
None found in the paths I tested. Double booking, late payments, refunds, cross-guest access and database constraints all held.
The two items closest to P0 are listed under P1 and are worth checking before anything else.

**P1 (wrong results, broken flow, or a launch blocker)**
1. **Dependency vulnerabilities (S14.3).** Upgrade `chi` to v5.3.0, and confirm the Go version Render builds with is 1.27.3 or later.
2. **No rate limit on search, quote or booking (S1.28, S14.2).** A script can hammer the server.
3. **Chapa payment with a wrong amount sits silently (S6.9).** The money is received, the booking is not confirmed, and no admin screen shows it.
4. **Past dates are accepted in search (S1.8)** and **far-future dates return a hotel with no price (S1.10).**
5. **Place-only search is refused (S1.29).**
6. **Account deletion and data export do not exist (S8.6).** Google Play requires deletion.
7. **No hotel cancellation of a confirmed booking, no date changes, no dispute path (S7.8, section 2).**
8. **No metrics or alerts, and no minimum app version (S17.2, S16.2).**
9. **Pay-at-hotel records no commission (S6.8),** which is a revenue gap, not a bug.

**P2 (polish)**
- Security headers (`nosniff`, HSTS) on API responses (S14.1).
- Lists have no real paging beyond a fixed limit (S15.6).
- A night with no inventory row counts as free (S1.5). The database constraint prevents overselling, so this is a design note.
- The list shows sold-out hotels marked, rather than excluding them (S1.2): a product decision.

## 5. Decisions I need from you before Phase B
- S1.2: keep sold-out hotels in the list (as now), or exclude them as the brief says?
- Pay-at-hotel and the payment model, as discussed earlier.
- What the app should do with a Chapa payment whose amount does not match: alert admins, refund automatically, or both.

Phase B starts when you approve this list. I have not changed any application code.

---

# Phase B: fixes

Decisions you gave: keep sold-out hotels in the list (S1.2), keep pay at hotel, and alert the admins **and** a support address when a
Chapa payment does not match. Nothing here is committed; the changes are in your working folder for you to review and push.

## What changed

| Scenario | Fix | Tested by | Result |
|---|---|---|---|
| S14.3 dependencies | `chi` upgraded from v5.1.0 to v5.3.0 (the 2 library vulnerabilities are gone). The Dockerfile now builds with `golang:1.27` (latest patch), which Render uses | `govulncheck` | The 2 `chi` findings are cleared. 12 standard-library findings remain on **my computer's** Go 1.27.0; they are fixed in Go 1.27.2/1.27.3. **Not verified on Render until you deploy** |
| S1.28, S14.2 rate limits | A per-address limit: 600 reads and 60 booking actions a minute by default in production (`RATE_LIMIT_READ_PER_MIN`, `RATE_LIMIT_WRITE_PER_MIN`); off in development and tests. Payment webhooks and sign-in are never limited here | `internal/server/protect_test.go` (5 tests), Audit S1.28, S14.2 | PASS |
| S14.1 headers | `X-Content-Type-Options: nosniff` and `Referrer-Policy` always; HSTS when the public address is https | `protect_test.go`, Audit S14.1 | PASS |
| S1.8 past dates | Search refuses a check-in before today (Addis Ababa time) with 400 | Audit S1.8 | PASS |
| S1.10 far future | Search refuses dates more than 365 days ahead (400) | Audit S1.10 | PASS |
| S1.29 place only | Search without dates returns hotels with a "from" price and `dates_known: false` | Audit S1.29 | PASS |
| S1.2 sold-out hotels | **Decision: keep them in the list**, marked unavailable. The test now states that rule | Audit S1.2 | PASS (rule changed by you) |
| S1.5 missing inventory row | **Kept as designed:** rows are created on demand. The test now proves what matters: such a night cannot be oversold | Audit S1.5 | PASS (rule changed) |
| S1.11 leap day | A real search cannot reach 2028 any more (365-day limit), so the night count is tested directly, plus the horizon | `NightCountsAcrossLeapDaysAndYearEnds`, Audit S1.11 | PASS (test replaced) |
| S6.9 payment mismatch | The mismatch is recorded once (`payment_issues`, **migration 00022**), the admins and `SUPPORT_EMAIL` are emailed once, and a new admin page **Payment issues** lists it and lets staff mark it resolved with a note | Chapa test: raised once, emailed once, listed, resolved once, guests refused | PASS |
| S8.6 account deletion and export | `DELETE /me/account` and `GET /me/export`, plus a **Delete my account** action in the guest app. Refused while a booking is active; removes the profile, full ID number, phones and notes on bookings, review text, codes; revokes every session and phone; frees the email address. Bookings, the ledger and the hotel's copy of the name are kept as records | `accounts_test.go` (2 tests) | PASS |
| **New finding** access after sign-out | **A valid signed token kept working after logout, suspension or deletion until it expired.** Every request now also checks that the account is active and the phone is not revoked | `accounts_test.go` | PASS |

API documentation (`openapi.yaml`) and `.env.example` are updated.

## Final results
- Backend: the whole suite passes. Audit: **42 of 42 pass** (before: 34 pass, 8 fail). Domain tests: 35 of 35.
- Both apps typecheck. The guest app and the admin page changed; the partner app did not.

## Remaining risks
1. **The rate limit trusts the proxy's address headers.** A client that sets its own `X-Forwarded-For` could dodge a per-address limit. Render sets these headers itself, but a limit at the edge (a CDN or Render's own protection) is the stronger fix.
2. **Several areas were never tested (NOT TESTED above):** provider sandbox flows (3-D Secure and the like), exactly-once notifications, a booking spike under load, query plans, backups, the OpenAPI document against real behaviour, and booking-versus-cancel races.
3. **Pay at hotel records no commission** and holds no money. This is the open payment-model decision.
4. **Missing features, not bugs:** a hotel cancelling a confirmed booking, changing dates, a dispute path, relocation, and hotel-side support. `DISPUTED` and `NEEDS_RELOCATION` exist in the database but nothing can set them.
5. **No metrics, alerts or minimum-app-version check.** You would learn about problems from users. External monitoring (for example Sentry and Render's alerts) is the quickest fix.
6. **Deleted accounts keep a few records** (the booking, ledger and the hotel's copy of the guest's name). Have a lawyer confirm that is right for Ethiopia and for the app stores.
7. **Lists are not paged** (latest 100 for guests, 200 for hotels).

## Decisions you still owe
- Pay at hotel: how is the commission collected (an online deposit, a monthly invoice to the hotel, or a fee added for the guest)?
- Who handles disputes, hotel cancellations and date changes, and under what rules?
- The legal retention period for guest data (ID numbers and the hotel's copy).
- How long a guest account deletion should wait after a stay ends (today: allowed once nothing is active).

## Pre-launch checklist
1. **Apply migration 00022** to Supabase (say "run the migration"); until then the Chapa mismatch alert cannot save.
2. Set on Render: `SUPPORT_EMAIL`, `ADMIN_EMAILS`; optionally `RATE_LIMIT_READ_PER_MIN` and `RATE_LIMIT_WRITE_PER_MIN`.
3. Deploy, then confirm the build used Go 1.27.3 or later (run `govulncheck` on a fresh checkout).
4. Publish the guest update (it carries the delete-account action).
5. Chapa: finish live activation, set the live key and the webhook secret.
6. Decide the payment model for pay at hotel.
7. Choose an SMS provider, or launch with email sign-in only.
8. Set `ID_RETENTION_DAYS` after legal advice.
9. Turn on backups and point-in-time recovery in Supabase and test a restore.
10. Add error monitoring and alerts.
11. Load-test booking creation against a staging copy of the database.
12. Store listing, privacy policy and terms.

---

# Second testing pass (the areas that were NOT TESTED)

Test file: `backend/internal/api/audit_more_test.go` (build tag `audit`). 13 tests: 10 pass, 1 is skipped, 2 fail and are real findings. No application code was changed in this pass.

| Area | Test | Result |
|---|---|---|
| S2.4 | A hotel whose only room type has 0 rooms opens, shows none left, and is listed as unavailable | Pass |
| S1.16 | Names in Amharic, Japanese, Arabic and accented Latin are found by search | **Skipped**: the embedded test database is WIN1252, not UTF8. Check it on a UTF8 database (production Supabase is UTF8). One more thing to check there: search lowers case with the database's `lower()`, so with a "C" collation `Ü` and `ü` would not match. |
| S3.6 | A coupon code works in lower case, upper case and with spaces around it | Pass |
| S5.5 | Hotel accepts while the guest cancels (6 races): always either CONFIRMED with the room taken and no refund, or CANCELLED with the room back once and one refund | Pass |
| S5.5b | 20 cancels of one booking at the same instant: exactly one refund | Pass |
| S6.6a | A late "failed" after "succeeded" does not undo the paid booking | Pass |
| **S6.6b** | A "succeeded" that arrives after an earlier "failed" | **FAIL (finding F1)** |
| S6.10 | The gateway refuses a refund: nothing is recorded as paid, it is tried again when the gateway recovers, and paid exactly once | Pass |
| **S9.3** | Hiding a hotel that has a confirmed booking to come | **FAIL (finding F2)** |
| S10.1 | Request, confirmation and cancellation emails are sent once even when the action is repeated | Pass |
| S10.2 | A failing mail server does not undo an accepted booking; the voucher still works | Pass |
| S11 | Review rating 0, 6, -1, 100 and a 1001-character text are refused; a review before the stay ends is refused; another guest cannot review; one review per stay | Pass |
| S15.4 | 60 guests book at the same instant, 5 rooms a night: all 60 succeed, every night has exactly 5 rooms taken | Pass |
| S17.1 | Logs contain the path and request id, and no token or query value | Pass |

Speed of the spike (S15.4), on one machine with a local database, so only a lower bound: 60 simultaneous bookings finished in about 0.15 seconds; p50 about 125 ms, p95 about 140 ms, p99 about 150 ms. A staging test against the real Supabase connection pool is still needed (checklist item 11).

## New findings

**F1 (money): a payment that succeeds after we recorded it as failed is ignored.** The payment stays "failed" and the booking stays HELD, so the hold expires while the guest's money has been received. This can happen when the gateway reports failure first (for example a retried attempt) and success later, or when webhooks arrive out of order. Suggested fix: when a verified "succeeded" arrives for a payment in "failed", move it to "succeeded". If the booking is still HELD, mark it paid. If the hold has expired or the room is gone, queue a refund and raise a payment issue for the admins (the alert built for Chapa mismatches).

**F2 (operations): hiding a hotel does not warn the admin about its confirmed future bookings.** The guest's booking and voucher keep working (good), but the admin gets no warning or list. Suggested fix: the hide request answers with the count and codes of upcoming bookings, and asks for a confirmation flag before it hides the hotel.

Both are small fixes. Waiting for your yes before changing code.

## Fixes for F1 and F2 (done)

- **F1:** `processPaymentEvent` (`payments.go`) now also accepts a "succeeded" result for a payment recorded as "failed". The booking is confirmed if it is still held; otherwise the deposit is refunded through the normal refund queue. A warning is logged. The Chapa check (`chapaSettle`) now re-checks a "failed" payment instead of stopping. This also covers a payment we closed ourselves when the guest switched method.
- **F2:** taking an active hotel off the list (hidden or draft) while it has unfinished bookings is refused with `409 HAS_UPCOMING_BOOKINGS` and the booking codes. Sending `confirm_hide: true` goes ahead; the admin page asks "Take the hotel off the list anyway?" and resends. The bookings and vouchers keep working.

Both audit tests now pass. Whole suite: all packages pass, including all audit tests (one skipped: S1.16, needs a UTF8 database).

Not changed: a guest who pays twice (two methods open at once) would get a refund for the second payment, not a second booking; this was already the case.
