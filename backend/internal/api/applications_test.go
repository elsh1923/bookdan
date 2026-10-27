package api_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func applicationBody(hotel, email string) map[string]any {
	return map[string]any{
		"hotel_name": hotel, "hotel_type": "guesthouse", "city": "Addis Ababa", "area": "Kazanchis", "address": "Kazanchis, behind the blue church",
		"hotel_phone": "+251 11 555 0199", "description": "Twelve clean rooms near the centre.", "rooms_count": 12, "pay_at_hotel": true,
		"owner_name": "Selam Haile", "owner_email": email, "owner_phone": "+251 911 000 222", "legal_name": "Selam Hospitality PLC", "license_number": "AA/2/0456/2018",
	}
}

func TestAHotelAppliesAnAdminApprovesAndTheOwnerIsInvitedToADraftHotel(t *testing.T) {
	ctx := context.Background()
	pool.Exec(ctx, `DELETE FROM hotel_applications`)
	defer pool.Exec(ctx, `DELETE FROM hotel_applications`)
	admin := adminClient(t)
	anon := newClient(t, "partner")
	email := realAddress("owner")
	name := fmt.Sprintf("Selam Guesthouse %d", time.Now().UnixNano()%100000)

	adminMails := len(mails.to("admin@bookdan.test"))
	r := anon.do("POST", "/hotel-applications", applicationBody(name, strings.ToUpper(email)))
	if r.status != 200 || !strings.HasPrefix(r.str("reference"), "HA-") {
		t.Fatalf("apply: %d %s", r.status, r.raw)
	}
	if got := mails.to(email); len(got) != 1 || !strings.Contains(got[0].subject, name) {
		t.Errorf("the applicant gets a receipt: %v", got)
	}
	if len(mails.to("admin@bookdan.test")) != adminMails+1 {
		t.Errorf("the admins are told")
	}

	list := admin.do("GET", "/admin/applications", nil)
	var id string
	for _, a := range list.body["applications"].([]any) {
		if m := a.(map[string]any); m["hotel_name"] == name {
			id = m["id"].(string)
			if m["status"] != "pending" || m["owner_email"] != email {
				t.Errorf("the email is stored in lower case and the application is pending: %v", m)
			}
		}
	}
	if id == "" {
		t.Fatalf("the admin sees it: %s", list.raw)
	}
	if x := anon.do("GET", "/admin/applications", nil); x.status != 401 {
		t.Errorf("not public: %d", x.status)
	}

	app := admin.do("POST", "/admin/applications/"+id+"/approve", nil)
	if app.status != 200 || app.str("property_id") == "" {
		t.Fatalf("approve: %d %s", app.status, app.raw)
	}
	prop := app.str("property_id")
	var status string
	var staff int
	pool.QueryRow(ctx, `SELECT status FROM properties WHERE id = $1`, prop).Scan(&status)
	pool.QueryRow(ctx, `SELECT count(*) FROM property_staff ps JOIN users u ON u.id = ps.user_id WHERE ps.property_id = $1 AND ps.role = 'owner' AND u.email = $2`, prop, email).Scan(&staff)
	if status != "draft" || staff != 1 {
		t.Errorf("a draft hotel with the applicant as its owner: %s %d", status, staff)
	}
	if hits := newClient(t, "guest").do("GET", "/search?q="+strings.ReplaceAll(name, " ", "%20"), nil); strings.Contains(string(hits.raw), prop) {
		t.Errorf("a draft is not public")
	}
	var sawInvite, sawApproved bool
	for _, m := range mails.to(email) {
		sawInvite = sawInvite || strings.Contains(m.subject, "You were added to")
		sawApproved = sawApproved || strings.Contains(m.subject, "approved")
	}
	if !sawInvite || !sawApproved {
		t.Errorf("the owner gets the invitation and the good news: %v", mails.to(email))
	}
	// The owner can sign in to the partner app and sees their hotel.
	owner := newClient(t, "partner")
	owner.token = signInWithEmailedCode(owner, "/auth/otp/request", "/auth/otp/verify", email).str("token")
	if p := owner.do("GET", "/partner/bookings", nil); p.status != 200 || p.str("property", "name") != name {
		t.Errorf("the owner reaches their hotel: %d %s", p.status, p.raw)
	}
	if again := admin.do("POST", "/admin/applications/"+id+"/approve", nil); again.status != 409 {
		t.Errorf("approved twice: %d", again.status)
	}
	if again := admin.do("POST", "/admin/applications/"+id+"/reject", map[string]string{"note": "Too late now"}); again.status != 409 {
		t.Errorf("rejecting an approved one: %d", again.status)
	}
}

func TestAnAdminCanRejectAnApplicationAndTheApplicantReadsWhy(t *testing.T) {
	ctx := context.Background()
	pool.Exec(ctx, `DELETE FROM hotel_applications`)
	defer pool.Exec(ctx, `DELETE FROM hotel_applications`)
	admin := adminClient(t)
	anon := newClient(t, "partner")
	email := realAddress("rej")
	name := fmt.Sprintf("Doubtful Inn %d", time.Now().UnixNano()%100000)
	anon.do("POST", "/hotel-applications", applicationBody(name, email))
	var id string
	pool.QueryRow(ctx, `SELECT id FROM hotel_applications WHERE hotel_name = $1`, name).Scan(&id)

	if r := admin.do("POST", "/admin/applications/"+id+"/reject", map[string]string{"note": " "}); r.status != 400 {
		t.Errorf("a reason is required: %d", r.status)
	}
	if r := admin.do("POST", "/admin/applications/"+id+"/reject", map[string]string{"note": "The licence number does not match the register."}); r.status != 200 {
		t.Fatalf("reject: %d %s", r.status, r.raw)
	}
	var last string
	for _, m := range mails.to(email) {
		if strings.Contains(m.subject, "About your application") {
			last = m.text
		}
	}
	if !strings.Contains(last, "does not match the register") {
		t.Errorf("the applicant reads the reason: %q", last)
	}
	var props int
	pool.QueryRow(ctx, `SELECT count(*) FROM properties WHERE name = $1`, name).Scan(&props)
	if props != 0 {
		t.Errorf("a rejected application creates no hotel")
	}
	if x := admin.do("POST", "/admin/applications/00000000-0000-0000-0000-000000000000/reject", map[string]string{"note": "Unknown"}); x.status != 404 {
		t.Errorf("unknown: %d", x.status)
	}
}

func TestTheApplicationFormIsValidatedAndGuardedAgainstBotsAndFlooding(t *testing.T) {
	ctx := context.Background()
	pool.Exec(ctx, `DELETE FROM hotel_applications`)
	defer pool.Exec(ctx, `DELETE FROM hotel_applications`)
	anon := newClient(t, "partner")
	email := realAddress("val")
	ok := func() map[string]any { return applicationBody("Valid Hotel", email) }

	for name, change := range map[string]func(m map[string]any){
		"no name":         func(m map[string]any) { m["hotel_name"] = " " },
		"bad type":        func(m map[string]any) { m["hotel_type"] = "castle" },
		"no city":         func(m map[string]any) { m["city"] = "" },
		"no address":      func(m map[string]any) { m["address"] = "x" },
		"bad hotel phone": func(m map[string]any) { m["hotel_phone"] = "call me" },
		"no rooms":        func(m map[string]any) { m["rooms_count"] = 0 },
		"too many rooms":  func(m map[string]any) { m["rooms_count"] = 5000 },
		"bad email":       func(m map[string]any) { m["owner_email"] = "not-an-email" },
		"no owner phone":  func(m map[string]any) { m["owner_phone"] = "" },
		"no legal name":   func(m map[string]any) { m["legal_name"] = "" },
		"no licence":      func(m map[string]any) { m["license_number"] = "" },
		"long text":       func(m map[string]any) { m["description"] = strings.Repeat("a", 2001) },
	} {
		body := ok()
		change(body)
		if r := anon.do("POST", "/hotel-applications", body); r.status != 400 {
			t.Errorf("%s must be refused: %d %s", name, r.status, r.raw)
		}
	}
	var n int
	pool.QueryRow(ctx, `SELECT count(*) FROM hotel_applications`).Scan(&n)
	if n != 0 {
		t.Fatalf("refused applications leave nothing behind: %d", n)
	}

	// The trap field: a bot is told it worked and nothing is kept (and nobody is emailed).
	bot := ok()
	bot["website"] = "http://spam.example"
	if r := anon.do("POST", "/hotel-applications", bot); r.status != 200 {
		t.Errorf("a bot is told it worked: %d", r.status)
	}
	pool.QueryRow(ctx, `SELECT count(*) FROM hotel_applications`).Scan(&n)
	if n != 0 || len(mails.to(email)) != 0 {
		t.Errorf("a bot's application is dropped: %d rows, %d emails", n, len(mails.to(email)))
	}

	// Two waiting applications per email; the third is refused.
	for i := 0; i < pendingApplicationsPerEmail; i++ {
		b := ok()
		b["hotel_name"] = fmt.Sprintf("Valid Hotel %d", i)
		if r := anon.do("POST", "/hotel-applications", b); r.status != 200 {
			t.Fatalf("application %d: %d %s", i, r.status, r.raw)
		}
	}
	if r := anon.do("POST", "/hotel-applications", ok()); r.status != 409 || r.str("error", "code") != "APPLICATION_WAITING" {
		t.Errorf("too many waiting for one email: %d %s", r.status, r.raw)
	}
	// And a flood from one connection is slowed down.
	pool.Exec(ctx, `UPDATE hotel_applications SET status = 'rejected', decided_at = now(), decided_by = 'test'`)
	var refused bool
	for i := 0; i < 8 && !refused; i++ {
		b := applicationBody(fmt.Sprintf("Flood Hotel %d", i), realAddress(fmt.Sprintf("flood%d", i)))
		if r := anon.do("POST", "/hotel-applications", b); r.status == 429 {
			refused = true
		}
	}
	if !refused {
		t.Errorf("many applications from one connection in an hour are refused")
	}
}

const pendingApplicationsPerEmail = 2

func TestThePublicApplicationPageIsServedWithAStrictPolicy(t *testing.T) {
	for path, want := range map[string]string{"/apply": "text/html", "/apply/apply.js": "javascript", "/apply/style.css": "text/css"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != 200 || !strings.Contains(res.Header.Get("Content-Type"), want) || len(body) == 0 {
			t.Errorf("%s: %d %s", path, res.StatusCode, res.Header.Get("Content-Type"))
		}
		if path == "/apply" {
			csp := res.Header.Get("Content-Security-Policy")
			if !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "frame-ancestors 'none'") || res.Header.Get("X-Frame-Options") != "DENY" {
				t.Errorf("a strict policy: %q", csp)
			}
			if strings.Contains(string(body), "<script>") || strings.Contains(string(body), "onclick=") {
				t.Errorf("no inline script on a page with a strict policy")
			}
		}
	}
}
