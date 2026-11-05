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

// flaky is a push service and a mail service that fail a set number of times and then work.
type flaky struct {
	mu       sync.Mutex
	failures int
	calls    int
	pushed   []message
	mailed   []sentMail
}

func (f *flaky) Notify(_ context.Context, tokens []string, title, body string, data map[string]string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.failures > 0 {
		f.failures--
		return nil, errors.New("the push service is down")
	}
	f.pushed = append(f.pushed, message{tokens, title, body, data})
	return nil, nil
}

func (f *flaky) SendMail(_ context.Context, to, subject, text, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.failures > 0 {
		f.failures--
		return errors.New("the mail service is down")
	}
	f.mailed = append(f.mailed, sentMail{to, subject, text})
	return nil
}

func (f *flaky) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func serverWith(t *testing.T, f *flaky) *api.Server {
	t.Helper()
	cfg := config.Config{Env: "test", DemoMode: true, PublicURL: "http://example.test", PaymentProvider: "dev", PaymentWebhookSecret: "whsec"}
	providers := api.DevProviders(cfg)
	providers.Push, providers.Mail = f, f
	s, err := api.New(context.Background(), cfg, pool, keys, providers)
	if err != nil {
		t.Fatal(err)
	}
	srv2 := httptest.NewServer(server.New(cfg, pool, func(r chi.Router) { s.Routes(r) }))
	t.Cleanup(srv2.Close)
	return s
}

type outboxRow struct {
	attempts int
	sent     bool
	failed   bool
	payload  string
	lastErr  string
}

func outboxID(t *testing.T, tag string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `SELECT id FROM outbox WHERE payload::text LIKE '%' || $1 || '%' ORDER BY id DESC LIMIT 1`, tag).Scan(&id); err != nil {
		t.Fatalf("no outbox row for %s: %v", tag, err)
	}
	return id
}

func outboxRowByID(t *testing.T, id int64) outboxRow {
	t.Helper()
	var r outboxRow
	var lastErr *string
	if err := pool.QueryRow(context.Background(), `SELECT attempts, sent_at IS NOT NULL, failed_at IS NOT NULL, payload::text, last_error FROM outbox WHERE id = $1`, id).Scan(&r.attempts, &r.sent, &r.failed, &r.payload, &lastErr); err != nil {
		t.Fatalf("outbox row %d: %v", id, err)
	}
	if lastErr != nil {
		r.lastErr = *lastErr
	}
	return r
}

func makeDue(tag string) {
	pool.Exec(context.Background(), `UPDATE outbox SET next_attempt_at = now() WHERE sent_at IS NULL AND failed_at IS NULL AND payload::text LIKE '%' || $1 || '%'`, tag)
}

func TestAFailedAlertIsTriedAgainUntilItGetsThrough(t *testing.T) {
	ctx := context.Background()
	f := &flaky{failures: 2}
	s := serverWith(t, f)
	tag := fmt.Sprintf("tok-retry-%d", time.Now().UnixNano())

	s.QueuePush(ctx, []string{tag}, "New booking request", "Guest", map[string]string{"channel": "booking_alerts"})
	id := outboxID(t, tag)
	if r := outboxRowByID(t, id); r.sent || r.attempts != 1 || r.lastErr == "" {
		t.Fatalf("the first attempt failed and the message waits: %+v", r)
	}
	s.DrainOutbox(ctx)
	if f.callCount() != 1 {
		t.Fatalf("a message that is not due yet is left alone: %d calls", f.callCount())
	}

	makeDue(tag)
	s.DrainOutbox(ctx) // attempt 2 fails too
	if r := outboxRowByID(t, id); r.sent || r.attempts != 2 {
		t.Fatalf("second attempt: %+v", r)
	}
	makeDue(tag)
	s.DrainOutbox(ctx) // attempt 3 works
	r := outboxRowByID(t, id)
	if !r.sent || r.attempts != 3 {
		t.Fatalf("third attempt: %+v", r)
	}
	if r.payload != "{}" {
		t.Errorf("the text of a delivered message is erased: %s", r.payload)
	}
	if len(f.pushed) != 1 || f.pushed[0].tokens[0] != tag || f.pushed[0].title != "New booking request" {
		t.Fatalf("delivered exactly once with its content: %+v", f.pushed)
	}
	// And it is not sent again.
	makeDue(tag)
	s.DrainOutbox(ctx)
	if len(f.pushed) != 1 {
		t.Fatalf("a delivered message is never sent again: %d", len(f.pushed))
	}
}

func TestAMessageThatIsNoLongerWorthSendingIsDropped(t *testing.T) {
	ctx := context.Background()
	f := &flaky{failures: 100}
	s := serverWith(t, f)
	tag := fmt.Sprintf("tok-expire-%d", time.Now().UnixNano())
	s.QueuePush(ctx, []string{tag}, "New booking request", "Guest", map[string]string{"channel": "booking_alerts"})
	id := outboxID(t, tag)
	// A booking alert is worth 15 minutes; pretend they have passed.
	pool.Exec(ctx, `UPDATE outbox SET expires_at = now() - interval '1 minute', next_attempt_at = now() WHERE payload::text LIKE '%' || $1 || '%'`, tag)
	before := f.callCount()
	s.DrainOutbox(ctx)
	r := outboxRowByID(t, id)
	if f.callCount() != before {
		t.Fatalf("an expired alert is not sent")
	}
	if !r.failed || r.lastErr == "" || r.sent {
		t.Fatalf("it is closed with the reason: %+v", r)
	}
}

func TestAMessageIsGivenUpAfterEightFailures(t *testing.T) {
	ctx := context.Background()
	f := &flaky{failures: 100}
	s := serverWith(t, f)
	to := fmt.Sprintf("giveup-%d@realmail.dev", time.Now().UnixNano())
	if err := s.QueueMail(ctx, to, "Subject", "Text", ""); err != nil {
		t.Fatal(err)
	}
	id := outboxID(t, to)
	for i := 0; i < 7; i++ {
		makeDue(to)
		s.DrainOutbox(ctx)
	}
	r := outboxRowByID(t, id)
	if r.attempts != 8 || !r.failed || r.sent {
		t.Fatalf("eight attempts, then it is given up: %+v", r)
	}
	if f.callCount() != 8 {
		t.Fatalf("exactly eight sends: %d", f.callCount())
	}
	makeDue(to)
	s.DrainOutbox(ctx)
	if f.callCount() != 8 {
		t.Fatalf("a given-up message is left alone: %d", f.callCount())
	}
}

func TestAMessageLeftBehindByAStoppedServerIsSent(t *testing.T) {
	ctx := context.Background()
	f := &flaky{}
	s := serverWith(t, f)
	to := fmt.Sprintf("crash-%d@realmail.dev", time.Now().UnixNano())
	// A server wrote the message, claimed it, and stopped before it finished: attempts is 1 and the claim has run out.
	if _, err := pool.Exec(ctx, `INSERT INTO outbox (kind, payload, expires_at, attempts, next_attempt_at)
		VALUES ('email', jsonb_build_object('to', $1::text, 'subject', 'Left behind', 'text', 'Hello', 'html', ''), now() + interval '1 hour', 1, now() - interval '1 minute')`, to); err != nil {
		t.Fatal(err)
	}
	s.DrainOutbox(ctx)
	if len(f.mailed) != 1 || f.mailed[0].to != to || f.mailed[0].subject != "Left behind" {
		t.Fatalf("the next pass sends it: %+v", f.mailed)
	}
}

func TestServersDrainingTogetherSendEachMessageOnce(t *testing.T) {
	ctx := context.Background()
	f := &flaky{}
	servers := []*api.Server{serverWith(t, f), serverWith(t, f), serverWith(t, f)}
	tag := fmt.Sprintf("many-%d", time.Now().UnixNano())
	for i := 0; i < 30; i++ {
		if _, err := pool.Exec(ctx, `INSERT INTO outbox (kind, payload, expires_at, attempts, next_attempt_at)
			VALUES ('email', jsonb_build_object('to', $1::text, 'subject', 'Bulk', 'text', 'x', 'html', ''), now() + interval '1 hour', 0, now() - interval '1 minute')`, fmt.Sprintf("%s-%d@realmail.dev", tag, i)); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for _, s := range servers {
		wg.Add(1)
		go func(s *api.Server) {
			defer wg.Done()
			s.DrainOutbox(ctx)
		}(s)
	}
	wg.Wait()
	seen := map[string]int{}
	for _, m := range f.mailed {
		seen[m.to]++
	}
	if len(seen) != 30 {
		t.Fatalf("all thirty are sent: %d", len(seen))
	}
	for to, n := range seen {
		if n != 1 {
			t.Errorf("%s was sent %d times", to, n)
		}
	}
}
