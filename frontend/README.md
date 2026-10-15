# BookDan mobile (React Native + Expo)

Monorepo for the Guest and Partner apps. The Go backend lives in `../backend`.

```
apps/guest        Guest app (Expo Router)
apps/partner      Partner app for hotel staff (Expo Router): sign-in, booking inbox with answer countdown
packages/domain   Money (integer santim), dates, Ethiopian calendar, Zod schemas
packages/i18n     en, am (om, ti later)
packages/ui       Theme and shared components
packages/api-client  Api interface + mock; swapped for the generated client later
```

## Run

Run these from this `frontend/` folder.

```bash
npm install
npm run guest        # guest app
npm run partner      # partner app (hotel staff)
npm test             # domain unit tests
npm run typecheck
```

The guest app currently talks to a **mock API** that follows the contract in `../backend/api/openapi.yaml`
(`apps/guest/app.json` -> `extra.apiMode`). Every simulated screen shows a SIMULATED label.

## Progress against the spec (section 17)

- E0 Foundations: monorepo, theme, i18n (en/am), Ethiopic font, domain package with tests
- E1 Discovery: welcome, search, results (filters, pagination, stale label), property page
- E2 Booking: sign-in (phone or email code), quote, hold with server-time countdown and idempotency key, payment hand-off with polling and recovery after the app is killed, silent session refresh with the checkout draft kept
- E3 Voucher: downloaded once, saved encrypted on the phone, opens offline; removed after the stay and on sign-out
- Partner E4 (started): staff sign-in, booking inbox sorted by deadline with a server-clock countdown, accept, decline with a required reason, missed deadlines closed automatically, session refresh. Push alerts for new bookings are built (channel, sound, token registration, tap to open, settings checklist, test alert); they need a development build and a Firebase project, see apps/partner/docs/push-notifications.md. Next: rooms tape chart, walk-ins, offline outbox and sync, QR check-in
- Offline: search and property pages are cached to the phone (public data only)
- Waiting on decisions or accounts: full price breakdown (money rules), cancel with refund, map view (native build), Sentry, Afaan Oromoo and Tigrinya (native reviewers), real photos
