# BookDan: Launch Review Report

**For:** owners, investors and partners (no technical knowledge needed)
**Date:** 18 November 2026
**Scope:** a full review of the BookDan hotel booking app: the guest app, the hotel app, the admin portal and the server behind them.

---

## 1. The short version

BookDan does what a hotel booking app should do. A guest can search, compare, book, pay, change or cancel and get help. A hotel can accept or decline requests, check guests in and see what it is owed. The BookDan team can add hotels, set prices, run offers and pay hotels.

We tested every one of these journeys, 84 scenarios in all, plus 235 automated checks that run on every change. **No fault was found in the app.** The few failures we saw were mistakes in our own test scripts, and we fixed those.

What stands between BookDan and a public launch is **business set-up, not app building**: live payments, SMS codes, phone alerts and real hotels.

**Overall verdict: the app is ready for a closed test with real people. It is not yet ready for the public until the items in section 5 are done.**

---

## 2. What we tested

| Stage | What | Scenarios | Result |
|---|---|---|---|
| 1 | Health check: the server, the database, the code and the automated checks | n/a | Healthy |
| 2 | Guest journey: sign up, search, book, pay, cancel, change dates, reviews, offers, help | 34 | 34 passed |
| 3 | Hotel journey: accept, decline, time-outs, check-in, no-shows, availability, earnings | 27 | 27 passed |
| 4 | Admin and money: publishing hotels, offers, member levels, payouts, applications, support | 23 | 23 passed |
| 5 | Comparison with MakeMyTrip's hotel features | n/a | Core features match |
| 6 | Launch readiness: store rules, privacy, payments, monitoring | n/a | Items to finish |

**How we tested:** the scripts acted as real guests, hotels and admins against a full copy of the server. They did not use a phone screen, so a few people should also try the app by hand (see section 6).

---

## 3. What works, in plain words

**For guests**
- Search by place, dates and guests. Sort and filter by price, rating, type, amenities, breakfast and deals.
- See photos, a map, room options, reviews and the cancellation rules before paying.
- Book with a 15-minute hold. Pay online, or pay at the hotel where the hotel allows it.
- If the hotel doesn't answer in 15 minutes, the booking is declined and the guest is refunded automatically.
- Cancel, change dates, get a voucher, save favourites, and ask to be told when a sold-out hotel has rooms.
- Member levels (Bronze 3%, Silver 5%, Gold 8%) reward guests who stay at different hotels.
- Get help, raise a dispute, or ask for relocation if a hotel cannot host them.
- Delete their own account in the app.

**For hotels**
- Accept or decline requests, check guests in, and mark no-shows.
- Close rooms for dates (for example, repairs).
- See earnings, and what is owed and when it is paid.
- Guest ID numbers are stored encrypted and shown only in short form. Every time a full number is viewed, it is logged.

**For the BookDan team**
- Add a hotel as a draft. It goes live only when it has a priced room and its business and payout details.
- Run offers. Edit member levels. Approve hotel applications.
- Pay hotels. The system refuses overpayment, refuses payment without a reference, and refuses payment to a hotel with no payout details.
- Handle help requests, hide or restore reviews, suspend users, and see business figures and a log of every change.

**Money is protected.** A record of every payment, refund and commission is kept and never edited, only added to. In testing, the amount owed to a hotel dropped by exactly the amount paid, to the cent. BookDan takes a 15% commission, with VAT on that fee.

---

## 4. How BookDan compares with MakeMyTrip (hotels)

BookDan matches MakeMyTrip on the features that matter for hotels: search and filters, maps, photo galleries, reviews, saved hotels, coupons, loyalty levels, free-cancellation rules, a bookings list (Upcoming, Past, Cancelled, Failed), change and cancel, help and disputes.

It is stronger for Ethiopia in these ways: hotel-side tools are included, there is an automatic refund when a hotel is silent, guest IDs are encrypted, and local payment is supported.

**What it lacks:** the number of hotels. MakeMyTrip lists tens of thousands, while BookDan has demo hotels until real ones are added. It also lacks smaller extras such as price alerts, group bookings, travel-wallet cashback and guest photos. We do not consider those launch blockers.

---

## 5. What must be done before the public launch

| # | Item | Who | Status |
|---|---|---|---|
| 1 | **Live payments:** get Chapa to approve the BookDan business account and set the live keys | Owner | Waiting on Chapa |
| 2 | **Real hotels:** onboard 10 to 20 real hotels with true photos, prices and payout details | Owner and sales | Not started |
| 3 | **Launch mode:** switch the live server from "staging" to the real-launch setting | Technical helper | Not done |
| 4 | **SMS codes:** connect an SMS provider (guide in the repo). Email codes already work. | Owner and helper | Not done |
| 5 | **Phone alerts:** set up Firebase (guide in the repo) | Technical helper | Not done |
| 6 | **Legal review:** a lawyer checks the privacy policy and terms. We drafted both. | Owner | Draft ready |
| 7 | **Support email:** set a real support address on the server | Owner | Not done |
| 8 | **Backups:** confirm daily database backups and test one restore | Technical helper | To check |
| 9 | **Monitoring:** add a free uptime monitor, and set where error alerts are sent | Technical helper | To check |
| 10 | **Hotel owner sign-in on the admin site** (optional): turn on if owners should use it | Owner | Off |

**Already done (this review):**
- The production app build now uses the live server, not demo data.
- Privacy policy, terms and account-deletion pages exist and are linked inside the app.
- Account deletion works in the app.

*Note: the pages go public once the server is redeployed.*

---

## 6. Things the scripts could not check

Please test these by hand, on real phones, before opening to the public:
- How each screen looks and feels on small and large phones.
- A real payment from start to finish, with a small amount once Chapa is live.
- Real SMS and phone alerts arriving.
- Time-based events in real time: the 15-minute answer window, and the 48-hour free-cancellation cut-off.
- Amharic text on every screen.
- Poor-network behaviour: weak signal, going offline, coming back online.

---

## 7. Path to Google Play

1. Create a Google Play Developer account (one-time fee about USD 25, with identity verification).
2. Build the production bundle with EAS and upload it.
3. Complete the store listing: description, screenshots, icon, privacy policy link, and the data-safety form (the privacy page is the guide).
4. Run a **closed test**. New personal accounts must run one for about 14 days with at least 12 testers.
5. Apply for production release, then monitor reviews and alerts closely for the first weeks.

**Suggested order:** items 1 to 5 in section 5 in parallel with the closed test. Open to the public once real payment and alerts have been tried by testers.

---

## 8. Risks to watch

| Risk | Why it matters | What reduces it |
|---|---|---|
| Few hotels at launch | An empty-feeling app loses users | Launch city by city, such as Addis Ababa, with a strong hotel base |
| Payment provider delays | No real money until approved | Start the Chapa activation now |
| Hotels not answering requests | Guests get declined and may leave | Phone alerts and onboarding hotels on the 15-minute rule |
| Unreviewed legal text | Store or legal trouble | Lawyer review before launch |
| Single server and database | An outage stops bookings | Uptime monitor, alerts and tested backups |

---

## 9. Conclusion

The product is built and tested. The remaining work is getting real hotels, real payments and real alerts connected, and a short test with real people. With those done, BookDan is ready to launch.
