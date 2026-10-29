# BookDan: full status report (27 October 2026)

Covers the backend, the guest app, the partner (hotel) app and the admin portal. Sources: the audit (`audit-phase-a.md`), the logic review (`logic-review.md`), and a hands-on browser test of all three front ends on 27 October.

## 1. What exists

| Part | What it does | State |
|---|---|---|
| Backend (Go, PostgreSQL) | Search, quotes, holds, payments, hotel answers, cancellations, refunds, ledger, check-in, reviews, notifications, admin API | Built, tested, deployed on Render + Supabase |
| Guest app (Expo, Android first) | Search with filters and map, hotel page, checkout with review card, Chapa payment, bookings (upcoming/past), saved, voucher with QR, reviews, other dates, account export and deletion, English and Amharic | Built; tested in a browser and on a phone earlier |
| Partner app (Expo) | Hotel staff sign-in, booking list, accept/decline, QR and PIN check-in, rooms chart and blocks, earnings, alerts, English and Amharic | Built; tested in a browser |
| Admin portal (web, `/admin`) | Hotels, rooms, prices, photos, map pin, staff, business and payout details, coupons, payouts, payment issues | Built; tested in a browser |

## 2. Test results

**Backend automated tests:** every package passes. The audit suite has 55 tests: 54 pass, 1 skipped (non-Latin names, needs a UTF-8 database).

| Area | Result |
|---|---|
| Search (words, filters, dates, past and far-future dates, paging, 1,000 searches with 100 workers) | Pass |
| Pricing, coupons (any letter case), currency | Pass |
| Booking creation, holds, idempotency | Pass |
| Concurrency: 60 guests at once, 20 simultaneous cancels, accept racing cancel | Pass; no oversold room, one refund |
| Payments: duplicate, late and out-of-order results, amount mismatch alert, refund retry | Pass (after fixing F1) |
| Cancellation and refunds | Pass |
| Authorization (guest, hotel, admin, deleted or revoked accounts) | Pass |
| Hotel side, notifications sent once, mail outage does not undo a booking | Pass |
| Reviews: bounds, ownership, one per stay | Pass |
| Logs hold no tokens or query secrets | Pass |
| Rate limits and security headers | Pass |

**Bugs found and fixed during this work** (all covered by tests now):
1. Money received after a payment was recorded as failed was ignored (F1).
2. Hiding a hotel with future bookings gave the admin no warning (F2).
3. Earlier audit: 8 failures (search validation, past and far dates, logout and suspension not ending access at once, no payment-mismatch alert, and others). All fixed.

**Hands-on browser test (27 October):**
- Admin portal: hide-hotel warning and confirm, payment issues screen, hotel list. Works.
- Partner app: sign-in, bookings, check-in, rooms, earnings, settings. Works.
- Guest app: home, results, "other dates that work", opening an alternative stay, email sign-in, profile. Works.

## 3. Not tested yet (and why)

| Item | Why | What it needs |
|---|---|---|
| Real Chapa live payments | Only test mode and a fake Chapa were used | Chapa live activation, then one small real payment and refund |
| Delete-account confirmation dialog | The dialog does nothing in a browser | One tap on an Android phone |
| Real SMS and email delivery at scale | No SMS provider; email only checked with a test sender | Provider decision and real addresses |
| Non-Latin hotel names in search (Amharic, Arabic) | The local test database is not UTF-8 | A check on Supabase or a UTF-8 test database |
| Load against the real database | The 60-guest spike ran locally (p99 about 150 ms) | A staging copy of Supabase |
| Push alerts on real phones with the latest build | Needs devices | A phone test |
| Migration down and up with real data | Hard to automate | A manual rehearsal on a copy |
| Backups and restore | Not set up | Supabase point-in-time recovery, then a restore test |
| iOS | Android first | Apple developer account and a build |
| Partner app visual redesign | Only fonts and colours changed | Design work |

## 4. Features that are NOT built

**Guest side**
- Changing the dates of a booking.
- Raising a dispute or complaint.
- A support button, help chat or support tickets.
- Phone (SMS) sign-in. Email sign-in only.
- Offline use.
- Paying with other currencies (ETB only).
- Airport transfers and other add-ons.

**Hotel side**
- Hotel cancelling a confirmed booking, with a rule for what happens to the guest.
- Relocating a guest to another hotel (the state exists in the database; nothing sets it).
- Disputes (the state exists in the database; nothing sets it).
- Offline check-in when the hotel has no internet.
- Monthly statements for hotels.
- Replying to guest reviews.
- Hotel self-signup (admins create hotels).
- A support channel for hotels.

**Payments and money**
- Pay at hotel records no commission and holds no money. The model (deposit plus balance at the hotel, or an invoice to the hotel) is undecided.
- Direct Telebirr integration (Telebirr is reached through Chapa).
- Partial refunds on a dispute; automated payouts to hotels (admin sees payouts; transfer is manual).

**Admin and operations**
- Admin "Calls" tab and hiding the hotel phone number until booking (agreed in principle, paused).
- Analytics dashboard (revenue, conversion, cancellations).
- Error monitoring, alerts, a minimum-app-version check, and metrics.
- Paged lists (guests see the latest 100 bookings, hotels 200).
- Automatic erasure of ID numbers after a retention period (needs the legal decision, then `ID_RETENTION_DAYS`).

**Launch and legal**
- Privacy policy, terms of service and store listings.
- Legal confirmation that deleted accounts may keep the booking, ledger and the hotel's copy of the name.

## 5. Decisions still owed by you
1. Pay at hotel: how the commission (15%) is collected.
2. Who handles disputes, hotel cancellations and date changes, and the rules.
3. Legal retention period for ID numbers.
4. SMS provider, or launch with email only.
5. Email provider (Brevo is set; Resend was compared).
6. Whether to build the Calls tab and support button before launch.

## 6. Steps before launch
1. Push the code and let Render deploy. Confirm it builds on Go 1.27.
2. Migration 00022 is already applied on Supabase.
3. On Render set `SUPPORT_EMAIL` and `ADMIN_EMAILS`.
4. Publish the guest and partner updates (`eas update`); build a new guest APK for the native splash and logo icon.
5. Chapa live activation, live key and webhook secret.
6. Turn on Supabase backups and test a restore.
7. Add error monitoring (for example Sentry).
8. Privacy policy, terms, store listings.
9. A real-money test: book, pay, accept, cancel and refund with one small amount.

## 7. Known risks
- The rate limit trusts the proxy's address headers; an edge limit is stronger.
- No monitoring: problems would be found by users first.
- Pay-at-hotel bookings earn no commission until the model is decided.
- Guests who contact hotels directly can skip the platform; the Calls tab and masked contacts were the planned answer.

## 8. Progress since this report (guest-side build plan)

| Step | Feature | State |
|---|---|---|
| 1 | Support tickets (guest app, admin Support tab, emails, deletion erases the guest's words) | Built and tested. Migration 00023 applied on Supabase. |
| 2 | Disputes (booking frozen, admin decision: reject, refund with hotel paying, refund with BookDan paying; guest can withdraw) | Built and tested. Migration 00024 **not yet applied** on Supabase. |
| 3 | Changing the dates of a booking (once, same price, free-cancel window only; hotel alerted, guest emailed) | Built and tested. Migration 00025 **not yet applied** on Supabase. Different prices are refused (cancel and rebook): charging or refunding a difference needs partial refunds and a second payment, not built. An old offline voucher shows the old dates until the guest opens the app online; the hotel check-in checks the server dates. |
| 4 | Display currencies (USD and EUR shown next to ETB; admin sets the rate; stale rates hidden; typo guard) | Built and tested. Migration 00026 **not yet applied** on Supabase. Charging and refunds stay in ETB only. After applying it, set the rates in the admin page (Rates), otherwise guests see ETB only. |
| 5 | Phone (SMS) sign-in | Built and tested against fakes: Africa's Talking and Twilio behind one setting, plus `off` for an email-only launch. **Not yet run against a real provider**: needs your account and the first-real-SMS check in `docs/sms-setup.md`. No migration. |

Known limits of disputes: one dispute per booking; if the hotel was already paid out, "hotel pays" leaves it owing that amount against its next payout; hotels are not alerted on their phones when a dispute opens (the admin contacts them).

## 9. Partial refunds (built)

Part of a payment can go back while the booking stands: a dispute can be settled for part of the price (`refunded_partial_hotel_pays`, `refunded_partial_platform_pays`), and an admin can give a goodwill credit from any support request about a confirmed booking (admin page, "Refund part of the payment").
- **Who pays:** the hotel (its share, BookDan's commission and the VAT on it shrink in proportion) or BookDan (expense `dispute_expense`, the hotel keeps its share).
- **Rules:** more than 0 and less than the payment; several partial refunds are allowed up to what is left; a later cancellation or full refund returns only what is left, and reverses the hotel's money only for that part.
- **Money movement:** one gateway refund per partial refund, each with its own reference, retried until accepted. Migration 00027 (applied on Supabase).
- **Still true:** if the hotel was already paid out, "hotel pays" leaves it owing that part against its next payout. Chapa's partial-refund acceptance was tested only against a fake: confirm it with one small real refund on the Chapa test account. Date changes to a different price are still refused (they would also need a second payment for a higher price).

## 10. Hotel-side and operations features (built, tested)

| Feature | What it does | Migration |
|---|---|---|
| Hotel cancels a confirmed booking | Owners cancel with a reason; the guest is refunded in full and the hotel bears it | 00028 |
| Cannot host / relocation | Any staff can mark it; a support request opens; the guest may cancel at once; support restores or refunds | 00028 |
| Hotel side of disputes | The hotel is alerted, reads the guest's account and adds its own statement | 00028 |
| Hotel support channel | Hotel staff write to BookDan support from the partner app | 00029 |
| Review replies | Owners answer, edit or remove a reply | 00030 |
| Monthly statements and pay-at-hotel commission | Statement per month (app, CSV, email); commission on pay-at-hotel stays booked at completion; receipts from hotels | 00031 |
| Call log and phone masking | The hotel's phone is shown only after confirmation; taps on "Call the hotel" are counted | 00032 |
| Analytics, error alerts, paged lists | Admin numbers; webhook and email alerts for server problems and failed refunds; keyset paging | none |
| Hotel self-signup | Public application page and admin review | 00033 |
| Offline check-in | The phone verifies the voucher signature itself and sends the check-in when online | none |

Migrations 00028 to 00033 are applied on Supabase (27 October).

## 11. Added on 29 October: review moderation, support alerts, admin accounts, date changes at a different price

| Feature | What it does | Migration |
|---|---|---|
| Hide abusive reviews | Admin Reviews tab: hide with a reason, show again. A hidden review leaves the hotel page, its rating and the owner's list; the hotel cannot answer it; the author still sees it | 00034 |
| Phone alert when support answers | A push (names the request, never the answer) to the guest or the hotel staff member who wrote the ticket; tapping it opens the thread. Email as before | none |
| Admin accounts and roles | Each BookDan staff member has their own login, a role (super, support, finance) and an optional authenticator-app second step; lockout after 5 wrong attempts; disabling takes effect at once; the last super admin is protected; the shared setup password stops working for an email once it has its own account | 00035 |
| Date change at a different price | The guest sees the price first and confirms it. Cheaper: the difference is refunded and the hotel bears it. Dearer: the guest pays the difference (second payment; new nights held 15 minutes; moves when paid; late money refunded). Paid at the hotel: reprices only. Cancelling afterwards refunds both payments | 00036 |

Run migrations 00034 to 00036 on Supabase before deploying this code. After deploying, add the admins in the Admins tab and then remove `ADMIN_PASSWORD` from the server settings.

Limits: the authenticator second step is optional (no recovery codes: a super admin resets it); the admin token lasts 8 hours; a date change can be made once per booking and not for coupon or non-refundable bookings; the offline voucher of a moved booking shows the old dates until the app is online.

## 12. Added on 29 October (second round): saved hotels on the server, per-IP limit on sign-in codes, partner app redesign

| Feature | What it does | Migration |
|---|---|---|
| Saved hotels on the server | The wishlist is kept on the phone and on the server (up to 200 hotels), so it follows the guest to a new phone. Removals made offline are remembered and sent later. Part of the data export; deleted with the account | 00037 |
| Per-IP limit on sign-in codes | One internet connection can ask for at most 30 sign-in codes in 10 minutes (`OTP_MAX_PER_IP`, 0 = off), on top of 5 per address. Generous because many people share one mobile-network address | 00037 (index) |
| Partner app redesign | Every screen has the guest app's navy header with rounded corners, the tab bar is dark navy, the sign-in screen has a navy hero, and the booking list shows three numbers: to answer, arriving today, in the hotel | none |

Run migration 00037 on Supabase before deploying. Date changes at a different price, admin accounts and review moderation (section 11) need 00034 to 00036, which are already applied.


## 14. The hotel portal (/hotel)

A separate website for hotels, at the same address as the admin page with /hotel at the end. Owners and receptionists sign in with the same emailed or texted code as the hotel app.

| Who | What they see |
|---|---|
| Owner | Home (what needs an answer today, arrivals, what BookDan owes), Bookings (accept, decline, check in, no-show, cancel), Availability (14-day chart, walk-in and out-of-service blocks), My hotel, Rooms and prices, Photos, Team, Booking alerts, Earnings, Statements, Reviews and replies, Help from BookDan |
| Receptionist | Home, Bookings (without cancel), Availability, Help |
| BookDan super admin | Everything in the admin page, plus **Hotel changes**: a log of what each hotel changed, when and by whom |

Rules: an owner edits only their own hotels. Room, price, photo and team changes go live at once and are logged. Only BookDan staff can publish, hide or verify a hotel, change the commission, see or change business and payout details, and manage coupons. An owner can add receptionists but not other owners. No database change is needed.
