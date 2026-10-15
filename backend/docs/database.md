# Database

PostgreSQL with PostGIS, in the `app` schema. Hosted on Supabase for development and staging; the same
migrations also run on plain PostgreSQL (for example the Docker image in `docker-compose.yml`).

## What is in it

| Migration | Contents |
|---|---|
| `00001_schema_and_helpers` | `app` and `extensions` schemas, PostGIS, helper triggers, locks the schema away from Supabase's public roles |
| `00002_identity` | users, one-time codes, devices (with push tokens), rotating refresh tokens |
| `00003_catalogue` | hotels, staff, landmarks, room types, rooms, rates, holiday boosts, pricing settings |
| `00004_bookings` | nightly inventory, quotes, bookings with the status state machine and history, idempotency keys, reviews |
| `00005_payments_and_ledger` | payments, gateway callbacks, the double-entry money ledger |
| `00006_voucher_sync_audit` | vouchers, offline sync actions, change log, audit log |
| `00007_lock_down` | row level security on every table |

## Rules the database enforces itself

These hold even if application code has a bug. Each one has a test in `internal/dbtest`.

- **No overselling:** `inventory.held + booked <= capacity`. Holds use a guarded `UPDATE`; 25 guests racing for 3 rooms produce exactly 3 winners.
- **Booking status:** only the transitions in `booking_transition_allowed()` are possible; every change writes a history row (who and why via `SET LOCAL app.actor / app.reason`).
- **Price is frozen:** a booking's amounts cannot change after creation, and amounts must add up (`total = base + service + VAT`, `deposit + balance = total`).
- **Ledger:** append-only (no update, delete or truncate), and every transaction's entries must add up to zero per currency, checked at commit.
- **Payments:** at most one open payment per booking, one successful payment per booking, and a duplicate gateway callback is rejected by a unique key.
- **Append-only history:** `booking_events`, `audit_log`, `payment_events`.
- **A room belongs to its own hotel's room type** (composite foreign key).

## Setting up Supabase

1. Create a project at supabase.com. Pick the region closest to your guests (there is no African region; Europe is usually the closest). Save the **database password** somewhere safe; it is shown once.
2. Use a **separate project for development**. Never point a development machine at a project holding real guest data.
3. Open **Project Settings, Database, Connection string** and copy the **Session pooler** string (port 5432). It works over IPv4, which most home networks need. Put your password in it. If the password has special characters (`@ # / ? %`), URL-encode them.
4. Put it in `backend/.env` (this file is git-ignored, and never paste it into chat or commit it):

   ```
   DATABASE_URL=postgresql://postgres.<project-ref>:<PASSWORD>@aws-0-<region>.pooler.supabase.com:5432/postgres?sslmode=require
   ```
5. Create the tables and load the demo hotels:

   ```bash
   cd backend
   go run ./cmd/migrate up      # creates everything (PostGIS is enabled by the first migration)
   go run ./cmd/migrate seed    # demo hotels, rooms and availability (development only)
   go run ./cmd/migrate status  # lists applied migrations
   go run ./cmd/api
   curl http://localhost:8080/v1/readyz   # ready once the database answers
   ```

If the first migration says it cannot create PostGIS, enable it once in the dashboard (Database, Extensions, search "postgis") and run `up` again.

### Things to know about Supabase

- **The app does not use Supabase's own API or auth.** The Go server talks to Postgres directly. In the dashboard, under API settings, you can leave the Data API off. Our tables are in the `app` schema, which that API never exposes, and row level security is on as a second lock.
- **Do not put the `anon` or `service_role` keys in the mobile apps.** They are not needed and the apps never talk to the database.
- **Production:** use a separate project, on a paid plan with daily backups and point-in-time recovery, and set `APP_ENV=production`. `migrate seed` and `migrate down` refuse to run in production.
- **Connection pooling:** the session pooler is fine for development and for a single API server. If you later run many server instances, use the transaction pooler (port 6543) with pgx's `default_query_exec_mode=simple_protocol`.

## Local PostgreSQL instead

`docker compose up -d` starts PostgreSQL 16 with PostGIS on port 5432 (needs Docker Desktop running). Use
`DATABASE_URL=postgres://bookdan:bookdan@localhost:5432/bookdan?sslmode=disable`.

## Open decisions that touch the database

- `pricing_config` holds **placeholder** percentages and `confirmed = false`. The quote code will refuse to price bookings in production until the owner supplies and confirms them (see `design.md`, DECISION 1).
- The refund policy is not in the database yet (DECISION 2).

## Tests

```bash
go test ./...                       # the database tests start an embedded PostgreSQL, no Docker needed
SKIP_DB_TESTS=1 go test ./...       # skip them
TEST_DATABASE_URL=postgres://... go test ./internal/dbtest   # run against a database with real PostGIS
```

The embedded PostgreSQL has no PostGIS, so there the two location columns are swapped for text. Everything else
is the real migration. Run the tests against a real PostGIS database before a release.
