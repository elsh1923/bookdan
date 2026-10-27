package ops

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu   sync.Mutex
	subj []string
	body []string
}

func (r *recorder) send(_ context.Context, subject, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.subj, r.body = append(r.subj, subject), append(r.body, body)
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.subj)
}

func TestTheSameProblemAlertsOncePerQuietPeriodAndCountsTheRest(t *testing.T) {
	reset()
	defer reset()
	rec := &recorder{}
	Configure(rec.send)
	clock := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	global.now = func() time.Time { return clock }

	for i := 0; i < 50; i++ {
		Report("api-500 POST /v1/bookings", "Server error", "request abc")
	}
	Wait()
	if rec.count() != 1 {
		t.Fatalf("fifty identical failures are one alert: %d", rec.count())
	}
	Report("refund-failed", "A refund cannot be paid", "payment p1") // a different problem is not held back
	Wait()
	if rec.count() != 2 {
		t.Fatalf("another kind alerts at once: %d", rec.count())
	}
	clock = clock.Add(Throttle + time.Minute)
	Report("api-500 POST /v1/bookings", "Server error", "request xyz")
	Wait()
	if rec.count() != 3 || !strings.Contains(rec.body[2], "49 more of the same") {
		t.Fatalf("after the quiet period it alerts again and says how many were missed: %d %q", rec.count(), rec.body)
	}
	if !strings.HasPrefix(rec.subj[0], "BookDan: ") {
		t.Errorf("subjects are recognisable: %q", rec.subj[0])
	}
}

func TestNoMoreThanTheHourlyCapIsSent(t *testing.T) {
	reset()
	defer reset()
	rec := &recorder{}
	Configure(rec.send)
	for i := 0; i < HourlyCap+15; i++ {
		Report("kind-"+string(rune('a'+i%26))+string(rune('a'+i/26)), "Something", "x")
	}
	Wait()
	if rec.count() != HourlyCap {
		t.Fatalf("a storm of different problems is capped: %d", rec.count())
	}
}

func TestWithoutADestinationAlertsAreOnlyLoggedAndNothingBreaks(t *testing.T) {
	reset()
	defer reset()
	Configure()
	Report("x", "No one to tell", "nothing happens")
	Wait()
	Configure(nil, nil)
	Report("y", "Still no one", "")
	Wait()
}

func TestAFailingDestinationNeverBlocksOrCrashes(t *testing.T) {
	reset()
	defer reset()
	Configure(func(context.Context, string, string) error { return errors.New("slack is down") })
	start := time.Now()
	Report("z", "Subject", "detail")
	if time.Since(start) > 200*time.Millisecond {
		t.Errorf("Report returns at once")
	}
	Wait()
}

func TestTheWebhookPostsTheMessageForSlackAndDiscord(t *testing.T) {
	var got map[string]string
	var ctype string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ctype = r.Header.Get("Content-Type")
		json.Unmarshal(b, &got)
	}))
	defer srv.Close()
	if err := Webhook(srv.URL)(context.Background(), "BookDan: a problem", "details here"); err != nil {
		t.Fatal(err)
	}
	if ctype != "application/json" || got["text"] != got["content"] || !strings.Contains(got["text"], "a problem") || !strings.Contains(got["text"], "details here") {
		t.Errorf("one message under both keys: %v %s", got, ctype)
	}
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusForbidden) }))
	defer bad.Close()
	if err := Webhook(bad.URL)(context.Background(), "s", "b"); err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("a refusing webhook is an error: %v", err)
	}
	long := strings.Repeat("x", 5000)
	Webhook(srv.URL)(context.Background(), "s", long)
	if len(got["text"]) > 1810 {
		t.Errorf("a long message is cut: %d", len(got["text"]))
	}
}

func TestMailSendsToEveryAddress(t *testing.T) {
	var to []string
	send := func(_ context.Context, addr, subject, text, html string) error {
		to = append(to, addr)
		if addr == "bad@x.test" {
			return errors.New("refused")
		}
		return nil
	}
	err := Mail(send, []string{"a@x.test", "bad@x.test", "b@x.test"})(context.Background(), "s", "b")
	if len(to) != 3 || err == nil {
		t.Errorf("every address is tried and the first failure is reported: %v %v", to, err)
	}
}
