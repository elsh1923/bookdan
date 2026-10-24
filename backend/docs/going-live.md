# Going live: running the real backend and what is left

The mobile apps no longer need the built-in demo. The Go API implements the whole booking journey on PostgreSQL; the apps
talk to it when `EXPO_PUBLIC_API_MODE=http`. Three outside services are still stand-ins (see the checklist at the end).

## 1. Run everything on your computer (nothing to install)

```bash
cd backend
go run ./cmd/devserver
```

First run downloads a private PostgreSQL (about 60 MB, once), creates the tables and loads the demo hotels. Data is kept in
`~/.bookdan-dev`. The API is at `http://localhost:8080/v1/healthz`.

- Guest sign-in: any email or `+2519…` number, code `123456`.
- Hotel sign-in (partner app): `owner@bookdan.test`, code `123456`. Only people registered as hotel staff can sign in there.
- Payments open a **test payment page** served by the API (Approve or Decline). No real money moves.
- Alerts and codes are printed in the server window instead of being sent.

### Point the apps at it

Create `frontend/apps/guest/.env.local` and `frontend/apps/partner/.env.local` (they are git-ignored):

```
EXPO_PUBLIC_API_MODE=http
EXPO_PUBLIC_API_URL=http://localhost:8080
```

Then start the app as usual (`npx expo start --clear`). Delete the file to go back to the built-in demo.

**On a real phone** the URL must be your computer's address on the Wi-Fi, not `localhost`: for example
`EXPO_PUBLIC_API_URL=http://192.168.1.20:8080` (find it with `ipconfig`). Allow port 8080 through the Windows firewall, and set
`PUBLIC_URL=http://192.168.1.20:8080` in `backend/.env` so the test payment page link works from the phone.

## 2. Run it on a real database

Use Supabase or the Docker database in `docker-compose.yml` (these have PostGIS, which turns on "near a landmark" search):

```bash
cp .env.example .env        # put the connection string in DATABASE_URL
go run ./cmd/migrate up
go run ./cmd/migrate seed   # demo hotels, development only
go run ./cmd/api
```

The tests also run on a database with PostGIS: `TEST_DATABASE_URL=postgres://… go test ./...`. The PostGIS code path
(`ST_DWithin`, `ST_X/ST_Y`) has **not** been exercised yet because this computer has no PostGIS: run the tests that way once.

### Real sign-in codes by email

**On Render's free plan use Brevo** (`EMAIL_PROVIDER=brevo`): Render blocks the mail ports (25, 465, 587) there, so Gmail by SMTP times out. Brevo is called over HTTPS, which is never blocked, and its free plan is 300 emails a day with no card. Setup: Brevo, Senders, add and verify the sender address; SMTP & API, API keys, create a **restricted** key that can send transactional email; set `EMAIL_PROVIDER=brevo`, `BREVO_API_KEY` and `EMAIL_FROM` (a verified sender) on the server. Test from your computer: `$env:EMAIL_TEST_TO="you@gmail.com"; go test ./internal/api -run TestBrevoLive -v`.

**Gmail by SMTP** (`EMAIL_PROVIDER=smtp`) works from your computer and on paid Render plans. Set these on the server:

- `EMAIL_PROVIDER=smtp`
- `SMTP_USER`: the sending address, for example a Gmail account made for BookDan
- `SMTP_PASSWORD`: for Gmail an **app password** (turn on 2-step verification, then Google Account, Security, App passwords). Never the normal password.
- `EMAIL_FROM`: for example `BookDan <yourbookdan@gmail.com>` (Gmail only sends as the signed-in address)
- `SMTP_HOST` (default `smtp.gmail.com`) and `SMTP_PORT` (default `587`; `465` works too). Brevo, Mailgun and Amazon SES work with their own SMTP details.

Test the settings from your computer before deploying: `$env:EMAIL_TEST_TO="you@gmail.com"; go test ./internal/api -run TestSMTPLive -v` (reads `backend/.env`).

While `DEMO_MODE=true`, real addresses get a real random code by email, and addresses on `DEMO_ACCOUNT_DOMAINS` (default `bookdan.test`, `example.com`, `example.org`) keep the fixed code `123456` with nothing sent, so the demo hotel owner still works. Phone numbers keep the demo code until an SMS provider exists. If the mail server fails, the app is told (`SEND_FAILED`). Gmail allows roughly 500 messages a day.

### How a hotel gets on BookDan (registered by the platform admin)

1. **Platform admin** opens `/admin` and chooses **Add a hotel** (starts as a draft).
2. Admin adds the hotel's **rooms and prices**, then the **Business** tab: legal name, trade licence, tax number, contact person, and where BookDan pays the hotel. The payout account number is stored encrypted and only its last four digits are shown; showing it in full is recorded in the audit log. A hotel cannot be published without these details.
3. Admin adds the **owner** under **Staff** with their email. The owner gets an **invitation email** (which hotel, how to install the BookDan Partner app, to sign in with that email). Set `PARTNER_APP_URL` to the app download link so the email includes it.
4. The owner opens the hotel app, enters the email and the emailed code. Only people added by the admin get in; anyone else is told the email is not registered with a hotel. Their entry shows **Joined** in the Staff tab (until then **Invited**).
5. **Owners add their own receptionists** in the admin page (they get the same invitation). Owners cannot add or remove owners, and cannot touch other hotels or the business record. Removing someone stops their alerts at once.
6. **Booking alerts** (admin page, tab Booking alerts, owners and admins): choose who is alerted first, set each person's shift and switch alerts off if needed. A request that nobody answers escalates to the owners, then a reminder push and email go out 5 minutes before it is declined and refunded. A person with "No phone" next to their name has not signed in to the hotel app yet. SMS or phone-call backup is not built (needs an Ethiopian SMS provider).
6. The owner can use the admin page for photos, rooms and prices only when `ADMIN_OWNERS=true` and sign-in codes are real (demo accounts are always refused there). A platform admin publishes the hotel when it is ready.

### The admin page (hotels, rooms, prices, photos, staff)

Open `/admin` on the server (for example `https://bookdan.onrender.com/admin`). Set two variables on the server to turn it on:

- `ADMIN_EMAILS`: the platform admins, comma separated.
- `ADMIN_PASSWORD`: at least 16 random characters. Platform admins sign in with their email and this password (8 tries per 10 minutes).

Platform admins manage every hotel: details, rooms and prices, photos and staff. Hotel owners can manage only their own hotels (details, rooms, prices, photos) if you set `ADMIN_OWNERS=true`, but **only turn that on when sign-in codes are real**, because with the fixed demo code anyone could sign in as an owner. Receptionists cannot use the page. A hotel starts as a draft and can be published once it has a room with a price. Photos are resized on upload (1600 px and a 640 px thumbnail) and stored in the database. `PUBLIC_URL` must be the real https address so photo links work in the apps. Every change is written to the audit log.

### Reset the demo data (before an investor demo)

The database refuses to delete money and booking history one row at a time, so test bookings cannot be removed individually. To start clean:

```bash
go run ./cmd/migrate reset          # shows what would be lost, changes nothing
go run ./cmd/migrate reset --yes    # rebuilds the tables and reloads the 8 demo hotels and owner@bookdan.test
```

It refuses to run when `APP_ENV=production` or when any payment came from a real gateway. Phones are signed out afterwards.

## 3. What is real

Sign-in (SMS/email code, rotating refresh tokens that revoke a stolen session), search and hotel details with live availability,
quotes for 1 to 9 rooms, holds that take all rooms on all nights or none, safe retries (Idempotency-Key), payment sessions,
signed payment callbacks that are handled once, a balanced money ledger, hotel accept and decline, automatic closing of expired
holds and unanswered requests with refunds, signed offline vouchers, encrypted national-ID storage with an audited reveal.
Every rule is covered by tests that run on a real PostgreSQL (`go test ./...`).

## 4. Checklist to production

| # | Item | What to do |
|---|------|------------|
| 1 | **Sign-in codes** | **Email: built** (`EMAIL_PROVIDER=smtp`, see below). **SMS: still to build:** write an `OTPSender` in `internal/api/providers.go` for an SMS provider that reaches Ethiopian numbers and set `OTP_PROVIDER`. Until then phone numbers use the fixed demo code and production refuses to start. |
| 2 | **Payments** | Open merchant accounts (Telebirr, Chapa). Write a `Gateway` (create the payment page, refund). Point each gateway's callback at `POST /v1/webhooks/payments/{provider}` and sign it as described in `api/openapi.yaml` (or adapt the check to the gateway's own signature). Set `PAYMENT_PROVIDER` and `PAYMENT_WEBHOOK_SECRET`. |
| 3 | **Push messages** | Built (hotel alerts and guest updates). You need a Firebase project, the key as `FCM_SERVICE_ACCOUNT_B64` with `PUSH_PROVIDER=fcm` on the server, `google-services.json` for both apps, and a new build of each: see `docs/push-notifications-setup.md`. |
| 4 | **Hosting** | Build the image (`docker build -t bookdan-api backend`) and run it on any host that gives you https (Render, Fly.io, Railway, a VPS). Set `APP_ENV=production`, `PUBLIC_URL=https://…`, `DATABASE_URL`, and the secrets (generate with `openssl rand -base64 32`). Run `/migrate up` before each release. |
| 5 | **Point the apps at production** | In each app's `eas.json` build profile add `"env": { "EXPO_PUBLIC_API_MODE": "http", "EXPO_PUBLIC_API_URL": "https://your-api" }`, then build again (`eas build`). An update alone cannot change which server a build talks to unless the value is also set when you run `eas update`. |
| 6 | **Confirm the money rules** | `pricing_config` holds placeholders (no service charge, 15% VAT, full payment online, no commission). Production refuses to price stays until you set the real values and `confirmed = true` (`design.md`, open decisions). |
| 7 | **Hotels and staff** | Built: see "How a hotel gets on BookDan" below. |
| 8 | **ID retention** | Built: a daily job erases the full ID number `ID_RETENTION_DAYS` days after check-out (and 7 days after a booking that never became a stay). It is **off until you set `ID_RETENTION_DAYS`** on the server: decide the period with legal advice (DECISION 5), then set it. |
| 9 | **Abuse limits** | Sign-in codes are limited per identifier. Add per-IP limits at the host or a proxy before launch. |
| 10 | **Not built yet** | Hotel offline tools (rooms chart, walk-ins, QR check-in scanning, statements), reviews, cancellation by the guest, a map view. |

## 5. Security notes

- Secrets live in environment variables only. Production refuses to start without them, with demo mode on, over plain http,
  or with any of the stand-ins selected.
- Tokens, ID numbers and codes are never logged. National IDs are AES-256-GCM encrypted; the key is outside the database.
- A refresh token works once. Reuse revokes the whole session family.
- Payment callbacks are accepted only with a valid HMAC signature and each event id is applied once.
