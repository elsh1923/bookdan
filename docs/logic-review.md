# BookDan: every business situation the app handles

This is a review of the code as it is today, for discussion with the investor. For each situation it says what the app does, and
where the answer is still open. "Decision" marks a question for the business, not a bug.

## A. Searching and holding a room

| Situation | What the app does | Open point |
|---|---|---|
| Guest searches for a place or dates | Every hotel that matches the words and filters is listed. Hotels with no room for the whole stay stay in the list, marked "Sold out for these dates". Sorted by recommended, price, rating or distance. | None |
| A hotel is full for part of the stay, or for the exact dates | A separate section, "Other dates that work", lists up to 10 such hotels with up to 3 stays each: the same number of nights one or two days earlier or later, or part of the requested stay. Only nights the hotel can sell are offered, never a past night, never a room that cannot fit the party. Tapping one opens the hotel with those dates. Nothing is held until the guest books, and the booking checks every night again. | Decision: show these at all? The price shown is a "from" price for the first night; the quote gives the exact total |
| Two guests book the last room at once | Holds take every room for every night, or none. The second guest is refused as sold out. | None |
| Guest holds a room but does not pay | The hold lasts 15 minutes, then the room is released automatically. | Decision: is 15 minutes right? |
| Guest taps Pay twice, or the app retries | Idempotency keys make the second request return the same booking, never a second charge. | None |
| Price changes between search and payment | The guest sees a quote with a time limit. The price is fixed when the hold is made. | None |
| First-time coupon used again | A "first booking only" coupon is refused if the guest has a booking that is still active or finished. | None |
| Coupon on a booking | Coupons are paid online only. A coupon booking cannot be paid at the hotel. | Decision: keep this rule? |

## B. The hotel's answer

| Situation | What the app does | Open point |
|---|---|---|
| Hotel accepts in time | The booking is confirmed, the guest is told, and the voucher appears. | None |
| Hotel declines | It gives a reason (fully booked, room unavailable, maintenance, other). The guest is refunded automatically. | None |
| Hotel does not answer within 15 minutes | The booking is declined and the guest refunded. Alerts escalate to owners (if the hotel chose that) and a reminder goes out 5 minutes before. | Decision: is 15 minutes right for every hotel? |
| Hotel answers after the deadline | Refused with "the time to answer has passed". The guest has already been refunded. | None |
| Nobody on duty sees the alert | Alerts go to every person on duty; the first answer wins. Off-shift staff get nothing. | Decision: is there a phone call backup? (Not built.) |
| Hotel tries to accept a booking that is not paid | Refused. Only a paid booking can be accepted. | None |

## C. Payments

| Situation | What the app does | Open point |
|---|---|---|
| Guest pays online (Chapa) | A hosted page takes Telebirr, CBE Birr, M-Pesa, bank or card. The result is checked with Chapa before the booking moves on. | Live keys and account approval pending |
| Chapa says paid, but the amount differs | The booking is not confirmed. An error is logged for a person to check. | **Gap:** there is no admin screen for these cases yet |
| Chapa's notice never arrives | The app's own check asks Chapa when the guest opens the booking. | None |
| Payment arrives after the hold ended | The rooms were released, so the money goes back to the guest automatically. | None |
| Refund fails | It is retried by the server's background jobs. | Decision: how many retries, and who is told? |
| Guest pays at the hotel | The booking moves straight to the hotel. No money passes through BookDan, and no commission is recorded. | **Key decision** (see the payment note) |
| Payment model | Online payment takes the full total, or a deposit if the hotel sets one. | Decision: full, deposit, or pay at the hotel (see the payment note) |
| Price not confirmed | In production, prices are refused until the business confirms the tax and commission figures. | Decision: confirm the figures |
| Telebirr through a separate merchant account | Not built. Telebirr is reached through Chapa. | Decision: is that enough? |

## D. Cancellations and refunds

| Situation | What the app does | Open point |
|---|---|---|
| Guest cancels before the hotel answers | Full refund. | None |
| Guest cancels a confirmed, refundable booking more than 48 hours before check-in | Full refund. | Decision: is 48 hours right? |
| Guest cancels a confirmed booking within 48 hours | No refund. | Decision: any partial refund? |
| Guest cancels a non-refundable booking | No refund once the hotel has confirmed it. | Decision: how is non-refundable explained? |
| Guest cancels after the stay started | Refused. | None |
| Guest wants to change dates | **Not built.** It waits for the payment model. | Decision: allow changes? Who pays any difference? |
| Hotel cancels a confirmed booking | **Not possible in the app.** There is no route for it. | **Gap:** how does a hotel cancel after confirming? |
| Refund sent to a card or account | Refunds go back through Chapa. Chapa's fees are not returned. | Decision: who bears Chapa's fee on a refund? |

## E. Check-in and the stay

| Situation | What the app does | Open point |
|---|---|---|
| Guest arrives with the QR code or PIN | The receptionist scans or types it. A code for another hotel is refused. | None |
| Guest arrives before the check-in day | Refused until the day comes. | None |
| Guest does not arrive | The hotel marks a no-show. The hotel keeps its share, and the rooms for later nights are released. | Decision: should the guest be charged? |
| Stay ends and the hotel never marked check-in | The booking completes automatically at the end of the stay. | None |
| Guest owes money at the hotel (pay at hotel) | The app shows the balance to collect. The hotel collects it. | Tied to the payment model |
| Hotel blocks rooms (repairs, events) or takes a walk-in | Blocks use the same stock as online bookings, so nothing can be sold twice. | None |
| Booking for someone else | The guest names the traveller and a phone number. | Decision: is the traveller consent needed? |

## F. Money owed to hotels

| Situation | What the app does | Open point |
|---|---|---|
| Money for a stay | Three states: upcoming (not yet payable), ready (payable after check-in or the stay ends), and paid. | None |
| Commission | 15% of the room price, plus VAT on the commission. | Decision: confirm the 15% |
| Payouts | An admin records each payout by hand against the ledger. | Decision: when are payouts made, and who approves them? |
| Pay-at-hotel bookings | Not in the ledger, so no commission and no payout. | **Gap:** see the payment note |
| Hotel's bank details | Stored encrypted; only the last four digits show; a full reveal is logged. | None |

## G. Disputes and support

| Situation | What the app does | Open point |
|---|---|---|
| Guest disagrees with a charge or a stay | The booking can be marked "disputed" in the data, but **there is no way to raise one in the app.** | **Gap:** a support process |
| Guest needs help during a stay | **Not built:** no support button or support ticket. | **Gap:** see the support discussion |
| Hotel needs help | No support channel in the hotel app. | Decision: who answers hotels? |

## H. Guests and accounts

| Situation | What the app does | Open point |
|---|---|---|
| Sign-in by email code | Working. Codes are limited per address. | None |
| Sign-in by phone number | **Not possible in production.** The server refuses to start with the phone code provider, because SMS is not built. | **Gap:** choose an SMS provider, or launch email only |
| Guest deletes their profile | Their name and ID details are deleted. **The account and their booking history are not deleted.** | **Gap:** Google Play requires full account deletion |
| Guest's ID number | Stored encrypted. Shown masked. Erased after the retention period. | Decision: the legal retention period |
| Guest's ID photo | Removed from the app. The database tables from migration 00021 are still there, empty. | Decision: drop the tables? |
| Guest reviews a stay | One review per stay, with star ratings. A hotel cannot reply yet. | Decision: hotel replies? |
| Coupon reuse | Limited by the coupon rules. | None |

## I. Hotels and their staff

| Situation | What the app does | Open point |
|---|---|---|
| Hotel signs up | BookDan's admin adds it as a draft. It cannot be published without rooms, prices and business details. | None |
| Hotel owner and receptionists | The owner adds receptionists. Owners cannot touch other hotels or the business details. | None |
| A staff member leaves | Removing them stops their alerts immediately. | None |
| Shifts | Each person has a start and end time in Addis Ababa time. Off-shift staff are not alerted. | None |
| Hotel is hidden | It disappears from search. Existing bookings are not cancelled. | Decision: what happens to its guests? |
| Hotel is deleted | Not possible from the app. | Decision: is deletion needed? |

## J. The admin side

| Situation | What the app does | Open point |
|---|---|---|
| Admin login | An email and a password, limited to 8 tries per 10 minutes. Hotel owners can sign in with an email code only if the business allows it. | None |
| Every change | Written to the audit log. | None |
| Staff roles for BookDan | Only platform admins can publish hotels, verify them, record payouts and see the business details. | Decision: more admin roles (support, finance)? |
| Admin alerts for calls | Not built (agreed to be built after the investor discussion). | Decision |

## K. Notifications

| Situation | What the app does | Open point |
|---|---|---|
| Hotel alert for a new booking | Push to the phones on duty, and an email. | None |
| Guest update on a booking | Push and email. | None |
| SMS | Not built. | Decision: which SMS provider |

## L. Things not built yet

- Offline check-in for hotels when there is no internet.
- Monthly statements for hotels.
- Date changes for guests.
- Hotel cancellation after confirmation.
- Dispute handling and a support ticket system.
- Phone sign-in (SMS).
- Account deletion that removes everything, as Google Play requires.
- Hotel replies to reviews.
- Airport transfers and other add-ons.

## Scope of this review

This review is based on reading the backend services, the database rules, and the apps' main screens. It does not cover every
screen in the hotel app, or the admin page's full layout. Each decision above needs a business answer before it becomes code.
