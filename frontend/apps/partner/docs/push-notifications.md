# Push alerts for new bookings (partner app)

When a guest pays, the hotel has 15 minutes to answer. The phone must make a loud, recognizable sound even when
it is locked and the app is closed. This is how it works and what is left to set up.

## How it works

```
guest pays -> Go server marks the booking PAID_AWAITING_HOTEL and starts the 15-minute deadline
           -> server sends an FCM message to every phone registered for that hotel
           -> Android shows it on the channel "booking_alerts" (highest importance, its own sound)
           -> staff tap it -> the app opens that booking -> Accept or Decline
```

- The **countdown and the escalation chain** (Telegram, SMS, automated call if nobody answers) run on the server.
  The app only displays the countdown, from the server's clock.
- The phone registers its FCM token with `PUT /v1/partner/devices/push-token` after sign-in, and again whenever
  FCM rotates the token. The server keeps it in `devices.push_token`.
- While the app is open, the alert still shows and still plays the sound.

## What is already built

- The channel `booking_alerts` (importance MAX, vibration, lock-screen visible) with the sound `assets/booking_alert.wav`.
- Permission request at setup, token registration and refresh, tap-to-open (also when the tap starts the app).
- A **settings checklist** (Settings, Alerts): permission, loud channel, registered, and battery saver, each with a word
  as well as an icon, plus a **test alert** that rings in 5 seconds so you can lock the phone and listen.
- The mock server accepts the registration, so the whole app flow works without a backend.

## What alerts need that the browser and Expo Go cannot give

Remote push needs **our own Android build** and **Firebase Cloud Messaging**. Expo Go cannot receive remote push
(since SDK 53) and a browser has no channels. The app says so on the checklist instead of failing silently.

### Steps (you do these once)

1. **Create a Firebase project** at console.firebase.google.com (a separate one for staging and for production).
2. **Add an Android app** with the package name `et.bookdan.partner`. Download `google-services.json`.
3. Put the file at `frontend/apps/partner/google-services.json` and add this to `app.json` under `expo.android`:
   `"googleServicesFile": "./google-services.json"`. It is git-ignored; for EAS builds, store it as an EAS file secret.
4. **Build a development build** (a real app on your phone, with hot reload):
   ```bash
   cd frontend/apps/partner
   npx eas-cli login
   npx eas-cli build:configure
   npx eas-cli build --profile development --platform android
   ```
   Install the APK on the phone, then run `npm run partner` and open it from the development build.
   (Alternative with Android Studio installed: `npx expo run:android`.)
5. **Server side:** in Firebase, Project settings, Service accounts, create a key for the HTTP v1 API. The Go server
   uses it to send messages. Keep it in the server's secret store, never in the app or the repository.

### The message the server sends

```json
{
  "message": {
    "token": "<the phone's FCM token>",
    "data": { "type": "new_booking", "booking_id": "<id>" },
    "android": {
      "priority": "high",
      "notification": { "title": "New booking", "body": "Answer within 15 minutes", "channel_id": "booking_alerts", "sound": "booking_alert" }
    }
  }
}
```

`high` priority is what lets it wake a sleeping phone. The `data` block is what the app reads to open the booking.
[verify] the exact `sound` name against the current Expo and FCM documentation when you build.

## Testing (acceptance test M8: alert on a locked low-end phone)

1. Install the development build, sign in, allow notifications when asked.
2. Open Settings, Alerts. Every row except battery should say **Done**. Work through any that say **To do**.
3. Tap **Send a test alert**, lock the phone, put it in your pocket. Within 5 seconds it should ring with the
   booking sound and show on the lock screen. Tap it: the booking opens.
4. Then repeat with a real push: Firebase console, Messaging, send a test message to the phone's token with the
   data fields above, and with the app fully closed.
5. Do this on the cheapest Android phone you have. Budget phones are where alerts get silenced.

## Phones that silence background apps

Xiaomi, Tecno, Infinix, Samsung and others stop apps to save battery. If alerts are late or missing:
- Settings, Battery: set this app to **Unrestricted** (the checklist button opens the page).
- Allow **autostart** and **lock the app in recent apps** where the phone has those options.
- Some Android versions need an extra permission for a full-screen alert on a ringing phone [verify].

The server's escalation chain (SMS, then an automated call) is the safety net when a push is missed.
