# BookDan: how the app works, and what is left

Status key: **Done** (built and tested) · **Stand-in** (works, but fake until an outside service is connected) · **Partial** · **Not built**.

Two apps, one backend: the **Guest app** (books rooms), the **Partner app** (hotel staff answer requests), and the **Go API**
(`https://bookdan.onrender.com`) on **Supabase** PostgreSQL. The database itself enforces the rules that protect money and rooms.

---

## 1. Sign-in and sessions (both apps)

```
App                         API                                   Database
 |  1. enter phone/email      |                                      |
 |--- POST /auth/otp/request->| validate (+2519… or email)           |
 |                            | max 5 codes / 10 min per identifier  |
 |                            | make 6-digit code, store only a hash |-> otp_challenges
 |                            | send it (OTPSender)                  |
 |  2. enter the code         |                                      |
 |--- POST /auth/otp/verify ->| 5 tries max, 5-minute life           |
 |    (X-App, X-Device-Id)    | guest: create user on first sign-in  |-> users
 |                            | partner: must be in property_staff   |   property_staff
 |                            | register the device                  |-> devices
 |<-- access token (15 min) --| refresh token (30 days, hash stored) |-> refresh_tokens
 |    + refresh token         |                                      |
 |  3. every request carries  |                                      |
 |     Authorization: Bearer  | token must be for THIS app           |
 |  4. token expired (401)    |                                      |
 |--- POST /auth/refresh ---->| refresh token works ONCE (rotation)  |
 |<-- new pair ---------------| a used one again = leak: whole       |
 |    (retry the failed call) | session family is revoked            |
```

- **Phone** accounts and **email** accounts are separate accounts (the same person using both has two). 
- Tokens are kept in the phone's secure storage. When a refresh fails for good the app signs the user out.
- Partner app: only people registered as hotel staff get in (`NOT_STAFF` otherwise). A guest token cannot call hotel routes, and the reverse.
- **Sign out** wipes the token, cached data and saved vouchers **on the phone**.

| Piece | Status |
|---|---|
| Code request/verify, limits, hashing, device registration | Done |
| Rotating refresh tokens with theft detection, silent refresh | Done |
| Guest vs hotel-staff separation | Done |
| **Sending the code by SMS / email** | **Stand-in** (fixed code `123456` in demo mode) |
| Sign out revokes the session **on the server** | **Not built** (sign-out is on the phone only; the refresh token stays valid until it expires) |
| Per-IP rate limits on code requests | Not built (limits are per identifier) |
| Linking one person's phone and email accounts | Not built |
| Suspend a user / revoke a device from an admin screen | Not built (the database supports it) |

---

## 2. Guest journey

```
Home / Search ──> Hotel page ──> Checkout (3 steps) ──> Booking screen ──> Voucher
 (live search,     (rooms, live     1 Review: rooms, price   hold countdown,   QR + PIN,
  landmarks,        availability)   2 Guest: name + ID       pay, waiting for  works offline
  filters)                          3 Pay: hold the rooms    hotel, result
```

| Step | What happens | Status |
|---|---|---|
| **Search** | By name, area, city, or *near a landmark* (PostGIS, 3 km); filters by type and amenities; party size and number of rooms; shows price and "available". 12 per page. | Done |
| **Map view** | Placeholder screen | Not built |
| **Hotel page** | Details, rooms with live "rooms left", house rules, reviews list, call button | Done |
| **Photos** | Hotels show a coloured placeholder. There is no photo upload or storage. | **Not built** |
| **Write a review** | Reviews are read-only (the table and rating view exist) | Not built |
| **My details and ID** | Name, ID type, ID number (encrypted in the database, only last four returned), nationality. Required before booking. Can be removed. | Done |
| **Quote** | Prices every night, 1–9 rooms of one type, adds VAT. Valid 10 minutes. | Done (rates are placeholders, see §5) |
| **Hold** | Takes every room on every night or none. 15 minutes. Safe to retry (idempotency key). Freezes a copy of the guest's profile for the hotel. | Done |
| **Pay online** | Telebirr/Chapa choice opens a payment page. Asking again while pending returns the same page, never a second charge. | **Stand-in** (fake test page) |
| **Pay at hotel** | For hotels that allow it: no payment page, goes straight to the hotel | Done |
| **After payment** | The app polls the booking; the gateway's callback (signed, handled once) moves it to "waiting for hotel" | Done |
| **Hotel answer** | Accept → Confirmed. Decline or no answer in 15 min → guest refunded automatically. | Done (refund money movement is a stand-in) |
| **Voucher** | Signed QR (Ed25519) + 4-digit PIN, saved on the phone for offline use, valid until the day after check-out | Done |
| **Notification to the guest** when the hotel answers | Built: confirmed, declined, or no answer in 15 minutes. Needs Firebase and a new build to actually arrive (`docs/push-notifications-setup.md`) | **Built, needs Firebase** |
| **Cancel a confirmed booking** (guest) and refund rules | Not built (refund policy is an open decision) |
| **Receipt / confirmation email or SMS** | Not built |
| Languages | English and Amharic (Amharic needs a native review). Oromo and Tigrinya not built. | Partial |

Interrupted payment: if the app is killed during payment, the next launch takes the guest back to that booking.

---

## 3. Hotel (Partner app) journey

```
Sign in ──> Inbox ──────────────> Booking screen ──> Accept / Decline(reason)
 (staff       "Waiting for your     guest profile,
  only)        answer" first,       Show ID number
               countdown            (audited)
```

| Step | What happens | Status |
|---|---|---|
| **Inbox** | Paid requests first, closest deadline at the top, then upcoming and recent. Held (unpaid) rooms are invisible to the hotel. | Done |
| **Guest profile** | Name, contact, nationality, ID type and **masked** ID number | Done |
| **Show ID number** | Decrypts the full number, writes an audit entry each time. Erased numbers return "gone". | Done |
| **Accept** | Rooms move from held to booked; the hotel's share is written to the ledger | Done |
| **Decline** | Needs a reason; rooms are released; guest is refunded | Done |
| **15-minute deadline** | Enforced by the server every 20 seconds, not by the phone's clock | Done |
| **Ringing alert for a new request** | App and server both built (FCM sender tested against a fake Firebase). Needs your Firebase key and a new build. | **Built, needs Firebase** |
| **Rooms chart, walk-ins, check-in by scanning the guest's QR, offline sync** | Routes exist and return "not built" | **Not built** |
| **Earnings / statements / payouts** | The ledger records everything; no screen or payout process | Not built |
| **Hotel onboarding** (add hotel, rooms, rates, staff, photos) | No admin portal. Done by SQL today. | **Not built** |
| Multiple hotels per account, roles (owner vs reception) | One hotel per account; roles stored, not used yet | Partial |

---

## 4. Money and safety rules (all enforced by the database and covered by tests)

- Never oversold: each night's rooms cannot go over capacity, even with many guests at once.
- Booking status can only move along allowed steps; every change is written to a history.
- The agreed price never changes after the hold. Money is whole santim.
- The ledger is append-only and every transaction balances (refund owed → refund paid, deposit held → hotel share).
- Payment results are accepted only with a valid signature and each event is applied once.
- National IDs are AES-256-GCM encrypted (key outside the database); the hotel's copy is frozen per booking.
- Logs never contain tokens, codes, IDs or request bodies.

---

## 5. What is left, in the order I would do it

**A. To take real money and real users (needs outside accounts)**
1. SMS provider for Ethiopian numbers + email provider → real sign-in codes.
2. Telebirr and Chapa merchant accounts → real payments and real refunds.
3. Firebase project, keys and new builds → ringing alerts for hotels and updates for guests (code is done).
4. Switch the server from `APP_ENV=staging` to `production` (it refuses until 1–3 are real).
5. Confirm the money rules: service charge, VAT, commission, payment split (placeholders today) and the refund/cancellation policy.
6. Decide ID retention (and schedule the erase job), accepted ID formats, and write the privacy notice and terms.

**B. To run a business**
7. Admin/onboarding tool for hotels, rooms, rates, staff and photos (photo storage too).
8. Server-side sign-out, per-IP limits, account deletion, push to guests, receipts.
9. Monitoring and alerts (Sentry), database backups, a staging copy of the server.

**C. Product features**
10. Hotel offline tools: rooms chart, walk-ins, QR check-in, statements.
11. Guest: cancel and rebook, write reviews, map view, wishlists on the server, more languages.
12. iPhone builds (Apple Developer account, TestFlight).
