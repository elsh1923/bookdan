# BookDan backend (Go)

HTTP API for the Guest and Partner mobile apps. The contract lives in [`api/openapi.yaml`](api/openapi.yaml).
Data model, security and build order: [`docs/design.md`](docs/design.md). The database: [`docs/database.md`](docs/database.md).

Stack (from the mobile and backend spec): Go, chi, pgx, sqlc, goose, River, PostgreSQL with PostGIS (Supabase).

## Status

- **Database: built and tested.** Seven migrations create identity, catalogue, bookings, payments, the money ledger,
  vouchers, sync and audit tables. The rules that protect money and rooms (no overselling, the booking status machine,
  a balanced append-only ledger, safe retries) are enforced by the database and covered by tests.
- **API:** `GET /v1/healthz` (liveness) and `GET /v1/readyz` (database reachable). Every other route in the contract
  still answers `501 NOT_IMPLEMENTED` with the standard error body. The endpoints come next, starting with sign-in.
- Production refuses to start with `DEMO_MODE=true` or without `DATABASE_URL`. Request logs contain no headers,
  query strings or bodies.

## Run

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
internal/config/    settings, .env loading, startup validation
internal/db/        PostgreSQL connection pool
internal/dbtest/    test database (embedded PostgreSQL) and tests for the database rules
internal/httpx/     response conventions (server_time, one error shape, stable error codes)
internal/server/    router and middleware
db/migrations/      goose migrations, embedded in the migrate binary
db/seed/            demo data
db/queries/         sqlc queries (generates internal/store)
docs/               design.md, database.md
api/openapi.yaml    API contract
```
