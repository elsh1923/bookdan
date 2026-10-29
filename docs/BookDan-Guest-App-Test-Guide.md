# BookDan Guest App: test guide

Work through the tables in order. Tick the box when what you see matches. Write down anything that does not.


# Before you start


## What you need

- The guest app installed from the new build (new logo), and a stable internet connection. A second phone is useful for the "saved hotels on a new phone" and "stay signed in on two phones" checks.
- A real email address you can open (the sign-in code is emailed). A second email address for the "another guest" checks.
- The partner (hotel) app signed in as the owner of your test hotel. Many guest scenarios need the hotel to accept, decline or check a guest in, so keep it ready.
- The admin page (https://bookdan.onrender.com/admin) signed in as a super admin, for coupons, exchange rates and to read what the system recorded.
- At least two published hotels, one with several room types, photos, breakfast and non-refundable options set, and "Pay at hotel" switched on. One hotel with a room type that has only one room helps the "sold out" checks.
- A coupon created in the admin page (for example TEST10, 10%, online payment) and USD and EUR rates entered in the Rates tab.
- Chapa in test mode: pay with the test details Chapa shows on its payment page. If your server uses live Chapa keys, every online payment is real money: use the smallest room price and refund yourself afterwards.

## How to read the tables

- Each row is one thing to try. "What to do" is what you tap or change; "What you should see" is the correct result.
- Tags in brackets say what else a row needs: [Hotel app] the partner app, [Admin] the admin page, [2nd phone], [Email] check your inbox, [Wait] time must pass, [Push] needs phone alerts (Firebase) to be set up.
- Money rules to remember: a stay more than 48 hours away is refunded in full when cancelled; inside 48 hours there is no refund; a non-refundable rate is never refunded once the hotel confirms; the hotel has 15 minutes to answer; a held room is released after 15 minutes if unpaid.
- To test "more than 48 hours" use dates 5 or more days ahead. To test "inside 48 hours" use tomorrow or today.
Time-based rules (48 hours, 15 minutes, 14 days) are the hardest to check. Where a row says [Wait], plan for it: for example start the 15-minute checks first and do other rows while the clock runs.


# A. First launch, language, currency and updates


| # | What to do | What you should see | Pass |
|---|---|---|---|
| A1 | Open the app for the first time (or clear its data). | A short loading screen with the logo, then the welcome screen: "Find a verified stay in Ethiopia", language and currency choices and Get started. | ☐ |
| A2 | Choose Amharic, then Get started. | The app switches to Amharic. Dates show Ethiopian dates under the calendar days (see D4). | ☐ |
| A3 | On the welcome screen tap the Currency row and choose a currency (for example USD). | The Currency screen opens; after choosing you return to the welcome screen showing the choice. Prices later show as approximate amounts, with the exact birr amount next to anything charged. | ☐ |
| A4 | Kill the app and reopen it. | It opens straight to the home screen (no welcome again), in the language you chose. | ☐ |
| A5 | Publish a new update (eas update) while the app is open in the background, then reopen the app twice. | A banner "A new version is ready" with Restart; Restart opens the new version. | ☐ |
| A6 | Open the app with airplane mode on. | The app opens; lists show "Showing saved results" or an offline message with Try again; nothing crashes. | ☐ |


# B. Sign-in and sessions


| # | What to do | What you should see | Pass |
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


# C. Home screen


| # | What to do | What you should see | Pass |
|---|---|---|---|
| C1 | Open Home. | Navy header with the logo, "Find your perfect stay", and a floating search card (place, dates, guests, Search). | ☐ |
| C2 | Tap the category pills (All, Hotel, Guesthouse, Lodge, Resort). | The lists below filter to that kind of stay. | ☐ |
| C3 | Look at the banner "Book now, pay at the hotel". | It is shown as information (it is not a button). To use it, search and look for hotels with the "Pay at hotel" feature (filter by amenity). | ☐ |
| C4 | Scroll the home carousels (Featured, Top-rated stays, Popular hotels, Popular guesthouses, Places to stay in a city). | Cards with photo, name, area, price per night, rating and a heart. Tap a card to open the hotel. | ☐ |
| C5 | Pull down to refresh and tap "Show others". | The lists reload and the featured hotels change. | ☐ |
| C6 | Open two hotels, return to Home. | "Recently viewed" lists them, newest first. | ☐ |
| C7 | With a booking made, look at "My bookings" on Home. | Your latest bookings show with status and total; "See all" opens the My bookings tab. | ☐ |


# D. Searching


| # | What to do | What you should see | Pass |
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


# E. Results, filters and map


| # | What to do | What you should see | Pass |
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


# F. The hotel page


| # | What to do | What you should see | Pass |
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


# G. Checkout and holding a room


| # | What to do | What you should see | Pass |
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


# H. Your details and ID


| # | What to do | What you should see | Pass |
|---|---|---|---|
| H1 | Profile, then "My details and ID". Fill name, ID type, ID number and nationality, then Save. | Saved: "National ID ••••1234". The full number is never shown again. | ☐ |
| H2 | Save with the name empty, a nonsense ID (too short or with symbols), or no nationality. | A clear message under the field; nothing is saved. | ☐ |
| H3 | Open the screen again and change the name. | "For your safety we never show it again. Enter it again to save changes." | ☐ |
| H4 | Tap "Remove my ID details", then tap again to confirm. | The details are removed; holding a room asks for them again. | ☐ |
| H5 | Hold a room and open the hotel's booking. [Hotel app] | The hotel sees your name, ID type, nationality and only the masked ID; "Show ID number" reveals it (and is recorded). | ☐ |


# I. Paying


| # | What to do | What you should see | Pass |
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


# J. The hotel answers


| # | What to do | What you should see | Pass |
|---|---|---|---|
| J1 | After paying, open the hotel app and tap Accept. [Hotel app] | Within seconds the guest booking shows "Booking confirmed" with the timeline step "Confirmed by the hotel" and a View voucher button. [Email] A confirmation email arrives (this is the receipt). [Push] A phone alert says the hotel confirmed. | ☐ |
| J2 | Pay a second booking and Decline it in the hotel app with a reason. [Hotel app] | "The hotel could not confirm: your payment will be refunded." The refund reaches you; [Email] an email says so. | ☐ |
| J3 | Pay a third booking and do nothing for 15 minutes. [Wait] | The booking becomes declined for no answer and you are refunded automatically; the hotel app shows the time ran out. | ☐ |
| J4 | Pay a booking and cancel it yourself before the hotel answers. | Full refund; the hotel's request disappears. | ☐ |
| J5 | Close the guest app while waiting. | You still get the phone alert when the hotel answers. [Push] Tapping it opens that booking. | ☐ |
| J6 | Tap "Notify me" on the waiting screen, allow notifications, then deny them in phone settings and open the card again. | It asks permission once; if denied it says "Notifications are off for this app" with Open settings. | ☐ |


# K. A confirmed booking


| # | What to do | What you should see | Pass |
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


# L. Cancelling


| # | What to do | What you should see | Pass |
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


# M. Changing the dates


| # | What to do | What you should see | Pass |
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


# N. After the stay: reviews and problems


| # | What to do | What you should see | Pass |
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


# O. My bookings and Saved


| # | What to do | What you should see | Pass |
|---|---|---|---|
| O1 | Open My bookings signed out. | "Sign in to see your bookings" with a Sign in button. | ☐ |
| O2 | Open it signed in with no bookings. | "No bookings yet" and Find a stay. | ☐ |
| O3 | Open it with several bookings. | Upcoming and Past stays; each card shows hotel, dates, total and a status badge (Held, Waiting for hotel, Confirmed, Checked in, Completed, Expired, Declined, Cancelled, Needs a new hotel, Under review). | ☐ |
| O4 | Pull to refresh; tap a booking. | The list reloads; the booking opens. | ☐ |
| O5 | Saved: tap hearts on three hotels. | They appear on the Saved tab; unsaving removes them. With nothing saved: "No saved stays yet". | ☐ |
| O6 | Sign in on a 2nd phone with the same email. [2nd phone] | The saved hotels from the first phone appear after a few seconds. | ☐ |
| O7 | Turn off internet, remove a saved hotel, turn internet on, wait a minute, check the 2nd phone. | The removal reaches the server and the hotel is gone on the other phone too; it does not come back. | ☐ |


# P. Help and support


| # | What to do | What you should see | Pass |
|---|---|---|---|
| P1 | Profile, then Help and support, then New request. Choose Question, write a subject and a message, Send. | "Sent. We answer here and by email." The request shows "Waiting for support". | ☐ |
| P2 | Send with a subject under 3 characters or an empty message. | A message says what is missing. | ☐ |
| P3 | Answer the request in the admin page (Support). [Admin] | The thread shows "Support answered". [Email] [Push] An email and a phone alert arrive; tapping the alert opens the thread. | ☐ |
| P4 | Reply in the thread. | Your reply appears and the status returns to waiting for support. | ☐ |
| P5 | Close the request. | "This request is closed. Start a new one if you still need help." You cannot reply. | ☐ |
| P6 | Open five requests without closing any, then a sixth. | "You have several requests waiting for an answer. Wait for a reply, or close one you no longer need." | ☐ |
| P7 | On a booking tap "Get help with this booking". | A new request already linked to that booking ("About booking BD-..."). | ☐ |
| P8 | Ask an admin for a goodwill partial refund on a confirmed booking. [Admin] | The booking shows "Already refunded to you" and the amount arrives. | ☐ |


# Q. Profile, language, currency and account


| # | What to do | What you should see | Pass |
|---|---|---|---|
| Q1 | Profile: switch the language between English and Amharic. | Every screen changes (including dates, buttons and messages). Report any text left in English. | ☐ |
| Q2 | Profile: tap the Currency row, or the currency button at the top of the search or home screen. | A Currency screen: a note that you are always charged in birr, Default (Ethiopian Birr), Popular currencies (AED, SAR, QAR, KWD, OMR, BHD, USD, GBP, EUR) and More currencies, with a search icon. Tap USD: you return, and prices show as "about" dollars. Choose ETB again to go back. If the admin has no fresh rate for a currency it is not listed. | ☐ |
| Q3 | Profile: open Help and support. | The support list (section P). | ☐ |
| Q4 | Profile: Delete my account while a stay is still to come. | "You have a booking that is not finished. Cancel it, or wait until the stay is over, then delete your account." | ☐ |
| Q5 | Delete the account with no open bookings. | The account closes; your name, ID number, review text, support words and saved hotels are erased; you are signed out; booking records remain. Signing in again with the same email starts an empty account. | ☐ |
| Q6 | Look at the privacy notes on the details screen and the profile. | "Your ID is stored encrypted ... only the last four characters show." | ☐ |


# R. Offline, errors and edge cases


| # | What to do | What you should see | Pass |
|---|---|---|---|
| R1 | Turn airplane mode on while on a hotel page, then tap Hold. | A clear "No internet connection" message with Try again; no double booking afterwards. | ☐ |
| R2 | Turn internet off during payment, then on again. | The booking screen keeps polling and shows the real result when the connection returns. | ☐ |
| R3 | Open an old link or booking that does not exist (use a deleted account's booking). | A "something went wrong" or not-found screen with a way back; no crash. | ☐ |
| R4 | Rotate the phone and use the largest text size in phone settings. | Screens remain usable; nothing is cut off. | ☐ |
| R5 | Search for a stay starting today after 12:00. | Booking is possible; cancelling follows the same-day rule (L4). | ☐ |
| R6 | Book exactly 30 nights, then 31. | 30 works; 31 is refused with the 30-night message. | ☐ |
| R7 | Use the Android back button on every screen. | It goes to the previous screen; from checkout it does not lose the held room. | ☐ |
| R8 | Kill the server connection (airplane mode) while the confirmed booking is open. | The booking and voucher are still readable offline. | ☐ |


# S. Privacy and safety checks


| # | What to do | What you should see | Pass |
|---|---|---|---|
| S1 | Before booking, search the hotel page for its phone number. | It is not shown; it appears only after the hotel confirms. | ☐ |
| S2 | Look at your ID anywhere in the guest app. | Only the last four characters are ever shown. | ☐ |
| S3 | Sign in as a second guest and open the first guest's booking link (if you have it). | Not found: you cannot see another guest's booking, voucher or support thread. | ☐ |
| S4 | Sign out and press Back. | No private data is shown. | ☐ |
| S5 | Ask for sign-in codes many times from one network (more than 30 in 10 minutes, for different emails). | "Too many codes requested from this connection. Try again in a few minutes." | ☐ |


# What to send back to me

For every row that does not pass, send: the row number, what you did, what you saw (a screenshot helps), which phone and Android version, and the time it happened (so the server logs can be read). Also tell me which rows you could not test and why.


## Things that cannot be fully tested yet

- Phone alerts (hotel answers, support answers, dispute updates) need Firebase to be connected and a build made with it.
- Phone (SMS) sign-in needs a text-message provider account; until then it answers "not available" and email sign-in is the way in.
- Real money: Chapa live payments and refunds, including partial refunds, have not been tried with real money.
- iPhone: there is no iPhone build yet.

_Total checks in this guide: 163._

Total checks in this guide: 163.
