# Installable demo apps for investors (Android)

Both apps can be built as normal Android apps (an APK file) that anyone can install on their phone from a link.
The build happens in Expo's cloud, so nothing needs installing on this PC beyond what you already have.

## 1. One-time setup (about 10 minutes)

1. Create a free Expo account at expo.dev (you do this yourself, with your own email and password).
2. In a terminal:
   ```bash
   npx eas-cli login          # sign in with the account you just made
   ```

## 2. Build each app (about 15 to 30 minutes each, mostly waiting in Expo's queue)

Run these from the `frontend/apps` folders. The first time in each app, `eas init` links it to your Expo account
(say yes when it asks to create the project).

```bash
cd frontend/apps/guest
npx eas-cli init
npx eas-cli build --platform android --profile preview

cd ../partner
npx eas-cli init
npx eas-cli build --platform android --profile preview
```

When a build finishes, the terminal prints a **link** and a QR code. That link opens an install page on expo.dev.
Send each link to the investor. The two apps are separate links: **BookDan** (guests) and **BookDan Partner** (hotels).

If the build asks about Android credentials (a signing key), choose **Generate new keystore** and let Expo keep it.
Back the key up later before any real release (losing it blocks updates).

## 3. How the investor installs it

1. Open the link on an **Android** phone.
2. Download the APK. Android will say it is from an unknown source: tap **Settings** and allow installing from that
   source once. This is normal for apps shared outside the Play Store.
3. Open the app.

**iPhone:** an iPhone cannot install an APK. It needs an Apple Developer account (about $99 a year) and a
TestFlight invitation. If your investor uses an iPhone, either lend them an Android phone for the demo, or tell me
and I will set up the iOS build once you have the account.

## 4. What the investor will see, and what to tell them

These are real apps with a real interface, but **the data is simulated**:

- Hotels, prices and bookings come from built-in demo data, not a server. A badge on the screens says **SIMULATED**.
- No real SMS or email is sent (the sign-in code is always **123456**) and no real money moves.
- The guest app and the partner app do **not** talk to each other yet. A booking made in the guest app will not
  appear in the partner app. Say this upfront: "the apps are finished, the server that connects them is next."

What genuinely works on the phone: browsing and search, hotel pages, sign-in, the 15-minute hold with countdown, the
simulated payment (including a declined payment and recovery after the app is closed), the confirmed booking with
its QR voucher **that opens with no internet**, Amharic and English, and on the partner side the inbox with the
answer countdown, accept, decline with a reason, and missed-deadline handling.

## 5. A 6-minute demo script

**Guest app (BookDan)**
1. Open it, pick a language, tap **Get started**. Show the home screen, then switch to Amharic in Profile.
2. Search a landmark ("Meskel Square"), open a hotel, tap **Select room**.
3. Sign in (any number, code `123456`). Checkout asks for your ID first: tap **Add my ID**, enter a name and any ID number (the app never shows it again, only its last four characters). Then **Hold this room** and point at the 15-minute countdown.
4. Choose Telebirr, pay. On the payment page tap **Decline** to show the failure message, then retry and **Approve**.
5. Wait about 8 seconds: **Booking confirmed**. Open **View voucher** and show the QR and the hidden PIN.
6. **Turn on airplane mode** and reopen the voucher. It still works. This is a strong moment: hotels have weak signal.

**Partner app (BookDan Partner)**
1. Sign in (code `123456`). Show the inbox: bookings waiting for an answer, closest deadline first, with countdowns.
2. Settings, **Add a test booking**, go back to Bookings: a new one arrives with a fresh 15 minutes.
3. Tap the guest line on a booking: the **Guest profile** shows name, contact, nationality and the ID masked. **Show ID number** reveals it (in production this is logged), **Hide** clears it.
4. **Accept** one. **Decline** another and show that a reason is required and that the guest is refunded.
5. Settings, **Alerts**: show the checklist, tap **Send a test alert**, lock the phone. It rings with the booking
   sound. Tap it and the booking opens. (This local test works without Firebase. Real alerts from a server need the
   Firebase setup in `apps/partner/docs/push-notifications.md`.)

## 6. Before you send it

- Install both on a real phone and run the script once yourself. The browser preview is not the real app.
- Make sure the phone's clock and time zone are right.
- The build is a **preview**, not for the Play Store. Nothing here is a launch version.
- Do not share the Expo login. Share only the install links.

## Updating the demo later (no reinstall)

Both apps are set up for over-the-air updates. After the investor has installed a build made with this setup,
you can change screens, text, colors, translations and logic and send it to their phone in about a minute:

```bash
cd C:\Users\HP\Desktop\bookDan\frontend\apps\guest
npx eas-cli@latest update --channel preview --message "what changed"

cd ..\partner
npx eas-cli@latest update --channel preview --message "what changed"
```

- The next time the investor opens the app (or comes back to it), the update downloads quietly. A small bar says
  **"A new version is ready"** and **Restart** applies it. The app never restarts by itself, so it cannot interrupt
  a payment or a hotel answering a booking.
- An update only reaches builds with the **same app version** (`version` in `app.json`, now `0.1.0`).
- You need a **new build** (a new link) only when something native changes: a new library, a new permission, the
  app icon or name, or when you raise the app `version`. If you change the version, rebuild and send the new link.
- Test every update on your own phone first. An update goes straight to everyone who has the app.
- Updates only work in builds made after this setup. A build made earlier cannot receive them.
