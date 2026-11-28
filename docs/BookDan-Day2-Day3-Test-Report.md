# BookDan: Day 2 and Day 3 test report

**Date:** 28 November 2026
**Written for:** non-technical readers
**What was tested:** the admin portal (adding and managing hotels) and the hotel application page.

## Update: all 5 problems fixed and re-tested

The same day, all five problems below were fixed, and the 11 failed checks were repeated on the local test server. **All 57 checks now pass** (those marked * with the same simulation limits). All automated server tests pass. The fixes reach the live website only after the code is pushed.

| # | Problem | What changed | Re-test |
|---|---|---|---|
| 1 | Owner could be turned into a receptionist | Adding someone already on the team is refused with an explanation. To change a role, remove them and add them again. A mistyped email now gets a clear message. | X14 passes |
| 2 | Pages too wide on a phone | Fixed in both portals. All 16 admin menu pages tried, the 6 hotel tabs and the 12 hotel-portal pages fit a 375-pixel phone screen. Wide tables scroll inside their own box. | Z3, Z5 to Z10 pass |
| 3 | "Saved" message vanished | A green message now stays for 4 seconds after saving, in both portals. | X10 passes |
| 4 | Account viewing not shown | "Hotel changes" lists "viewed the full payout account number" (untick the box at the top to see staff actions). | AD-P8 passes |
| 5 | Same application sent twice | The same hotel from the same email is refused while it waits. A different hotel is still accepted. | AP3 passes |

*The sections below describe the first test run, before these fixes.*

## 1. Summary

| Day | Checks | Passed | Failed |
|---|---|---|---|
| Day 2: adding hotels and their map location | 19 | 17 | 2 |
| Day 3: rooms, photos, staff, alerts, business, publishing, applications | 38 | 29 | 9 |
| **Total** | **57** | **46** | **11** |

The 11 failures come from **5 problems**. Nothing crashed, and the server reported no internal errors.

### Where this was tested
- On the **local test server** on this computer, with made-up hotels. **The live website was not signed in to or changed.**
- The local server runs the latest code, including the "Pay at hotel" fix, which was **not yet on the live site** when tested.
- Checks were done by operating the web pages in a browser. Some actions of phones and guests were imitated by sending the same requests to the server.

### Simulated or partly checked (marked * below)
| Check | What was simulated |
|---|---|
| AD-H8 current location | The browser's location answer was imitated, both "allow" and "refuse" |
| AD-P3 invitations | Emails were prepared and recorded, but not delivered (the test server sends no real email) |
| AD-P4, Y3 staff sign-in and phone | Sign-in and phone registration were imitated without a real phone |
| What guests see | Checked from the server's answers, not on a phone screen |

## 2. Problems found

| # | Problem | How serious | Checks affected |
|---|---|---|---|
| 1 | **A hotel owner can be turned into a receptionist by mistake.** Typing an owner's email again with the role "Receptionist" changes their role with no warning or message. In the test, the hotel was left with **no owner**. Adding the same person twice also gives no message. | **High** | X14 |
| 2 | **Hotel pages are too wide on a phone.** On a phone-size screen, the hotel list and hotel pages must be scrolled sideways or appear shrunk. A one-line style change was tried in the browser and corrected the pages tried (details, rooms, alerts). It is not yet applied. | Medium | Z3, Z5, Z6, Z7, Z8, Z9, Z10 |
| 3 | **No "Saved" confirmation.** Changes are saved, but the message disappears instantly, so the user cannot tell. | Medium | X10 (also seen on alert rules and publishing) |
| 4 | **Viewing a bank account number is not shown in "Hotel changes".** The screen says the view was logged, and the system does record it, but staff cannot see that record on the page. | Medium | AD-P8 |
| 5 | **The same hotel application can be sent twice.** A third one from the same email is refused. This limit is built in, but it allows an exact duplicate. | Low | AP3 |

### Other things noticed (not counted as failures)
- A hotel created from an application gets a **default map pin in central Addis Ababa**, even for a hotel in Adama. Correct the pin before publishing.
- A hotel created from an application is **not ticked "Verified"**. Tick it yourself.
- An invalid email in "Add a person" shows a message about a **phone number**, which is confusing.
- "Hotel changes" shows only changes made by hotels until you untick the box at the top.

## 3. Features and functions tested, in plain words

### Adding a hotel
| ID | Feature | What it does | Result |
|---|---|---|---|
| X8 | Examples in boxes | Grey examples and hints show what to type | Pass |
| AD-H12 | Verified badge | New hotels start ticked as verified | Pass |
| AD-H2 | Required name | A hotel cannot be created without a name | Pass |
| X9 | Length limits | Name stops at 120 characters, description at 2,000 | Pass |
| AD-H3 | Create as draft | New hotels are hidden from guests until published. A double-click creates only one | Pass |
| AD-H11 | Pay at hotel switch | Chosen per hotel and remembered | Pass |
| AD-H10 | Amharic text, directions, rules, times, features | All are saved | Pass |
| X10 | Edit and save | Changes are kept, but no confirmation shows | **Fail** |
| X11 | Leave without saving | Unsaved changes are thrown away | Pass |
| Z3 | General page check | Too wide on a phone | **Fail** |

### The location map
| ID | Feature | What it does | Result |
|---|---|---|---|
| X12 | Zoom and move | The map fills its box | Pass |
| AD-H4 | Click and drag the pin | Sets the hotel's position | Pass |
| AD-H6 | Paste numbers | Moves the pin to those coordinates | Pass |
| AD-H9 | Type numbers | The pin follows | Pass |
| AD-H7 | Wrong text or non-Google link | Red message, pin stays | Pass |
| AD-H5 | Google Maps short link | A real short link placed the pin | Pass |
| AD-H8 | Use my current location | Places the pin, or explains when refused | Pass* |
| X13 | Saved position | Same place after reopening | Pass |
| Z4 | General map check | Map draws fully on a narrow screen and after reload | Pass |

### Rooms and prices
| ID | Feature | What it does | Result |
|---|---|---|---|
| AD-R1 | Add a room | Name, guests, number of rooms, price, breakfast, no-refund discount | Pass |
| AD-R2 | Limits | Refuses price under 100 birr, over 20 guests, discount over 50% | Pass |
| AD-R3 | Words in number boxes | Refused with an explanation | Pass |
| AD-R4 | Optional extras | Without them, guests see only the normal price | Pass |
| AD-R5 | Change a room | New price used in new guest quotes | Pass |
| AD-R6 | Protect bookings | Cannot cut room numbers below those booked | Pass |
| AD-R7 | Room photos | Upload, reorder, delete, at most 10 | Pass |
| AD-R8 | Remove a room | Unused rooms can be removed, booked rooms cannot | Pass |
| Z5 | General check | Too wide on a phone | **Fail** |

### Hotel photos
| ID | Feature | What it does | Result |
|---|---|---|---|
| AD-P1 | Upload, cover, delete | The chosen cover is the picture guests see first | Pass |
| AD-P2 | Bad files | Refuses files over 10 MB and non-pictures | Pass |
| Z6 | General check | Too wide on a phone | **Fail** |

### Staff
| ID | Feature | What it does | Result |
|---|---|---|---|
| AD-P3 | Invite owner and receptionist | Listed as "Invited", invitation prepared | Pass* |
| AD-P4 | After sign-in | Shows "Joined" with the role | Pass* |
| AD-P5 | Remove a receptionist | They lose access at once | Pass |
| X14 | Duplicate or invalid entry | Invalid refused. Duplicate and role change accepted silently | **Fail** |
| Z7 | General check | Too wide on a phone | **Fail** |

### Booking alerts
| ID | Feature | What it does | Result |
|---|---|---|---|
| AD-P6 | Alert rules and shifts | Everyone at once or receptionists first; shifts and on/off saved | Pass |
| Y1 | Waiting time limits | 0 and 20 minutes are refused by the browser | Pass |
| Y2 | Past shift | Saved and shown | Pass |
| Y3 | Phone status | "Signed in" or "No phone", and which alerts are off | Pass* |
| Z8 | General check | Too wide on a phone | **Fail** |

### Business and payment details
| ID | Feature | What it does | Result |
|---|---|---|---|
| AD-P7 | Save business details | Missing items refused; only the last 4 account digits show | Pass |
| AD-P8 | Show full account number | Asks first and shows it, but the record is not visible in "Hotel changes" | **Fail** |
| X15 | Payment methods | Bank, Telebirr and CBE Birr appear correctly under Payouts once a hotel has paid bookings | Pass |
| Z9 | General check | Too wide on a phone | **Fail** |

### Publishing and hiding
| ID | Feature | What it does | Result |
|---|---|---|---|
| AD-P9 | Safety before publishing | Refused without business details or a priced room | Pass |
| AD-P10 | Publish | All three test hotels appeared in guest search | Pass |
| AD-P11 | Hide a booked hotel | Warns, names the booking, hides only after "yes"; the booking remains | Pass |
| Z10 | General check | Too wide on a phone | **Fail** |

### Hotel application page (public)
| ID | Feature | What it does | Result |
|---|---|---|---|
| AP1 | Empty form | Points to the missing boxes | Pass |
| AP2 | Send an application | Thank-you message with a reference number | Pass |
| AP3 | Send again | The identical application was accepted a second time | **Fail** |
| X16 | Phone screen | Fits and is easy to type in | Pass |

### Handling applications
| ID | Feature | What it does | Result |
|---|---|---|---|
| AP4 | Read an application | All details are shown | Pass |
| AP5 | Reject | A reason is required and recorded; an email to the applicant is prepared | Pass |
| AP6 | Approve | Creates a draft hotel and invites the owner | Pass |
| Z11 | General check | Fits a phone screen | Pass |

### Also confirmed along the way
- Only the hotel with the switch on shows the **"Pay at hotel"** label to guests.
- A hotel with **one room** shows as sold out after one booking, and a second booking is refused.
- Amharic text, check-in times and house rules reach the guest side.

## 4. Test data left on the local server
Test hotels starting with "QA", test staff, a test guest, test bookings and test applications. They exist only on this computer.

## 5. Recommended next steps
1. Fix problem 1 (staff roles) before real hotels are added.
2. Fix problems 2, 3 and 4. They are small changes.
3. Decide whether problem 5 matters.
4. Repeat the failed checks, then repeat Days 2 and 3 quickly on the live site yourself.
5. Continue with Day 4, the guest app, which needs a real phone.
