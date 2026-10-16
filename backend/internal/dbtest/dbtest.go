// Package dbtest gives tests a real PostgreSQL with the project's migrations applied.
//
// It starts an embedded PostgreSQL (no Docker needed). That build has no PostGIS, so when PostGIS is
// missing the two location columns and their indexes are swapped for plain text before applying; everything
// else (constraints, triggers, the ledger, the state machine) is the real migration. Against a database with
// PostGIS (CI, Supabase, docker compose) the migrations are applied untouched.
package dbtest

import (
	"database/sql"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/bookdan/backend/db"
)

// URL is the connection string of the test database, for tests that need their own pool.
var URL string

// HasPostGIS is false when tests run on the embedded database, which has no PostGIS.
var HasPostGIS bool

var (
	seedPoint    = regexp.MustCompile(`ST_SetSRID\(ST_MakePoint\(([^,]+), ([^)]+)\), 4326\)::geography`)
	seedVarPoint = regexp.MustCompile(`ST_SetSRID\(ST_MakePoint\(v\.lng, v\.lat\), 4326\)::geography`)
)

// SeedSQL returns the demo seed, with the PostGIS calls turned into text when PostGIS is missing.
func SeedSQL() string {
	if HasPostGIS {
		return db.DevSeed
	}
	s := seedVarPoint.ReplaceAllString(db.DevSeed, `'POINT(' || v.lng || ' ' || v.lat || ')'`)
	return seedPoint.ReplaceAllString(s, `'SRID=4326;POINT($1 $2)'`)
}

// Start launches the database for a test binary. Call it from TestMain and defer the returned stop function.
func Start() (*sql.DB, func(), error) {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		// An existing database (for example PostGIS in CI). It is wiped first so tests start clean.
		conn, err := sql.Open("pgx", url)
		if err != nil {
			return nil, nil, err
		}
		if _, err := conn.Exec(`DROP SCHEMA IF EXISTS app CASCADE; DROP TABLE IF EXISTS goose_db_version`); err != nil {
			return nil, nil, err
		}
		URL = url
		HasPostGIS = true
		if err := migrate(conn, false); err != nil {
			return nil, nil, err
		}
		return conn, func() { conn.Close() }, nil
	}

	port, err := freePort()
	if err != nil {
		return nil, nil, err
	}
	base := filepath.Join(os.TempDir(), "bookdan-embedded-pg")
	dataDir, err := os.MkdirTemp(base, "data-")
	if err != nil {
		if err := os.MkdirAll(base, 0o755); err != nil {
			return nil, nil, err
		}
		if dataDir, err = os.MkdirTemp(base, "data-"); err != nil {
			return nil, nil, err
		}
	}
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Logger(io.Discard). // the server log is noisy: tests deliberately trigger errors
		Version(embeddedpostgres.V16).
		Port(uint32(port)).
		Username("postgres").Password("postgres").Database("bookdan_test").
		BinariesPath(filepath.Join(base, "bin")).
		RuntimePath(filepath.Join(dataDir, "run")).
		DataPath(filepath.Join(dataDir, "data")))
	// Test packages run in parallel and each starts a database; unpacking the shared binaries must not overlap.
	unlock := lockDir(base)
	startErr := pg.Start()
	unlock()
	if startErr != nil {
		return nil, nil, fmt.Errorf("start embedded postgres: %w", startErr)
	}
	stop := func() {
		_ = pg.Stop()
		_ = os.RemoveAll(dataDir)
	}

	URL = fmt.Sprintf("postgres://postgres:postgres@localhost:%d/bookdan_test?sslmode=disable", port)
	conn, err := sql.Open("pgx", URL)
	if err != nil {
		stop()
		return nil, nil, err
	}
	var hasPostGIS bool
	if err := conn.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'postgis')`).Scan(&hasPostGIS); err != nil {
		stop()
		return nil, nil, err
	}
	HasPostGIS = hasPostGIS
	if err := migrate(conn, !hasPostGIS); err != nil {
		stop()
		return nil, nil, err
	}
	return conn, func() { conn.Close(); stop() }, nil
}

func migrate(conn *sql.DB, withoutPostGIS bool) error {
	var fsys fs.FS = db.Migrations
	if withoutPostGIS {
		fsys = withoutPostGISFS()
	}
	goose.SetBaseFS(fsys)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(conn, "migrations")
}

var (
	createPostGIS = regexp.MustCompile(`(?m)^CREATE EXTENSION IF NOT EXISTS postgis[^\n]*\n`)
	geoColumn     = regexp.MustCompile(`geography\(Point, 4326\)`)
	geoIndex      = regexp.MustCompile(`(?m)^CREATE INDEX [^\n]*USING gist \(location\);\n`)
)

// withoutPostGISFS returns the migrations with the PostGIS-only parts replaced by plain text.
func withoutPostGISFS() fs.FS {
	out := fstest.MapFS{}
	entries, _ := fs.ReadDir(db.Migrations, "migrations")
	for _, e := range entries {
		b, _ := fs.ReadFile(db.Migrations, "migrations/"+e.Name())
		s := string(b)
		s = createPostGIS.ReplaceAllString(s, "")
		s = geoIndex.ReplaceAllString(s, "")
		s = geoColumn.ReplaceAllString(s, "text")
		out["migrations/"+e.Name()] = &fstest.MapFile{Data: []byte(s)}
	}
	return out
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Tx runs fn in a transaction that is rolled back afterwards, so tests never see each other's rows.
func Tx(t *testing.T, conn *sql.DB, fn func(tx *sql.Tx)) {
	t.Helper()
	tx, err := conn.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`SET LOCAL search_path = app, extensions, public`); err != nil {
		t.Fatalf("search_path: %v", err)
	}
	fn(tx)
}

// MustFail asserts that err is a PostgreSQL error whose message contains want.
func MustFail(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error containing %q, got none", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected an error containing %q, got: %v", want, err)
	}
}

// lockDir takes a simple cross-process lock (a file that exists while held). A lock older than three minutes is stale.
func lockDir(base string) (unlock func()) {
	path := filepath.Join(base, "start.lock")
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			f.Close()
			return func() { os.Remove(path) }
		}
		if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) > 3*time.Minute {
			os.Remove(path)
		}
		time.Sleep(150 * time.Millisecond)
	}
	return func() {}
}

// StartDev runs an embedded PostgreSQL that keeps its data in dir between runs, migrates it and loads the demo data.
// It is for `go run ./cmd/devserver`: a local backend with no Docker and no account. It has no PostGIS, so the
// "near a landmark" search is off there; everything else is the real thing.
func StartDev(dir string, port uint32) (url string, stop func(), err error) {
	base := filepath.Join(dir, "bin")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return "", nil, err
	}
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Logger(io.Discard).
		Version(embeddedpostgres.V16).
		Port(port).
		Username("postgres").Password("postgres").Database("bookdan_dev").
		BinariesPath(base).
		RuntimePath(filepath.Join(dir, "run")).
		DataPath(filepath.Join(dir, "data")))
	url = fmt.Sprintf("postgres://postgres:postgres@localhost:%d/bookdan_dev?sslmode=disable", port)
	stop = func() {}
	// A database left running by an earlier run (for example one that was killed) is reused.
	if probe, err := sql.Open("pgx", url); err == nil {
		running := probe.Ping() == nil
		probe.Close()
		if running {
			goto ready
		}
	}
	{
		unlock := lockDir(base)
		startErr := pg.Start()
		unlock()
		if startErr != nil {
			return "", nil, fmt.Errorf("start embedded postgres: %w", startErr)
		}
		stop = func() { _ = pg.Stop() }
	}
ready:
	conn, err := sql.Open("pgx", url)
	if err != nil {
		stop()
		return "", nil, err
	}
	defer conn.Close()
	var hasPostGIS bool
	_ = conn.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'postgis')`).Scan(&hasPostGIS)
	HasPostGIS = hasPostGIS
	if err := migrate(conn, !hasPostGIS); err != nil {
		stop()
		return "", nil, err
	}
	if _, err := conn.Exec(`SET search_path = app, extensions, public; ` + SeedSQL()); err != nil {
		stop()
		return "", nil, fmt.Errorf("demo data: %w", err)
	}
	return url, stop, nil
}
