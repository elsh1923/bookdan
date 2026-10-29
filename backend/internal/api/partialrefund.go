package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
	"github.com/bookdan/backend/internal/ops"
)

// Partial refunds. Part of what a guest paid goes back while the booking stands: a dispute settled for part of the price, or a goodwill
// credit from support. Rules:
//   - only a booking whose hotel share is booked (CONFIRMED, CHECKED_IN, COMPLETED, NO_SHOW, or DISPUTED while it is being settled);
//   - strictly less than the payment (a refund of everything is a cancellation or a full dispute outcome), and never more than is still
//     refundable: the payment minus the partial refunds already made. Several partial refunds are possible; a later cancellation or full
//     refund returns only what is left;
//   - who bears it: "hotel" (the hotel's share, BookDan's commission and the VAT on it shrink in proportion to the part refunded, as if that
//     part of the stay had been cancelled) or "platform" (BookDan pays from its own money, ledger expense dispute_expense, and the hotel keeps
//     its share);
//   - the money moves when the gateway accepts it (runRefunds), one gateway refund per partial refund, and is retried until it does.
// If the hotel was already paid out, "hotel" leaves it owing that part against its next payout.

// refundable is what can still be refunded on a booking's online payments: all that was paid, what is left to refund, and each payment's own share of it.
// Most bookings have one payment. A booking moved to dearer dates has a second one (the difference); PaymentID is the main one.
type refundable struct {
	PaymentID string
	Currency  string
	Paid      int64
	Left      int64
	Payments  []payLeft
}

type payLeft struct {
	ID   string
	Left int64
}

// lockRefundable finds the booking's succeeded payments and locks them, so two refunds cannot both spend the same money.
func lockRefundable(ctx context.Context, tx pgx.Tx, bookingID string) (refundable, error) {
	var r refundable
	rows, err := tx.Query(ctx, `SELECT p.id, p.currency, p.amount_santim, p.amount_santim - coalesce((SELECT sum(x.amount_santim) FROM partial_refunds x WHERE x.payment_id = p.id), 0)::bigint
		FROM payments p WHERE p.booking_id = $1 AND p.status = 'succeeded' ORDER BY (p.kind = 'booking') DESC, p.created_at FOR UPDATE OF p`, bookingID)
	if err != nil {
		return r, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, cur string
		var amount, left int64
		if err := rows.Scan(&id, &cur, &amount, &left); err != nil {
			return r, err
		}
		if r.PaymentID == "" {
			r.PaymentID, r.Currency = id, cur
		}
		r.Paid += amount
		r.Left += left
		r.Payments = append(r.Payments, payLeft{id, left})
	}
	if err := rows.Err(); err != nil {
		return r, err
	}
	if r.PaymentID == "" {
		return r, pgx.ErrNoRows
	}
	return r, nil
}

// scaleEntry is amount * part / whole rounded half away from zero (whole mono-currency santim, never floats).
func scaleEntry(amount, part, whole int64) int64 {
	neg := amount < 0
	if neg {
		amount = -amount
	}
	v := (amount*part + whole/2) / whole
	if neg {
		return -v
	}
	return v
}

// reverseShareOfConfirmed books the reversal of `part` of `whole` (the payment) of what accepting the booking recorded, and owes the guest
// `part`. Rounding differences go to the hotel's share so the transaction balances exactly.
func reverseShareOfConfirmed(ctx context.Context, tx pgx.Tx, bookingID, paymentID, currency, memo string, part, whole int64) error {
	rows, err := tx.Query(ctx, `SELECT e.account_code, e.amount_santim FROM ledger_entries e JOIN ledger_transactions t ON t.id = e.transaction_id
		WHERE t.booking_id = $1 AND t.kind = 'booking_confirmed' AND e.account_code <> 'guest_deposits_held'`, bookingID)
	if err != nil {
		return err
	}
	var entries []entry
	var sum int64
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.account, &e.santim); err != nil {
			rows.Close()
			return err
		}
		e.santim = -scaleEntry(e.santim, part, whole) // the reversal
		sum += e.santim
		entries = append(entries, e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(entries) == 0 {
		return accrueRefund(ctx, tx, bookingID, paymentID, currency, part)
	}
	// What the reversal returns must equal what is refunded; the hotel's share absorbs the rounding.
	if diff := part - sum; diff != 0 {
		for i := range entries {
			if entries[i].account == "property_payable" {
				entries[i].santim += diff
				break
			}
		}
	}
	kept := entries[:0]
	for _, e := range entries {
		if e.santim != 0 {
			kept = append(kept, e)
		}
	}
	kept = append(kept, entry{"refunds_payable", -part})
	return ledger(ctx, tx, bookingID, paymentID, "refund", memo, currency, kept...)
}

var partialStatuses = map[string]bool{"CONFIRMED": true, "CHECKED_IN": true, "COMPLETED": true, "NO_SHOW": true, "DISPUTED": true}

// partialRefund records a partial refund inside a transaction. The booking must be locked by the caller.
func (s *Server) partialRefund(ctx context.Context, tx pgx.Tx, bookingID string, ticketID *string, payer string, amount int64, reason, who string) error {
	if payer != "hotel" && payer != "platform" {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "payer must be hotel or platform")
	}
	if len([]rune(strings.TrimSpace(reason))) < 3 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Write the reason (at least 3 characters)")
	}
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1`, bookingID).Scan(&status); err != nil {
		return err
	}
	if !partialStatuses[status] {
		return fail(http.StatusConflict, codeInvalidState, "Part of the payment can be refunded only on a booking the hotel confirmed and that was not cancelled")
	}
	rf, err := lockRefundable(ctx, tx, bookingID)
	if err == pgx.ErrNoRows {
		return fail(http.StatusBadRequest, "NO_PAYMENT", "Nothing was paid online for this booking, so there is nothing to refund.")
	}
	if err != nil {
		return err
	}
	if amount <= 0 || amount >= rf.Paid {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, fmt.Sprintf("A partial refund is more than 0 and less than the payment (%s)", formatETB(rf.Paid)))
	}
	if amount > rf.Left {
		return fail(http.StatusConflict, "REFUND_TOO_LARGE", fmt.Sprintf("Only %s is left to refund on this payment", formatETB(rf.Left)))
	}
	// One gateway refund returns money of one payment: take it from the first payment that still has enough.
	var from string
	var biggest int64
	for _, p := range rf.Payments {
		if p.Left >= amount {
			from = p.ID
			break
		}
		if p.Left > biggest {
			biggest = p.Left
		}
	}
	if from == "" {
		return fail(http.StatusConflict, "REFUND_TOO_LARGE", fmt.Sprintf("At most %s can be refunded in one go on this booking", formatETB(biggest)))
	}
	rf.PaymentID = from
	if _, err := tx.Exec(ctx, `INSERT INTO partial_refunds (payment_id, booking_id, amount_santim, payer, reason, ticket_id, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		rf.PaymentID, bookingID, amount, payer, strings.TrimSpace(reason), ticketID, who); err != nil {
		return err
	}
	if payer == "platform" {
		return ledger(ctx, tx, bookingID, rf.PaymentID, "refund", "partial refund (BookDan pays): "+strings.TrimSpace(reason), rf.Currency,
			entry{"dispute_expense", amount}, entry{"refunds_payable", -amount})
	}
	return reverseShareOfConfirmed(ctx, tx, bookingID, rf.PaymentID, rf.Currency, "partial refund (hotel pays): "+strings.TrimSpace(reason), amount, rf.Paid)
}

// runPartialRefunds sends the owed partial refunds to the gateway, one by one, and records each in the ledger once the gateway accepts it.
func (s *Server) runPartialRefunds(ctx context.Context) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM partial_refunds WHERE status = 'owed' ORDER BY created_at LIMIT 50`)
	if err != nil {
		slog.Error("partial refunds: list", "err", err)
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		err := s.tx(ctx, func(tx pgx.Tx) error {
			var ref, cur, bookingID, paymentID string
			var amount int64
			err := tx.QueryRow(ctx, `SELECT p.provider_ref, p.currency, r.booking_id, r.payment_id, r.amount_santim FROM partial_refunds r JOIN payments p ON p.id = r.payment_id
				WHERE r.id = $1 AND r.status = 'owed' FOR UPDATE OF r SKIP LOCKED`, id).Scan(&ref, &cur, &bookingID, &paymentID, &amount)
			if err == pgx.ErrNoRows {
				return nil // someone else is paying it, or it is done
			}
			if err != nil {
				return err
			}
			if err := s.p.Gateway.Refund(ctx, ref, id, amount, cur); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE partial_refunds SET status = 'paid', paid_at = now() WHERE id = $1`, id); err != nil {
				return err
			}
			return ledger(ctx, tx, bookingID, paymentID, "refund", "refund paid to the guest (partial)", cur,
				entry{"refunds_payable", amount}, entry{"gateway_clearing", -amount})
		})
		if err != nil {
			slog.Error("partial refund failed, will retry", "refund", id, "err", err)
			ops.Report("refund-failed", "A refund could not be paid", "Partial refund "+id+": "+err.Error()+"\nIt is tried again every 20 seconds. If it keeps failing, refund the guest by hand.")
		}
	}
}

// adminTicketRefund gives a guest part of their payment back from a support request (a goodwill credit), outside a dispute. A dispute that
// is still open is settled with its own decision instead.
func (s *Server) adminTicketRefund(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	var in struct {
		Payer  string `json:"payer"`
		Amount int64  `json:"amount_santim"`
		Note   string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	note, ok := cleanText(in.Note, 3, 1000)
	if !ok {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Write the reason (3 to 1000 characters); the guest reads it")
	}
	ctx, id := r.Context(), chi.URLParam(r, "id")
	var who string
	if err := s.pool.QueryRow(ctx, `SELECT coalesce(email, phone, id::text) FROM users WHERE id = $1`, claimsFrom(r).Sub).Scan(&who); err != nil {
		return err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var kind, tStatus string
		var booking *string
		err := tx.QueryRow(ctx, `SELECT kind, status, booking_id FROM support_tickets WHERE id = $1 FOR UPDATE`, id).Scan(&kind, &tStatus, &booking)
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if booking == nil {
			return fail(http.StatusConflict, codeInvalidState, "This request is not about a booking")
		}
		if kind == "dispute" && tStatus != "closed" {
			return fail(http.StatusConflict, "DISPUTE_OPEN", "Settle the dispute with a decision (it can be a partial refund)")
		}
		if tStatus == "closed" {
			return fail(http.StatusConflict, codeInvalidState, "This request is closed")
		}
		var bStatus string
		if err := tx.QueryRow(ctx, `SELECT status FROM bookings WHERE id = $1 FOR UPDATE`, *booking).Scan(&bStatus); err != nil {
			return err
		}
		if err := s.partialRefund(ctx, tx, *booking, &id, in.Payer, in.Amount, note, who); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO support_messages (ticket_id, from_staff, author, body) VALUES ($1, true, $2, $3)`,
			id, who, "We refunded "+formatETB(in.Amount)+" of your payment.\n\n"+note); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE support_tickets SET status = 'answered', updated_at = now() WHERE id = $1`, id); err != nil {
			return err
		}
		return s.audit(r, tx, "partial_refund", map[string]any{"ticket_id": id, "booking_id": *booking, "amount_santim": in.Amount, "payer": in.Payer})
	})
	if err != nil {
		return err
	}
	s.notifyTicketOwner(ctx, id, "We refunded "+formatETB(in.Amount)+" of your payment.\n\n"+note)
	s.runRefunds(ctx)
	respond(w, http.StatusOK, httpx.NowMeta())
	return nil
}
