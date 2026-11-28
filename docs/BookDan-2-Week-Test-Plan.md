# BookDan: 2-week test plan, page by page

**14 days, 78 pages, 525 checks.** Each page says how to open it, what to do and what you should see.

## Before Day 1
- Test on the **live server** (https://bookdan.onrender.com) with **Chapa test keys**. The phone apps read the live server, not the local one.
- Push your latest code, wait for Render to show **Live**, then publish both app updates to the preview channel.
- You need: 2 Android phones (guest, hotel), a computer, 4 email addresses you can open (guest, second guest, owner, receptionist) and the super admin sign-in.
- Test hotels: **A** pay online only; **B** pay at hotel allowed, with a non-refundable discount; **C** only one room.
- Use made-up business and bank details. Never real card details.

## How to work each day
1. Do the pages in order. Finish a page before starting the next.
2. Mark each check **P** (pass), **F** (fail) or **S** (skipped, with the reason).
3. For each F note what you did and what you saw, and take a screenshot.
4. At the end of the day send me the failed rows. I fix them; re-test them the next morning.

The same list is in **BookDan-2-Week-Test-Plan.csv**, which opens in Excel.

## The two weeks
One plan day is one test day, 14 in a row. If you rest on some days, that is fine: keep the order, and create the dated bookings on the day **before** you start Day 8.

| Day | What | Pages | Checks |
|---|---|---|---|
| 1 | Public pages, admin sign-in and the hotel list | 3 | 17 |
| 2 | Admin: adding a hotel and its location | 2 | 19 |
| 3 | Admin: rooms, photos, staff, alerts, business, applications | 8 | 38 |
| 4 | Guest app: first launch, sign-in and home | 6 | 46 |
| 5 | Guest app: search, results, map and hotel page | 3 | 50 |
| 6 | Guest app: checkout, your details, coupons and paying | 4 | 45 |
| 7 | Hotel app answers requests | 4 | 25 |
| 8 | Hotel app: check-in and rooms | 2 | 17 |
| 9 | Hotel app: money, reviews, support and settings; guest voucher | 8 | 50 |
| 10 | Hotel web portal | 13 | 58 |
| 11 | Guest app: my bookings, saved, cancel and change dates | 4 | 32 |
| 12 | Reviews, problems and support, from guest to admin | 7 | 46 |
| 13 | Admin: money, insights and settings | 9 | 46 |
| 14 | Profile, offline, privacy, whole system and sign-off | 5 | 36 |

## Dated bookings (create at the end of Day 7)
Pay and have the hotel accept each one straight away. "Day 8" means the real date on which you will test Day 8.

| Booking | Check-in | Used for |
|---|---|---|
| 1 | Day 8, 2 nights | QR check-in |
| 2 | Day 8, 1 night, pay at hotel | PIN check-in and money to collect |
| 3 | Day 8, 1 night | No-show |
| 4 | Day 8, 1 night (check the guest in) | Review, dispute and payout |
| 5 | Same as 4, at a second hotel | Member level |
| 6 | 5 or more days after Day 11, refundable | Cancel and change dates |
| 7 | 5 or more days after Day 11, refundable | Date change with extra payment |
| 8 | Day 12, refundable | Late cancel on Day 11 (inside 48 hours) |
| 9 | 5 or more days after Day 11, non-refundable | No-refund check |

If you pause between Day 7 and Day 8, create these on the day **before** you start Day 8.

## Check you saw everything
Every screen of the guest app and hotel app, every menu page of both portals, the public pages, the emails and the automatic actions are listed. If you find a button or screen that is not in this plan, write it down and send it to me.

## What two weeks still cannot cover
- The dispute deadline of 14 days after check-out (row N9), unless you return to it later.
- The automatic erasing of full ID numbers after the retention period.
- Phone alerts and SMS, until Firebase and an SMS provider are set up. Mark them **S**.
- Real money, until Chapa approves the live account.
- iPhone, the Play Store build (test it in Google's closed test), many users at the same time, and a specialist security test.

---

## Day 1: Public pages, admin sign-in and the hotel list
*Goal: Make sure the server and the front door work.* (3 pages, 17 checks)

### 1.1 Public website: Server check and legal pages
**Open:** Browser: bookdan.onrender.com/v1/healthz, /privacy, /terms, /delete-account

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| W1 | Open https://bookdan.onrender.com/v1/healthz and /v1/readyz. | Status "ok" and "ready". | ☐ |
| W2 | Open /privacy, /terms and /delete-account on the live site, on a computer and a phone. | All three load, read correctly, show a contact, and the links between them work. | ☐ |
| X1 | Open each legal page in dark mode and with large browser zoom. | Text stays readable. | ☐ |

### 1.2 Admin portal: Sign-in
**Open:** bookdan.onrender.com/admin

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-S1 | Open /admin. Sign in with a wrong password. | Refused with a clear message. | ☐ |
| AD-S2 | Sign in as super admin with the right password. | The portal opens with the menu: Hotels, Guests, Money, Insights, Settings. | ☐ |
| AD-S3 | Choose "Hotel owner" and send a code to an owner email. | If owner sign-in is on, a code arrives and only that owner's hotels show. If off, a clear message. | ☐ |
| AD-S4 | Press Sign out, then press the browser Back button. | Signed out; no private data shows. | ☐ |
| AD-S5 | Open the portal on a phone-sized window. | Menu and forms stay usable. | ☐ |
| X2 | If the authenticator app is on for this admin, sign in with a wrong 6-digit code, then the right one. | Wrong refused; right one opens the portal. | ☐ |
| X3 | Enter a wrong password many times in a row. | Further tries are slowed or blocked for a while. | ☐ |
| X4 | Press Enter instead of clicking Sign in. | The form submits. | ☐ |
| Z1 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 1.3 Admin portal: All hotels
**Open:** Menu: Hotels, All hotels

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-H1 | Open All hotels. | Each hotel shows name, city, type, room and photo counts, and Published, Draft or Hidden. | ☐ |
| X5 | Click a hotel, then "All hotels" to go back. | The hotel opens and you return to the list. | ☐ |
| X6 | Refresh the browser on a hotel page. | The same page opens again. | ☐ |
| X7 | Click every menu item once. | Every page opens without an error message. | ☐ |
| Z2 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

**End of Day 1:** passed ____ , failed ____ , skipped ____

---

## Day 2: Admin: adding a hotel and its location
*Goal: Create Hotels A, B and C.* (2 pages, 19 checks)

### 2.1 Admin portal: Add a hotel and hotel Details tab
**Open:** Menu: Hotels, Add a hotel

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-H2 | Add a hotel and press Create with the name empty. | Refused with a message saying what is missing. | ☐ |
| AD-H3 | Fill every field, using the examples shown under each, and create it. | The hotel opens as a Draft. | ☐ |
| AD-H10 | Save Amharic description and directions, house rules (one per line), check-in and check-out times, features. | All are still there after reload and appear in the guest app later. | ☐ |
| AD-H11 | Leave "Guests may pay at the hotel" off for Hotel A, on for Hotel B. | Saved as chosen. | ☐ |
| AD-H12 | Check "Verified by BookDan" is ticked on a new hotel. | Ticked by default; the badge shows in the guest app after publishing. | ☐ |
| X8 | Check every box shows an example, and the longer ones a hint. | Examples disappear when typing; hints stay. | ☐ |
| X9 | Type very long text in name and description. | Stops at the limit; the page stays tidy. | ☐ |
| X10 | Edit a saved hotel, change the type and features, save, reload. | "Saved"; changes remain. | ☐ |
| X11 | Change something and leave without saving. | Nothing changes. | ☐ |
| Z3 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 2.2 Admin portal: Location on the map
**Open:** Inside the hotel form

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-H4 | Map: tap the map, then drag the pin. | Green "Pin set" line with numbers that change with the pin. | ☐ |
| AD-H5 | Map: paste a short Google Maps link (Share, Copy link). | "Looking up the link", then "Pin placed from your link". Check the building. | ☐ |
| AD-H6 | Map: paste numbers such as 9.0108, 38.7613. | Pin moves there. | ☐ |
| AD-H7 | Map: paste nonsense, then a non-Google link. | Red message; the pin does not move. | ☐ |
| AD-H8 | Map: press "Use my current location" and allow, then repeat and refuse. | Pin at your position; on refusal a clear message. | ☐ |
| AD-H9 | Map: open "Type the numbers instead" and change them. | Pin follows the typed numbers. | ☐ |
| X12 | Zoom the map in and out and drag it around. | The whole box fills with map; no grey patches. | ☐ |
| X13 | Save, reopen the hotel, look at the pin. | Same place as saved. | ☐ |
| Z4 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

**End of Day 2:** passed ____ , failed ____ , skipped ____

---

## Day 3: Admin: rooms, photos, staff, alerts, business, applications
*Goal: Publish the three hotels.* (8 pages, 38 checks)

### 3.1 Admin portal: Rooms and prices tab
**Open:** Open a hotel, Rooms & prices

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-R1 | Add a room with name, guests, count, features, price, breakfast price and non-refundable discount. | The room appears in the table. | ☐ |
| AD-R2 | Enter a price of 50 birr, 25 guests per room, and a discount of 60. | Each is refused: price is 100 to 1,000,000 birr, guests 1 to 20, discount 1 to 50. | ☐ |
| AD-R3 | Type a word (for example available) in Breakfast and save. | Refused with a message asking for a price in birr; nothing is saved. | ☐ |
| AD-R4 | Leave breakfast and discount empty on another room. | Saved; guests see no breakfast or cheaper rate. | ☐ |
| AD-R5 | Edit price, capacity and count of a room and save. | New values stay after reload; new guest searches show the new price. | ☐ |
| AD-R6 | Reduce the room count below the number already booked. | Refused, or a clear warning. | ☐ |
| AD-R7 | Upload room photos (several at once), make one first, delete one. | Order changes; the deleted one is gone. At most 10. | ☐ |
| AD-R8 | Remove a room that has no bookings, then one that has. | The first is removed; the second is refused with a reason. | ☐ |
| Z5 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 3.2 Admin portal: Photos tab
**Open:** Open a hotel, Photos

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-P1 | Upload several hotel photos, choose a cover, delete one. | The cover shows first in the guest app. | ☐ |
| AD-P2 | Upload a very large file and a non-image file. | Handled with a clear message; nothing breaks. | ☐ |
| Z6 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 3.3 Admin portal: Staff tab
**Open:** Open a hotel, Staff

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-P3 | Staff: add an owner email and a receptionist email. | Both show "Invited"; each gets an invitation email. | ☐ |
| AD-P4 | Have both sign in to the hotel app, then reload Staff. | They show "Joined" with the right role. | ☐ |
| AD-P5 | Remove a receptionist. | They can no longer see this hotel. | ☐ |
| X14 | Add the same person twice, and an invalid email. | Refused with a message. | ☐ |
| Z7 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 3.4 Admin portal: Booking alerts tab
**Open:** Open a hotel, Booking alerts

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-P6 | Booking alerts: choose "Everyone on duty at once", then "Receptionists first" with minutes; set shift times; switch some alert kinds off. | Saved; "No phone" or "Signed in" is correct for each person. | ☐ |
| Y1 | Choose "Receptionists first" and type 0, then 20 minutes. | Out-of-range values are refused (a request lasts 15 minutes in all). | ☐ |
| Y2 | Give one receptionist a shift that has already ended today. | Saved and shown. | ☐ |
| Y3 | Look at each person: "Signed in" or "No phone", and what they have muted. | Matches who has signed in on a phone with notifications. | ☐ |
| Z8 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 3.5 Admin portal: Business tab
**Open:** Open a hotel, Business

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-P7 | Business tab: save with a field missing, then fill all and save. | Missing field refused; then saved, and only the last four digits of the account show. | ☐ |
| AD-P8 | Press "Show the full account number". | It asks first, shows it, and the view is recorded in Hotel changes. | ☐ |
| X15 | Choose each payment method (bank, mobile money) and save. | Each saves and shows in Payouts. | ☐ |
| Z9 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 3.6 Admin portal: Publishing and hiding
**Open:** Open a hotel, Details, Status

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-P9 | Try publishing a hotel with no priced room, then with no business details. | Refused each time, saying what is missing. | ☐ |
| AD-P10 | Publish Hotels A, B and C (C has one single room). | All three show in guest search. | ☐ |
| AD-P11 | Hide a hotel that has an upcoming booking. | A warning lists it; it hides only after you confirm; the booking stays valid. | ☐ |
| Z10 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 3.7 Hotel application page: Apply form
**Open:** bookdan.onrender.com/apply

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AP1 | Open /apply and send the form empty. | Required fields are pointed out. | ☐ |
| AP2 | Send a complete application. | A thank-you screen (and a reference). | ☐ |
| AP3 | Send the same hotel and email again. | Told an application is already waiting. | ☐ |
| X16 | Fill it on a phone browser. | Every box is easy to reach and type in. | ☐ |

### 3.8 Admin portal: Applications
**Open:** Menu: Hotels, Applications

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AP4 | Admin, Applications: check all details are shown. | Matches what was typed. | ☐ |
| AP5 | Reject one with an empty reason, then with a reason. | Empty refused; with reason it is rejected and the applicant is told. | ☐ |
| AP6 | Approve another. | A draft hotel is created, the owner is invited, "Open the hotel" works. | ☐ |
| Z11 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

**End of Day 3:** passed ____ , failed ____ , skipped ____

---

## Day 4: Guest app: first launch, sign-in and home
*Goal: The first five minutes of a new guest.* (6 pages, 46 checks)

### 4.1 Guest app: Loading and welcome screen
**Open:** Clear the app data and open it

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| A1 | Open the app for the first time (or clear its data). | A short loading screen with the logo, then the welcome screen: "Find a verified stay in Ethiopia", language and currency choices and Get started. | ☐ |
| A2 | Choose Amharic, then Get started. | The app switches to Amharic. Dates show Ethiopian dates under the calendar days (see D4). | ☐ |
| A3 | On the welcome screen tap the Currency row and choose a currency (for example USD). | The Currency screen opens; after choosing you return to the welcome screen showing the choice. Prices later show as approximate amounts, with the exact birr amount next to anything charged. | ☐ |
| A4 | Kill the app and reopen it. | It opens straight to the home screen (no welcome again), in the language you chose. | ☐ |
| Y4 | On the welcome screen look at the city cards (Addis Ababa, Bahir Dar, Lalibela) and the three promises. | Pictures and text show correctly in both languages. | ☐ |
| Y5 | Look for any "Demo" text or demo buttons in the app. | None on the live version. | ☐ |
| Z12 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 4.2 Guest app: Updates and starting offline
**Open:** Publish an update; airplane mode

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| A5 | Publish a new update (eas update) while the app is open in the background, then reopen the app twice. | A banner "A new version is ready" with Restart; Restart opens the new version. | ☐ |
| A6 | Open the app with airplane mode on. | The app opens; lists show "Showing saved results" or an offline message with Try again; nothing crashes. | ☐ |
| X17 | If the server asks for a newer app version, open the app. | A "Please update BookDan" screen with a button. Mark S if this cannot be set up. | ☐ |
| Z13 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 4.3 Guest app: Sign in
**Open:** Tap My bookings or Hold this room

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| B1 | Tap anything that needs an account (My bookings, Hold this room). Choose Email, type your email, tap Send code. | "Code sent to your email". [Email] A 6-digit code arrives within a minute. | ☐ |
| B2 | Type the code and tap Verify and continue. | You are signed in and returned to where you were (not the home screen). | ☐ |
| B3 | Type a wrong code. | "That code is not right. Try again." Five wrong tries cancel that code; request a new one. | ☐ |
| B4 | Use an invalid email (no @) and tap Send code. | "Enter a valid email address." and nothing is sent. | ☐ |
| B5 | Request codes for the same email six times within 10 minutes. | After 5: "Too many codes requested. Wait a few minutes and try again." | ☐ |
| B6 | Choose Phone and try to sign in. | If phone sign-in is switched off: "Phone sign-in is not available right now. Use your email instead." If a text-message provider is connected: a code arrives by SMS and works like email. | ☐ |
| B7 | Wait for a code to age more than 5 minutes, then use it. | It is refused; request a new one. | ☐ |
| B8 | Profile, then Sign out. Reopen the app. | You are signed out; My bookings asks you to sign in; saved details on the phone are not shown to the next person. | ☐ |
| B9 | Stay signed in for a day or more (or leave the app idle for 20 minutes), then open My bookings. | It loads without asking you to sign in again (the session renews silently). | ☐ |
| B10 | Sign in with the same email on a 2nd phone. [2nd phone] | Both phones work. Your bookings and saved hotels show on both. | ☐ |
| Z14 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 4.4 Guest app: Sign-in methods
**Open:** Profile, Sign-in methods

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| X18 | Open the page. | Shows email and phone, each added or "Not added". | ☐ |
| X19 | Add a second email or phone with its code (wrong code first). | Wrong refused; then "Added. You can now sign in with either." | ☐ |
| X20 | If text messages are off, try to add a phone. | Told phone numbers cannot be added yet. | ☐ |
| X21 | Add an address that already has another guest account, and agree to merge. | Asks first; then both accounts' bookings appear together. | ☐ |
| X22 | Try to add the email of a hotel or admin account. | Told it cannot be merged. | ☐ |
| X23 | Sign out and sign in with the newly added method. | Same account and bookings. | ☐ |
| Z15 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 4.5 Guest app: Home
**Open:** Home tab

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| C1 | Open Home. | Header with the logo and "Find your perfect stay", and a search card (place, dates, guests, Search). | ☐ |
| C2 | Tap each field of the search card. | Each one opens the search screen. | ☐ |
| C3 | Tap the three quick buttons (Search, Use my location, My bookings). | Each opens the matching screen. | ☐ |
| C4 | Tap the category pills: All, Hotel, Guesthouse, Lodge, Resort. | The lists below show only that kind of stay. | ☐ |
| C5 | Swipe "Offers for you" and tap both cards (pay at the hotel, member rewards). | Cards slide smoothly; "See all" goes where it says. | ☐ |
| C6 | Look at "Featured for you" and tap "See all", then tap a hotel card. | Search opens; a card opens that hotel. Cards show photo, name, price per night and rating. | ☐ |
| C7 | Read "Why book with BookDan" (safe payments, fast answers, pay your way). | Three cards, readable, nothing cut off. | ☐ |
| C8 | Open two hotels, then return to Home. | "Recently viewed" lists them, newest first. | ☐ |
| C9 | Scroll to "More stays" and pull down to refresh. | More hotels listed; the page reloads without errors. | ☐ |
| C10 | Look at "rooms left" and "best price" badges on cards and the heart button. | Badges only where true; the heart saves and unsaves. | ☐ |
| Z16 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 4.6 Guest app: Currency
**Open:** Profile, Currency

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| Y6 | Open Currency and look at Default, Popular and More currencies. | Ethiopian Birr is the default; others listed. | ☐ |
| Y7 | Search for a currency by name, then type nonsense. | The list narrows; "No currency matches your search". | ☐ |
| Y8 | Choose USD and look at home, results, hotel page and checkout. | Prices are a guide in USD; the amount charged is shown in birr. | ☐ |
| Y9 | Change back to ETB. | All prices return to birr. | ☐ |
| Y10 | Read the note about exchange rates. | The rate source is named; if rates are missing the app says so. | ☐ |
| Z17 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

**End of Day 4:** passed ____ , failed ____ , skipped ____

---

## Day 5: Guest app: search, results, map and hotel page
*Goal: Finding the right room.* (3 pages, 50 checks)

### 5.1 Guest app: Search
**Open:** Home, search card

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| D1 | Tap the place box and type part of a hotel name, an area or a city. | Suggestions appear as you type: landmarks, areas, cities and hotels. Tapping a hotel opens it directly. | ☐ |
| D2 | Type a landmark (for example a well-known square) and search. | Hotels within about 3 km of the landmark are listed, with the distance ("0.4 km to ..."). | ☐ |
| D3 | Tap "Use my location" and allow location. | Results are sorted nearest first with "x km from you". If you refuse: "Allow location to find hotels near you, or type a place instead." | ☐ |
| D4 | Open the date picker. Try picking a past day, then a stay longer than 30 nights. | Past days cannot be picked. A stay over 30 nights shows "A stay can be up to 30 nights." In Amharic each day also shows its Ethiopian date. | ☐ |
| D5 | Pick check-in then check-out, tap Done. | The search card shows both dates and the number of nights. | ☐ |
| D6 | Open Guests and rooms. Set 1 room and 3 adults, then 3 rooms and 2 adults. | The counters work; each room needs at least one adult, so the app does not let adults fall below rooms. | ☐ |
| D7 | Search with children added (1 adult + 1 child). | Search works; the child count is carried to the booking. | ☐ |
| D8 | Run several searches, then open the search screen again. | "Recent searches" lists them (up to 6); Clear removes them. | ☐ |
| D9 | Open the place box with nothing typed and tap one of the "Popular places". | It searches that place with the dates and guests you already chose. | ☐ |
| X24 | Set 2 rooms, adults and children. | Each room needs at least one adult; the choice is kept. | ☐ |
| X25 | Make three searches, return, tap a recent search, then Clear. | Recent searches listed, reusable and cleared. | ☐ |
| Y11 | Tap an item under "Popular places" and one under "Popular areas". | Each searches that place. | ☐ |
| Y12 | Tap check-in, then check-out in the calendar. | The prompts change; the chosen range is highlighted. | ☐ |
| Z18 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 5.2 Guest app: Results, filters, sort and map
**Open:** Press Search

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| E1 | Search for dates and look at the list. | Each card: photo, name, area, type, price per night ("from"), rating and review count, "Replies in about x min" when known, heart, and tags such as Breakfast available or Cheaper non-refundable rate. | ☐ |
| E2 | Open Sort and choose each option. | Recommended, Price low to high, Price high to low, Guest rating, and Nearest to me (when you use location). The list reorders. | ☐ |
| E3 | Open Filters: choose a property type, then amenities (Wi-Fi, parking, pool...), then a price range, then 4+ rating, then Breakfast and Deals. Tap Apply. | Only matching hotels remain and the count updates ("Showing 3 properties"). Reset clears everything. The amenity list shows only features that some hotel in the results has. | ☐ |
| E4 | Search for dates when one hotel is full (book all its rooms first, or use a one-room type). | That hotel stays in the list marked "Sold out for these dates". | ☐ |
| E5 | Search the same sold-out dates. | A section "Other dates that work" lists hotels with free rooms a day or two earlier or later, or part of the stay. Tapping a stay opens the hotel with those dates. | ☐ |
| E6 | Search a place with no hotels (nonsense words). | "No stays found. Try different dates, fewer filters, or another area." | ☐ |
| E7 | Tap Map (needs the new build). | A map with price labels; tapping a price opens that hotel card. An old build shows "Update the app to see these hotels on a map." | ☐ |
| E8 | Turn on airplane mode, then search. | "No internet connection" with Try again, or the last results marked as saved. | ☐ |
| E9 | Tap the heart on a result card. | The heart fills and the hotel appears in Saved (see M). | ☐ |
| X26 | Search dates when Hotel C is full. | "Other dates that work" suggests dates; tapping one updates the search. | ☐ |
| Y13 | Use each price band, the rating choices, "Breakfast available", "Cheaper non-refundable rates", property type and amenities, then Reset. | Results match each filter; Reset clears all; the count "Showing N properties" is right. | ☐ |
| Y14 | Switch between list and map; tap a price on the map. | The hotel card opens. | ☐ |
| Y15 | Look for the "Lowest price in these results", "Only N left" and distance badges. | Shown only where true. | ☐ |
| Z19 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 5.3 Guest app: Hotel page
**Open:** Tap a hotel

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| F1 | Open a hotel. | Photo gallery with "3 of 12"; name, area, rating, "Verified" badge, price bar at the bottom. | ☐ |
| F2 | Swipe the photos and tap one. | A full-screen gallery you can swipe, with a strip of small pictures. | ☐ |
| F3 | Tap Share. | Your phone's share sheet opens with the hotel name and area. | ☐ |
| F4 | Switch between Overview, Amenities, Reviews and Property rules. | Each tab shows its content. "View all (n)" expands the amenities. | ☐ |
| F5 | Read "How to find us". | The hotel's directions in words (landmarks) in English; in Amharic if the hotel set it. | ☐ |
| F6 | Tap "Open in Maps". | Your maps app opens at the hotel. | ☐ |
| F7 | Look for a phone number before booking. | There is no phone number or Call button before the booking is confirmed. | ☐ |
| F8 | Scroll to the rooms. | Each room type: photo, name, how many sleep, "Only n left", and its options: Room only or with breakfast, each as Free cancellation or Non-refundable (cheaper) when the hotel offers them. | ☐ |
| F9 | Open the room photos. | A thumbnail with a count; tapping opens that room's own gallery. | ☐ |
| F10 | Read Reviews. | Average, a bar for each star count, four category scores (cleanliness, location, staff, value) and written reviews with the guest's first name, "Verified stay", and the hotel's reply under the review when there is one. | ☐ |
| F11 | Look at "Replies in about x min". | Shown only after the hotel has answered at least three requests. | ☐ |
| F12 | Tap Select room on an option. | You go to checkout (sign in first if needed) with that room and option. | ☐ |
| F13 | Open a hotel from "Other dates that work". | The hotel opens with those dates already set. | ☐ |
| HX1 | Scroll the hotel page and use the tabs Overview, Amenities, Reviews, Rules. | The sliding tabs jump to the right part; the top bar stays. | ☐ |
| HX2 | Open the photo gallery and swipe through every photo. | All photos load; closing returns to the page. | ☐ |
| Y16 | Tap Share and send it to yourself. | A message with the hotel name, area and rating. | ☐ |
| Y17 | Tap the heart, then look at Saved. | Added, and removed when tapped again. | ☐ |
| Y18 | Tap the Verified badge text. | Explains that the property was visited and confirmed. | ☐ |
| Y19 | Check "Replies in about N min", check-in and check-out times, "Sleeps N" and "+ taxes at checkout". | Match the hotel settings (reply time appears after some answered bookings). | ☐ |
| Y20 | In Location: read the address, nearby distances and location rating; tap "Open in Maps". | Correct; maps opens at the pin. | ☐ |
| Y21 | Read reviews: "Verified stay" and "Reply from the hotel". | Shown where they exist. | ☐ |
| Z20 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

**End of Day 5:** passed ____ , failed ____ , skipped ____

---

## Day 6: Guest app: checkout, your details, coupons and paying
*Goal: From room to payment with Chapa test.* (4 pages, 45 checks)

### 6.1 Guest app: Checkout
**Open:** Hotel page, choose a room

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| G1 | On checkout, step 1 (Review). | The stay, room and option, dates and nights, the price per night times nights times rooms, VAT (15%) and the total. A line states the cancellation policy that matches the option you chose. | ☐ |
| G2 | Choose a non-refundable option. | The policy changes to "Non-refundable: no refund once the hotel confirms. If the hotel cannot accept your booking, you get everything back." and the price is lower. | ☐ |
| G3 | Enter a coupon that exists (for example TEST10) and tap Apply. | "Coupon TEST10 applied", "You save ...", a coupon line in the price and a lower total. | ☐ |
| G4 | Enter a wrong coupon code, then one that is expired or used up. | "That coupon cannot be used." The total does not change. | ☐ |
| G5 | Use a first-booking-only coupon on an account that already has a booking. [Admin to create it] | Refused. | ☐ |
| G6 | Tap Remove on a coupon. | The total returns to the full price. | ☐ |
| G7 | Continue to step 2 (Guest) with no ID saved. | "Add your ID to book" with an Add my ID button; the app does not let you hold the room without it (see H). | ☐ |
| G8 | Add special requests with the chips (late arrival, high floor...), and type your own. | The requests are kept; up to 400 characters. They appear on the hotel's booking screen. [Hotel app] | ☐ |
| G9 | Turn on "I am booking for someone else". Leave the name empty and try to continue; then fill name and phone. | Name required ("Enter the name of the guest who will stay"); a phone with letters is refused; once valid it continues. The hotel sees who is really staying. [Hotel app] | ☐ |
| G10 | Book 2 rooms (rooms stepper) for 3 adults. | The price is per room times rooms; the booking says "2 × room name". | ☐ |
| G11 | Tap Hold this room. | You land on the booking screen: "Room held for you", a 15-minute countdown and the payment choices. | ☐ |
| G12 | Try to hold the last room twice at the same moment (two phones). [2nd phone] | One succeeds; the other sees "This room was just taken". | ☐ |
| G13 | Wait on the quote screen for more than 10 minutes, then tap Hold. | "The price expired. Please review again." | ☐ |
| G14 | Hold a room and do nothing for 15 minutes. [Wait] | The booking shows "The hold expired. The room was released. You have not been charged." and the room is free for others. | ☐ |
| X27 | Go back and forth between the three steps. | Everything typed is kept. | ☐ |
| Y22 | Enter letters in the other guest phone number. | "Enter a phone number with digits only". | ☐ |
| Y23 | While on checkout, let the admin remove the chosen option (for example breakfast). | "That option is not available any more", with a way back to rooms. | ☐ |
| Y24 | Check the stay summary: dates, from and until times, guests, children, rooms, "Only N left". | All correct. | ☐ |
| Y25 | With a member level and no coupon, look at the price lines. | "member: you save" line appears. | ☐ |
| Z21 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 6.2 Guest app: My details and ID
**Open:** Profile, My details and ID

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| H1 | Profile, then "My details and ID". Fill name, ID type, ID number and nationality, then Save. | Saved: "National ID ••••1234". The full number is never shown again. | ☐ |
| H2 | Save with the name empty, a nonsense ID (too short or with symbols), or no nationality. | A clear message under the field; nothing is saved. | ☐ |
| H3 | Open the screen again and change the name. | "For your safety we never show it again. Enter it again to save changes." | ☐ |
| H4 | Tap "Remove my ID details", then tap again to confirm. | The details are removed; holding a room asks for them again. | ☐ |
| H5 | Hold a room and open the hotel's booking. [Hotel app] | The hotel sees your name, ID type, nationality and only the masked ID; "Show ID number" reveals it (and is recorded). | ☐ |
| Z22 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 6.3 Admin portal and guest app: Coupons
**Open:** Admin: Money, Coupons; app: checkout

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| CP1 | Admin: create TEST10 (10%), FLAT200 (200 birr), BIG50 (50%, at most 300), FIRSTONLY (20%, first booking only). | Each listed with a plain rule. | ☐ |
| CP2 | Create the same code again. | Refused: the code already exists. | ☐ |
| CP3 | Apply each in the app on a 2,000 birr room. | 200 off; 200 off; 300 off (capped); only for a new guest. | ☐ |
| CP4 | Create coupons with a past last day, a future first day, a minimum booking, one total use, one hotel only; use each wrongly. | Each is refused with a message. | ☐ |
| CP5 | Switch a coupon off, try it, switch on, try again. | Refused when off, works when on. | ☐ |
| CP6 | After a paid coupon booking, look at Coupons and at the hotel's earnings. | "Used 1 time", BookDan paid the discount; the hotel's share is unchanged. | ☐ |

### 6.4 Guest app: Payment and return from Chapa
**Open:** A held booking, Pay online

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| I1 | On the held booking choose "Pay online" and tap Pay. | The Chapa page opens in an in-app browser (Telebirr, CBE Birr, M-Pesa, bank or card). | ☐ |
| I2 | Complete the payment (test details). | You return to the app: "Confirming your payment", then "Waiting for the hotel". You do not need to stay on the screen. | ☐ |
| I3 | Close the payment page without paying, then tap "Open the payment page again". | The same page opens; you are never charged twice. | ☐ |
| I4 | Start paying, then kill the app and reopen it. | The app takes you straight back to that booking. | ☐ |
| I5 | Make a payment fail (cancel on the Chapa page or use a failing test card). | "The payment did not go through. You have not been charged. Try again." and you can pay again while the hold lasts. | ☐ |
| I6 | Tap Pay twice quickly. | One payment page, one charge. | ☐ |
| I7 | Choose "Pay at the hotel" (on a hotel that allows it) and tap Reserve. | No payment page. The booking goes straight to "Waiting for the hotel"; the voucher later shows "Pay at the hotel" with the amount. | ☐ |
| I8 | Try "Pay at the hotel" with a coupon applied. | "This booking has a coupon, so it is paid online." Only online payment is offered. | ☐ |
| I9 | Try "Pay at the hotel" on a hotel that does not allow it. | The option is not shown. | ☐ |
| I10 | Let the hold run out while the payment page is open, then pay. [Wait] | If the room was already released the money is refunded automatically; the booking stays expired. | ☐ |
| I11 | Check your email after paying. [Email] | "Your booking request at ..." is received. | ☐ |
| X28 | After paying, look at the screen that returns from Chapa. | It shows the right booking and result, never a blank page. | ☐ |
| Z23 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

**End of Day 6:** passed ____ , failed ____ , skipped ____

---

## Day 7: Hotel app answers requests
*Goal: Complete the first bookings on both sides. Then create the dated bookings.* (4 pages, 25 checks)

### 7.1 Hotel app: Sign in
**Open:** Open BookDan for hotels

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-B1 | Sign in with an email that is not hotel staff. | Told this account is not staff. | ☐ |
| PA-B2 | Sign in as the owner with the emailed code; wrong code first. | Wrong code refused; right code opens Bookings. | ☐ |
| Z24 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

### 7.2 Hotel app: Bookings tab
**Open:** Bookings

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-B3 | Look at the counters: To answer, Arriving today, In the hotel. | Numbers match the bookings. | ☐ |
| PA-B4 | When a guest pays, watch the app. | The request appears with a 15-minute countdown (and a phone alert if set up). | ☐ |
| X29 | Pull down to refresh with no bookings, then with several. | Empty message, then the correct list. | ☐ |
| Z25 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

### 7.3 Hotel app: Booking request
**Open:** Tap a waiting request

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-B5 | Open the request. | Dates, room, guests, children, requests, breakfast, non-refundable, paid or pay at hotel, masked ID. | ☐ |
| PA-B6 | Press "Show ID number", then "Hide". | Full number only while shown; the view is logged. | ☐ |
| PA-B7 | Accept a request. | Guest sees Confirmed at once. | ☐ |
| PA-B8 | Decline one for each reason: no rooms, not available, maintenance, other. | Guest is declined and refunded. | ☐ |
| PA-B9 | Let one time out. | "Time is up"; guest refunded automatically. | ☐ |
| PA-B10 | Answer in the app a request already answered in the web portal. | "This booking was already answered." | ☐ |
| PA-B11 | Turn internet off and try to accept. | Told answering needs internet. | ☐ |
| Z26 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

### 7.4 Guest app: Waiting for the hotel
**Open:** Guest booking page

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| J1 | After paying, open the hotel app and tap Accept. [Hotel app] | Within seconds the guest booking shows "Booking confirmed" with the timeline step "Confirmed by the hotel" and a View voucher button. [Email] A confirmation email arrives (this is the receipt). [Push] A phone alert says the hotel confirmed. | ☐ |
| J2 | Pay a second booking and Decline it in the hotel app with a reason. [Hotel app] | "The hotel could not confirm: your payment will be refunded." The refund reaches you; [Email] an email says so. | ☐ |
| J3 | Pay a third booking and do nothing for 15 minutes. [Wait] | The booking becomes declined for no answer and you are refunded automatically; the hotel app shows the time ran out. | ☐ |
| J4 | Pay a booking and cancel it yourself before the hotel answers. | Full refund; the hotel's request disappears. | ☐ |
| J5 | Close the guest app while waiting. | You still get the phone alert when the hotel answers. [Push] Tapping it opens that booking. | ☐ |
| J6 | Tap "Notify me" on the waiting screen, allow notifications, then deny them in phone settings and open the card again. | It asks permission once; if denied it says "Notifications are off for this app" with Open settings. | ☐ |
| X30 | Tap "Notify me" and answer the request. | A message arrives. Mark S without Firebase. | ☐ |
| Y26 | On the booking screen read reference, room, meal, cancellation, guests with children, your requests and total. | All match what you chose. | ☐ |
| Y27 | On an expired or declined booking tap "Search again". | Search opens. | ☐ |
| Z27 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

> **At the end of today, create the dated bookings in the table near the top.**

**End of Day 7:** passed ____ , failed ____ , skipped ____

---

## Day 8: Hotel app: check-in and rooms
*Goal: The front desk on arrival day.* (2 pages, 17 checks)

### 8.1 Hotel app: Check-in tab
**Open:** Check-in

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-C1 | Check-in tab: refuse the camera permission. | Told how to allow it; manual entry still works. | ☐ |
| PA-C2 | Scan today's guest voucher. | Right guest shown; Check in works; "Checked in". | ☐ |
| PA-C3 | Check in by reference and PIN. | Works. | ☐ |
| PA-C4 | Scan the same voucher again. | "Already checked in." | ☐ |
| PA-C5 | Scan a voucher for a future date, a cancelled booking, another hotel, and a random QR code. | Each refused with its own message. | ☐ |
| PA-C6 | Enter a wrong PIN five times. | Locked for 30 minutes; the QR still works. | ☐ |
| PA-C7 | Check in a pay-at-hotel guest. | Shows the exact amount to collect. | ☐ |
| PA-C8 | Open Check-in once online, switch off internet, scan today's voucher, save. | "Saved. It is sent when online." and a waiting counter. | ☐ |
| PA-C9 | Turn internet on. | The check-in is sent; admin and guest show it. | ☐ |
| PA-C10 | On a booking, mark "Guest did not arrive" (check-in day). | Asks first; then no-show; later nights go back on sale. | ☐ |
| Z28 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

### 8.2 Hotel app: Rooms tab
**Open:** Rooms

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-R1 | Look at the nights: free, booked, being paid, yours. | Numbers match real bookings. | ☐ |
| PA-R2 | Tap a night and add a walk-in with no name, then with a name. | Name required; then added and the free count drops. | ☐ |
| PA-R3 | Block rooms for several nights. | Guest app cannot book them. | ☐ |
| PA-R4 | Block more rooms than are free. | "Not enough free rooms". | ☐ |
| PA-R5 | "Give the rooms back". | Asks first; rooms are on sale again. | ☐ |
| Z29 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

**End of Day 8:** passed ____ , failed ____ , skipped ____

---

## Day 9: Hotel app: money, reviews, support and settings; guest voucher
*Goal: Everything else in the hotel app.* (8 pages, 50 checks)

### 9.1 Guest app: Confirmed booking and voucher
**Open:** My bookings, a confirmed booking

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| K1 | Open the confirmed booking. | Status Confirmed, hotel, room, dates with check-in and check-out times, guests, payment method, total and the timeline. | ☐ |
| K2 | Tap View voucher. | A QR code and "Show PIN" (the 4-digit PIN is hidden until you tap). A note says to give the PIN only if the QR does not scan. | ☐ |
| K3 | Turn on airplane mode and open the voucher. | It still opens: "Saved on this phone. Works without internet." | ☐ |
| K4 | On a phone that never opened the booking, open it offline. | "Voucher is not on this phone yet. Connect to the internet once to download it." | ☐ |
| K5 | Tap Directions on the voucher and "Call the hotel" on the booking. | Maps opens; the hotel's phone number is now visible and the dialler opens. (Each tap is counted in the admin Calls tab.) [Admin] | ☐ |
| K6 | Tap Add to calendar. | Your calendar shows the stay with check-in and check-out. | ☐ |
| K7 | On a pay-at-hotel booking open the voucher. | "Pay at the hotel" with the exact balance to bring; an online-paid booking shows "Nothing to pay". | ☐ |
| K8 | At the hotel (or with the partner app), scan the voucher or type the reference and PIN. [Hotel app] | The hotel sees the guest and checks them in. The guest booking becomes Checked in, and the cancel button disappears. | ☐ |
| K9 | Check-in with a wrong PIN five times. [Hotel app] | The PIN locks for 30 minutes; the QR code still works. | ☐ |
| Y28 | Tap "Download voucher" on a phone that has not opened it. | The voucher appears and then works offline. | ☐ |
| Y29 | Tap Show PIN and Hide PIN. | The 4 digits show and hide. | ☐ |
| Z30 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 9.2 Hotel app: Confirmed booking: problems
**Open:** Bookings, a confirmed booking

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-P1 | As owner, "Cancel this booking". | Asks first; guest refunded in full; hotel bears the refund. | ☐ |
| PA-P2 | As receptionist, look for Cancel. | Not allowed (owner only). | ☐ |
| PA-P3 | "We cannot host this guest": try each reason. | "Waiting for BookDan support"; the guest is told; it shows in admin Support. | ☐ |
| PA-P4 | When a guest opens a dispute, open the booking and send the hotel's account (try under 10 characters first). | Short refused; then sent; payment shown as held. | ☐ |
| PA-P5 | "Get help" on a booking. | A request linked to that booking. | ☐ |
| Z31 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

### 9.3 Hotel app: Earnings
**Open:** Earnings tab

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-E1 | Earnings: check BookDan owes you, After check-in, Paid to you. | Match the admin Payouts page. | ☐ |
| PA-E2 | Open a booking line and recalculate: price, fee 15%, VAT on fee, you receive. | The sum is correct. | ☐ |
| PA-E3 | Look after a pay-at-hotel stay. | Commission owed is shown; "You owe BookDan" when larger than owed to you. | ☐ |
| Z32 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

### 9.4 Hotel app: Monthly statements
**Open:** Earnings, Monthly statements

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-E4 | Monthly statements: open this month, share it. | Opening and closing balance add up; sharing works. | ☐ |
| Y30 | Choose earlier months, including one with no stays. | Correct figures; "No stays ended this month". | ☐ |
| Y31 | Check each line: paid online or at the hotel, fee and VAT. | Online share, commission, collected at hotel, paid to you and you paid add up to the closing balance. | ☐ |
| Y32 | Tap "Share this statement". | A readable text with the same figures. | ☐ |
| Z33 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

### 9.5 Hotel app: Guest reviews
**Open:** Settings, Guest reviews

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-E5 | Reviews: publish a reply (under 3 characters first), change it, remove it. | Short refused; reply shows in the guest app and disappears when removed. | ☐ |
| X31 | Open it before any review exists. | "No reviews yet". | ☐ |
| Z34 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

### 9.6 Hotel app: Owner and receptionist rights
**Open:** Sign in as receptionist

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-E6 | Sign in as receptionist and open Earnings, Statements, Reviews. | Owner only. | ☐ |
| Y33 | As receptionist, answer a request, check a guest in and block a room. | All allowed. | ☐ |
| Y34 | As receptionist, open a confirmed booking. | No cancel button; message says only an owner can. | ☐ |
| Y35 | Remove the receptionist in the admin portal while they are signed in, then use the app. | Access ends. | ☐ |
| Z35 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

### 9.7 Hotel app: Help and support
**Open:** Settings, Help and support

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-T1 | Help and support: new request, reply, close. | Appears in admin Support; replies arrive. | ☐ |
| Y36 | Send with a subject under 3 characters, then with no message. | Each is refused with a message. | ☐ |
| Y37 | Send one "Question" and one "Problem". | Both listed, newest activity first. | ☐ |
| Y38 | Open one after the admin answered. | The answer is shown as from Support. | ☐ |
| Y39 | Close a request and try to reply. | Closed; a new request is needed. | ☐ |
| Z36 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

### 9.8 Hotel app: Settings and alerts
**Open:** Settings tab

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-T2 | Settings: change language to Amharic and back. | All screens change. | ☐ |
| PA-T3 | An account with two hotels: switch hotel. | Each shows only its own data. | ☐ |
| PA-T4 | Alerts checklist: allow notifications, check each step. | Each shows Done or To do correctly. | ☐ |
| PA-T5 | "Send a test alert", lock the phone. | Phone rings in about 5 seconds (needs Firebase). | ☐ |
| PA-T6 | "Alerts I want": switch kinds off and on. | Requests always on; others follow your choice. | ☐ |
| PA-T7 | "Apply to list your hotel" link (sign-in screen). | Opens the apply page. | ☐ |
| PA-T8 | Publish an update and reopen twice. | Update banner; Restart loads the new version. | ☐ |
| PA-T9 | Sign out. | Back at sign-in; no data visible. | ☐ |
| Z37 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. | ☐ |

**End of Day 9:** passed ____ , failed ____ , skipped ____

---

## Day 10: Hotel web portal
*Goal: The /hotel website, page by page.* (13 pages, 58 checks)

### 10.1 Hotel web portal: Sign-in
**Open:** bookdan.onrender.com/hotel

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-S1 | Open /hotel, sign in as owner with the code. | Home opens. | ☐ |
| HW-S5 | Sign in with an email linked to no hotel. | Clear message. | ☐ |
| X32 | Wrong code, then Sign out and press Back. | Wrong refused; no data after sign-out. | ☐ |
| Z38 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.2 Hotel web portal: Home
**Open:** Menu: Home

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-S2 | Check tiles: Waiting for reply, Arriving, Staying, Leaving, BookDan owes you. | Match the hotel app. | ☐ |
| HW-S3 | Look at "Answer these now" and "Next arrivals". | Correct bookings. | ☐ |
| HW-S4 | Sign in as receptionist. | Owner-only pages are hidden. | ☐ |
| Z39 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.3 Hotel web portal: Bookings
**Open:** Menu: Bookings

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-B1 | Bookings: use every filter. | Each lists the right bookings; empty ones say "Nothing here". | ☐ |
| HW-B2 | Accept, Decline (with reason), Check in, No-show, Cancel booking on suitable bookings. | Each asks where risky and updates guest app and admin. | ☐ |
| Z40 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.4 Hotel web portal: Availability calendar and blocks
**Open:** Menu: Availability

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-B3 | Availability: Earlier, Today, Later. | Dates move; colours: green free, amber partly, red sold out. | ☐ |
| HW-B4 | Add a walk-in and an out-of-service block; then Release. | Counts change and return. | ☐ |
| HW-B5 | Block with the end date before the start. | Refused. | ☐ |
| Z41 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.5 Hotel web portal: My hotel
**Open:** Menu: My hotel

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-H1 | My hotel: change description, phone, pin (paste a map link), rules; save. | Live in the guest app at once; admin Hotel changes records it. | ☐ |
| HW-H6 | Look for business or payout details and Verified or Status. | Not editable by the owner. | ☐ |
| Z42 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.6 Hotel web portal: Rooms and prices
**Open:** Menu: Rooms & prices

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-H2 | Rooms and prices: add, edit, remove a room; change a price. | Same behaviour as in the admin portal. | ☐ |
| Y40 | Add a room type with breakfast and a non-refundable discount. | Appears in the list and in the guest app. | ☐ |
| Y41 | Type a word in price or breakfast, and 60 in the discount. | Refused with a message. | ☐ |
| Y42 | Upload room photos, "Make first", delete one. | The guest app room card follows. | ☐ |
| Y43 | Remove a never-booked room, then a booked one. | First removed; second refused. | ☐ |
| Z43 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.7 Hotel web portal: Photos
**Open:** Menu: Photos

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-H3 | Hotel and room photos: upload, cover, delete. | Appear in the guest app. | ☐ |
| Y44 | Upload several photos at once. | Progress shown; all appear. | ☐ |
| Y45 | "Make cover" on another photo. | The guest app shows the new cover. | ☐ |
| Y46 | Delete a photo; upload a non-image file. | Removed after confirming; the file is refused. | ☐ |
| Z44 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.8 Hotel web portal: Team
**Open:** Menu: Team

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-H4 | Staff: invite and remove a receptionist. | Works; an owner cannot remove an owner (only an admin can). | ☐ |
| Y47 | Add a receptionist and read the invitation email. | Listed as "Invited"; email arrives. | ☐ |
| Y48 | After they sign in, reload. | "Joined". | ☐ |
| Y49 | Remove the receptionist; look for Remove beside an owner. | Removed after confirming; owners cannot be removed by an owner. | ☐ |
| Z45 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.9 Hotel web portal: Booking alerts
**Open:** Menu: Booking alerts

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-H5 | Booking alerts: change rules and shifts. | Saved; admin shows the same. | ☐ |
| Y50 | Switch between the two alert modes and save. | Saved. | ☐ |
| Y51 | Set shift times and switch a person off. | Kept after reload. | ☐ |
| Y52 | Compare with the admin portal. | Identical. | ☐ |
| Z46 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.10 Hotel web portal: Earnings
**Open:** Menu: Earnings

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-M1 | Earnings: tiles and booking lines, payments list. | Same numbers as app and admin. | ☐ |
| Y53 | Check the four totals: owes you now, ready, stays to come, already paid. | Match the hotel app and admin Payouts. | ☐ |
| Y54 | Check each booking line: guest paid, fee, VAT, you receive, ready or not yet. | Sums are correct. | ☐ |
| Y55 | Read the payments list and any "You owe BookDan" warning. | Match recorded payouts. | ☐ |
| Z47 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.11 Hotel web portal: Statements
**Open:** Menu: Statements

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-M2 | Statements: choose different months. | Correct stays and balances; empty month says so. | ☐ |
| Y56 | Check opening balance, online share, paid and closing balance. | They add up. | ☐ |
| Y57 | Open last month and compare with the hotel app. | Same figures. | ☐ |
| Z48 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.12 Hotel web portal: Reviews
**Open:** Menu: Reviews

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-M3 | Reviews: post a reply, delete it. | Visible then gone in the guest app. | ☐ |
| Y58 | Open before any review exists. | "No reviews yet". | ☐ |
| Y59 | Post a reply under 3 characters. | Refused. | ☐ |
| Y60 | Edit by deleting and posting again. | The guest app shows the new reply. | ☐ |
| Z49 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

### 10.13 Hotel web portal: Help from BookDan and phone view
**Open:** Menu: Help from BookDan

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-M4 | Help from BookDan: new question, reply, close conversation. | Seen and answered in admin Support. | ☐ |
| HW-M5 | Use the portal on a phone browser. | Usable without sideways scrolling problems. | ☐ |
| Y61 | Send a question with an empty subject. | Refused. | ☐ |
| Y62 | Open every menu page on a phone. | All usable. | ☐ |
| Z50 | General checks for this page. | Page loads without error. Works after refresh. Usable on a phone browser. Spelling is correct. | ☐ |

**End of Day 10:** passed ____ , failed ____ , skipped ____

---

## Day 11: Guest app: my bookings, saved, cancel and change dates
*Goal: Everything a guest does after booking.* (4 pages, 32 checks)

### 11.1 Guest app: My bookings
**Open:** My bookings tab

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| O1 | Open My bookings signed out. | "Sign in to see your bookings" with a Sign in button. | ☐ |
| O2 | Open it signed in with no bookings. | "No bookings yet" and Find a stay. | ☐ |
| O3 | Open My bookings with several bookings and tap each tab: Upcoming, Completed, Cancelled, Failed. | The sliding tabs show a count each; every booking is in the right tab, with hotel, dates, total and status. | ☐ |
| O4 | Pull to refresh; tap a booking. | The list reloads; the booking opens. | ☐ |
| Z51 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 11.2 Guest app: Saved
**Open:** Saved tab

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| O5 | Saved: tap hearts on three hotels. | They appear on the Saved tab; unsaving removes them. With nothing saved: "No saved stays yet". | ☐ |
| O6 | Sign in on a 2nd phone with the same email. [2nd phone] | The saved hotels from the first phone appear after a few seconds. | ☐ |
| O7 | Turn off internet, remove a saved hotel, turn internet on, wait a minute, check the 2nd phone. | The removal reaches the server and the hotel is gone on the other phone too; it does not come back. | ☐ |
| Z52 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 11.3 Guest app: Cancelling
**Open:** A booking, cancel card

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| L1 | Open a confirmed refundable booking for 5+ days ahead and scroll to the cancel card. | It says "You will get X back" and "Free cancellation until <date and time>". | ☐ |
| L2 | Tap Cancel booking, then Keep booking. | Nothing changes. | ☐ |
| L3 | Cancel it (Yes, cancel). | Status Cancelled: "Your booking was cancelled and X is on its way back to you." The hotel app gets an alert; [Email] a cancellation email. The room is free again in search. | ☐ |
| L4 | Make a booking for tomorrow or today, get it confirmed, open the cancel card. | It says the free period has ended and you will not get a refund. Cancelling is still allowed until the hotel checks you in; the booking is then cancelled with no refund and the hotel keeps its share. | ☐ |
| L5 | Book a non-refundable room, get it confirmed, open the cancel card. | "This booking is non-refundable." Cancel is allowed but nothing comes back. | ☐ |
| L6 | Pay a booking and cancel before the hotel answers. | Full refund. | ☐ |
| L7 | Cancel a pay-at-hotel booking. | "You had not paid anything online, so there is nothing to refund." The room is freed. | ☐ |
| L8 | Look at a booking that has been checked in, or a stay that is over. | No cancel button. A message tells you to contact the hotel; use "Report a problem" (section P). | ☐ |
| L9 | Ask the hotel to cancel the confirmed booking, or to say it cannot host you. [Hotel app] | Cancelled by the hotel: full refund, an email and a support request. "Cannot host": the booking says "Needs a new hotel" and you can cancel for a full refund at once. | ☐ |
| Z53 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 11.4 Guest app: Change dates
**Open:** A confirmed booking, Change dates

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| M1 | Open a confirmed refundable booking 5+ days away and tap "Change dates". | A calendar to pick new dates. The button is not shown for non-refundable bookings, bookings with a coupon, bookings less than 48 hours away, or ones already moved once. | ☐ |
| M2 | Pick new dates with the same number of nights and the same price, tap "See the price". | "The new dates cost the same. There is nothing to pay or refund." Confirm moves the booking; the hotel app gets an alert; [Email] "Dates changed". The first dates are shown as "First booked". | ☐ |
| M3 | On another booking pick a shorter stay (fewer nights). | The price card shows the new total, then "The new dates cost X less. The difference is sent back the way you paid." Confirm: "Move and get X back". The booking moves and the difference is refunded. | ☐ |
| M4 | On another booking pick a longer stay. | "The new dates cost X more. You pay the difference now." Confirm: "Pay X and move". The Chapa page opens for the difference only. | ☐ |
| M5 | Pay the difference. | The booking screen shows "waiting for payment", then moves to the new dates by itself with the new total. | ☐ |
| M6 | Start a dearer change, then on the booking tap "Keep my old dates". | The held nights are released; the booking is unchanged and can be changed again. | ☐ |
| M7 | Start a dearer change and do not pay for 15 minutes. [Wait] | "The time to pay has ended. Your booking stays on the old dates." | ☐ |
| M8 | Start a dearer change, close the payment page, return to the booking. | A card "Your new dates are waiting for payment" with Pay and a countdown; Pay reopens the same page, or a new one after a failure. | ☐ |
| M9 | Pick new dates where the room is full. | "The room is not free on all of those nights. Try other dates." | ☐ |
| M10 | Try a second change on a booking already moved. | "This booking was already moved once." | ☐ |
| M11 | Change dates on a pay-at-hotel booking to a different length. | It moves and the amount to pay at the hotel changes; nothing is charged online. | ☐ |
| M12 | After a dearer change, cancel the booking. | Everything you paid, both payments, is refunded. | ☐ |
| Z54 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

**End of Day 11:** passed ____ , failed ____ , skipped ____

---

## Day 12: Reviews, problems and support, from guest to admin
*Goal: What happens when a guest needs help or complains.* (7 pages, 46 checks)

### 12.1 Guest app: Review and report a problem
**Open:** A finished stay

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| N1 | After check-out day open a finished stay. | A review form: stars, four optional category ratings and a comment (up to 1000 characters). Before check-out there is no form. | ☐ |
| N2 | Post a review. | "Your review" appears; you cannot write a second one. It shows on the hotel page with your first name. | ☐ |
| N3 | Reply to your review as the hotel owner. [Hotel app] | The reply appears under your review on the hotel page and on your booking ("Reply from the hotel"). | ☐ |
| N4 | Ask an admin to hide your review. [Admin] | It disappears from the hotel page and rating, but you still see it on your booking. | ☐ |
| N5 | During or after a stay open "Report a problem with this stay". | Choose a reason (not as described, refused a room, charged wrong, marked no-show, other), write at least 10 characters, Open the dispute. | ☐ |
| N6 | Look at the booking after opening a dispute. | Status "Under review"; "See your dispute" opens the conversation; the hotel has been alerted. [Hotel app] | ☐ |
| N7 | Withdraw the dispute. | The booking returns to what it was; you cannot open another for it. | ☐ |
| N8 | Let an admin decide (reject, refund, part refund). [Admin] | You get a note in the thread, [Email] an email, and a refund when upheld; the outcome shows (Not upheld, Refunded in full...). | ☐ |
| N9 | Try to open a dispute 15 days after check-out, or for a stay that has not started. | Refused with a message (use Help and support after 14 days; cancel instead if the stay has not started). | ☐ |
| Z55 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 12.2 Guest app: Help and support
**Open:** Profile, Help and support

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| P1 | Profile, then Help and support, then New request. Choose Question, write a subject and a message, Send. | "Sent. We answer here and by email." The request shows "Waiting for support". | ☐ |
| P2 | Send with a subject under 3 characters or an empty message. | A message says what is missing. | ☐ |
| P3 | Answer the request in the admin page (Support). [Admin] | The thread shows "Support answered". [Email] [Push] An email and a phone alert arrive; tapping the alert opens the thread. | ☐ |
| P4 | Reply in the thread. | Your reply appears and the status returns to waiting for support. | ☐ |
| P5 | Close the request. | "This request is closed. Start a new one if you still need help." You cannot reply. | ☐ |
| P6 | Open five requests without closing any, then a sixth. | "You have several requests waiting for an answer. Wait for a reply, or close one you no longer need." | ☐ |
| P7 | On a booking tap "Get help with this booking". | A new request already linked to that booking ("About booking BD-..."). | ☐ |
| P8 | Ask an admin for a goodwill partial refund on a confirmed booking. [Admin] | The booking shows "Already refunded to you" and the amount arrives. | ☐ |
| Z56 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 12.3 Admin portal: Support list
**Open:** Menu: Guests, Support

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-T1 | Support: open the list. | "Needs an answer" and "Waiting for the guest" are right, newest first. | ☐ |
| AD-T2 | Send an answer; then close another without answering. | Guest gets it in the app and by email. | ☐ |
| Y63 | Open a request linked to a booking. | Hotel, guest, dates, status and amounts paid and refunded are shown. | ☐ |
| Z57 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 12.4 Admin portal: Support request: refunds, relocation, disputes
**Open:** Open a request

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-T3 | Goodwill refund: BookDan pays; then hotel pays; then try the whole amount. | Booking stands; payer is charged correctly; full amount refused. | ☐ |
| AD-T4 | Relocation: Restore on one; Refund in full on another. | First confirmed again; second refunded, hotel bears it. | ☐ |
| AD-T5 | Dispute: try each decision (reject, full refund hotel pays, full refund BookDan pays, part refund by each). | Refunds and hotel earnings match the choice; asks before settling. | ☐ |
| Z58 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 12.5 Admin portal: Users
**Open:** Menu: Guests, Users

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-U1 | Users: search by email, phone, name; filter by status. | Right people; "Nobody matches" when none. | ☐ |
| AD-U2 | Suspend a guest with a reason; try to use that account. | Blocked. | ☐ |
| AD-U3 | Reinstate. | Can sign in and book again. | ☐ |
| AD-U4 | "Sign out everywhere". | All their devices need to sign in again. | ☐ |
| Y64 | Look at one guest, one owner and one receptionist. | Role, hotels, bookings count, signed-in devices and joined date are correct. | ☐ |
| Z59 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 12.6 Admin portal: Reviews
**Open:** Menu: Guests, Reviews

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-U5 | Reviews: filters and sorting; hide one with a reason; "Show again". | Hidden review and its rating leave the guest app, then return. | ☐ |
| Y65 | Use the status filter, sort by newest and lowest rating, and search. | Lists match. | ☐ |
| Y66 | Hide with no reason, then with a reason. | A reason is asked and kept; shows who hid it. | ☐ |
| Y67 | Look at a review with a hotel reply. | The reply is shown. | ☐ |
| Z60 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 12.7 All apps: Emails
**Open:** The inboxes of your test guest, owner and admin

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| Y87 | Request a booking. | "Your booking request at (hotel)" arrives with the right details. | ☐ |
| Y88 | Hotel accepts one, declines one, ignores one. | "Booking confirmed", "Booking not available" and "Booking not confirmed" emails. | ☐ |
| Y89 | Cancel one with refund, one pay-at-hotel, one too late. | Three "Booking cancelled" emails, each saying the right thing about money. | ☐ |
| Y90 | Hotel cancels one and reports it cannot host another. | "cancelled by the hotel" and "cannot host you" emails. | ☐ |
| Y91 | Change dates. | "Dates changed" email with the new dates and total. | ☐ |
| Y92 | Check support replies, dispute results, staff invitations and the application decision. | Each email arrives, with no password or full ID number in it. | ☐ |
| Y93 | Check the admin inbox after a dispute or payment problem. | The BookDan team is emailed. | ☐ |
| Y94 | Read all emails on a phone; check the spam folder. | Readable, correct sender, not marked as spam. | ☐ |

**End of Day 12:** passed ____ , failed ____ , skipped ____

---

## Day 13: Admin: money, insights and settings
*Goal: Check every figure.* (9 pages, 46 checks)

### 13.1 Admin portal: Payment issues
**Open:** Menu: Guests, Payment issues

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-M1 | Payment issues: open each "Needs a person" and mark resolved with a note. | Moves to resolved with your name. | ☐ |
| Y68 | Read each issue: booking code, asked amount, paid amount, date. | Clear and matching the Chapa test dashboard. | ☐ |
| Y69 | Check "Hotel cannot host the guest" items after Day 9. | Listed or visible in Support, and can be followed up. | ☐ |
| Y70 | Reload after resolving. | Resolved items show who and the note. | ☐ |
| Z61 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 13.2 Admin portal: Payouts, receipts and statements
**Open:** Menu: Money, Payouts

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-M2 | Payouts: compare To pay now, After check-in, Paid so far. | Only checked-in or finished stays are payable. | ☐ |
| AD-M3 | Record a payout: no reference, too much, then correct. | First two refused; then owed falls by exactly that amount. | ☐ |
| AD-M4 | Show payout history; Download CSV for the month. | History correct; file opens in Excel with right totals. | ☐ |
| AD-M5 | When a hotel owes BookDan, "Record payment received". | Asks to confirm; balance updates. | ☐ |
| Y71 | Look at a hotel with no business details. | Told to add them before paying. | ☐ |
| Y72 | Check the payment method and last four digits on each card. | Match the Business tab. | ☐ |
| Z62 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 13.3 Admin portal: Member tiers
**Open:** Menu: Money, Member tiers

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-M6 | Member tiers: change a discount, try 90%, save, restore. | 90% refused; guest app shows the new level discount. | ☐ |
| AD-M7 | After a guest finishes stays at 2 or more hotels, check their level in the app. | Level and discount shown; not combined with a coupon; online only. | ☐ |
| Z63 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 13.4 Admin portal: Exchange rates
**Open:** Menu: Money, Rates

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-M8 | Rates: Fetch current rates; type a rate by hand; type a wildly wrong one. | Saved; warning on odd values; guest app shows converted prices. | ☐ |
| Y73 | Read the status of each currency: Shown to guests, Out of date, Not set. | Correct. | ☐ |
| Y74 | Compare "typed" rates and fetched rates after "Fetch current rates". | The page says how many were updated and left as they were. | ☐ |
| Y75 | Change a rate and check the guest app after reopening. | New converted price. | ☐ |
| Z64 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 13.5 Admin portal: Analytics
**Open:** Menu: Insights, Analytics

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-I1 | Analytics: 7, 30, 90 days. | Booking value, commission, refunds and counts match your test bookings. | ☐ |
| AD-I2 | Click the "needs a person" items. | Open the right pages. | ☐ |
| Y76 | Check the steps from price to stay: quotes, holds, paid, confirmed. | Counts and percentages are sensible. | ☐ |
| Y77 | Check the daily charts and the hotels table (requests, accepted, no answer, reply time, cancellations). | Match what you did this week. | ☐ |
| Y78 | Check paid online, pay at hotel, new and returning guests. | Correct. | ☐ |
| Z65 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 13.6 Admin portal: Calls
**Open:** Menu: Insights, Calls

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-I3 | Calls: tap "Call the hotel" in the guest app, then open Calls. | The call is counted for that hotel. | ☐ |
| Y79 | Choose 7, 30 and 90 days. | Figures change correctly. | ☐ |
| Y80 | Read "By hotel" and "Latest calls". | Calls, bookings with calls, last call, booking code and guest name are right. | ☐ |
| Z66 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 13.7 Admin portal: Hotel changes
**Open:** Menu: Hotels, Hotel changes

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-I4 | Hotel changes: look for the edits made this week. | Who, what and when are listed. | ☐ |
| Y81 | Filter to changes made by hotels. | Only those show. | ☐ |
| Y82 | Click a hotel name in the list. | The hotel opens. | ☐ |
| Y83 | Find your price, photo, staff and payout-account-view actions. | All listed with person and time. | ☐ |
| Z67 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 13.8 Admin portal: Admins and roles
**Open:** Menu: Settings, Admins

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-I5 | Admins: add a support and a finance admin. | Temporary password shown once; they must choose their own. | ☐ |
| AD-I6 | Sign in as support, then as finance. | Each sees only its pages and cannot do the other's actions. | ☐ |
| AD-I7 | Reset a password; remove an authenticator; switch an admin off. | Each takes effect. | ☐ |
| AD-I8 | Try to remove your own super admin rights. | Refused. | ☐ |
| Y84 | Sign in as a new admin with the temporary password. | Must choose a new password of at least 12 characters before anything else. | ☐ |
| Y85 | Lock a test admin with five wrong passwords, then look at the list. | Shown as locked; a super admin can unlock or reset. | ☐ |
| Z68 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

### 13.9 Admin portal: My account
**Open:** Menu: Settings, My account

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-I9 | My account: change password (try a short one first). | Short refused; new one works. | ☐ |
| AD-I10 | Set up the authenticator app, sign in with the code, use one recovery code, make new codes, turn it off. | Each works; a used recovery code fails the second time. | ☐ |
| Y86 | Sign in using a recovery code instead of the authenticator code. | Works once; the number of codes left goes down. | ☐ |
| Z69 | General checks for this page. | Page loads without error. Buttons cannot be double-submitted. Works after refresh (Ctrl+F5). Usable on a narrow window. Spelling is correct. | ☐ |

**End of Day 13:** passed ____ , failed ____ , skipped ____

---

## Day 14: Profile, offline, privacy, whole system and sign-off
*Goal: Finish and decide.* (5 pages, 36 checks)

### 14.1 Guest app: Profile, language, currency and account
**Open:** Profile tab

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| Q1 | Profile: switch the language between English and Amharic. | Every screen changes (including dates, buttons and messages). Report any text left in English. | ☐ |
| Q2 | Profile: tap the Currency row, or the currency button at the top of the search or home screen. | A Currency screen: a note that you are always charged in birr, Default (Ethiopian Birr), Popular currencies (AED, SAR, QAR, KWD, OMR, BHD, USD, GBP, EUR) and More currencies, with a search icon. Tap USD: you return, and prices show as "about" dollars. Choose ETB again to go back. If the admin has no fresh rate for a currency it is not listed. | ☐ |
| Q3 | Profile: open Help and support. | The support list (section P). | ☐ |
| Q4 | Profile: Delete my account while a stay is still to come. | "You have a booking that is not finished. Cancel it, or wait until the stay is over, then delete your account." | ☐ |
| Q5 | Delete the account with no open bookings. | The account closes; your name, ID number, review text, support words and saved hotels are erased; you are signed out; booking records remain. Signing in again with the same email starts an empty account. | ☐ |
| Q6 | Look at the privacy notes on the details screen and the profile. | "Your ID is stored encrypted ... only the last four characters show." | ☐ |
| X33 | Tap "Privacy policy" and "Terms of use". | Each opens the matching page in the browser. | ☐ |
| Z70 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 14.2 Guest app: Offline, errors and unusual cases
**Open:** Airplane mode and edge cases

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| R1 | Turn airplane mode on while on a hotel page, then tap Hold. | A clear "No internet connection" message with Try again; no double booking afterwards. | ☐ |
| R2 | Turn internet off during payment, then on again. | The booking screen keeps polling and shows the real result when the connection returns. | ☐ |
| R3 | Open an old link or booking that does not exist (use a deleted account's booking). | A "something went wrong" or not-found screen with a way back; no crash. | ☐ |
| R4 | Rotate the phone and use the largest text size in phone settings. | Screens remain usable; nothing is cut off. | ☐ |
| R5 | Search for a stay starting today after 12:00. | Booking is possible; cancelling follows the same-day rule (L4). | ☐ |
| R6 | Book exactly 30 nights, then 31. | 30 works; 31 is refused with the 30-night message. | ☐ |
| R7 | Use the Android back button on every screen. | It goes to the previous screen; from checkout it does not lose the held room. | ☐ |
| R8 | Kill the server connection (airplane mode) while the confirmed booking is open. | The booking and voucher are still readable offline. | ☐ |
| Z71 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 14.3 Guest app: Privacy and safety
**Open:** Across the app

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| S1 | Before booking, search the hotel page for its phone number. | It is not shown; it appears only after the hotel confirms. | ☐ |
| S2 | Look at your ID anywhere in the guest app. | Only the last four characters are ever shown. | ☐ |
| S3 | Sign in as a second guest and open the first guest's booking link (if you have it). | Not found: you cannot see another guest's booking, voucher or support thread. | ☐ |
| S4 | Sign out and press Back. | No private data is shown. | ☐ |
| S5 | Ask for sign-in codes many times from one network (more than 30 in 10 minutes, for different emails). | "Too many codes requested from this connection. Try again in a few minutes." | ☐ |
| Z72 | General checks for this page. | Phone Back button works. Amharic text complete. Largest text size: nothing cut off. Airplane mode: clear message. Loading shows, no endless spinner. | ☐ |

### 14.4 Whole system: End to end
**Open:** All apps together

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| E2E1 | Do one full journey with fresh accounts: search, pay, accept, check in, finish, review, payout. | No step needs a workaround. | ☐ |
| E2E2 | Add up all test payments, refunds, commissions and payouts, and compare with Analytics, Payouts and Chapa test dashboard. | They agree to the birr. | ☐ |
| E2E3 | Change something in each portal and see it in the other apps. | Updates appear without reinstalling. | ☐ |
| E2E4 | Try a hotel owner opening another hotel's address in the portal. | Refused. | ☐ |
| E2E5 | Leave all portals open for an hour, then act. | Session renews or asks to sign in; nothing is lost silently. | ☐ |
| E2E6 | Check Amharic on every screen of both apps. | No missing or broken text. | ☐ |
| E2E7 | Open both apps on a small cheap phone and a weak network. | Usable; clear loading and error messages. | ☐ |
| E2E8 | Re-run every row that failed earlier. | All pass, or are listed as open problems. | ☐ |

### 14.5 Whole system: Things that happen by themselves
**Open:** Guest app, hotel app and admin portal

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| Y95 | Look at a held booking after 15 minutes. [Wait] | Released; the room is on sale again. | ☐ |
| Y96 | Look at an unanswered request after 15 minutes. [Wait] | Declined and refunded. | ☐ |
| Y97 | Look at an unpaid date change after 15 minutes. [Wait] | Old dates kept. | ☐ |
| Y98 | Look at stays whose check-out day has passed. | Shown as completed; review is offered; the hotel can be paid. | ☐ |
| Y99 | Look at a pay-at-hotel stay after check-in or completion. | The commission the hotel owes is recorded. | ☐ |

**End of Day 14:** passed ____ , failed ____ , skipped ____

---

## Final sign-off
| Question | Yes / No |
|---|---|
| Can a new guest book, pay, stay and review without help? | |
| Can a hotel answer, check in and see its money? | |
| Can the BookDan team add hotels, help guests and pay hotels? | |
| Do all money totals agree? | |
| Is there any open failure that loses money, exposes private data or blocks booking? | |
| Ready for real testers? | |
