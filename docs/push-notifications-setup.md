# Push messages: setup (hotel alerts and guest updates)

Everything in the code is built and tested. What is left is the Firebase account, three secrets, and one new build of each app.

## What gets sent

| When | To | Message | Channel |
|---|---|---|---|
| A guest pays | every hotel phone | "New booking request" (loud, its own sound, 15-minute deadline) | `booking_alerts` |
| The hotel accepts | the guest | "Booking confirmed" (opens the booking and the voucher) | `booking_updates` |
| The hotel declines | the guest | "Booking not available… full refund" | `booking_updates` |
| The hotel does not answer in 15 minutes | the guest | "The hotel did not answer in time… full refund" | `booking_updates` |

The guest app asks for permission only when it has something to wait for: a card on the booking screen ("Get a message when the
hotel answers"). A dead token (app uninstalled) is forgotten automatically.

## 1. Firebase (once, about 15 minutes)

1. Go to <https://console.firebase.google.com>, **Add project** (name it `bookdan`; Analytics is not needed).
2. **Add an Android app** twice, in the same project:
   - package name `et.bookdan.partner`
   - package name `et.bookdan.guest`
   Download each `google-services.json`. Keep them apart. They are not secrets in the cryptographic sense, but do not commit them
   (they are git-ignored).
3. **Project settings, Service accounts, Generate new private key.** This downloads a JSON file. **This one is a real secret.**

## 2. Give the servers and builds their keys

**The server (Render).** Turn the service-account file into one line of text and set two variables:

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("C:\path\to\service-account.json"))
```

On Render, **Environment**: `PUSH_PROVIDER=fcm` and `FCM_SERVICE_ACCOUNT_B64=<the long text>`. Save; Render redeploys.
The server reads the key, signs in to Google by itself and sends through the FCM v1 API. It refuses to start if the key is invalid.

**The apps (EAS).** Store each app's `google-services.json` as a secret file, once per app:

```bash
cd frontend/apps/guest
npx eas-cli@latest env:create --name GOOGLE_SERVICES_JSON --type file --value ./google-services.json --visibility secret --environment preview

cd ../partner
npx eas-cli@latest env:create --name GOOGLE_SERVICES_JSON --type file --value ./google-services.json --visibility secret --environment preview
```

(Put each app's own file in its own folder first. Delete the copies afterwards if you like.) The builds pick it up through
`app.config.js`; without the file the app still builds, but cannot receive messages.

## 3. Build both apps again (a native change, so an update is not enough)

The guest app gained a native module (`expo-notifications`), so its version is now **0.2.0**. Phones that still have the old 0.1.0
install keep working but do not receive new updates, which is intentional: an update that needs the new module would crash them.

```bash
cd frontend/apps/guest   && npx eas-cli@latest build --platform android --profile preview
cd ../partner            && npx eas-cli@latest build --platform android --profile preview
```

Install the new APKs (uninstall the old ones first if Android refuses). After this, updates with `eas update --channel preview`
work again.

## 4. Test it (two phones, or one phone and a browser)

1. Guest phone: sign in, book a room, pay on the test page. On the booking screen tap **Notify me** and allow notifications.
2. Hotel phone: sign in as the owner. It rings when the guest pays. (Settings, Alerts shows a checklist and a test alert.)
3. On the hotel phone tap **Accept**. The guest phone shows "Booking confirmed" even if the app is closed; tapping it opens the booking.
4. Try **Decline**, and a request left unanswered for 15 minutes.

## If a message does not arrive

- Check Render's logs for `push failed`. A message such as `google sign-in for FCM failed` means the service-account key is wrong;
  `fcm send failed (403)` usually means the Firebase Cloud Messaging API is not enabled for the project (Google Cloud console, APIs).
- The phone must be a real Android phone with Google services, running the new build (not Expo Go, not a browser).
- Budget phones may kill background apps: turn off battery saving for the app (the hotel app has a checklist for this).
- iPhone is not supported yet (it needs an Apple developer account and APNs).
