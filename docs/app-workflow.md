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
| **Sending the code by email** | **Built** (SMTP: Gmail app password, Brevo, Mailgun, SES). Real addresses get a real random code; demo addresses keep `123456`. | **Done** |
| **Sending the code by SMS** | **Stand-in** (fixed code `123456` while no SMS provider is connected) |
| Sign out revokes the session **on the server** (refresh tokens ended, the phone stops getting alerts; an access token already issued lasts up to 15 minutes) | Done |
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
| **Photos** | Uploaded in the admin page, resized, shown in search and on the hotel page; a hotel without photos still shows a coloured placeholder | **Done** |
| **Write a review** | Built: after check-out, 1 to 5 stars and an optional comment (up to 1000 characters), one per booking, shown with the guest's first name and counted in the hotel's rating | **Done** |
| **My details and ID** | Name, ID type, ID number (encrypted in the database, only last four returned), nationality. Required before booking. Can be removed. | Done |
| **Quote** | Prices every night, 1–9 rooms of one type, adds VAT. Valid 10 minutes. | Done (rates are placeholders, see §5) |
| **Hold** | Takes every room on every night or none. 15 minutes. Safe to retry (idempotency key). Freezes a copy of the guest's profile for the hotel. | Done |
| **Pay online** | Telebirr/Chapa choice opens a payment page. Asking again while pending returns the same page, never a second charge. | **Stand-in** (fake test page) |
| **Pay at hotel** | For hotels that allow it: no payment page, goes straight to the hotel | Done |
| **After payment** | The app polls the booking; the gateway's callback (signed, handled once) moves it to "waiting for hotel" | Done |
| **Hotel answer** | Accept → Confirmed. Decline or no answer in 15 min → guest refunded automatically. | Done (refund money movement is a stand-in) |
| **Voucher** | Signed QR (Ed25519) + 4-digit PIN, saved on the phone for offline use, valid until the day after check-out | Done |
| **Notification to the guest** when the hotel answers | Built: confirmed, declined, or no answer in 15 minutes. Needs Firebase and a new build to actually arrive (`docs/push-notifications-setup.md`) | **Built, needs Firebase** |
| **Cancel a booking** (guest) | Built. Before the hotel answers: full refund. After it confirmed: full refund until 48 hours before check-in (12:00 Addis time), none after; not possible once the stay has started. The app shows the exact outcome before confirming. The hotel gets an alert. A refund also reverses the hotel's share in the ledger. | **Done** |
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
| **Who is alerted, reminders, shifts** | Built. Everyone on duty is alerted at once and the first to answer wins (no rotation: one missed phone would lose the booking). Per hotel: "everyone" or "receptionists first, owners after 1-10 minutes of no answer". Per person: alerts on/off and a shift (Addis time, may pass midnight). 5 minutes before the request runs out: reminder push to everyone on duty and the owners, plus an email to the owners and the business contact. Set on the admin page, tab Booking alerts. | **Done** |
| **Check-in with the voucher QR** | Built. Partner app tab Check-in: scan the guest's voucher QR (signature checked on the server) or type the booking reference + 4-digit PIN (5 wrong PINs lock it for 30 minutes, the QR still works). The receptionist sees the guest, room, dates and any amount to collect, then confirms. Allowed from the check-in date until 06:00 on the check-out date; twice is harmless; who did it and when is recorded. A confirmed booking can also be checked in from its booking screen. Needs internet (offline check-in is not built). Needs a **new partner build** (camera). | **Done, needs new build** |
| **Hotel earnings and payouts** | Built. Owners see Earnings in the partner app: what BookDan owes them, what is waiting for check-in, what was paid, and every online-paid booking split into guest paid, BookDan fee, VAT on the fee and what they receive. Money becomes payable once the guest has checked in or the stay is over (until then the guest can still cancel). Platform admins use the admin page, Payouts: they transfer the money by bank or Telebirr themselves, then record it with the transfer reference (never more than is payable; ledger entry, audit log, email to the owners). Bookings paid at the hotel are not included. BookDan's commission is 15% of what the guest paid (`pricing_config`, migration 00015), plus 15% VAT on that fee. | **Done** |
| **Room options** | Built. A room is sold as room only or with breakfast, each with free cancellation or non-refundable (cheaper). The hotel sets, per room type on the admin page, what breakfast adds per night and the non-refundable discount; leave them empty to offer neither. Non-refundable: nothing back once the hotel confirms (everything back before it answers, or if it declines). The hotel app shows "Breakfast included" and "Non-refundable" on the booking. | **Done** |
| **Coupons** | Built. Admin page, Coupons: percent or fixed, cap, minimum booking, dates, uses in total and per guest, first booking only, one hotel or all. The discount comes off the room price before VAT and **BookDan pays for it**: the hotel still receives its full share (ledger account coupon_expense). Online payment only. A use is given back when the booking is refunded or expires. | **Done** |
| **Room photos** | Built. Each room type has its own photos (up to 10, uploaded on the admin page, Rooms tab, "Photos" button). They show as a thumbnail on the room card with a count, and open full screen. The hotel gallery is separate. | **Done** |
| **More hotel features** | Built. Besides generator, hot water, Wi-Fi, parking and pay at hotel: air conditioning, pool, restaurant, gym, spa, airport shuttle, laundry and a 24-hour front desk. The filter list shows only features that some hotel in the results has. | **Done** |
| **"Near me" and map** | Built. "Near me" uses the phone's position: hotels come back nearest first with "2.1 km from you". The Map view shows every hotel of a search as a price label on an OpenStreetMap map (no map key needed); tapping one opens its card. The map is a WebView, a native module, so it needs a **new guest build**; older builds show "update the app" instead. OpenStreetMap's free tiles suit a demo; switch to a paid tile provider or Google Maps before heavy public use. | **Done, needs new guest build** |
| **Change dates of a booking** | **Not built.** A new date can cost more, and the guest would then have to pay the difference; that needs the real payment gateway. Today the guest cancels (refund rules apply) and books again. | **Waits for payments** |
| **Rooms chart, walk-ins, check-in by scanning the guest's QR, offline sync** | Routes exist and return "not built" | **Not built** |
| **Earnings / statements / payouts** | The ledger records everything; no screen or payout process | Not built |
| **Hotel onboarding** | Built: the platform admin registers the hotel, rooms, prices, business and payout details, and the owner in the admin page; the owner gets an invitation email and signs in to the hotel app with their email; owners add their own receptionists; the admin sees who is Invited or Joined. | **Done** |
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
1. SMS provider for Ethiopian numbers → real sign-in codes by phone (email codes are built, they only need your sending account).
2. Telebirr and Chapa merchant accounts → real payments and real refunds.
3. Firebase project, keys and new builds → ringing alerts for hotels and updates for guests (code is done).
4. Switch the server from `APP_ENV=staging` to `production` (it refuses until 1–3 are real).
5. Confirm the money rules: service charge, VAT, commission, payment split (placeholders today) and the refund/cancellation policy.
6. Decide ID retention (and schedule the erase job), accepted ID formats, and write the privacy notice and terms.

**B. To run a business**
7. Admin/onboarding tool for hotels, rooms, rates, staff and photos (photo storage too).
8. Per-IP limits, account deletion, receipts.
9. Monitoring and alerts (Sentry), database backups, a staging copy of the server.

**C. Product features**
10. Hotel offline tools: rooms chart, walk-ins, QR check-in, statements.
11. Guest: map view, wishlists on the server, more languages. (A hotel replying to reviews, and removing abusive reviews from the admin page, are not built.)
12. iPhone builds (Apple Developer account, TestFlight).

**D. Before the public launch: iPhone**
13. Apple Developer account (about $99 a year), iPhone builds and TestFlight for testers.
14. Push on iPhone: an Apple push key in Firebase, and the app changes for iOS tokens (today push is Android only).
15. Hotel alerts on iPhone cannot override silent mode or Do Not Disturb without Apple's critical-alerts permission, so hotels should use Android phones or get a backup alert (SMS or phone call).
16. One test round on a real iPhone for layout issues (keyboard, notch, bottom bar, back gesture).
