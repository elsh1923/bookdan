package api_test

import (
	"context"
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

// secondServer is another process of the same API on the same database.
func secondServer(t *testing.T) *api.Server {
	t.Helper()
	cfg := config.Config{Env: "test", DemoMode: true, PublicURL: "http://example.test", PaymentProvider: "dev", PaymentWebhookSecret: "whsec"}
	a2, err := api.New(context.Background(), cfg, pool, keys, api.DevProviders(cfg))
	if err != nil {
		t.Fatal(err)
	}
	s2 := httptest.NewServer(server.New(cfg, pool, func(r chi.Router) { a2.Routes(r) }))
	t.Cleanup(s2.Close)
	return a2
}

func TestOnlyOneServerHoldsTheSweeperLeaseAtATime(t *testing.T) {
	ctx := context.Background()
	other := secondServer(t)
	name := fmt.Sprintf("test-lease-%d", time.Now().UnixNano())
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM job_leases WHERE name = $1`, name) })

	if !app.HoldLease(ctx, name, time.Minute) {
		t.Fatal("the first server takes a free lease")
	}
	if other.HoldLease(ctx, name, time.Minute) {
		t.Fatal("a second server must not take a lease that is held")
	}
	if !app.HoldLease(ctx, name, time.Minute) {
		t.Fatal("the holder renews its own lease")
	}

	// A server that shuts down gives it back at once.
	app.ReleaseLease(ctx, name)
	if !other.HoldLease(ctx, name, time.Minute) {
		t.Fatal("after a release the other server takes over without waiting")
	}
	if app.HoldLease(ctx, name, time.Minute) {
		t.Fatal("and the first no longer holds it")
	}

	// A server that dies (never releases) is replaced once its lease runs out.
	name2 := name + "-b"
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM job_leases WHERE name = $1`, name2) })
	if !app.HoldLease(ctx, name2, 150*time.Millisecond) {
		t.Fatal("take a short lease")
	}
	if other.HoldLease(ctx, name2, time.Minute) {
		t.Fatal("not yet expired")
	}
	time.Sleep(250 * time.Millisecond)
	if !other.HoldLease(ctx, name2, time.Minute) {
		t.Fatal("an expired lease is taken over")
	}
}

func TestManyServersRacingForALeaseGiveItToExactlyOne(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("test-race-%d", time.Now().UnixNano())
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM job_leases WHERE name = $1`, name) })
	servers := []*api.Server{app, secondServer(t), secondServer(t), secondServer(t)}
	var wg sync.WaitGroup
	var mu sync.Mutex
	winners := 0
	for _, s := range servers {
		wg.Add(1)
		go func(s *api.Server) {
			defer wg.Done()
			if s.HoldLease(ctx, name, time.Minute) {
				mu.Lock()
				winners++
				mu.Unlock()
			}
		}(s)
	}
	wg.Wait()
	if winners != 1 {
		t.Fatalf("exactly one server holds the lease, got %d", winners)
	}
}

func TestADailyJobRunsOncePerDayInTotal(t *testing.T) {
	ctx := context.Background()
	other := secondServer(t)
	name := fmt.Sprintf("test-job-%d", time.Now().UnixNano())
	t.Cleanup(func() { pool.Exec(ctx, `DELETE FROM job_runs WHERE name = $1`, name) })
	if !app.ClaimDay(ctx, name, "2030-01-01") {
		t.Fatal("the first server to ask runs the job")
	}
	if other.ClaimDay(ctx, name, "2030-01-01") || app.ClaimDay(ctx, name, "2030-01-01") {
		t.Fatal("nobody else runs it that day")
	}
	if !other.ClaimDay(ctx, name, "2030-01-02") {
		t.Fatal("the next day is a new claim")
	}
}
