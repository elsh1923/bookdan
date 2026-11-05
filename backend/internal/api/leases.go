package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

// Running more than one server. Every server answers requests, but the timer that sweeps (every 20 seconds) and the daily jobs must not all run on every
// server: the work would repeat and the cadence would multiply with the number of servers. A lease in the database names one server as the sweeper for the next
// minute; it renews it on every pass, and if it dies another server takes over within that minute. Each daily job is claimed by day, so it runs once in total.
// (Work that a request triggers, such as a hotel opening its bookings, still sweeps on that server: it is safe to do twice, because rows are claimed with
// SKIP LOCKED.)

const leaseTTL = 60 * time.Second

// HoldLease takes or renews the lease `name` for this server. It is true while this server holds it (also used by tests).
func (s *Server) HoldLease(ctx context.Context, name string, ttl time.Duration) bool {
	var holder string
	err := s.pool.QueryRow(ctx, `INSERT INTO job_leases (name, holder, until) VALUES ($1, $2, now() + $3::interval)
		ON CONFLICT (name) DO UPDATE SET holder = EXCLUDED.holder, until = EXCLUDED.until
		WHERE job_leases.until < now() OR job_leases.holder = EXCLUDED.holder
		RETURNING holder`, name, s.instance, fmt.Sprintf("%d milliseconds", ttl.Milliseconds())).Scan(&holder)
	if err != nil {
		// No row back means somebody else holds it; anything else is a database problem, which the next pass tries again.
		if !errors.Is(err, pgx.ErrNoRows) {
			slog.Error("job lease", "name", name, "err", err)
		}
		return false
	}
	return holder == s.instance
}

// ReleaseLease gives the lease back at once (on shutdown), so another server need not wait for it to run out.
func (s *Server) ReleaseLease(ctx context.Context, name string) {
	if _, err := s.pool.Exec(ctx, `UPDATE job_leases SET until = now() WHERE name = $1 AND holder = $2`, name, s.instance); err != nil {
		slog.Error("release job lease", "name", name, "err", err)
	}
}

// ClaimDay is true for the first server to ask about this job on this day (YYYY-MM-DD), and false for everyone after.
func (s *Server) ClaimDay(ctx context.Context, name, day string) bool {
	tag, err := s.pool.Exec(ctx, `INSERT INTO job_runs (name, day) VALUES ($1, $2::date) ON CONFLICT DO NOTHING`, name, day)
	if err != nil {
		slog.Error("claim daily job", "name", name, "err", err)
		return false
	}
	return tag.RowsAffected() == 1
}
