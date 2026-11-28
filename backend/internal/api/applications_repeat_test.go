package api_test

import (
	"context"
	"strings"
	"testing"
)

// The same hotel sent twice is refused while the first is waiting; a different hotel from the same owner is still welcome.
func TestTheSameHotelCannotBeAppliedForTwice(t *testing.T) {
	ctx := context.Background()
	pool.Exec(ctx, `DELETE FROM hotel_applications`)
	defer pool.Exec(ctx, `DELETE FROM hotel_applications`)
	anon := newClient(t, "guest")
	email := realAddress("repeat")
	if r := anon.do("POST", "/hotel-applications", applicationBody("Repeat Inn", email)); r.status != 200 {
		t.Fatalf("first: %d %s", r.status, r.raw)
	}
	r := anon.do("POST", "/hotel-applications", applicationBody("  repeat INN ", strings.ToUpper(email)))
	if r.status != 409 || r.str("error", "code") != "APPLICATION_WAITING" || !strings.Contains(r.str("error", "message"), "already sent an application for this hotel") {
		t.Fatalf("the same hotel again: %d %s", r.status, r.raw)
	}
	if r := anon.do("POST", "/hotel-applications", applicationBody("Repeat Inn Annex", email)); r.status != 200 {
		t.Fatalf("a second, different hotel: %d %s", r.status, r.raw)
	}
}
