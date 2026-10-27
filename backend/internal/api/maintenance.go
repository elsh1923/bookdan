package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
	"github.com/bookdan/backend/internal/ops"
)

// Once a day (after midnight in Addis Ababa, and when the server starts) the server tidies what time has overtaken:
//   - a stay that is over becomes COMPLETED (whether or not the hotel used the app to check the guest in: many check guests in at the desk);
//   - guest ID numbers are erased once the retention period is over (ID_RETENTION_DAYS; off until the owner chooses it).
// A guest who never came is not guessed: the hotel marks them a no-show itself (partnerNoShow below).

// RunMaintenance runs the daily jobs once (also used by tests).
func (s *Server) RunMaintenance(ctx context.Context) {
	s.completeStays(ctx)
	s.accrueHotelCommissions(ctx)
	s.emailStatements(ctx)
	s.eraseOldIDs(ctx)
}

func (s *Server) completeStays(ctx context.Context) {
	n := 0
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := actor(ctx, tx, "system", "stay ended"); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE bookings SET status = 'COMPLETED' WHERE status IN ('CHECKED_IN', 'CONFIRMED') AND check_out < $1::date`, s.addisToday())
		n = int(tag.RowsAffected())
		return err
	})
	if err != nil {
		slog.Error("completing finished stays failed", "err", err)
		ops.Report("job-complete-stays", "The daily job that completes stays failed", err.Error())
		return
	}
	if n > 0 {
		slog.Info("finished stays completed", "count", n)
	}
}

// eraseOldIDs removes the full ID number from the hotel's copy of a guest's details, keeping the type and the last four characters.
// The copy of a booking that never became a stay (a hold that ran out, a request the hotel declined, a cancellation) goes after 7 days;
// the copy of a real stay goes IDRetentionDays after check-out.
func (s *Server) eraseOldIDs(ctx context.Context) {
	days := s.cfg.IDRetentionDays
	if days <= 0 {
		slog.Warn("guest ID numbers are never erased: set ID_RETENTION_DAYS (how many days after check-out a hotel keeps the full number)")
		return
	}
	cutoff := s.now().UTC().Add(3*time.Hour).AddDate(0, 0, -days).Format("2006-01-02")
	var stays, dead int64
	if err := s.pool.QueryRow(ctx, `SELECT purge_id_numbers($1::date)`, cutoff).Scan(&stays); err != nil {
		slog.Error("erasing old ID numbers failed", "err", err)
		return
	}
	tag, err := s.pool.Exec(ctx, `UPDATE booking_guests bg SET id_number_enc = NULL, purged_at = now() FROM bookings b
		WHERE b.id = bg.booking_id AND bg.purged_at IS NULL AND b.status IN ('EXPIRED', 'DECLINED_REFUNDED', 'CANCELLED_REFUNDED')
		  AND coalesce(b.cancelled_at, b.created_at) < now() - interval '7 days'`)
	if err != nil {
		slog.Error("erasing the ID numbers of bookings that never happened failed", "err", err)
		return
	}
	dead = tag.RowsAffected()
	if stays+dead > 0 {
		slog.Info("guest ID numbers erased", "after_stays", stays, "never_happened", dead)
		if _, err := s.pool.Exec(ctx, `INSERT INTO audit_log (action, details) VALUES ('id_numbers_erased', jsonb_build_object('after_stays', $1::bigint, 'never_happened', $2::bigint, 'retention_days', $3::int))`, stays, dead, days); err != nil {
			slog.Error("audit of ID erasure failed", "err", err)
		}
	}
}

// partnerNoShow marks a guest who did not arrive. Only a confirmed booking whose check-in day has come. The hotel keeps its share
// (nothing is refunded: the guest neither came nor cancelled), and the rooms for the nights still to come go back on sale.
func (s *Server) partnerNoShow(w http.ResponseWriter, r *http.Request) error {
	ctx, id, uid := r.Context(), chi.URLParam(r, "id"), claimsFrom(r).Sub
	propIDs, _, err := s.staffHotels(ctx, uid)
	if err != nil {
		return err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		var status, roomType string
		var in, out time.Time
		var rooms int
		err := tx.QueryRow(ctx, `SELECT status, room_type_id, check_in, check_out, rooms FROM bookings WHERE id = $1 AND property_id = ANY($2::uuid[]) FOR UPDATE`, id, propIDs).
			Scan(&status, &roomType, &in, &out, &rooms)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if status == "NO_SHOW" {
			return nil // already marked: the same answer again
		}
		if status != "CONFIRMED" {
			return fail(http.StatusConflict, codeInvalidState, "Only a confirmed booking can be marked as a no-show")
		}
		today := s.addisToday()
		if pgDate(in) > today {
			return fail(http.StatusConflict, codeInvalidState, "The guest's check-in day has not come yet")
		}
		if err := actor(ctx, tx, "staff:"+uid, "guest did not arrive"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bookings SET status = 'NO_SHOW' WHERE id = $1`, id); err != nil {
			return err
		}
		// The nights from today on are free again for other guests; nights already past stay as they were.
		from := today
		if pgDate(in) > from {
			from = pgDate(in)
		}
		if pgDate(out) > from {
			if _, err := tx.Exec(ctx, `UPDATE inventory SET booked = greatest(booked - $4, 0) WHERE room_type_id = $1 AND night >= $2::date AND night < $3::date`, roomType, from, pgDate(out), rooms); err != nil {
				return err
			}
		}
		return s.audit(r, tx, "guest_no_show", map[string]any{"booking_id": id})
	})
	if err != nil {
		return err
	}
	b, err := scanPartner(s.pool.QueryRow(ctx, partnerSelect+` WHERE b.id = $1`, id), len(propIDs) > 1)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, b)
	return nil
}

var _ = httpx.CodeInvalidRequest
