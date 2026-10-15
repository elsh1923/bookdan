package dbtest_test

import (
	"database/sql"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bookdan/backend/internal/dbtest"
)

var conn *sql.DB

func TestMain(m *testing.M) {
	if os.Getenv("SKIP_DB_TESTS") == "1" {
		fmt.Println("SKIP_DB_TESTS=1: skipping database tests")
		return
	}
	c, stop, err := dbtest.Start()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cannot start the test database:", err)
		os.Exit(1)
	}
	conn = c
	code := m.Run()
	stop()
	os.Exit(code)
}

// ---- fixtures: each helper inserts what a test needs and returns the new id.

func one(t *testing.T, tx *sql.Tx, query string, args ...any) string {
	t.Helper()
	var id string
	if err := tx.QueryRow(query, args...).Scan(&id); err != nil {
		t.Fatalf("fixture failed: %v\n%s", err, query)
	}
	return id
}

func newUser(t *testing.T, tx *sql.Tx) string {
	return one(t, tx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, fmt.Sprintf("guest-%d@example.com", time.Now().UnixNano()))
}

func newProperty(t *testing.T, tx *sql.Tx) string {
	return one(t, tx, `INSERT INTO properties (name, type, city, area, address, location, phone)
		VALUES ('Test Hotel', 'hotel', 'Addis Ababa', 'Bole', 'Bole, Addis Ababa', 'SRID=4326;POINT(38.78 8.99)', '+251 11 000 0000') RETURNING id`)
}

func newRoomType(t *testing.T, tx *sql.Tx, property string, rooms int) string {
	return one(t, tx, `INSERT INTO room_types (property_id, name, capacity, total_rooms) VALUES ($1, 'Standard', 2, $2) RETURNING id`, property, rooms)
}

func newQuote(t *testing.T, tx *sql.Tx, user, property, roomType string) string {
	return one(t, tx, `INSERT INTO quotes (user_id, property_id, room_type_id, check_in, check_out, guests, currency,
		base_santim, service_santim, vat_santim, total_santim, deposit_santim, balance_due_santim, price_snapshot, expires_at)
		VALUES ($1, $2, $3, '2026-11-01', '2026-11-02', 1, 'ETB', 320000, 0, 48000, 368000, 368000, 0, '{}', now() + interval '10 minutes')
		RETURNING id`, user, property, roomType)
}

func newBooking(t *testing.T, tx *sql.Tx) (booking, user, property string) {
	user = newUser(t, tx)
	property = newProperty(t, tx)
	rt := newRoomType(t, tx, property, 3)
	quote := newQuote(t, tx, user, property, rt)
	booking = one(t, tx, `INSERT INTO bookings (code, quote_id, user_id, property_id, room_type_id, check_in, check_out, guests, guest_name, currency,
		base_santim, service_santim, vat_santim, total_santim, deposit_santim, balance_due_santim, hold_expires_at)
		VALUES ($1, $2, $3, $4, $5, '2026-11-01', '2026-11-02', 1, 'Hanna Tesfaye', 'ETB', 320000, 0, 48000, 368000, 368000, 0, now() + interval '15 minutes')
		RETURNING id`, fmt.Sprintf("BD-%06d", time.Now().UnixNano()%1000000), quote, user, property, rt)
	return booking, user, property
}

// ---- identity

func TestUserNeedsAnIdentifierAndEmailIsLowerCase(t *testing.T) {
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		_, err := tx.Exec(`INSERT INTO users (display_name) VALUES ('nobody')`)
		dbtest.MustFail(t, err, "users_has_identifier")
	})
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		_, err := tx.Exec(`INSERT INTO users (email) VALUES ('Mixed.Case@Example.com')`)
		dbtest.MustFail(t, err, "users_email_check")
	})
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		_, err := tx.Exec(`INSERT INTO users (phone) VALUES ('0911234567')`)
		dbtest.MustFail(t, err, "users_phone_check") // must be E.164
	})
}

// ---- inventory: the database itself refuses to sell more rooms than exist

func TestInventoryRefusesToBeOversold(t *testing.T) {
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		rt := newRoomType(t, tx, newProperty(t, tx), 1)
		if _, err := tx.Exec(`INSERT INTO inventory (room_type_id, night, capacity) VALUES ($1, '2026-11-01', 1)`, rt); err != nil {
			t.Fatal(err)
		}
		hold := `UPDATE inventory SET held = held + 1 WHERE room_type_id = $1 AND night = '2026-11-01' AND held + booked < capacity`
		first, _ := tx.Exec(hold, rt)
		second, _ := tx.Exec(hold, rt)
		if n, _ := first.RowsAffected(); n != 1 {
			t.Fatalf("the first hold should win, rows=%d", n)
		}
		if n, _ := second.RowsAffected(); n != 0 {
			t.Fatalf("the second hold must find the room taken, rows=%d", n)
		}
		// Even a careless direct write cannot break the rule.
		_, err := tx.Exec(`UPDATE inventory SET held = 5 WHERE room_type_id = $1`, rt)
		dbtest.MustFail(t, err, "inventory_never_oversold")
	})
}

func TestParallelHoldsOnTheLastRoomsExactlyCapacityWin(t *testing.T) {
	// Real concurrency, so this runs on committed rows rather than inside one rolled-back transaction.
	var property, roomType string
	setup, _ := conn.Begin()
	setup.Exec(`SET LOCAL search_path = app, extensions, public`)
	property = newProperty(t, setup)
	roomType = newRoomType(t, setup, property, 3)
	if _, err := setup.Exec(`INSERT INTO inventory (room_type_id, night, capacity) VALUES ($1, '2026-12-01', 3)`, roomType); err != nil {
		t.Fatal(err)
	}
	if err := setup.Commit(); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(`DELETE FROM app.properties WHERE id = $1`, property) // cascades to room types and inventory

	const guests = 25
	var won int64
	var wg sync.WaitGroup
	for i := 0; i < guests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := conn.Exec(`UPDATE app.inventory SET held = held + 1
				WHERE room_type_id = $1 AND night = '2026-12-01' AND held + booked < capacity`, roomType)
			if err != nil {
				t.Errorf("hold failed: %v", err)
				return
			}
			if n, _ := res.RowsAffected(); n == 1 {
				atomic.AddInt64(&won, 1)
			}
		}()
	}
	wg.Wait()
	if won != 3 {
		t.Fatalf("%d guests tried for 3 rooms but %d won", guests, won)
	}
}

// ---- bookings: the status machine and the frozen price

func TestBookingStatusMachine(t *testing.T) {
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		b, _, _ := newBooking(t, tx)

		// Skipping the payment step is illegal.
		_, err := tx.Exec(`UPDATE bookings SET status = 'CONFIRMED' WHERE id = $1`, b)
		dbtest.MustFail(t, err, "illegal booking status change HELD -> CONFIRMED")
	})
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		b, _, _ := newBooking(t, tx)
		if _, err := tx.Exec(`SET LOCAL app.actor = 'guest'`); err != nil {
			t.Fatal(err)
		}
		// Paying moves it on, and it must then carry the hotel's deadline.
		_, err := tx.Exec(`UPDATE bookings SET status = 'PAID_AWAITING_HOTEL' WHERE id = $1`, b)
		dbtest.MustFail(t, err, "awaiting_has_deadline")
	})
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		b, _, _ := newBooking(t, tx)
		steps := []struct{ to, extra string }{
			{"PAID_AWAITING_HOTEL", ", hotel_deadline_at = now() + interval '15 minutes', paid_at = now()"},
			{"CONFIRMED", ""},
			{"CHECKED_IN", ""},
			{"COMPLETED", ""},
		}
		for _, s := range steps {
			if _, err := tx.Exec(`UPDATE bookings SET status = $2`+s.extra+` WHERE id = $1`, b, s.to); err != nil {
				t.Fatalf("legal step to %s failed: %v", s.to, err)
			}
		}
		// A finished stay is not reopened, and it cannot be cancelled afterwards.
		// (A savepoint lets the transaction carry on after the expected rejection.)
		tx.Exec(`SAVEPOINT before_illegal`)
		_, err := tx.Exec(`UPDATE bookings SET status = 'CANCELLED_REFUNDED' WHERE id = $1`, b)
		dbtest.MustFail(t, err, "illegal booking status change COMPLETED -> CANCELLED_REFUNDED")
		tx.Exec(`ROLLBACK TO SAVEPOINT before_illegal`)

		// The database wrote the history by itself: created (null -> HELD) plus four moves.
		var n int
		tx.QueryRow(`SELECT count(*) FROM booking_events WHERE booking_id = $1`, b).Scan(&n)
		if n != 5 {
			t.Fatalf("expected 5 history rows, got %d", n)
		}
	})
}

func TestExpiredAndDeclinedBookingsAreFinal(t *testing.T) {
	for _, terminal := range []string{"EXPIRED"} {
		dbtest.Tx(t, conn, func(tx *sql.Tx) {
			b, _, _ := newBooking(t, tx)
			if _, err := tx.Exec(`UPDATE bookings SET status = $2 WHERE id = $1`, b, terminal); err != nil {
				t.Fatal(err)
			}
			_, err := tx.Exec(`UPDATE bookings SET status = 'CONFIRMED' WHERE id = $1`, b)
			dbtest.MustFail(t, err, "illegal booking status change")
		})
	}
}

func TestBookingHistoryRecordsWhoAndWhy(t *testing.T) {
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		b, _, _ := newBooking(t, tx)
		tx.Exec(`SET LOCAL app.actor = 'worker:hold-expiry'`)
		tx.Exec(`SET LOCAL app.reason = 'hold timed out'`)
		if _, err := tx.Exec(`UPDATE bookings SET status = 'EXPIRED' WHERE id = $1`, b); err != nil {
			t.Fatal(err)
		}
		var from, to, actor, reason sql.NullString
		err := tx.QueryRow(`SELECT from_status, to_status, actor, reason FROM booking_events WHERE booking_id = $1 ORDER BY id DESC LIMIT 1`, b).
			Scan(&from, &to, &actor, &reason)
		if err != nil || from.String != "HELD" || to.String != "EXPIRED" || actor.String != "worker:hold-expiry" || reason.String != "hold timed out" {
			t.Fatalf("history row wrong: %v %v %v %v %v", from, to, actor, reason, err)
		}
	})
}

func TestBookingPriceIsFrozenAndMustAddUp(t *testing.T) {
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		b, _, _ := newBooking(t, tx)
		_, err := tx.Exec(`UPDATE bookings SET total_santim = total_santim - 1000, deposit_santim = deposit_santim - 1000 WHERE id = $1`, b)
		dbtest.MustFail(t, err, "booking amounts cannot be changed")
	})
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		user, property := newUser(t, tx), newProperty(t, tx)
		rt := newRoomType(t, tx, property, 1)
		_, err := tx.Exec(`INSERT INTO quotes (user_id, property_id, room_type_id, check_in, check_out, guests, currency,
			base_santim, service_santim, vat_santim, total_santim, deposit_santim, balance_due_santim, price_snapshot, expires_at)
			VALUES ($1, $2, $3, '2026-11-01', '2026-11-02', 1, 'ETB', 320000, 0, 48000, 999999, 368000, 0, '{}', now())`, user, property, rt)
		dbtest.MustFail(t, err, "quote_amounts_add_up")
	})
}

func TestRoomMustBelongToItsOwnHotelsRoomType(t *testing.T) {
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		hotelA, hotelB := newProperty(t, tx), newProperty(t, tx)
		typeOfA := newRoomType(t, tx, hotelA, 1)
		_, err := tx.Exec(`INSERT INTO rooms (property_id, room_type_id, number) VALUES ($1, $2, '101')`, hotelB, typeOfA)
		dbtest.MustFail(t, err, "violates foreign key")
	})
}

// ---- payments

func TestOneOpenPaymentPerBookingAndDuplicateCallbacksAreIgnored(t *testing.T) {
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		b, _, _ := newBooking(t, tx)
		insert := `INSERT INTO payments (booking_id, provider, provider_ref, amount_santim, currency) VALUES ($1, 'mock', $2, 368000, 'ETB')`
		if _, err := tx.Exec(insert, b, "ref-1"); err != nil {
			t.Fatal(err)
		}
		// Retrying "pay" must not open a second charge.
		_, err := tx.Exec(insert, b, "ref-2")
		dbtest.MustFail(t, err, "payments_one_pending_per_booking")
	})
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		// The gateway calls back twice with the same event id (acceptance test M4).
		cb := `INSERT INTO payment_events (provider, event_id, payload) VALUES ('mock', 'evt-1', '{"ok":true}')`
		if _, err := tx.Exec(cb); err != nil {
			t.Fatal(err)
		}
		_, err := tx.Exec(cb)
		dbtest.MustFail(t, err, "payment_events_provider_event_id_key")
	})
}

// ---- ledger

func ledgerTx(t *testing.T, tx *sql.Tx) string {
	return one(t, tx, `INSERT INTO ledger_transactions (kind, memo) VALUES ('adjustment', 'test') RETURNING id`)
}

func TestLedgerTransactionsMustBalance(t *testing.T) {
	// Balanced: commits (checked at commit time, so use a real transaction we then clean up by rolling back).
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		id := ledgerTx(t, tx)
		for _, e := range []struct {
			acct string
			amt  int64
		}{{"gateway_clearing", 368000}, {"guest_deposits_held", -368000}} {
			if _, err := tx.Exec(`INSERT INTO ledger_entries (transaction_id, account_code, amount_santim, currency) VALUES ($1, $2, $3, 'ETB')`, id, e.acct, e.amt); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := tx.Exec(`SET CONSTRAINTS ALL IMMEDIATE`); err != nil {
			t.Fatalf("a balanced transaction was refused: %v", err)
		}
	})
	// Unbalanced: refused.
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		id := ledgerTx(t, tx)
		tx.Exec(`INSERT INTO ledger_entries (transaction_id, account_code, amount_santim, currency) VALUES ($1, 'gateway_clearing', 368000, 'ETB')`, id)
		tx.Exec(`INSERT INTO ledger_entries (transaction_id, account_code, amount_santim, currency) VALUES ($1, 'guest_deposits_held', -300000, 'ETB')`, id)
		_, err := tx.Exec(`SET CONSTRAINTS ALL IMMEDIATE`)
		dbtest.MustFail(t, err, "does not balance")
	})
	// A transaction with a single entry is refused.
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		id := ledgerTx(t, tx)
		tx.Exec(`INSERT INTO ledger_entries (transaction_id, account_code, amount_santim, currency) VALUES ($1, 'gateway_clearing', 100, 'ETB')`, id)
		_, err := tx.Exec(`SET CONSTRAINTS ALL IMMEDIATE`)
		dbtest.MustFail(t, err, "needs at least two entries")
	})
}

func TestLedgerMixedCurrenciesMustEachBalance(t *testing.T) {
	dbtest.Tx(t, conn, func(tx *sql.Tx) {
		id := ledgerTx(t, tx)
		// ETB and USD cannot cancel each other out.
		tx.Exec(`INSERT INTO ledger_entries (transaction_id, account_code, amount_santim, currency) VALUES ($1, 'gateway_clearing', 500, 'ETB')`, id)
		tx.Exec(`INSERT INTO ledger_entries (transaction_id, account_code, amount_santim, currency) VALUES ($1, 'guest_deposits_held', -500, 'USD')`, id)
		_, err := tx.Exec(`SET CONSTRAINTS ALL IMMEDIATE`)
		dbtest.MustFail(t, err, "does not balance")
	})
}

func TestLedgerIsAppendOnly(t *testing.T) {
	// Committed rows, because append-only must hold across transactions.
	setup, _ := conn.Begin()
	setup.Exec(`SET LOCAL search_path = app, extensions, public`)
	id := ledgerTx(t, setup)
	setup.Exec(`INSERT INTO ledger_entries (transaction_id, account_code, amount_santim, currency) VALUES ($1, 'gateway_clearing', 100, 'ETB'), ($1, 'guest_deposits_held', -100, 'ETB')`, id)
	if err := setup.Commit(); err != nil {
		t.Fatal(err)
	}

	for _, stmt := range []string{
		`UPDATE app.ledger_entries SET amount_santim = 1 WHERE transaction_id = $1`,
		`DELETE FROM app.ledger_entries WHERE transaction_id = $1`,
		`UPDATE app.ledger_transactions SET memo = 'edited' WHERE id = $1`,
		`DELETE FROM app.ledger_transactions WHERE id = $1`,
	} {
		_, err := conn.Exec(stmt, id)
		dbtest.MustFail(t, err, "append-only")
	}
	_, err := conn.Exec(`TRUNCATE app.ledger_entries`)
	dbtest.MustFail(t, err, "append-only")
}

func TestAuditAndHistoryAreAppendOnly(t *testing.T) {
	if _, err := conn.Exec(`INSERT INTO app.audit_log (action) VALUES ('test.append_only')`); err != nil {
		t.Fatal(err)
	}
	_, err := conn.Exec(`UPDATE app.audit_log SET action = 'changed' WHERE action = 'test.append_only'`)
	dbtest.MustFail(t, err, "append-only")
	_, err = conn.Exec(`DELETE FROM app.audit_log WHERE action = 'test.append_only'`)
	dbtest.MustFail(t, err, "append-only")
}

// ---- data safety

func TestEveryTableHasRowLevelSecurity(t *testing.T) {
	rows, err := conn.Query(`SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'app' AND c.relkind = 'r' AND NOT c.relrowsecurity ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		rows.Scan(&name)
		t.Errorf("table app.%s has no row level security", name)
	}
}

func TestReferenceDataIsInPlace(t *testing.T) {
	var accounts int
	conn.QueryRow(`SELECT count(*) FROM app.ledger_accounts`).Scan(&accounts)
	if accounts != 6 {
		t.Fatalf("expected 6 ledger accounts, got %d", accounts)
	}
	var confirmed bool
	if err := conn.QueryRow(`SELECT confirmed FROM app.pricing_config`).Scan(&confirmed); err != nil || confirmed {
		t.Fatalf("pricing must start unconfirmed until the owner decides: confirmed=%v err=%v", confirmed, err)
	}
}

// ---- demo data (keep this test last: it commits rows)

func TestDemoSeedLoadsAndIsSafeToRunTwice(t *testing.T) {
	seed := dbtest.SeedSQL()
	for run := 1; run <= 2; run++ {
		if _, err := conn.Exec(seed); err != nil {
			t.Fatalf("seed run %d failed: %v", run, err)
		}
	}
	count := func(q string) int {
		var n int
		if err := conn.QueryRow(q).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	for _, c := range []struct {
		what  string
		query string
		want  int
	}{
		{"hotels", `SELECT count(*) FROM app.properties WHERE id::text LIKE 'a0000000-%'`, 8},
		{"room types", `SELECT count(*) FROM app.room_types WHERE id::text LIKE 'b0000000-%'`, 16},
		{"physical rooms", `SELECT count(*) FROM app.rooms WHERE property_id::text LIKE 'a0000000-%'`, 55},
		{"nights of availability", `SELECT count(*) FROM app.inventory WHERE room_type_id::text LIKE 'b0000000-%'`, 16 * 180},
		{"rate plans", `SELECT count(*) FROM app.rate_plans WHERE room_type_id::text LIKE 'b0000000-%'`, 16},
		{"landmarks", `SELECT count(*) FROM app.landmarks`, 6},
		{"hotel owners", `SELECT count(*) FROM app.property_staff`, 1},
	} {
		if got := count(c.query); got != c.want {
			t.Errorf("%s: want %d, got %d (the second run must not add duplicates)", c.what, c.want, got)
		}
	}
}
