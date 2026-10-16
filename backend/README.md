# BookDan backend (Go)

HTTP API for the Guest and Partner mobile apps. The contract lives in [`api/openapi.yaml`](api/openapi.yaml).
Data model, security and build order: [`docs/design.md`](docs/design.md). The database: [`docs/database.md`](docs/database.md).

Stack (from the mobile and backend spec): Go, chi, pgx, sqlc, goose, River, PostgreSQL with PostGIS (Supabase).

## Status

- **The booking journey is real.** Sign-in, search, quotes (1 to 9 rooms), holds, payment sessions and callbacks, hotel
  accept and decline, expiry and refunds, signed vouchers, encrypted ID storage with audited reveal: all built on PostgreSQL and
  covered by end-to-end tests (`internal/api`). The apps use it with `EXPO_PUBLIC_API_MODE=http`.
- **Three outside services are stand-ins** (sign-in codes, Telebirr/Chapa payments, push alerts). They sit behind interfaces in
  `internal/api/providers.go`, and production refuses to start while any stand-in is selected.
- **Not built yet:** the hotel's offline tools (rooms chart, walk-ins, QR check-in, statements), reviews, guest cancellation.
- Full checklist to production: [`docs/going-live.md`](docs/going-live.md).
- Production refuses to start with demo mode, without secrets, over http, or without `DATABASE_URL`. Request logs contain no
  headers, query strings or bodies.

## Run

Nothing to install: `go run ./cmd/devserver` starts a private database with demo hotels and the API (see [`docs/going-live.md`](docs/going-live.md)).
With your own database:

```bash
cp .env.example .env                 # then put your Supabase connection string in .env (see docs/database.md)
go run ./cmd/migrate up              # create the tables
go run ./cmd/migrate seed            # demo hotels (development only)
go run ./cmd/api                     # http://localhost:8080
```

`.env` is read automatically. It is git-ignored: never commit it.

## Test

```bash
go vet ./...
go test ./... -race        # includes database tests on an embedded PostgreSQL (first run downloads it)
```

## Layout

```
cmd/api/            API entrypoint
cmd/migrate/        migrations: up, down (dev only), status, seed (dev only)
cmd/devserver/      whole backend on your computer: private database, demo data, API (development only)
internal/config/    settings, .env loading, startup validation
internal/db/        PostgreSQL connection pool
internal/dbtest/    test database (embedded PostgreSQL) and tests for the database rules
internal/httpx/     response conventions (server_time, one error shape, stable error codes)
internal/server/    router and middleware
internal/api/       the endpoints (auth, catalogue, bookings, payments, partner), integrations, background sweeper
db/migrations/      goose migrations, embedded in the migrate binary
db/seed/            demo data
db/queries/         sqlc queries (generates internal/store)
docs/               design.md, database.md
api/openapi.yaml    API contract
```
