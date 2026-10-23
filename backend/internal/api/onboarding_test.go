package api_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// mailRecorder stands in for the email service: it remembers every ordinary email (invitations and the like).
type mailRecorder struct {
	mu   sync.Mutex
	sent []sentMail
}

type sentMail struct{ to, subject, text string }

func (m *mailRecorder) SendMail(_ context.Context, to, subject, text, _ string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, sentMail{to, subject, text})
	return nil
}

func (m *mailRecorder) to(addr string) []sentMail {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []sentMail
	for _, s := range m.sent {
		if s.to == addr {
			out = append(out, s)
		}
	}
	return out
}

// realAddress is an email address that is not a demo address, so it gets a real emailed code.
func realAddress(prefix string) string {
	return fmt.Sprintf("%s.%d@realmail.dev", prefix, time.Now().UnixNano())
}

// signInWithEmailedCode signs in with the code that was actually "emailed" (a real address never gets the public demo code).
func signInWithEmailedCode(c *client, path, verifyPath, email string) resp {
	c.t.Helper()
	if r := c.do("POST", path, map[string]string{"type": "email", "value": email}); r.status != 200 {
		c.t.Fatalf("code request for %s: %d %s", email, r.status, r.raw)
	}
	sent := mailer.to(email)
	if len(sent) == 0 {
		c.t.Fatalf("no code was emailed to %s", email)
	}
	return c.do("POST", verifyPath, map[string]string{"type": "email", "value": email, "code": sent[len(sent)-1].code})
}

func businessBody(number string) map[string]any {
	if number == "" {
		number = "1000123456789"
	}
	return map[string]any{
		"legal_name": "Kazanchis Hospitality PLC", "license_number": "AA/2/0123/2018", "tin": "0012345678",
		"contact_name": "Almaz Bekele", "contact_phone": "+251 911 000 111", "contact_email": "almaz@kazanchis-hotel.test",
		"payout_method": "bank_transfer", "payout_account_name": "Kazanchis Hospitality PLC", "payout_account_number": number,
	}
}

func TestAnOwnerManagesTheirHotelAndTheirReceptionistsOnly(t *testing.T) {
	admin := adminClient(t)
	h1 := admin.do("POST", "/admin/hotels", newHotelBody(fmt.Sprintf("Owner One %d", time.Now().UnixNano()%100000)))
	h2 := admin.do("POST", "/admin/hotels", newHotelBody(fmt.Sprintf("Owner Two %d", time.Now().UnixNano()%100000)))
	ownerEmail, recEmail := realAddress("owner"), realAddress("rec")

	// The admin adds the owner: they get an invitation email and show as "invited".
	if r := admin.do("POST", "/admin/hotels/"+h1.str("id")+"/staff", map[string]string{"contact": ownerEmail, "role": "owner"}); r.status != 200 {
		t.Fatalf("add owner: %d %s", r.status, r.raw)
	}
	inv := mails.to(ownerEmail)
	if len(inv) != 1 || !strings.Contains(inv[0].subject, "You were added to Owner One") || !strings.Contains(inv[0].text, ownerEmail) || !strings.Contains(inv[0].text, "BookDan Partner") {
		t.Fatalf("the owner should receive an invitation that says how to sign in: %+v", inv)
	}
	statusOf := func(email string) string {
		d := admin.do("GET", "/admin/hotels/"+h1.str("id"), nil)
		for _, s := range d.body["staff"].([]any) {
			if m := s.(map[string]any); m["contact"] == email {
				return m["status"].(string)
			}
		}
		return "absent"
	}
	if statusOf(ownerEmail) != "invited" {
		t.Fatalf("a person who has not signed in yet is only invited")
	}

	// The owner signs in on the admin page with the emailed code, and now shows as joined.
	owner := newClient(t, "admin")
	v := signInWithEmailedCode(owner, "/admin/otp/request", "/admin/otp/verify", ownerEmail)
	if v.status != 200 || v.str("role") != "owner" {
		t.Fatalf("owner sign-in: %d %s", v.status, v.raw)
	}
	owner.token = v.str("token")
	if statusOf(ownerEmail) != "joined" {
		t.Fatalf("after signing in the owner has joined")
	}

	// They see only their hotel, and cannot touch another one, create a hotel, add an owner, or see the business record.
	if list := owner.do("GET", "/admin/hotels", nil); len(list.body["items"].([]any)) != 1 {
		t.Fatalf("the owner should see only their own hotel: %s", list.raw)
	}
	if r := owner.do("GET", "/admin/hotels/"+h2.str("id"), nil); r.status != 404 {
		t.Fatalf("another hotel must look like it does not exist: %d", r.status)
	}
	if r := owner.do("POST", "/admin/hotels", newHotelBody("Brand new")); r.status != 403 {
		t.Fatalf("owners cannot create hotels: %d", r.status)
	}
	if r := owner.do("POST", "/admin/hotels/"+h1.str("id")+"/staff", map[string]string{"contact": realAddress("x"), "role": "owner"}); r.status != 403 {
		t.Fatalf("owners cannot add another owner: %d", r.status)
	}
	if r := owner.do("POST", "/admin/hotels/"+h2.str("id")+"/staff", map[string]string{"contact": recEmail, "role": "receptionist"}); r.status != 404 {
		t.Fatalf("owners cannot add staff to another hotel: %d", r.status)
	}
	if d := owner.do("GET", "/admin/hotels/"+h1.str("id"), nil); d.body["business"] != nil {
		t.Fatalf("the business record is for platform admins only")
	}
	if r := owner.do("PUT", "/admin/hotels/"+h1.str("id")+"/business", businessBody("")); r.status != 403 {
		t.Fatalf("owners cannot edit the business record: %d", r.status)
	}

	// The owner adds their own receptionist: invitation sent, receptionist invited.
	if r := owner.do("POST", "/admin/hotels/"+h1.str("id")+"/staff", map[string]string{"contact": recEmail, "role": "receptionist"}); r.status != 200 {
		t.Fatalf("an owner adds a receptionist: %d %s", r.status, r.raw)
	}
	if len(mails.to(recEmail)) != 1 || statusOf(recEmail) != "invited" {
		t.Fatalf("the receptionist should be invited by email")
	}
	// The receptionist signs in to the hotel app (not the admin page) and then shows as joined.
	if r := signInWithEmailedCode(newClient(t, "admin"), "/admin/otp/request", "/admin/otp/verify", recEmail); r.status != 403 {
		t.Fatalf("a receptionist cannot use the admin page: %d", r.status)
	}
	if statusOf(recEmail) != "invited" {
		t.Fatalf("being refused on the admin page is not joining")
	}
	app := newClient(t, "partner")
	if r := signInWithEmailedCode(app, "/auth/otp/request", "/auth/otp/verify", recEmail); r.status != 200 {
		t.Fatalf("a receptionist signs in to the hotel app: %d %s", r.status, r.raw)
	}
	if statusOf(recEmail) != "joined" {
		t.Fatalf("signing in to the hotel app marks staff as joined")
	}

	// An owner removes a receptionist but never an owner.
	d := admin.do("GET", "/admin/hotels/"+h1.str("id"), nil)
	var ownerID, recID string
	for _, s := range d.body["staff"].([]any) {
		m := s.(map[string]any)
		switch m["contact"] {
		case ownerEmail:
			ownerID = m["user_id"].(string)
		case recEmail:
			recID = m["user_id"].(string)
		}
	}
	if r := owner.do("DELETE", "/admin/hotels/"+h1.str("id")+"/staff/"+ownerID, nil); r.status != 403 {
		t.Fatalf("an owner cannot remove an owner: %d", r.status)
	}
	if r := owner.do("DELETE", "/admin/hotels/"+h1.str("id")+"/staff/"+recID, nil); r.status != 200 {
		t.Fatalf("an owner removes their receptionist: %d", r.status)
	}
	if statusOf(recEmail) != "absent" {
		t.Fatalf("the receptionist must be gone")
	}
	// Without a hotel they can no longer sign in to the hotel app.
	if r := signInWithEmailedCode(newClient(t, "partner"), "/auth/otp/request", "/auth/otp/verify", recEmail); r.status != 403 {
		t.Fatalf("a removed receptionist must be refused: %d", r.status)
	}
	// A stranger cannot use the admin page either.
	if r := signInWithEmailedCode(newClient(t, "admin"), "/admin/otp/request", "/admin/otp/verify", realAddress("stranger")); r.status != 403 {
		t.Fatalf("a stranger on the admin page: %d", r.status)
	}
	// An owner edits their own hotel but cannot give it the Verified badge.
	body := newHotelBody("Owner One Renamed")
	body["verified"] = true
	if r := owner.do("PUT", "/admin/hotels/"+h1.str("id"), body); r.status != 200 {
		t.Fatalf("owner edits own hotel: %d %s", r.status, r.raw)
	}
	if d := owner.do("GET", "/admin/hotels/"+h1.str("id"), nil); d.body["verified"] != false || d.body["name"] != "Owner One Renamed" {
		t.Fatalf("name changes, verified does not")
	}
}

func TestDemoAccountsCannotUseTheOwnerAdminPage(t *testing.T) {
	// The demo hotel owner has no inbox and a public code, so the admin page refuses it (the hotel app still accepts it).
	c := newClient(t, "admin")
	c.do("POST", "/admin/otp/request", map[string]string{"type": "email", "value": "owner@bookdan.test"})
	if r := c.do("POST", "/admin/otp/verify", map[string]string{"type": "email", "value": "owner@bookdan.test", "code": "123456"}); r.status != 403 {
		t.Fatalf("a demo account on the admin page: %d %s", r.status, r.raw)
	}
	hotelFor(t) // the hotel app still works for it
}

func TestTheBusinessRecordIsValidatedEncryptedAndItsRevealLogged(t *testing.T) {
	admin := adminClient(t)
	h := admin.do("POST", "/admin/hotels", newHotelBody(fmt.Sprintf("Biz House %d", time.Now().UnixNano()%100000)))
	id := h.str("id")
	for name, mutate := range map[string]func(map[string]any){
		"no legal name":    func(m map[string]any) { m["legal_name"] = "" },
		"no licence":       func(m map[string]any) { m["license_number"] = " " },
		"bad contact mail": func(m map[string]any) { m["contact_email"] = "nope" },
		"bad phone":        func(m map[string]any) { m["contact_phone"] = "abc" },
		"unknown method":   func(m map[string]any) { m["payout_method"] = "bitcoin" },
		"bad account":      func(m map[string]any) { m["payout_account_number"] = "12" },
	} {
		body := businessBody("")
		mutate(body)
		if r := admin.do("PUT", "/admin/hotels/"+id+"/business", body); r.status != 400 {
			t.Errorf("%s must be refused, got %d", name, r.status)
		}
	}
	// A first save needs the account number.
	noNumber := businessBody("")
	noNumber["payout_account_number"] = ""
	if r := admin.do("PUT", "/admin/hotels/"+id+"/business", noNumber); r.status != 400 {
		t.Fatalf("a first save needs the payout account number: %d", r.status)
	}
	if r := admin.do("PUT", "/admin/hotels/"+id+"/business", businessBody("1000 1234 5678")); r.status != 200 {
		t.Fatalf("save: %d %s", r.status, r.raw)
	}

	// The page sees only the last four; the database holds ciphertext.
	d := admin.do("GET", "/admin/hotels/"+id, nil)
	biz := d.body["business"].(map[string]any)
	if biz["payout_last4"] != "5678" || strings.Contains(string(d.raw), "100012345678") || biz["legal_name"] != "Kazanchis Hospitality PLC" {
		t.Fatalf("the business record must show only the last four of the account: %v", biz)
	}
	var enc []byte
	pool.QueryRow(context.Background(), `SELECT payout_enc FROM hotel_business WHERE property_id = $1`, id).Scan(&enc)
	if len(enc) == 0 || strings.Contains(string(enc), "100012345678") {
		t.Fatalf("the payout account must be stored encrypted")
	}

	// Editing without a number keeps the saved one; revealing returns it and is logged; only a platform admin can reveal.
	again := businessBody("")
	again["payout_account_number"] = ""
	again["legal_name"] = "Kazanchis Hospitality Share Company"
	if r := admin.do("PUT", "/admin/hotels/"+id+"/business", again); r.status != 200 {
		t.Fatalf("edit without a number: %d %s", r.status, r.raw)
	}
	rv := admin.do("POST", "/admin/hotels/"+id+"/business/payout/reveal", nil)
	if rv.status != 200 || rv.str("account_number") != "1000 1234 5678" {
		t.Fatalf("reveal: %d %s", rv.status, rv.raw)
	}
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE action = 'admin_payout_revealed' AND details->>'property_id' = $1`, id).Scan(&n)
	if n != 1 {
		t.Fatalf("every reveal must be logged, found %d", n)
	}
	var logged int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE details::text LIKE '%100012345678%' OR details::text LIKE '%1000 1234 5678%'`).Scan(&logged)
	if logged != 0 {
		t.Fatalf("the account number must never appear in the audit log")
	}
	if r := newClient(t, "admin").do("POST", "/admin/hotels/"+id+"/business/payout/reveal", nil); r.status != 401 {
		t.Fatalf("no token: %d", r.status)
	}
}
