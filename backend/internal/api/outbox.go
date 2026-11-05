package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/bookdan/backend/internal/ops"
)

// The outbox. Every phone alert and email is first written to the outbox table, then sent. If the send fails (the push service or the mail server is down for a
// minute) it is tried again with growing pauses, and if the server stops in the middle of a send the message is still there to be sent by the next pass. A message
// that is no longer worth sending (a new-booking alert after the request ran out) is dropped when its time is over.
//
// The first attempt is made at once, in the same call, so nothing is slower or later than before. The text of a message is erased as soon as it is delivered.
// A pass runs with the sweeper. It may deliver a message twice if a server stops after sending but before it records it: a repeated alert or email is the price
// of never losing one.
//
// Sign-in codes are not queued: the person is waiting, and must be told at once when one cannot be sent.

const (
	outboxMaxAttempts = 8
	outboxClaim       = 2 * time.Minute // how long a message is "being sent" before another pass may take it
	outboxSendTimeout = 20 * time.Second
)

// outboxPause is how long to wait after the nth failed attempt (1-based).
func outboxPause(attempt int) time.Duration {
	steps := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour}
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(steps) {
		attempt = len(steps)
	}
	return steps[attempt-1]
}

type pushPayload struct {
	Tokens []string          `json:"tokens"`
	Title  string            `json:"title"`
	Body   string            `json:"body"`
	Data   map[string]string `json:"data"`
}

type mailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

// enqueue writes the message and makes the first attempt now. It returns an error only when the message could not be written at all.
func (s *Server) enqueue(ctx context.Context, kind string, payload any, worth time.Duration) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// The send must not be cut short because the person who caused it closed their app.
	ctx = context.WithoutCancel(ctx)
	var id int64
	if err := s.pool.QueryRow(ctx, `INSERT INTO outbox (kind, payload, expires_at, attempts, next_attempt_at)
		VALUES ($1, $2, now() + $3::interval, 1, now() + $4::interval) RETURNING id`,
		kind, raw, fmt.Sprintf("%d seconds", int(worth.Seconds())), fmt.Sprintf("%d seconds", int(outboxClaim.Seconds()))).Scan(&id); err != nil {
		return err
	}
	s.attempt(ctx, id, kind, raw, 1)
	return nil
}

// attempt sends one message and records the result.
func (s *Server) attempt(ctx context.Context, id int64, kind string, raw []byte, attempts int) {
	sendCtx, cancel := context.WithTimeout(ctx, outboxSendTimeout)
	defer cancel()
	var err error
	switch kind {
	case "push":
		var p pushPayload
		if err = json.Unmarshal(raw, &p); err == nil {
			err = s.deliverPush(sendCtx, p)
		}
	case "email":
		var m mailPayload
		if err = json.Unmarshal(raw, &m); err == nil {
			err = s.p.Mail.SendMail(sendCtx, m.To, m.Subject, m.Text, m.HTML)
		}
	default:
		err = fmt.Errorf("unknown outbox kind %q", kind)
	}
	if err == nil {
		if _, e := s.pool.Exec(ctx, `UPDATE outbox SET sent_at = now(), payload = '{}'::jsonb, last_error = NULL WHERE id = $1`, id); e != nil {
			slog.Error("outbox: record delivery", "id", id, "err", e)
		}
		return
	}
	slog.Error("outbox: send failed", "id", id, "kind", kind, "attempt", attempts, "err", err)
	if attempts >= outboxMaxAttempts {
		if _, e := s.pool.Exec(ctx, `UPDATE outbox SET failed_at = now(), last_error = $2 WHERE id = $1`, id, err.Error()); e != nil {
			slog.Error("outbox: record failure", "id", id, "err", e)
		}
		ops.Report("outbox-failed", "A notification could not be delivered", fmt.Sprintf("A %s message (outbox %d) failed %d times; the last error: %s", kind, id, attempts, err))
		return
	}
	if _, e := s.pool.Exec(ctx, `UPDATE outbox SET next_attempt_at = now() + $2::interval, last_error = $3 WHERE id = $1`,
		id, fmt.Sprintf("%d seconds", int(outboxPause(attempts).Seconds())), err.Error()); e != nil {
		slog.Error("outbox: schedule retry", "id", id, "err", e)
	}
}

// DrainOutbox sends the messages that are due: ones whose first attempt failed, and ones a stopped server was sending (also used by tests).
func (s *Server) DrainOutbox(ctx context.Context) {
	rows, err := s.pool.Query(ctx, `UPDATE outbox SET attempts = attempts + 1, next_attempt_at = now() + $1::interval
		WHERE id IN (SELECT id FROM outbox WHERE sent_at IS NULL AND failed_at IS NULL AND next_attempt_at <= now() ORDER BY id LIMIT 50 FOR UPDATE SKIP LOCKED)
		RETURNING id, kind, payload, attempts, expires_at < now()`, fmt.Sprintf("%d seconds", int(outboxClaim.Seconds())))
	if err != nil {
		slog.Error("outbox: claim", "err", err)
		return
	}
	type due struct {
		id       int64
		kind     string
		payload  []byte
		attempts int
		expired  bool
	}
	var list []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.kind, &d.payload, &d.attempts, &d.expired); err != nil {
			rows.Close()
			slog.Error("outbox: read", "err", err)
			return
		}
		list = append(list, d)
	}
	rows.Close()
	for _, d := range list {
		if d.expired {
			// Not worth sending any more: say so, and do not count it as a failure to investigate.
			if _, err := s.pool.Exec(ctx, `UPDATE outbox SET failed_at = now(), payload = '{}'::jsonb, last_error = 'expired before it could be delivered' WHERE id = $1`, d.id); err != nil {
				slog.Error("outbox: expire", "id", d.id, "err", err)
			}
			continue
		}
		s.attempt(ctx, d.id, d.kind, d.payload, d.attempts)
	}
}

// tidyOutbox removes delivered messages after a week and failed ones after a month (daily).
func (s *Server) tidyOutbox(ctx context.Context) {
	if _, err := s.pool.Exec(ctx, `DELETE FROM outbox WHERE (sent_at IS NOT NULL AND sent_at < now() - interval '7 days') OR (failed_at IS NOT NULL AND failed_at < now() - interval '30 days')`); err != nil {
		slog.Error("outbox: tidy", "err", err)
	}
}

// QueuePush and QueueMail are the ways the rest of the server sends a notification. (Exported for tests.)
func (s *Server) QueuePush(ctx context.Context, tokens []string, title, body string, data map[string]string) {
	if len(tokens) == 0 {
		return
	}
	worth := 6 * time.Hour
	if data["channel"] == "booking_alerts" {
		worth = 15 * time.Minute // a request lasts 15 minutes; after that an alert is only noise
	}
	if err := s.enqueue(ctx, "push", pushPayload{tokens, title, body, data}, worth); err != nil {
		slog.Error("outbox: could not queue an alert", "err", err)
	}
}

// QueueMail queues an email and returns an error only when it could not be queued. Delivery problems are retried, not returned.
func (s *Server) QueueMail(ctx context.Context, to, subject, text, html string) error {
	if s.p.Mail == nil {
		return nil // no email service configured
	}
	return s.enqueue(ctx, "email", mailPayload{to, subject, text, html}, 48*time.Hour)
}
