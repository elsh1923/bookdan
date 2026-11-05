package api_test

import (
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/bookdan/backend/internal/api"
	"github.com/bookdan/backend/internal/config"
	"github.com/bookdan/backend/internal/server"
)

type refundCall struct {
	ref, key string
	amount   int64
}

// refundGateway fails a set number of refunds and then pays them; it records every call, and can run a check while a refund is "in flight".
type refundGateway struct {
	api.Gateway
	mu       sync.Mutex
	failures int
	calls    []refundCall
	inFlight func()
}

func (g *refundGateway) Refund(_ context.Context, ref, key string, amount int64, _ string) error {
	if g.inFlight != nil {
		g.inFlight()
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.calls = append(g.calls, refundCall{ref, key, amount})
	if g.failures > 0 {
		g.failures--
		return errors.New("the gateway refused the refund")
	}
	return nil
}

func (g *refundGateway) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.calls)
}

func serverWithGateway(t *testing.T, g *refundGateway) *api.Server {
	t.Helper()
	cfg := config.Config{Env: "test", DemoMode: true, PublicURL: "http://example.test", PaymentProvider: "dev", PaymentWebhookSecret: "whsec"}
	providers := api.DevProviders(cfg)
	g.Gateway = providers.Gateway
	providers.Gateway = g
	s, err := api.New(context.Background(), cfg, pool, keys, providers)
	if err != nil {
		t.Fatal(err)
	}
	srv2 := httptest.NewServer(server.New(cfg, pool, func(r chi.Router) { s.Routes(r) }))
	t.Cleanup(srv2.Close)
	return s
}

// owedRefund makes a booking whose deposit is owed back, without letting the main server refund it: the refund is claimed for an hour while the hotel declines,
// then released, so the test decides which server pays it and how.
func owedRefund(t *testing.T, night int, key string) (g *client, id string) {
	t.Helper()
	g, _ = supportGuest(t, "rfc")
	b := hold(g, quote(g, meskel, standard, night, night+1, 1, 1).str("id"), key)
	id = b.str("id")
	payOnline(t, g, id, "succeeded")
	pool.Exec(context.Background(), `UPDATE payments SET refund_claimed_until = now() + interval '1 hour' WHERE booking_id = $1`, id)
	if r := hotelFor(t).do("POST", "/partner/bookings/"+id+"/decline", map[string]string{"reason": "fully_booked"}); r.status != 200 {
		t.Fatalf("decline: %d %s", r.status, r.raw)
	}
	pool.Exec(context.Background(), `UPDATE payments SET refund_claimed_until = NULL WHERE booking_id = $1`, id)
	return g, id
}

func paymentState(id string) (status string, attempts int, frozen *int64, started bool) {
	pool.QueryRow(context.Background(), `SELECT status, refund_attempts, refund_amount_santim, refund_started_at IS NOT NULL FROM payments WHERE booking_id = $1`, id).Scan(&status, &attempts, &frozen, &started)
	return
}

func makeRefundDue(id string) {
	pool.Exec(context.Background(), `UPDATE payments SET refund_claimed_until = now() WHERE booking_id = $1`, id)
}

func TestARefundTheGatewayRefusesIsRetriedForTheSameAmountAndPaidOnce(t *testing.T) {
	ctx := context.Background()
	gw := &refundGateway{failures: 2}
	s := serverWithGateway(t, gw)
	_, id := owedRefund(t, 345, fmt.Sprintf("rfc-1-%d", time.Now().UnixNano()))
	want := paidOf(id)

	s.Sweep(ctx) // attempt 1 fails
	status, attempts, frozen, started := paymentState(id)
	if status != "succeeded" || attempts != 1 || frozen == nil || *frozen != want || !started {
		t.Fatalf("a refused refund stays owed, with its amount fixed: %s attempts %d frozen %v started %v (want %d)", status, attempts, frozen, started, want)
	}
	s.Sweep(ctx)
	if gw.count() != 1 {
		t.Fatalf("a refund that was just tried waits before it is asked again: %d calls", gw.count())
	}
	makeRefundDue(id)
	s.Sweep(ctx) // attempt 2 fails
	makeRefundDue(id)
	s.Sweep(ctx) // attempt 3 works
	if status, _, _, _ := paymentState(id); status != "refunded" {
		t.Fatalf("the third try pays it: %s", status)
	}
	if gw.count() != 3 {
		t.Fatalf("three asks: %d", gw.count())
	}
	for _, c := range gw.calls {
		if c.amount != want || c.ref != gw.calls[0].ref || c.key != "" {
			t.Fatalf("every try asks for the same payment and amount: %+v (want %d)", c, want)
		}
	}
	var entries int
	pool.QueryRow(ctx, `SELECT count(*) FROM ledger_transactions WHERE booking_id = $1 AND kind = 'refund' AND memo = 'refund paid to the guest'`, id).Scan(&entries)
	if entries != 1 {
		t.Fatalf("the guest is paid once, so the ledger records it once: %d", entries)
	}
	makeRefundDue(id)
	s.Sweep(ctx)
	if gw.count() != 3 {
		t.Fatalf("a refunded payment is left alone: %d", gw.count())
	}
}

func TestNothingIsHeldWhileTheGatewayIsAsked(t *testing.T) {
	ctx := context.Background()
	gw := &refundGateway{}
	s := serverWithGateway(t, gw)
	_, id := owedRefund(t, 346, fmt.Sprintf("rfc-2-%d", time.Now().UnixNano()))
	var checked bool
	var lockErr error
	gw.inFlight = func() {
		// While the gateway is being asked, the payment can be changed by someone else at once: no lock is held.
		c, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		_, lockErr = pool.Exec(c, `UPDATE payments SET refund_last_error = 'probe' WHERE booking_id = $1`, id)
		checked = true
	}
	s.Sweep(ctx)
	if !checked {
		t.Fatal("the gateway was never asked")
	}
	if lockErr != nil {
		t.Fatalf("a refund must not hold a lock on the payment while it waits for the gateway: %v", lockErr)
	}
	if status, _, _, _ := paymentState(id); status != "refunded" {
		t.Fatalf("and it is still paid: %s", status)
	}
}

func TestARefundLeftHalfDoneByAStoppedServerIsFinishedWithTheSameAmount(t *testing.T) {
	ctx := context.Background()
	gw := &refundGateway{}
	s := serverWithGateway(t, gw)
	_, id := owedRefund(t, 347, fmt.Sprintf("rfc-3-%d", time.Now().UnixNano()))
	want := paidOf(id)
	// A server claimed the refund, asked the gateway, and stopped before it recorded anything: the claim has run out.
	pool.Exec(ctx, `UPDATE payments SET refund_amount_santim = $2, refund_started_at = now() - interval '5 minutes', refund_claimed_until = now() - interval '3 minutes', refund_attempts = 1 WHERE booking_id = $1`, id, want)
	s.Sweep(ctx)
	if status, _, _, _ := paymentState(id); status != "refunded" {
		t.Fatalf("the next pass finishes it: %s", status)
	}
	if gw.count() != 1 || gw.calls[0].amount != want {
		t.Fatalf("it asks once more for the amount decided at the start: %+v", gw.calls)
	}
}

func TestOnlyOneServerRefundsAPaymentAtATime(t *testing.T) {
	ctx := context.Background()
	gw := &refundGateway{}
	servers := []*api.Server{serverWithGateway(t, gw), serverWithGateway(t, gw), serverWithGateway(t, gw)}
	_, id := owedRefund(t, 348, fmt.Sprintf("rfc-4-%d", time.Now().UnixNano()))
	var wg sync.WaitGroup
	for _, s := range servers {
		wg.Add(1)
		go func(s *api.Server) {
			defer wg.Done()
			s.Sweep(ctx)
		}(s)
	}
	wg.Wait()
	var mine int
	for _, c := range gw.calls {
		if c.amount == paidOf(id) {
			mine++
		}
	}
	if mine != 1 {
		t.Fatalf("three servers sweeping together refund the guest once, not %d times", mine)
	}
	if status, _, _, _ := paymentState(id); status != "refunded" {
		t.Fatalf("%s", status)
	}
}

func TestAPartialRefundCannotBeAddedOnceTheFullRefundHasStarted(t *testing.T) {
	g, _ := supportGuest(t, "rfc5")
	hotel := hotelFor(t)
	admin := adminClient(t)
	id, tid := bookingWithTicket(t, g, hotel, 101, fmt.Sprintf("rfc-5-%d", time.Now().UnixNano()))
	pool.Exec(context.Background(), `UPDATE payments SET refund_started_at = now() WHERE booking_id = $1`, id)
	if r := goodwill(admin, tid, "platform", paidOf(id)/4, "A goodwill credit."); r.status == 200 {
		t.Fatalf("money that is already being refunded in full cannot also be refunded in part: %s", r.raw)
	}
	if owed, paid := partialRows(id); owed+paid != 0 {
		t.Fatalf("nothing was created: %d %d", owed, paid)
	}
}
