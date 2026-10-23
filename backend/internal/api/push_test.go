package api_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- push alerts (uses the shared test server in api_test.go)

type message struct {
	tokens      []string
	title, body string
	data        map[string]string
}

// recorder stands in for Firebase: it remembers every alert and reports the token "dead-token" as unregistered.
type recorder struct {
	mu   sync.Mutex
	msgs []message
}

func (r *recorder) Notify(_ context.Context, tokens []string, title, body string, data map[string]string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, message{tokens, title, body, data})
	var invalid []string
	for _, t := range tokens {
		if t == "dead-token" {
			invalid = append(invalid, t)
		}
	}
	return invalid, nil
}

// to returns the alerts sent to one phone token.
func (r *recorder) to(token string) []message {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []message
	for _, m := range r.msgs {
		for _, t := range m.tokens {
			if t == token {
				out = append(out, m)
			}
		}
	}
	return out
}

func registerToken(c *client, path, token string) resp {
	return c.do("PUT", path, map[string]string{"token": token, "token_type": "fcm", "device_id": c.deviceID, "platform": "android", "app_version": "1.0.0"})
}

func TestTheGuestIsToldWhatTheHotelDecided(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("p"))
	withProfile(guest)
	hotel := hotelFor(t)
	gtoken := fmt.Sprintf("guest-phone-%d", time.Now().UnixNano())
	htoken := fmt.Sprintf("hotel-phone-%d", time.Now().UnixNano())
	if r := registerToken(guest, "/me/devices/push-token", gtoken); r.status != 200 {
		t.Fatalf("guest registers a phone: %d %s", r.status, r.raw)
	}
	if r := registerToken(hotel, "/partner/devices/push-token", htoken); r.status != 200 {
		t.Fatalf("hotel registers a phone: %d %s", r.status, r.raw)
	}
	if r := registerToken(hotel, "/me/devices/push-token", "x"); r.status != 401 {
		t.Fatalf("a hotel token must not register as a guest phone: %d", r.status)
	}
	if r := guest.do("PUT", "/me/devices/push-token", map[string]string{"token": "", "token_type": "fcm", "device_id": guest.deviceID, "platform": "android"}); r.status != 400 {
		t.Fatalf("an empty token is refused: %d", r.status)
	}

	// 1. The guest pays: the HOTEL is alerted on its loud channel, the guest is not.
	q := quote(guest, meskel, standard, 100, 101, 1, 1)
	b := hold(guest, q.str("id"), "key-push-1")
	payOnline(t, guest, b.str("id"), "succeeded")
	alerts := push.to(htoken)
	if len(alerts) != 1 || alerts[0].data["type"] != "new_booking" || alerts[0].data["booking_id"] != b.str("id") || alerts[0].data["channel"] != "booking_alerts" {
		t.Fatalf("the hotel should get one new-booking alert: %+v", alerts)
	}
	if len(push.to(gtoken)) != 0 {
		t.Fatalf("the guest must not be alerted for their own payment")
	}

	// 2. The hotel accepts: the guest is told, with the booking to open.
	hotel.do("POST", "/partner/bookings/"+b.str("id")+"/accept", nil)
	got := push.to(gtoken)
	if len(got) != 1 || got[0].title != "Booking confirmed" || got[0].data["status"] != "confirmed" || got[0].data["booking_id"] != b.str("id") ||
		got[0].data["type"] != "booking_update" || !strings.Contains(got[0].body, "Meskel Square Palace Hotel") {
		t.Fatalf("the guest should hear that it is confirmed: %+v", got)
	}

	// 3. The hotel declines another one.
	q2 := quote(guest, meskel, standard, 102, 103, 1, 1)
	b2 := hold(guest, q2.str("id"), "key-push-2")
	payOnline(t, guest, b2.str("id"), "succeeded")
	hotel.do("POST", "/partner/bookings/"+b2.str("id")+"/decline", map[string]string{"reason": "fully_booked"})
	if got := push.to(gtoken); len(got) != 2 || got[1].data["status"] != "declined" || !strings.Contains(got[1].body, "refund") {
		t.Fatalf("the guest should hear about the decline and the refund: %+v", got)
	}

	// 4. The hotel does not answer: the sweeper closes it and the guest is told, once.
	q3 := quote(guest, meskel, standard, 104, 105, 1, 1)
	b3 := hold(guest, q3.str("id"), "key-push-3")
	payOnline(t, guest, b3.str("id"), "succeeded")
	pool.Exec(context.Background(), `UPDATE bookings SET hotel_deadline_at = now() - interval '1 minute' WHERE id = $1`, b3.str("id"))
	app.Sweep(context.Background())
	got = push.to(gtoken)
	if len(got) != 3 || got[2].data["status"] != "no_response" {
		t.Fatalf("the guest should hear that the hotel did not answer: %+v", got)
	}
	app.Sweep(context.Background())
	if len(push.to(gtoken)) != 3 {
		t.Fatalf("the guest must be told only once")
	}
}

func TestPhonesThatFirebaseForgetsAreRemoved(t *testing.T) {
	guest := newClient(t, "guest")
	guest.signIn("email", uniqueEmail("q"))
	withProfile(guest)
	hotel := hotelFor(t)
	registerToken(guest, "/me/devices/push-token", "dead-token")
	q := quote(guest, meskel, standard, 110, 111, 1, 1)
	b := hold(guest, q.str("id"), "key-push-dead")
	payOnline(t, guest, b.str("id"), "succeeded")
	hotel.do("POST", "/partner/bookings/"+b.str("id")+"/accept", nil)
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM devices WHERE push_token = 'dead-token'`).Scan(&n)
	if n != 0 {
		t.Fatalf("an unregistered token should be forgotten, %d remain", n)
	}
}

func TestSigningOutEndsTheSessionAndSilencesThePhone(t *testing.T) {
	hotel := hotelFor(t)
	token := fmt.Sprintf("signout-phone-%d", time.Now().UnixNano())
	registerToken(hotel, "/partner/devices/push-token", token)

	// Signing out revokes the refresh token: it can no longer be exchanged for a new session.
	if r := hotel.do("POST", "/auth/logout", map[string]string{"refresh_token": hotel.refresh}); r.status != 200 {
		t.Fatalf("logout: %d %s", r.status, r.raw)
	}
	if r := hotel.do("POST", "/auth/refresh", map[string]string{"refresh_token": hotel.refresh}); r.status != 401 {
		t.Fatalf("a refresh token must not work after sign-out: %d", r.status)
	}
	// The phone is forgotten: it no longer rings for this account.
	var n int
	pool.QueryRow(context.Background(), `SELECT count(*) FROM devices WHERE push_token = $1`, token).Scan(&n)
	if n != 0 {
		t.Fatalf("the signed-out phone must lose its alert registration")
	}
	// It answers the same for a repeat, an unknown token and an empty one, so it cannot be used to probe for valid tokens.
	for _, body := range []map[string]string{{"refresh_token": hotel.refresh}, {"refresh_token": "never-issued"}, {"refresh_token": ""}} {
		if r := hotel.do("POST", "/auth/logout", body); r.status != 200 {
			t.Fatalf("logout with %q: %d", body["refresh_token"], r.status)
		}
	}
}
