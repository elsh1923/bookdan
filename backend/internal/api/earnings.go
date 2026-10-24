package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
)

// What a hotel earns, from the ledger.
//
// For each online-paid booking the ledger holds the hotel's share (property_payable), BookDan's commission and the VAT on it.
// A cancelled booking reverses all of it, so its share is zero. A hotel's money is in one of three states:
//   - upcoming: a confirmed stay that has not happened yet (the guest can still cancel, so it is not payable);
//   - ready:    the guest has checked in, or the stay is over: payable;
//   - paid:     already transferred to the hotel (hotel_payouts).
// What BookDan owes the hotel today is ready minus paid. Bookings paid at the hotel never pass through BookDan, so they are not here.

// bookingMoneySQL is one row per booking that has money in the ledger. $1 limits it to hotels (NULL = all).
const bookingMoneySQL = `
	SELECT b.id, b.property_id, b.code, b.guest_name, b.check_in, b.check_out, b.status, b.currency,
	       coalesce(sum(-e.amount_santim) FILTER (WHERE e.account_code = 'property_payable'), 0)::bigint AS net,
	       coalesce(sum(-e.amount_santim) FILTER (WHERE e.account_code = 'commission_revenue'), 0)::bigint AS commission,
	       coalesce(sum(-e.amount_santim) FILTER (WHERE e.account_code = 'vat_on_commission_payable'), 0)::bigint AS vat,
	       coalesce(sum(e.amount_santim) FILTER (WHERE e.account_code IN ('guest_deposits_held', 'coupon_expense') AND t.kind = 'booking_confirmed'), 0)::bigint AS gross,
	       coalesce(sum(e.amount_santim) FILTER (WHERE e.account_code = 'coupon_expense' AND t.kind = 'booking_confirmed'), 0)::bigint AS discount,
	       (b.status IN ('CHECKED_IN', 'COMPLETED') OR (b.status = 'CONFIRMED' AND b.check_out <= (now() AT TIME ZONE 'Africa/Addis_Ababa')::date)) AS ready
	FROM bookings b JOIN ledger_transactions t ON t.booking_id = b.id JOIN ledger_entries e ON e.transaction_id = t.id
	WHERE ($1::uuid[] IS NULL OR b.property_id = ANY($1::uuid[]))
	GROUP BY b.id
	HAVING coalesce(sum(-e.amount_santim) FILTER (WHERE e.account_code = 'property_payable'), 0) <> 0`

type balance struct {
	PropertyID string `json:"id"`
	Name       string `json:"name"`
	Currency   string `json:"currency"`
	// Ready is payable now (checked in or stay over), Upcoming is not yet, Paid is what was transferred. ToPay is Ready minus Paid, never below 0.
	Ready    int64   `json:"ready_santim"`
	Upcoming int64   `json:"upcoming_santim"`
	Paid     int64   `json:"paid_santim"`
	ToPay    int64   `json:"to_pay_santim"`
	LastPaid *string `json:"last_paid_at"`
}

// balances returns the money position of the given hotels (nil = every hotel), in ETB.
func (s *Server) balances(ctx context.Context, q interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}, ids []string) ([]balance, error) {
	var arg any
	if ids != nil {
		arg = ids
	}
	rows, err := q.Query(ctx, `WITH m AS (`+bookingMoneySQL+`)
		SELECT p.id, p.name,
		  coalesce(sum(m.net) FILTER (WHERE m.ready), 0)::bigint,
		  coalesce(sum(m.net) FILTER (WHERE NOT m.ready), 0)::bigint,
		  coalesce((SELECT sum(amount_santim) FROM hotel_payouts hp WHERE hp.property_id = p.id), 0)::bigint,
		  (SELECT max(paid_at) FROM hotel_payouts hp WHERE hp.property_id = p.id)
		FROM properties p LEFT JOIN m ON m.property_id = p.id
		WHERE ($1::uuid[] IS NULL OR p.id = ANY($1::uuid[]))
		GROUP BY p.id ORDER BY p.name`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []balance{}
	for rows.Next() {
		b := balance{Currency: "ETB"}
		var last *time.Time
		if err := rows.Scan(&b.PropertyID, &b.Name, &b.Ready, &b.Upcoming, &b.Paid, &last); err != nil {
			return nil, err
		}
		if b.ToPay = b.Ready - b.Paid; b.ToPay < 0 {
			b.ToPay = 0
		}
		if last != nil {
			t := last.UTC().Format(time.RFC3339)
			b.LastPaid = &t
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

type earningLine struct {
	BookingID string `json:"booking_id"`
	Reference string `json:"reference"`
	GuestName string `json:"guest_name"`
	CheckIn   string `json:"check_in"`
	CheckOut  string `json:"check_out"`
	Status    string `json:"status"`
	// Gross is the booking's full price (what the guest paid plus any coupon); Discount is the part of it BookDan paid with a coupon.
	Gross      int64 `json:"gross_santim"`
	Discount   int64 `json:"discount_santim"`
	Commission int64 `json:"commission_santim"`
	VAT        int64 `json:"vat_santim"`
	Net        int64 `json:"net_santim"`
	Ready      bool  `json:"ready"`
}

type payoutLine struct {
	ID        string `json:"id"`
	Amount    int64  `json:"amount_santim"`
	Method    string `json:"method"`
	Reference string `json:"reference"`
	Note      string `json:"note,omitempty"`
	PaidAt    string `json:"paid_at"`
}

func (s *Server) payoutsOf(ctx context.Context, propertyID string, limit int) ([]payoutLine, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, amount_santim, method, reference, coalesce(note, ''), paid_at FROM hotel_payouts
		WHERE property_id = $1 ORDER BY paid_at DESC LIMIT $2`, propertyID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []payoutLine{}
	for rows.Next() {
		var p payoutLine
		var at time.Time
		if err := rows.Scan(&p.ID, &p.Amount, &p.Method, &p.Reference, &p.Note, &at); err != nil {
			return nil, err
		}
		p.PaidAt = at.UTC().Format(time.RFC3339)
		out = append(out, p)
	}
	return out, rows.Err()
}

// partnerEarnings shows owners what their hotels have earned and been paid. Receptionists do not see money.
func (s *Server) partnerEarnings(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	rows, err := s.pool.Query(ctx, `SELECT property_id FROM property_staff WHERE user_id = $1 AND role = 'owner' ORDER BY created_at`, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) == 0 {
		return fail(http.StatusForbidden, codeForbidden, "Only a hotel owner can see earnings")
	}
	bals, err := s.balances(ctx, s.pool, ids)
	if err != nil {
		return err
	}
	type hotelEarnings struct {
		balance
		Items   []earningLine `json:"items"`
		Payouts []payoutLine  `json:"payouts"`
	}
	out := make([]hotelEarnings, 0, len(bals))
	for _, b := range bals {
		lr, err := s.pool.Query(ctx, `WITH m AS (`+bookingMoneySQL+`) SELECT id, code, guest_name, check_in, check_out, status, gross, discount, commission, vat, net, ready
			FROM m WHERE property_id = $2 ORDER BY check_in DESC, code LIMIT 60`, []string{b.PropertyID}, b.PropertyID)
		if err != nil {
			return err
		}
		items := []earningLine{}
		for lr.Next() {
			var l earningLine
			var in, o time.Time
			if err := lr.Scan(&l.BookingID, &l.Reference, &l.GuestName, &in, &o, &l.Status, &l.Gross, &l.Discount, &l.Commission, &l.VAT, &l.Net, &l.Ready); err != nil {
				lr.Close()
				return err
			}
			l.CheckIn, l.CheckOut = pgDate(in), pgDate(o)
			items = append(items, l)
		}
		lr.Close()
		pays, err := s.payoutsOf(ctx, b.PropertyID, 20)
		if err != nil {
			return err
		}
		out = append(out, hotelEarnings{b, items, pays})
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Hotels []hotelEarnings `json:"hotels"`
	}{httpx.NowMeta(), out})
	return nil
}

// ---- admin: who is owed what, and recording a payout

type payoutRow struct {
	balance
	HasBusiness bool   `json:"has_business"`
	Method      string `json:"payout_method,omitempty"`
	AccountName string `json:"payout_account_name,omitempty"`
	Last4       string `json:"payout_last4,omitempty"`
}

func (s *Server) adminPayouts(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	ctx := r.Context()
	bals, err := s.balances(ctx, s.pool, nil)
	if err != nil {
		return err
	}
	out := make([]payoutRow, 0, len(bals))
	for _, b := range bals {
		pr := payoutRow{balance: b}
		err := s.pool.QueryRow(ctx, `SELECT payout_method, payout_account_name, payout_last4 FROM hotel_business WHERE property_id = $1`, b.PropertyID).
			Scan(&pr.Method, &pr.AccountName, &pr.Last4)
		if err == nil {
			pr.HasBusiness = true
		} else if err != pgx.ErrNoRows {
			return err
		}
		// Hotels with no money movement at all are left out, so the list shows only those that matter.
		if b.Ready+b.Upcoming+b.Paid != 0 {
			out = append(out, pr)
		}
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Hotels []payoutRow `json:"hotels"`
	}{httpx.NowMeta(), out})
	return nil
}

func (s *Server) adminHotelPayouts(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	pays, err := s.payoutsOf(r.Context(), id, 100)
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Payouts []payoutLine `json:"payouts"`
	}{httpx.NowMeta(), pays})
	return nil
}

// adminRecordPayout writes down a transfer that was already made to a hotel: the amount cannot exceed what is ready to pay.
func (s *Server) adminRecordPayout(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	if err := s.canManage(r, id); err != nil {
		return err
	}
	var in struct {
		Amount    int64  `json:"amount_santim"`
		Reference string `json:"reference"`
		Note      string `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Reference, in.Note = strings.TrimSpace(in.Reference), strings.TrimSpace(in.Note)
	if in.Amount <= 0 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "The amount must be more than zero")
	}
	if len(in.Reference) < 3 || len(in.Reference) > 80 || len(in.Note) > 300 {
		return fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "Give the transfer reference (3 to 80 characters)")
	}
	uid := claimsFrom(r).Sub
	var hotel, method string
	var payoutID string
	err := s.tx(r.Context(), func(tx pgx.Tx) error {
		ctx := r.Context()
		// Lock the hotel so two payouts cannot both spend the same money.
		if err := tx.QueryRow(ctx, `SELECT name FROM properties WHERE id = $1 FOR UPDATE`, id).Scan(&hotel); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT payout_method FROM hotel_business WHERE property_id = $1`, id).Scan(&method); err != nil {
			if err == pgx.ErrNoRows {
				return fail(http.StatusConflict, codeInvalidState, "Add the hotel's business and payout details first")
			}
			return err
		}
		bals, err := s.balances(ctx, tx, []string{id})
		if err != nil {
			return err
		}
		if len(bals) == 0 || in.Amount > bals[0].ToPay {
			return fail(http.StatusConflict, codeInvalidState, "That is more than is ready to be paid to this hotel")
		}
		var txID string
		if err := tx.QueryRow(ctx, `INSERT INTO ledger_transactions (property_id, kind, memo) VALUES ($1, 'payout', $2) RETURNING id`, id, "payout to the hotel: "+in.Reference).Scan(&txID); err != nil {
			return err
		}
		for _, e := range []entry{{"property_payable", in.Amount}, {"gateway_clearing", -in.Amount}} {
			if _, err := tx.Exec(ctx, `INSERT INTO ledger_entries (transaction_id, account_code, amount_santim, currency) VALUES ($1, $2, $3, 'ETB')`, txID, e.account, e.santim); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `INSERT INTO hotel_payouts (property_id, amount_santim, method, reference, note, paid_by, ledger_transaction_id)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7) RETURNING id`, id, in.Amount, method, in.Reference, in.Note, uid, txID).Scan(&payoutID); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_payout_recorded", id, map[string]any{"amount_santim": in.Amount, "reference": in.Reference, "payout_id": payoutID})
	})
	if err != nil {
		return err
	}
	s.mailPayout(r.Context(), id, hotel, in.Amount, in.Reference)
	bals, err := s.balances(r.Context(), s.pool, []string{id})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		PayoutID string  `json:"payout_id"`
		Balance  balance `json:"balance"`
	}{httpx.NowMeta(), payoutID, bals[0]})
	return nil
}

// mailPayout tells the hotel's owners (and the business contact) that money is on its way. Failure is only logged.
func (s *Server) mailPayout(ctx context.Context, propertyID, hotel string, amount int64, reference string) {
	rows, err := s.pool.Query(ctx, `SELECT coalesce(u.email, '') FROM property_staff ps JOIN users u ON u.id = ps.user_id WHERE ps.property_id = $1 AND ps.role = 'owner'
		UNION SELECT contact_email FROM hotel_business WHERE property_id = $1`, propertyID)
	if err != nil {
		return
	}
	defer rows.Close()
	text := "BookDan has paid " + formatETB(amount) + " to " + hotel + ".\n\nTransfer reference: " + reference + "\n\nYou can see every booking behind your earnings in the BookDan Partner app, under Earnings.\n\nBookDan\n"
	body := para("BookDan has paid "+formatETB(amount)+" to "+hotel+".") + para("Transfer reference: "+reference) + para("You can see every booking behind your earnings in the BookDan Partner app, under Earnings.")
	for rows.Next() {
		var to string
		if rows.Scan(&to) == nil && to != "" {
			s.sendMail(ctx, to, emailContent{"BookDan paid " + formatETB(amount) + " to " + hotel, text, frame("A payout was sent", body)})
		}
	}
}

// formatETB writes an amount in santim as birr, for emails: 123456 -> "ETB 1,234.56".
func formatETB(santim int64) string {
	birr, cents := santim/100, santim%100
	digits := itoa(birr)
	var groups []string
	for len(digits) > 3 {
		groups = append([]string{digits[len(digits)-3:]}, groups...)
		digits = digits[:len(digits)-3]
	}
	groups = append([]string{digits}, groups...)
	c := itoa(cents)
	if len(c) < 2 {
		c = "0" + c
	}
	return "ETB " + strings.Join(groups, ",") + "." + c
}
