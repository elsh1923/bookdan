// Package ops tells the people who run BookDan when something is wrong: an unhandled panic, a burst of server errors, a refund that cannot be paid,
// a background job that fails. An alert goes to a chat webhook (Slack, Discord and Teams all accept it) and/or to email. Alerts are throttled so a
// broken dependency produces one message every 15 minutes per problem instead of thousands, and capped per hour overall.
//
// An alert carries what happened and where (a route, a job, a request id), never a request body, a header or a token.
package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Sender delivers one alert. It must be safe to call from many goroutines.
type Sender func(ctx context.Context, subject, body string) error

const (
	// Throttle is the quiet time after an alert of a given kind, during which the same problem is only counted.
	Throttle = 15 * time.Minute
	// HourlyCap is the most alerts sent in any hour, whatever their kind.
	HourlyCap = 20
)

type notifier struct {
	mu      sync.Mutex
	senders []Sender
	last    map[string]time.Time
	skipped map[string]int
	sent    []time.Time
	now     func() time.Time
	wg      sync.WaitGroup
}

var global = &notifier{last: map[string]time.Time{}, skipped: map[string]int{}, now: time.Now}

// Configure sets where alerts go (replacing earlier senders). With none, alerts are only logged.
func Configure(senders ...Sender) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.senders = nil
	for _, s := range senders {
		if s != nil {
			global.senders = append(global.senders, s)
		}
	}
}

// Report raises an alert of a kind (one kind is one problem: "api-500 POST /v1/bookings", "refund-failed"). detail is added to the message.
// It returns at once; delivery happens in the background.
func Report(kind, subject, detail string) {
	global.report(kind, subject, detail)
}

func (n *notifier) report(kind, subject, detail string) {
	slog.Error("alert", "kind", kind, "subject", subject)
	n.mu.Lock()
	now := n.now()
	if last, ok := n.last[kind]; ok && now.Sub(last) < Throttle {
		n.skipped[kind]++
		n.mu.Unlock()
		return
	}
	// The overall cap: forget what is older than an hour, then see whether there is room.
	cut := now.Add(-time.Hour)
	kept := n.sent[:0]
	for _, t := range n.sent {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	n.sent = kept
	if len(n.sent) >= HourlyCap {
		n.skipped[kind]++
		n.mu.Unlock()
		return
	}
	n.last[kind] = now
	n.sent = append(n.sent, now)
	repeats := n.skipped[kind]
	n.skipped[kind] = 0
	senders := append([]Sender(nil), n.senders...)
	n.mu.Unlock()

	if len(senders) == 0 {
		return
	}
	body := detail
	if repeats > 0 {
		body += fmt.Sprintf("\n\n(%d more of the same since the last alert)", repeats)
	}
	for _, send := range senders {
		n.wg.Add(1)
		go func() {
			defer n.wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := send(ctx, "BookDan: "+subject, body); err != nil {
				slog.Error("sending an alert failed", "kind", kind, "err", err)
			}
		}()
	}
}

// Wait blocks until the alerts already raised have been delivered (used on shutdown and in tests).
func Wait() { global.wg.Wait() }

// reset clears the throttle (tests).
func reset() {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.last, global.skipped, global.sent, global.now = map[string]time.Time{}, map[string]int{}, nil, time.Now
}

// Webhook returns a Sender that posts {"text": "...", "content": "..."} to a chat webhook (Slack reads text, Discord reads content, Teams reads text).
func Webhook(url string) Sender {
	client := &http.Client{Timeout: 8 * time.Second}
	return func(ctx context.Context, subject, body string) error {
		msg := subject
		if body != "" {
			msg += "\n" + body
		}
		if len(msg) > 1800 {
			msg = msg[:1800] + "…"
		}
		payload, _ := json.Marshal(map[string]string{"text": msg, "content": msg})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("alert webhook: %w", err)
		}
		defer res.Body.Close()
		if res.StatusCode/100 != 2 {
			b, _ := io.ReadAll(io.LimitReader(res.Body, 300))
			return fmt.Errorf("alert webhook answered %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
		}
		return nil
	}
}

// Mail returns a Sender that emails every address with the given send function (the server's mail provider).
func Mail(send func(ctx context.Context, to, subject, text, html string) error, to []string) Sender {
	return func(ctx context.Context, subject, body string) error {
		var first error
		for _, addr := range to {
			if err := send(ctx, addr, subject, body, ""); err != nil && first == nil {
				first = err
			}
		}
		return first
	}
}
