package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/bookdan/backend/internal/httpx"
	"github.com/bookdan/backend/internal/ops"
)

// Monthly statements for hotels, and the commission on pay-at-hotel stays.
//
// Pay at hotel: the guest pays the hotel directly, so BookDan holds nothing for the stay. When the stay is completed, BookDan books its commission
// (and the VAT on it) against the hotel: a debit on property_payable, so it reduces what BookDan owes the hotel for online bookings, and if the hotel has
// only pay-at-hotel stays the balance goes negative: the hotel owes BookDan (balance.owes_santim). The hotel pays that by transfer and an admin
// records it as a receipt (the opposite of a payout). The commission is figured as for an online booking, on the booking's total.
//
// A statement covers one calendar month (Addis Ababa): the stays that ended in it (online ones with the hotel's share, commission and VAT; pay-at-hotel
// ones with the commission owed), the payouts and receipts of the month, and the balance at the start and at the end. Owners read it in the partner app
// (or as a CSV), the admin reads any hotel's, and the owners are emailed the previous month's statement once, after the month ends.

var monthRE = regexp.MustCompile(`^[0-9]{4}-(0[1-9]|1[0-2])$`)

type statementLine struct {
	Date       string `json:"date"` // the check-out day
	Reference  string `json:"reference"`
	Guest      string `json:"guest_name"`
	Nights     int    `json:"nights"`
	Method     string `json:"method"` // online | pay_at_hotel
	Status     string `json:"status"`
	Gross      int64  `json:"gross_santim"` // the booking's price: paid to BookDan (online) or to the hotel (pay at hotel)
	Discount   int64  `json:"discount_santim"`
	Commission int64  `json:"commission_santim"`
	VAT        int64  `json:"vat_santim"`
	Net        int64  `json:"net_santim"` // what it adds to the hotel's balance: its share (online) or minus the commission owed (pay at hotel)
}

type moneyMove struct {
	Date      string `json:"date"`
	Amount    int64  `json:"amount_santim"`
	Reference string `json:"reference"`
}

type statementOut struct {
	PropertyID string `json:"property_id"`
	Hotel      string `json:"hotel"`
	Month      string `json:"month"`
	From       string `json:"from"`
	To         string `json:"to"` // the last day
	Currency   string `json:"currency"`
	Opening    int64  `json:"opening_santim"`
	Closing    int64  `json:"closing_santim"`
	// Online stays: what the guests paid BookDan and how it splits.
	OnlineGross      int64 `json:"online_gross_santim"`
	OnlineCommission int64 `json:"online_commission_santim"`
	OnlineVAT        int64 `json:"online_vat_santim"`
	OnlineShare      int64 `json:"online_hotel_share_santim"`
	// Pay at hotel: what the hotel collected itself and the commission it owes.
	PayAtHotelCollected  int64           `json:"pay_at_hotel_collected_santim"`
	PayAtHotelCommission int64           `json:"pay_at_hotel_commission_santim"`
	PayAtHotelVAT        int64           `json:"pay_at_hotel_vat_santim"`
	Paid                 int64           `json:"paid_santim"`
	Received             int64           `json:"received_santim"`
	Lines                []statementLine `json:"lines"`
	Payouts              []moneyMove     `json:"payouts"`
	Receipts             []moneyMove     `json:"receipts"`
}

// addisMonth is the month ("2026-10") a moment falls in, in Addis Ababa.
func addisMonth(t time.Time) string { return t.UTC().Add(3 * time.Hour).Format("2006-01") }

func previousMonth(month string) string {
	t, _ := time.Parse("2006-01", month)
	return t.AddDate(0, -1, 0).Format("2006-01")
}

// monthRange returns the first day of the month and of the next one.
func monthRange(month string) (string, string, error) {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return "", "", err
	}
	return t.Format("2006-01-02"), t.AddDate(0, 1, 0).Format("2006-01-02"), nil
}

const addisDate = `(%s AT TIME ZONE 'Africa/Addis_Ababa')::date`

func (s *Server) statementFor(ctx context.Context, propertyID, month string) (statementOut, error) {
	st := statementOut{PropertyID: propertyID, Month: month, Currency: "ETB", Lines: []statementLine{}, Payouts: []moneyMove{}, Receipts: []moneyMove{}}
	from, next, err := monthRange(month)
	if err != nil {
		return st, fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "month must look like 2026-10")
	}
	st.From = from
	last, _ := time.Parse("2006-01-02", next)
	st.To = last.AddDate(0, 0, -1).Format("2006-01-02")
	if err := s.pool.QueryRow(ctx, `SELECT name FROM properties WHERE id = $1`, propertyID).Scan(&st.Hotel); err != nil {
		if err == pgx.ErrNoRows || isBadUUID(err) {
			return st, errNotFound
		}
		return st, err
	}
	ids := []string{propertyID}

	// The balance at a date: the hotel's share of every stay that ended before it, less payouts, plus receipts.
	balanceAt := func(day string) (int64, error) {
		var earned, paid, recv int64
		if err := s.pool.QueryRow(ctx, `WITH m AS (`+bookingMoneySQL+`) SELECT coalesce(sum(net), 0)::bigint FROM m WHERE ready AND check_out < $2::date`, ids, day).Scan(&earned); err != nil {
			return 0, err
		}
		if err := s.pool.QueryRow(ctx, `SELECT coalesce(sum(amount_santim), 0)::bigint FROM hotel_payouts WHERE property_id = $1 AND `+fmt.Sprintf(addisDate, "paid_at")+` < $2::date`, propertyID, day).Scan(&paid); err != nil {
			return 0, err
		}
		if err := s.pool.QueryRow(ctx, `SELECT coalesce(sum(amount_santim), 0)::bigint FROM hotel_receipts WHERE property_id = $1 AND `+fmt.Sprintf(addisDate, "received_at")+` < $2::date`, propertyID, day).Scan(&recv); err != nil {
			return 0, err
		}
		return earned - paid + recv, nil
	}
	if st.Opening, err = balanceAt(from); err != nil {
		return st, err
	}

	rows, err := s.pool.Query(ctx, `WITH m AS (`+bookingMoneySQL+`) SELECT m.code, m.guest_name, m.check_in, m.check_out, m.status, m.gross, m.discount, m.commission, m.vat, m.net, b.payment_method, b.total_santim
		FROM m JOIN bookings b ON b.id = m.id WHERE m.ready AND m.check_out >= $2::date AND m.check_out < $3::date ORDER BY m.check_out, m.code`, ids, from, next)
	if err != nil {
		return st, err
	}
	defer rows.Close()
	var movement int64
	for rows.Next() {
		var l statementLine
		var in, out time.Time
		var method *string
		var total int64
		if err := rows.Scan(&l.Reference, &l.Guest, &in, &out, &l.Status, &l.Gross, &l.Discount, &l.Commission, &l.VAT, &l.Net, &method, &total); err != nil {
			return st, err
		}
		l.Date, l.Nights, l.Method = pgDate(out), nightsBetween(pgDate(in), pgDate(out)), "online"
		if method != nil && *method == "pay_at_hotel" {
			l.Method, l.Gross = "pay_at_hotel", total
			st.PayAtHotelCollected += total
			st.PayAtHotelCommission += l.Commission
			st.PayAtHotelVAT += l.VAT
		} else {
			st.OnlineGross += l.Gross
			st.OnlineCommission += l.Commission
			st.OnlineVAT += l.VAT
			st.OnlineShare += l.Net
		}
		movement += l.Net
		st.Lines = append(st.Lines, l)
	}
	if err := rows.Err(); err != nil {
		return st, err
	}
	rows.Close()

	for _, q := range []struct {
		sql string
		dst *[]moneyMove
		sum *int64
	}{
		{`SELECT ` + fmt.Sprintf(addisDate, "paid_at") + `, amount_santim, reference FROM hotel_payouts WHERE property_id = $1 AND ` + fmt.Sprintf(addisDate, "paid_at") + ` >= $2::date AND ` + fmt.Sprintf(addisDate, "paid_at") + ` < $3::date ORDER BY paid_at`, &st.Payouts, &st.Paid},
		{`SELECT ` + fmt.Sprintf(addisDate, "received_at") + `, amount_santim, reference FROM hotel_receipts WHERE property_id = $1 AND ` + fmt.Sprintf(addisDate, "received_at") + ` >= $2::date AND ` + fmt.Sprintf(addisDate, "received_at") + ` < $3::date ORDER BY received_at`, &st.Receipts, &st.Received},
	} {
		mr, err := s.pool.Query(ctx, q.sql, propertyID, from, next)
		if err != nil {
			return st, err
		}
		for mr.Next() {
			var m moneyMove
			var d time.Time
			if err := mr.Scan(&d, &m.Amount, &m.Reference); err != nil {
				mr.Close()
				return st, err
			}
			m.Date = pgDate(d)
			*q.dst = append(*q.dst, m)
			*q.sum += m.Amount
		}
		mr.Close()
	}
	st.Closing = st.Opening + movement - st.Paid + st.Received
	return st, nil
}

func birr(santim int64) string {
	sign := ""
	if santim < 0 {
		sign, santim = "-", -santim
	}
	return fmt.Sprintf("%s%d.%02d", sign, santim/100, santim%100)
}

func csvCell(v string) string {
	// A cell that starts like a formula must not be run by a spreadsheet: it is prefixed so it stays text (a plain number such as -12.50 is left alone).
	if v != "" && strings.ContainsRune("=+-@", rune(v[0])) && !isNumber(v) {
		v = "'" + v
	}
	if strings.ContainsAny(v, ",\"\n\r") {
		return "\"" + strings.ReplaceAll(v, "\"", "\"\"") + "\""
	}
	return v
}

func isNumber(v string) bool {
	_, err := fmt.Sscanf(v, "%f", new(float64))
	return err == nil
}

// statementCSV is the statement as a spreadsheet: a header, one row per stay, then the totals. Amounts are in birr.
func statementCSV(st statementOut) string {
	var b strings.Builder
	row := func(cells ...string) {
		for i, c := range cells {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(csvCell(c))
		}
		b.WriteString("\r\n")
	}
	row("BookDan statement", st.Hotel, st.Month)
	row("Opening balance (ETB)", birr(st.Opening))
	row()
	row("Check-out", "Reference", "Guest", "Nights", "How paid", "Status", "Price", "Coupon discount", "BookDan commission", "VAT on commission", "Added to your balance")
	for _, l := range st.Lines {
		how := "paid online to BookDan"
		if l.Method == "pay_at_hotel" {
			how = "paid at the hotel"
		}
		row(l.Date, l.Reference, l.Guest, fmt.Sprint(l.Nights), how, l.Status, birr(l.Gross), birr(l.Discount), birr(l.Commission), birr(l.VAT), birr(l.Net))
	}
	row()
	row("Online bookings: the guests paid BookDan", birr(st.OnlineGross))
	row("Online bookings: BookDan commission", birr(st.OnlineCommission))
	row("Online bookings: VAT on the commission", birr(st.OnlineVAT))
	row("Online bookings: your share", birr(st.OnlineShare))
	row("Pay at hotel: collected by the hotel", birr(st.PayAtHotelCollected))
	row("Pay at hotel: commission you owe BookDan", birr(st.PayAtHotelCommission))
	row("Pay at hotel: VAT on that commission", birr(st.PayAtHotelVAT))
	for _, p := range st.Payouts {
		row("Paid to you "+p.Date, birr(p.Amount), p.Reference)
	}
	for _, p := range st.Receipts {
		row("Received from you "+p.Date, birr(p.Amount), p.Reference)
	}
	row("Closing balance (ETB): positive is owed to you, negative is owed to BookDan", birr(st.Closing))
	return b.String()
}

func (s *Server) parseMonth(r *http.Request) (string, error) {
	m := r.URL.Query().Get("month")
	if m == "" {
		return previousMonth(addisMonth(s.now())), nil
	}
	if !monthRE.MatchString(m) {
		return "", fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "month must look like 2026-10")
	}
	if m > addisMonth(s.now()) {
		return "", fail(http.StatusBadRequest, httpx.CodeInvalidRequest, "That month has not started yet")
	}
	return m, nil
}

func respondStatements(w http.ResponseWriter, r *http.Request, list []statementOut) {
	if r.URL.Query().Get("format") == "csv" && len(list) == 1 {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="bookdan-statement-`+list[0].Month+`.csv"`)
		_, _ = w.Write([]byte(statementCSV(list[0])))
		return
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		Statements []statementOut `json:"statements"`
	}{httpx.NowMeta(), list})
}

// partnerStatements: the owner's statements for one month (default: last month), one per hotel they own, or one hotel with ?property_id=.
func (s *Server) partnerStatements(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	owned, err := s.ownerHotels(ctx, claimsFrom(r).Sub)
	if err != nil {
		return err
	}
	if len(owned) == 0 {
		return fail(http.StatusForbidden, codeForbidden, "Only a hotel owner can read statements")
	}
	month, err := s.parseMonth(r)
	if err != nil {
		return err
	}
	if only := r.URL.Query().Get("property_id"); only != "" {
		found := false
		for _, id := range owned {
			if id == only {
				found = true
			}
		}
		if !found {
			return errNotFound
		}
		owned = []string{only}
	}
	list := []statementOut{}
	for _, id := range owned {
		st, err := s.statementFor(ctx, id, month)
		if err != nil {
			return err
		}
		list = append(list, st)
	}
	respondStatements(w, r, list)
	return nil
}

func (s *Server) adminStatement(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	month, err := s.parseMonth(r)
	if err != nil {
		return err
	}
	st, err := s.statementFor(r.Context(), chi.URLParam(r, "id"), month)
	if err != nil {
		return err
	}
	respondStatements(w, r, []statementOut{st})
	return nil
}

// ---- pay at hotel: the commission BookDan books, and the money the hotel pays back

// accrueHotelCommissions books BookDan's commission on completed pay-at-hotel stays that have none yet (also catches stays completed before this existed).
func (s *Server) accrueHotelCommissions(ctx context.Context) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM bookings WHERE status = 'COMPLETED' AND payment_method = 'pay_at_hotel' AND hotel_commission_booked_at IS NULL ORDER BY check_out LIMIT 500`)
	if err != nil {
		slog.Error("pay-at-hotel commission: list", "err", err)
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
			var total int64
			var cur string
			var marked *time.Time
			if err := tx.QueryRow(ctx, `SELECT total_santim, currency, hotel_commission_booked_at FROM bookings WHERE id = $1 AND status = 'COMPLETED' FOR UPDATE`, id).Scan(&total, &cur, &marked); err != nil {
				if err == pgx.ErrNoRows {
					return nil
				}
				return err
			}
			if marked != nil {
				return nil
			}
			var commBP, commVatBP int64
			if err := tx.QueryRow(ctx, `SELECT commission_bps, commission_vat_bps FROM pricing_config WHERE id = 1`).Scan(&commBP, &commVatBP); err != nil {
				return err
			}
			commission := (total*commBP + 5000) / 10000
			vat := (commission*commVatBP + 5000) / 10000
			if commission+vat > 0 {
				if err := ledger(ctx, tx, id, "", "adjustment", "pay at hotel: the hotel owes BookDan's commission and its VAT", cur,
					entry{"property_payable", commission + vat}, entry{"commission_revenue", -commission}, entry{"vat_on_commission_payable", -vat}); err != nil {
					return err
				}
			}
			_, err := tx.Exec(ctx, `UPDATE bookings SET hotel_commission_booked_at = now() WHERE id = $1`, id)
			return err
		})
		if err != nil {
			slog.Error("pay-at-hotel commission failed", "booking", id, "err", err)
			ops.Report("job-commission", "Booking the commission on a pay-at-hotel stay failed", "Booking "+id+": "+err.Error())
		}
	}
}

// adminRecordReceipt writes down money a hotel paid BookDan (the commission on its pay-at-hotel stays). It cannot exceed what the hotel owes.
func (s *Server) adminRecordReceipt(w http.ResponseWriter, r *http.Request) error {
	if err := requireAdmin(r); err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
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
	uid, ctx := claimsFrom(r).Sub, r.Context()
	var receiptID string
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var hotel string
		if err := tx.QueryRow(ctx, `SELECT name FROM properties WHERE id = $1 FOR UPDATE`, id).Scan(&hotel); err != nil {
			if err == pgx.ErrNoRows || isBadUUID(err) {
				return errNotFound
			}
			return err
		}
		bals, err := s.balances(ctx, tx, []string{id})
		if err != nil {
			return err
		}
		if len(bals) == 0 || in.Amount > bals[0].Owes {
			return fail(http.StatusConflict, codeInvalidState, "That is more than this hotel owes BookDan")
		}
		var txID string
		if err := tx.QueryRow(ctx, `INSERT INTO ledger_transactions (property_id, kind, memo) VALUES ($1, 'payout', $2) RETURNING id`, id, "received from the hotel: "+in.Reference).Scan(&txID); err != nil {
			return err
		}
		for _, e := range []entry{{"gateway_clearing", in.Amount}, {"property_payable", -in.Amount}} {
			if _, err := tx.Exec(ctx, `INSERT INTO ledger_entries (transaction_id, account_code, amount_santim, currency) VALUES ($1, $2, $3, 'ETB')`, txID, e.account, e.santim); err != nil {
				return err
			}
		}
		if err := tx.QueryRow(ctx, `INSERT INTO hotel_receipts (property_id, amount_santim, reference, note, received_by, ledger_transaction_id) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6) RETURNING id`,
			id, in.Amount, in.Reference, in.Note, uid, txID).Scan(&receiptID); err != nil {
			return err
		}
		return s.adminAudit(r, tx, "admin_receipt_recorded", id, map[string]any{"amount_santim": in.Amount, "reference": in.Reference, "receipt_id": receiptID})
	})
	if err != nil {
		return err
	}
	bals, err := s.balances(ctx, s.pool, []string{id})
	if err != nil {
		return err
	}
	respond(w, http.StatusOK, struct {
		httpx.Meta
		ReceiptID string  `json:"receipt_id"`
		Balance   balance `json:"balance"`
	}{httpx.NowMeta(), receiptID, bals[0]})
	return nil
}

// ---- the monthly email

// emailStatements sends each hotel's owners (and its business contact) last month's statement, once. A restart never repeats it.
func (s *Server) emailStatements(ctx context.Context) {
	month := previousMonth(addisMonth(s.now()))
	rows, err := s.pool.Query(ctx, `SELECT p.id FROM properties p WHERE p.status = 'active' AND NOT EXISTS (SELECT 1 FROM statement_emails e WHERE e.property_id = p.id AND e.month = $1)`, month)
	if err != nil {
		slog.Error("statement emails: list", "err", err)
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
		st, err := s.statementFor(ctx, id, month)
		if err != nil {
			slog.Error("statement emails: build", "hotel", id, "err", err)
			continue
		}
		sent := 0
		if len(st.Lines)+len(st.Payouts)+len(st.Receipts) > 0 || st.Opening != 0 {
			rr, err := s.pool.Query(ctx, `SELECT coalesce(u.email, '') FROM property_staff ps JOIN users u ON u.id = ps.user_id WHERE ps.property_id = $1 AND ps.role = 'owner'
				UNION SELECT contact_email FROM hotel_business WHERE property_id = $1`, id)
			if err != nil {
				slog.Error("statement emails: recipients", "hotel", id, "err", err)
				continue
			}
			var to []string
			for rr.Next() {
				var e string
				if rr.Scan(&e) == nil && e != "" && !strings.HasSuffix(e, "@deleted.invalid") {
					to = append(to, e)
				}
			}
			rr.Close()
			text := statementText(st)
			for _, e := range to {
				if err := s.p.Mail.SendMail(ctx, e, "Your BookDan statement for "+st.Month+": "+st.Hotel, text, ""); err != nil {
					slog.Error("statement emails: send", "to", e, "err", err)
					continue
				}
				sent++
			}
			if len(to) > 0 && sent == 0 {
				continue // nothing got through: try again tomorrow
			}
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO statement_emails (property_id, month, recipients) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, id, month, sent); err != nil {
			slog.Error("statement emails: record", "hotel", id, "err", err)
		}
	}
}

// statementText is the statement as an email: the summary and the CSV below it (a spreadsheet can import it).
func statementText(st statementOut) string {
	var b strings.Builder
	fmt.Fprintf(&b, "BookDan statement for %s: %s\n\n", st.Month, st.Hotel)
	fmt.Fprintf(&b, "Balance at the start of the month: %s ETB\n", birr(st.Opening))
	fmt.Fprintf(&b, "Stays that ended this month: %d\n", len(st.Lines))
	fmt.Fprintf(&b, "Your share of online bookings: %s ETB (BookDan commission %s, VAT on it %s)\n", birr(st.OnlineShare), birr(st.OnlineCommission), birr(st.OnlineVAT))
	if st.PayAtHotelCollected > 0 {
		fmt.Fprintf(&b, "Pay at hotel: you collected %s ETB and owe BookDan %s ETB commission (VAT %s)\n", birr(st.PayAtHotelCollected), birr(st.PayAtHotelCommission), birr(st.PayAtHotelVAT))
	}
	fmt.Fprintf(&b, "Paid to you: %s ETB\nReceived from you: %s ETB\n", birr(st.Paid), birr(st.Received))
	if st.Closing >= 0 {
		fmt.Fprintf(&b, "Balance at the end of the month: %s ETB, owed to you\n", birr(st.Closing))
	} else {
		fmt.Fprintf(&b, "Balance at the end of the month: %s ETB, owed to BookDan\n", birr(-st.Closing))
	}
	b.WriteString("\nThe same statement is in the BookDan Partner app, under Earnings, where you can also download it.\n\n--- spreadsheet (CSV) ---\n")
	b.WriteString(statementCSV(st))
	b.WriteString("\nBookDan\n")
	return b.String()
}
