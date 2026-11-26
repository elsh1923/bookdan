# BookDan: 7-day test plan

Every page and feature of the guest app, hotel app, hotel web portal, admin portal and public pages. **321 checks.**

## Before Day 1
- Test on the **live server** (https://bookdan.onrender.com) with **Chapa test keys**. The phone apps read the live server, not the local one.
- Push your latest code, wait for Render to show **Live**, then publish the update for both apps to the preview channel.
- You need: 2 Android phones (guest, hotel), a computer for the portals, 4 email addresses you can open (guest, second guest, owner, receptionist), and the super admin sign-in.
- Use made-up business and bank details. Never real card details.

## How to record
Mark each row: **P** (pass), **F** (fail) or **S** (skipped, say why). For every F, note what you did, what you saw, and take a screenshot.
The same list is in **BookDan-7-Day-Test-Plan.csv**, which opens in Excel.

## The week
| Day | What | Checks |
|---|---|---|
| 1 | Admin portal, setting up hotels | 44 |
| 2 | Guest app, discovering hotels | 59 |
| 3 | Booking, paying, and the hotel answering | 53 |
| 4 | Hotel app, everything else | 44 |
| 5 | Hotel web portal, cancelling and date changes | 42 |
| 6 | Admin portal, running the business | 45 |
| 7 | Account, offline, safety, re-test and sign-off | 34 |

Plan about 4 to 7 hours a day. If a day runs over, move the extra rows to Day 7 and write that down.

## Start these early (waiting is needed)
At the **end of Day 3**, create these bookings as paid and accepted, so later days have something to test:

| Booking | Dates | Used for |
|---|---|---|
| 1 | Check in on Day 4, 2 nights | QR check-in |
| 2 | Check in on Day 4, 1 night, pay at hotel | PIN check-in, money to collect |
| 3 | Check in on Day 4, 1 night | No-show |
| 4 | Check in on Day 4, 1 night, check in the guest | Review, dispute and payout on Day 6 |
| 5 | Same as 4 but at a second hotel | Member level on Day 6 |
| 6 | 5 or more days ahead, refundable | Cancel and change dates on Day 5 |
| 7 | 5 or more days ahead, refundable | Date change with extra payment on Day 5 |
| 8 | Check in on Day 6, refundable | Late cancel (inside 48 hours) on Day 5 |
| 9 | 5 or more days ahead, non-refundable | No refund check on Day 5 |

Rows marked **[Wait]** need time to pass (15 minutes for most). Start them, then do other rows.

## What cannot be finished in one week
- Anything that needs 14 days after check-out (the dispute deadline, row N9).
- The automatic erasing of full ID numbers after the retention period.
- Phone alerts and SMS rows, until Firebase and an SMS provider are set up. Mark them **S**.
- Real money. That waits for Chapa live approval.

---

## Day 1: Admin portal, setting up hotels
*Goal: Create everything the other days need.* (44 checks)

### Public pages: Server and legal pages

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| W1 | Open https://bookdan.onrender.com/v1/healthz and /v1/readyz. | Status "ok" and "ready". | ☐ |
| W2 | Open /privacy, /terms and /delete-account on the live site, on a computer and a phone. | All three load, read correctly, show a contact, and the links between them work. | ☐ |

### Admin portal: Sign-in

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-S1 | Open /admin. Sign in with a wrong password. | Refused with a clear message. | ☐ |
| AD-S2 | Sign in as super admin with the right password. | The portal opens with the menu: Hotels, Guests, Money, Insights, Settings. | ☐ |
| AD-S3 | Choose "Hotel owner" and send a code to an owner email. | If owner sign-in is on, a code arrives and only that owner's hotels show. If off, a clear message. | ☐ |
| AD-S4 | Press Sign out, then press the browser Back button. | Signed out; no private data shows. | ☐ |
| AD-S5 | Open the portal on a phone-sized window. | Menu and forms stay usable. | ☐ |

### Admin portal: All hotels and Add a hotel

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-H1 | Open All hotels. | Each hotel shows name, city, type, room and photo counts, and Published, Draft or Hidden. | ☐ |
| AD-H2 | Add a hotel and press Create with the name empty. | Refused with a message saying what is missing. | ☐ |
| AD-H3 | Fill every field, using the examples shown under each, and create it. | The hotel opens as a Draft. | ☐ |
| AD-H4 | Map: tap the map, then drag the pin. | Green "Pin set" line with numbers that change with the pin. | ☐ |
| AD-H5 | Map: paste a short Google Maps link (Share, Copy link). | "Looking up the link", then "Pin placed from your link". Check the building. | ☐ |
| AD-H6 | Map: paste numbers such as 9.0108, 38.7613. | Pin moves there. | ☐ |
| AD-H7 | Map: paste nonsense, then a non-Google link. | Red message; the pin does not move. | ☐ |
| AD-H8 | Map: press "Use my current location" and allow, then repeat and refuse. | Pin at your position; on refusal a clear message. | ☐ |
| AD-H9 | Map: open "Type the numbers instead" and change them. | Pin follows the typed numbers. | ☐ |
| AD-H10 | Save Amharic description and directions, house rules (one per line), check-in and check-out times, features. | All are still there after reload and appear in the guest app later. | ☐ |
| AD-H11 | Leave "Guests may pay at the hotel" off for Hotel A, on for Hotel B. | Saved as chosen. | ☐ |
| AD-H12 | Check "Verified by BookDan" is ticked on a new hotel. | Ticked by default; the badge shows in the guest app after publishing. | ☐ |

### Admin portal: Rooms and prices

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

### Admin portal: Hotel photos, staff, alerts, business

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-P1 | Upload several hotel photos, choose a cover, delete one. | The cover shows first in the guest app. | ☐ |
| AD-P2 | Upload a very large file and a non-image file. | Handled with a clear message; nothing breaks. | ☐ |
| AD-P3 | Staff: add an owner email and a receptionist email. | Both show "Invited"; each gets an invitation email. | ☐ |
| AD-P4 | Have both sign in to the hotel app, then reload Staff. | They show "Joined" with the right role. | ☐ |
| AD-P5 | Remove a receptionist. | They can no longer see this hotel. | ☐ |
| AD-P6 | Booking alerts: choose "Everyone on duty at once", then "Receptionists first" with minutes; set shift times; switch some alert kinds off. | Saved; "No phone" or "Signed in" is correct for each person. | ☐ |
| AD-P7 | Business tab: save with a field missing, then fill all and save. | Missing field refused; then saved, and only the last four digits of the account show. | ☐ |
| AD-P8 | Press "Show the full account number". | It asks first, shows it, and the view is recorded in Hotel changes. | ☐ |
| AD-P9 | Try publishing a hotel with no priced room, then with no business details. | Refused each time, saying what is missing. | ☐ |
| AD-P10 | Publish Hotels A, B and C (C has one single room). | All three show in guest search. | ☐ |
| AD-P11 | Hide a hotel that has an upcoming booking. | A warning lists it; it hides only after you confirm; the booking stays valid. | ☐ |

### Apply page: Hotel applications

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AP1 | Open /apply and send the form empty. | Required fields are pointed out. | ☐ |
| AP2 | Send a complete application. | A thank-you screen (and a reference). | ☐ |
| AP3 | Send the same hotel and email again. | Told an application is already waiting. | ☐ |
| AP4 | Admin, Applications: check all details are shown. | Matches what was typed. | ☐ |
| AP5 | Reject one with an empty reason, then with a reason. | Empty refused; with reason it is rejected and the applicant is told. | ☐ |
| AP6 | Approve another. | A draft hotel is created, the owner is invited, "Open the hotel" works. | ☐ |

**End of day:** passed ____ , failed ____ , skipped ____ . Send me the failed rows.

---

## Day 2: Guest app, discovering hotels
*Goal: First launch through the hotel page.* (59 checks)

### Guest app: First launch, language, currency and updates

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| A1 | Open the app for the first time (or clear its data). | A short loading screen with the logo, then the welcome screen: "Find a verified stay in Ethiopia", language and currency choices and Get started. | ☐ |
| A2 | Choose Amharic, then Get started. | The app switches to Amharic. Dates show Ethiopian dates under the calendar days (see D4). | ☐ |
| A3 | On the welcome screen tap the Currency row and choose a currency (for example USD). | The Currency screen opens; after choosing you return to the welcome screen showing the choice. Prices later show as approximate amounts, with the exact birr amount next to anything charged. | ☐ |
| A4 | Kill the app and reopen it. | It opens straight to the home screen (no welcome again), in the language you chose. | ☐ |
| A5 | Publish a new update (eas update) while the app is open in the background, then reopen the app twice. | A banner "A new version is ready" with Restart; Restart opens the new version. | ☐ |
| A6 | Open the app with airplane mode on. | The app opens; lists show "Showing saved results" or an offline message with Try again; nothing crashes. | ☐ |

### Guest app: Sign-in and sessions

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

### Guest app: Home screen

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

### Guest app: Searching

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

### Guest app: Results, filters and map

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

### Guest app: The hotel page

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

### Guest app: Extra checks on the hotel page

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HX1 | Scroll the hotel page and use the tabs Overview, Amenities, Reviews, Rules. | The sliding tabs jump to the right part; the top bar stays. | ☐ |
| HX2 | Open the photo gallery and swipe through every photo. | All photos load; closing returns to the page. | ☐ |

**End of day:** passed ____ , failed ____ , skipped ____ . Send me the failed rows.

---

## Day 3: Booking, paying, and the hotel answering
*Goal: The most important day. Create the "start early" bookings at the end.* (53 checks)

### Guest app: Checkout and holding a room

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

### Guest app: Your details and ID

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| H1 | Profile, then "My details and ID". Fill name, ID type, ID number and nationality, then Save. | Saved: "National ID ••••1234". The full number is never shown again. | ☐ |
| H2 | Save with the name empty, a nonsense ID (too short or with symbols), or no nationality. | A clear message under the field; nothing is saved. | ☐ |
| H3 | Open the screen again and change the name. | "For your safety we never show it again. Enter it again to save changes." | ☐ |
| H4 | Tap "Remove my ID details", then tap again to confirm. | The details are removed; holding a room asks for them again. | ☐ |
| H5 | Hold a room and open the hotel's booking. [Hotel app] | The hotel sees your name, ID type, nationality and only the masked ID; "Show ID number" reveals it (and is recorded). | ☐ |

### Guest app: Paying

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

### Guest app: The hotel answers

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| J1 | After paying, open the hotel app and tap Accept. [Hotel app] | Within seconds the guest booking shows "Booking confirmed" with the timeline step "Confirmed by the hotel" and a View voucher button. [Email] A confirmation email arrives (this is the receipt). [Push] A phone alert says the hotel confirmed. | ☐ |
| J2 | Pay a second booking and Decline it in the hotel app with a reason. [Hotel app] | "The hotel could not confirm: your payment will be refunded." The refund reaches you; [Email] an email says so. | ☐ |
| J3 | Pay a third booking and do nothing for 15 minutes. [Wait] | The booking becomes declined for no answer and you are refunded automatically; the hotel app shows the time ran out. | ☐ |
| J4 | Pay a booking and cancel it yourself before the hotel answers. | Full refund; the hotel's request disappears. | ☐ |
| J5 | Close the guest app while waiting. | You still get the phone alert when the hotel answers. [Push] Tapping it opens that booking. | ☐ |
| J6 | Tap "Notify me" on the waiting screen, allow notifications, then deny them in phone settings and open the card again. | It asks permission once; if denied it says "Notifications are off for this app" with Open settings. | ☐ |

### Coupons: Coupons in detail

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| CP1 | Admin: create TEST10 (10%), FLAT200 (200 birr), BIG50 (50%, at most 300), FIRSTONLY (20%, first booking only). | Each listed with a plain rule. | ☐ |
| CP2 | Create the same code again. | Refused: the code already exists. | ☐ |
| CP3 | Apply each in the app on a 2,000 birr room. | 200 off; 200 off; 300 off (capped); only for a new guest. | ☐ |
| CP4 | Create coupons with a past last day, a future first day, a minimum booking, one total use, one hotel only; use each wrongly. | Each is refused with a message. | ☐ |
| CP5 | Switch a coupon off, try it, switch on, try again. | Refused when off, works when on. | ☐ |
| CP6 | After a paid coupon booking, look at Coupons and at the hotel's earnings. | "Used 1 time", BookDan paid the discount; the hotel's share is unchanged. | ☐ |

### Hotel app: Sign-in and answering requests

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-B1 | Sign in with an email that is not hotel staff. | Told this account is not staff. | ☐ |
| PA-B2 | Sign in as the owner with the emailed code; wrong code first. | Wrong code refused; right code opens Bookings. | ☐ |
| PA-B3 | Look at the counters: To answer, Arriving today, In the hotel. | Numbers match the bookings. | ☐ |
| PA-B4 | When a guest pays, watch the app. | The request appears with a 15-minute countdown (and a phone alert if set up). | ☐ |
| PA-B5 | Open the request. | Dates, room, guests, children, requests, breakfast, non-refundable, paid or pay at hotel, masked ID. | ☐ |
| PA-B6 | Press "Show ID number", then "Hide". | Full number only while shown; the view is logged. | ☐ |
| PA-B7 | Accept a request. | Guest sees Confirmed at once. | ☐ |
| PA-B8 | Decline one for each reason: no rooms, not available, maintenance, other. | Guest is declined and refunded. | ☐ |
| PA-B9 | Let one time out. | "Time is up"; guest refunded automatically. | ☐ |
| PA-B10 | Answer in the app a request already answered in the web portal. | "This booking was already answered." | ☐ |
| PA-B11 | Turn internet off and try to accept. | Told answering needs internet. | ☐ |

**End of day:** passed ____ , failed ____ , skipped ____ . Send me the failed rows.

---

## Day 4: Hotel app, everything else
*Goal: Check-in, rooms, money, reviews, support, settings.* (44 checks)

### Guest app: A confirmed booking

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

### Hotel app: Check-in

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

### Hotel app: Problems with a confirmed booking

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-P1 | As owner, "Cancel this booking". | Asks first; guest refunded in full; hotel bears the refund. | ☐ |
| PA-P2 | As receptionist, look for Cancel. | Not allowed (owner only). | ☐ |
| PA-P3 | "We cannot host this guest": try each reason. | "Waiting for BookDan support"; the guest is told; it shows in admin Support. | ☐ |
| PA-P4 | When a guest opens a dispute, open the booking and send the hotel's account (try under 10 characters first). | Short refused; then sent; payment shown as held. | ☐ |
| PA-P5 | "Get help" on a booking. | A request linked to that booking. | ☐ |

### Hotel app: Rooms tab

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-R1 | Look at the nights: free, booked, being paid, yours. | Numbers match real bookings. | ☐ |
| PA-R2 | Tap a night and add a walk-in with no name, then with a name. | Name required; then added and the free count drops. | ☐ |
| PA-R3 | Block rooms for several nights. | Guest app cannot book them. | ☐ |
| PA-R4 | Block more rooms than are free. | "Not enough free rooms". | ☐ |
| PA-R5 | "Give the rooms back". | Asks first; rooms are on sale again. | ☐ |

### Hotel app: Earnings, statements, reviews

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-E1 | Earnings: check BookDan owes you, After check-in, Paid to you. | Match the admin Payouts page. | ☐ |
| PA-E2 | Open a booking line and recalculate: price, fee 15%, VAT on fee, you receive. | The sum is correct. | ☐ |
| PA-E3 | Look after a pay-at-hotel stay. | Commission owed is shown; "You owe BookDan" when larger than owed to you. | ☐ |
| PA-E4 | Monthly statements: open this month, share it. | Opening and closing balance add up; sharing works. | ☐ |
| PA-E5 | Reviews: publish a reply (under 3 characters first), change it, remove it. | Short refused; reply shows in the guest app and disappears when removed. | ☐ |
| PA-E6 | Sign in as receptionist and open Earnings, Statements, Reviews. | Owner only. | ☐ |

### Hotel app: Support, settings, alerts

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| PA-T1 | Help and support: new request, reply, close. | Appears in admin Support; replies arrive. | ☐ |
| PA-T2 | Settings: change language to Amharic and back. | All screens change. | ☐ |
| PA-T3 | An account with two hotels: switch hotel. | Each shows only its own data. | ☐ |
| PA-T4 | Alerts checklist: allow notifications, check each step. | Each shows Done or To do correctly. | ☐ |
| PA-T5 | "Send a test alert", lock the phone. | Phone rings in about 5 seconds (needs Firebase). | ☐ |
| PA-T6 | "Alerts I want": switch kinds off and on. | Requests always on; others follow your choice. | ☐ |
| PA-T7 | "Apply to list your hotel" link (sign-in screen). | Opens the apply page. | ☐ |
| PA-T8 | Publish an update and reopen twice. | Update banner; Restart loads the new version. | ☐ |
| PA-T9 | Sign out. | Back at sign-in; no data visible. | ☐ |

**End of day:** passed ____ , failed ____ , skipped ____ . Send me the failed rows.

---

## Day 5: Hotel web portal, cancelling and date changes
*Goal: The /hotel website, then guest changes.* (42 checks)

### Hotel web portal: Sign-in and home

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-S1 | Open /hotel, sign in as owner with the code. | Home opens. | ☐ |
| HW-S2 | Check tiles: Waiting for reply, Arriving, Staying, Leaving, BookDan owes you. | Match the hotel app. | ☐ |
| HW-S3 | Look at "Answer these now" and "Next arrivals". | Correct bookings. | ☐ |
| HW-S4 | Sign in as receptionist. | Owner-only pages are hidden. | ☐ |
| HW-S5 | Sign in with an email linked to no hotel. | Clear message. | ☐ |

### Hotel web portal: Bookings and calendar

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-B1 | Bookings: use every filter. | Each lists the right bookings; empty ones say "Nothing here". | ☐ |
| HW-B2 | Accept, Decline (with reason), Check in, No-show, Cancel booking on suitable bookings. | Each asks where risky and updates guest app and admin. | ☐ |
| HW-B3 | Availability: Earlier, Today, Later. | Dates move; colours: green free, amber partly, red sold out. | ☐ |
| HW-B4 | Add a walk-in and an out-of-service block; then Release. | Counts change and return. | ☐ |
| HW-B5 | Block with the end date before the start. | Refused. | ☐ |

### Hotel web portal: My hotel, rooms, photos, team, alerts

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-H1 | My hotel: change description, phone, pin (paste a map link), rules; save. | Live in the guest app at once; admin Hotel changes records it. | ☐ |
| HW-H2 | Rooms and prices: add, edit, remove a room; change a price. | Same behaviour as in the admin portal. | ☐ |
| HW-H3 | Hotel and room photos: upload, cover, delete. | Appear in the guest app. | ☐ |
| HW-H4 | Staff: invite and remove a receptionist. | Works; an owner cannot remove an owner (only an admin can). | ☐ |
| HW-H5 | Booking alerts: change rules and shifts. | Saved; admin shows the same. | ☐ |
| HW-H6 | Look for business or payout details and Verified or Status. | Not editable by the owner. | ☐ |

### Hotel web portal: Money, reviews, help

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| HW-M1 | Earnings: tiles and booking lines, payments list. | Same numbers as app and admin. | ☐ |
| HW-M2 | Statements: choose different months. | Correct stays and balances; empty month says so. | ☐ |
| HW-M3 | Reviews: post a reply, delete it. | Visible then gone in the guest app. | ☐ |
| HW-M4 | Help from BookDan: new question, reply, close conversation. | Seen and answered in admin Support. | ☐ |
| HW-M5 | Use the portal on a phone browser. | Usable without sideways scrolling problems. | ☐ |

### Guest app: Cancelling

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

### Guest app: Changing the dates

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

**End of day:** passed ____ , failed ____ , skipped ____ . Send me the failed rows.

---

## Day 6: Admin portal, running the business
*Goal: Support, money, people and figures.* (45 checks)

### Guest app: After the stay: reviews and problems

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

### Guest app: Help and support

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

### Admin portal: Support, disputes, relocation

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-T1 | Support: open the list. | "Needs an answer" and "Waiting for the guest" are right, newest first. | ☐ |
| AD-T2 | Send an answer; then close another without answering. | Guest gets it in the app and by email. | ☐ |
| AD-T3 | Goodwill refund: BookDan pays; then hotel pays; then try the whole amount. | Booking stands; payer is charged correctly; full amount refused. | ☐ |
| AD-T4 | Relocation: Restore on one; Refund in full on another. | First confirmed again; second refunded, hotel bears it. | ☐ |
| AD-T5 | Dispute: try each decision (reject, full refund hotel pays, full refund BookDan pays, part refund by each). | Refunds and hotel earnings match the choice; asks before settling. | ☐ |

### Admin portal: Users and reviews

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-U1 | Users: search by email, phone, name; filter by status. | Right people; "Nobody matches" when none. | ☐ |
| AD-U2 | Suspend a guest with a reason; try to use that account. | Blocked. | ☐ |
| AD-U3 | Reinstate. | Can sign in and book again. | ☐ |
| AD-U4 | "Sign out everywhere". | All their devices need to sign in again. | ☐ |
| AD-U5 | Reviews: filters and sorting; hide one with a reason; "Show again". | Hidden review and its rating leave the guest app, then return. | ☐ |

### Admin portal: Money

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-M1 | Payment issues: open each "Needs a person" and mark resolved with a note. | Moves to resolved with your name. | ☐ |
| AD-M2 | Payouts: compare To pay now, After check-in, Paid so far. | Only checked-in or finished stays are payable. | ☐ |
| AD-M3 | Record a payout: no reference, too much, then correct. | First two refused; then owed falls by exactly that amount. | ☐ |
| AD-M4 | Show payout history; Download CSV for the month. | History correct; file opens in Excel with right totals. | ☐ |
| AD-M5 | When a hotel owes BookDan, "Record payment received". | Asks to confirm; balance updates. | ☐ |
| AD-M6 | Member tiers: change a discount, try 90%, save, restore. | 90% refused; guest app shows the new level discount. | ☐ |
| AD-M7 | After a guest finishes stays at 2 or more hotels, check their level in the app. | Level and discount shown; not combined with a coupon; online only. | ☐ |
| AD-M8 | Rates: Fetch current rates; type a rate by hand; type a wildly wrong one. | Saved; warning on odd values; guest app shows converted prices. | ☐ |

### Admin portal: Insights and settings

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| AD-I1 | Analytics: 7, 30, 90 days. | Booking value, commission, refunds and counts match your test bookings. | ☐ |
| AD-I2 | Click the "needs a person" items. | Open the right pages. | ☐ |
| AD-I3 | Calls: tap "Call the hotel" in the guest app, then open Calls. | The call is counted for that hotel. | ☐ |
| AD-I4 | Hotel changes: look for the edits made this week. | Who, what and when are listed. | ☐ |
| AD-I5 | Admins: add a support and a finance admin. | Temporary password shown once; they must choose their own. | ☐ |
| AD-I6 | Sign in as support, then as finance. | Each sees only its pages and cannot do the other's actions. | ☐ |
| AD-I7 | Reset a password; remove an authenticator; switch an admin off. | Each takes effect. | ☐ |
| AD-I8 | Try to remove your own super admin rights. | Refused. | ☐ |
| AD-I9 | My account: change password (try a short one first). | Short refused; new one works. | ☐ |
| AD-I10 | Set up the authenticator app, sign in with the code, use one recovery code, make new codes, turn it off. | Each works; a used recovery code fails the second time. | ☐ |

**End of day:** passed ____ , failed ____ , skipped ____ . Send me the failed rows.

---

## Day 7: Account, offline, safety, re-test and sign-off
*Goal: Finish, repeat every failure, decide.* (34 checks)

### Guest app: My bookings and Saved

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| O1 | Open My bookings signed out. | "Sign in to see your bookings" with a Sign in button. | ☐ |
| O2 | Open it signed in with no bookings. | "No bookings yet" and Find a stay. | ☐ |
| O3 | Open My bookings with several bookings and tap each tab: Upcoming, Completed, Cancelled, Failed. | The sliding tabs show a count each; every booking is in the right tab, with hotel, dates, total and status. | ☐ |
| O4 | Pull to refresh; tap a booking. | The list reloads; the booking opens. | ☐ |
| O5 | Saved: tap hearts on three hotels. | They appear on the Saved tab; unsaving removes them. With nothing saved: "No saved stays yet". | ☐ |
| O6 | Sign in on a 2nd phone with the same email. [2nd phone] | The saved hotels from the first phone appear after a few seconds. | ☐ |
| O7 | Turn off internet, remove a saved hotel, turn internet on, wait a minute, check the 2nd phone. | The removal reaches the server and the hotel is gone on the other phone too; it does not come back. | ☐ |

### Guest app: Profile, language, currency and account

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| Q1 | Profile: switch the language between English and Amharic. | Every screen changes (including dates, buttons and messages). Report any text left in English. | ☐ |
| Q2 | Profile: tap the Currency row, or the currency button at the top of the search or home screen. | A Currency screen: a note that you are always charged in birr, Default (Ethiopian Birr), Popular currencies (AED, SAR, QAR, KWD, OMR, BHD, USD, GBP, EUR) and More currencies, with a search icon. Tap USD: you return, and prices show as "about" dollars. Choose ETB again to go back. If the admin has no fresh rate for a currency it is not listed. | ☐ |
| Q3 | Profile: open Help and support. | The support list (section P). | ☐ |
| Q4 | Profile: Delete my account while a stay is still to come. | "You have a booking that is not finished. Cancel it, or wait until the stay is over, then delete your account." | ☐ |
| Q5 | Delete the account with no open bookings. | The account closes; your name, ID number, review text, support words and saved hotels are erased; you are signed out; booking records remain. Signing in again with the same email starts an empty account. | ☐ |
| Q6 | Look at the privacy notes on the details screen and the profile. | "Your ID is stored encrypted ... only the last four characters show." | ☐ |

### Guest app: Offline, errors and edge cases

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

### Guest app: Privacy and safety checks

| ID | What to do | What you should see | P/F/S |
|---|---|---|---|
| S1 | Before booking, search the hotel page for its phone number. | It is not shown; it appears only after the hotel confirms. | ☐ |
| S2 | Look at your ID anywhere in the guest app. | Only the last four characters are ever shown. | ☐ |
| S3 | Sign in as a second guest and open the first guest's booking link (if you have it). | Not found: you cannot see another guest's booking, voucher or support thread. | ☐ |
| S4 | Sign out and press Back. | No private data is shown. | ☐ |
| S5 | Ask for sign-in codes many times from one network (more than 30 in 10 minutes, for different emails). | "Too many codes requested from this connection. Try again in a few minutes." | ☐ |

### Whole system: End to end and money

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

**End of day:** passed ____ , failed ____ , skipped ____ . Send me the failed rows.

---

## Final sign-off
| Question | Yes / No |
|---|---|
| Can a new guest book, pay, stay and review without help? | |
| Can a hotel answer, check in and see its money? | |
| Do all money totals agree? | |
| Are there any open failures that lose money, expose private data or block booking? | |
| Ready for real testers? | |
