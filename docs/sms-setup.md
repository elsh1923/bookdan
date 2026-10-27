# Phone (SMS) sign-in: setup

The server can send the sign-in code to an Ethiopian mobile number (+2519... or +2517...) with **Africa's Talking** or **Twilio**, or switch phone sign-in **off** so a launch uses email only. Both senders are written against the providers' documented HTTP APIs and tested against fakes of them. **They have not been run against the real services**: do the check at the end with your own phone before you rely on them.

## Which to choose

- **Africa's Talking** is built for African routes and is usually cheaper to Ethiopian numbers. Check in its dashboard that Ethiopia is enabled for your account and whether a sender id (for example `BookDan`) needs approval; unapproved ids are often replaced by a shared one.
- **Twilio** is well documented and reliable, but delivery to Ethiopian numbers often needs a registered sender and costs more per SMS.
- **Off** (`OTP_PROVIDER=off`): guests and hotel staff sign in with email only. The apps tell anyone who tries a phone number to use email.

Whichever you choose, one SMS carries the code and costs money, so the sign-in rules apply: at most 5 codes per number in 10 minutes, a code lasts 5 minutes, and the per-address request limit still applies.

## Settings (Render environment)

| Provider | Settings |
|---|---|
| Africa's Talking | `OTP_PROVIDER=africastalking`, `AT_USERNAME`, `AT_API_KEY`, optional `SMS_FROM` (approved sender id) |
| Twilio | `OTP_PROVIDER=twilio`, `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`, `SMS_FROM` (a Twilio number such as +1..., an approved alphanumeric id, or a Messaging Service id starting with `MG`) |
| Off | `OTP_PROVIDER=off` |

Production refuses to start with `OTP_PROVIDER=log`, with missing credentials, or with the Africa's Talking sandbox (`AT_SANDBOX=true` or the username `sandbox`).
With a real provider set, phone numbers get random codes (never the public demo code 123456), even if `DEMO_MODE` is on.

## First real SMS (do this once)

1. Set the variables on Render (or in `backend/.env` for a local run with `OTP_PROVIDER` and the credentials) and deploy.
2. In the guest app choose Phone, enter your own number, and tap Send code.
3. The SMS should arrive within seconds: "BookDan: your code is 123456. It expires in 5 minutes. Never share it." Enter it and sign in.
4. If nothing arrives, read the Render log line "sending the sign-in code failed": it holds the provider's reason (wrong key, no balance, sender not approved, country not enabled). The code itself is never logged.
5. Try a number on each Ethiopian network you expect guests to use (Ethio Telecom and Safaricom Ethiopia).

## What is not covered

- No delivery reports: a code the provider accepted but the network dropped looks like a success. The guest can ask for another code.
- No fallback: if the SMS provider is down, phone sign-in fails (email still works).
- Prices and sender-id rules change; check them with the provider before launch.
