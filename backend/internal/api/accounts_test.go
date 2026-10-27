package api_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestAGuestCanExportTheirDataAndNeverSeesTheFullIDNumber(t *testing.T) {
	n := time.Now().UnixNano()
	email := uniqueEmail("export")
	guest := newClient(t, "guest")
	guest.signIn("email", email)
	withProfile(guest)
	hold(guest, quote(guest, bole, boleSingle, 230, 231, 1, 1).str("id"), fmt.Sprintf("export-%d", n))

	r := guest.do("GET", "/me/export", nil)
	if r.status != 200 || r.str("email") != email {
		t.Fatalf("export: %d %s", r.status, r.raw)
	}
	if strings.Contains(string(r.raw), "4829 1730 5561") || strings.Contains(string(r.raw), "482917305561") {
		t.Fatalf("the full ID number must never be in the export")
	}
	if p, _ := r.body["profile"].(map[string]any); p == nil || p["id_last4"] == "" {
		t.Fatalf("the export holds the profile with the masked ID: %s", r.raw)
	}
	if b, _ := r.body["bookings"].([]any); len(b) != 1 {
		t.Fatalf("the export lists the guest's bookings: %s", r.raw)
	}
	// Another guest's export has nothing of this guest's.
	other := newClient(t, "guest")
	other.signIn("email", uniqueEmail("export2"))
	if o := other.do("GET", "/me/export", nil); strings.Contains(string(o.raw), email) {
		t.Fatalf("an export holds only the caller's data")
	}
	if anon := newClient(t, "guest").do("GET", "/me/export", nil); anon.status != 401 {
		t.Fatalf("an export needs a sign-in: %d", anon.status)
	}
}

func TestDeletingAnAccountRemovesWhatIdentifiesTheGuestAndKeepsTheRecordsOfMoney(t *testing.T) {
	ctx := context.Background()
	n := time.Now().UnixNano()
	email := uniqueEmail("delete")
	guest := newClient(t, "guest")
	guest.signIn("email", email)
	withProfile(guest)
	hotel := hotelFor(t)

	// A confirmed stay in the future blocks the deletion.
	id, _, _, _ := confirmedBooking(t, guest, hotel, 232, 234, fmt.Sprintf("del-%d", n))
	if r := guest.do("DELETE", "/me/account", map[string]string{"confirm": "DELETE"}); r.status != 409 || r.str("error", "code") != "ACTIVE_BOOKINGS" {
		t.Fatalf("an account with a stay to come cannot be deleted: %d %s", r.status, r.raw)
	}
	if r := guest.do("DELETE", "/me/account", map[string]string{"confirm": "yes"}); r.status != 400 {
		t.Fatalf("deleting needs the explicit confirmation: %d", r.status)
	}
	if r := guest.do("DELETE", "/me/account", nil); r.status != 400 {
		t.Fatalf("an empty request must not delete anything: %d", r.status)
	}

	// After the guest cancels, the account can go.
	if r := guest.do("POST", "/bookings/"+id+"/cancel", nil); r.status != 200 {
		t.Fatalf("setup: cancel %d %s", r.status, r.raw)
	}
	if r := guest.do("DELETE", "/me/account", map[string]string{"confirm": "DELETE"}); r.status != 200 {
		t.Fatalf("delete: %d %s", r.status, r.raw)
	}

	// The old session is over.
	if r := guest.do("GET", "/me/profile", nil); r.status != 401 {
		t.Errorf("the old token must stop working: %d", r.status)
	}
	var userID string
	pool.QueryRow(ctx, `SELECT user_id FROM bookings WHERE id = $1`, id).Scan(&userID)
	var profiles, devices, challenges int
	pool.QueryRow(ctx, `SELECT count(*) FROM guest_profiles WHERE user_id = $1`, userID).Scan(&profiles)
	pool.QueryRow(ctx, `SELECT count(*) FROM devices WHERE user_id = $1 AND (revoked_at IS NULL OR push_token IS NOT NULL)`, userID).Scan(&devices)
	pool.QueryRow(ctx, `SELECT count(*) FROM otp_challenges WHERE identifier = $1`, email).Scan(&challenges)
	if profiles != 0 || devices != 0 || challenges != 0 {
		t.Errorf("the profile and codes are deleted and every phone is revoked with no push token: %d %d %d", profiles, devices, challenges)
	}
	var addr, status string
	pool.QueryRow(ctx, `SELECT email, status FROM users WHERE id = $1`, userID).Scan(&addr, &status)
	if addr == email || status != "suspended" || !strings.HasSuffix(addr, "@deleted.invalid") {
		t.Errorf("the account is closed and the address freed: %q %q", addr, status)
	}
	var idNumberLeft bool
	var phone *string
	pool.QueryRow(ctx, `SELECT g.id_number_enc IS NOT NULL FROM booking_guests g WHERE g.booking_id = $1`, id).Scan(&idNumberLeft)
	pool.QueryRow(ctx, `SELECT guest_phone FROM bookings WHERE id = $1`, id).Scan(&phone)
	if idNumberLeft || phone != nil {
		t.Errorf("the ID number on the hotel's copy is erased and the phone cleared: id left=%v phone=%v", idNumberLeft, phone)
	}

	// The records of money and stays remain.
	var ledger, bookings int
	pool.QueryRow(ctx, `SELECT count(*) FROM ledger_transactions WHERE booking_id = $1`, id).Scan(&ledger)
	pool.QueryRow(ctx, `SELECT count(*) FROM bookings WHERE id = $1`, id).Scan(&bookings)
	if ledger == 0 || bookings != 1 {
		t.Errorf("the booking and its ledger entries are kept: bookings=%d ledger=%d", bookings, ledger)
	}

	// The same address can sign up again and starts empty.
	again := newClient(t, "guest")
	again.signIn("email", email)
	if list := again.do("GET", "/bookings", nil); strings.Contains(string(list.raw), id) {
		t.Errorf("a new account with the same address has none of the old bookings")
	}
	if p := again.do("GET", "/me/profile", nil); p.body["profile"] != nil {
		t.Errorf("a new account has no profile: %s", p.raw)
	}
}
