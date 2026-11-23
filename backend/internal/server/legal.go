package server

import (
	"html"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/bookdan/backend/internal/config"
)

// Public pages the app stores require: a privacy policy, terms of use and how to delete an account.
// They are plain HTML served by the API itself, so one deploy keeps the app and its legal pages together.
// The text is a plain-language draft and should be reviewed by a lawyer before the public launch.

const legalStyle = `<style>body{font:16px/1.6 system-ui,sans-serif;max-width:720px;margin:0 auto;padding:24px 16px;color:#0B192C}
h1{color:#E05A47}h2{margin-top:2em}a{color:#E05A47}.note{color:#555;font-size:14px}</style>`

func legalPage(title, body string) http.HandlerFunc {
	page := "<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">" +
		"<title>" + html.EscapeString(title) + " | BookDan</title>" + legalStyle + "</head><body><h1>" + html.EscapeString(title) + "</h1>" +
		"<p class=\"note\">Last updated: 18 November 2026</p>" + body + "</body></html>"
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = w.Write([]byte(page))
	}
}

func mountLegal(r chi.Router, cfg config.Config) {
	contact := "our support team through the Help section of the BookDan app"
	if e := strings.TrimSpace(cfg.SupportEmail); e != "" {
		contact = "us at <a href=\"mailto:" + html.EscapeString(e) + "\">" + html.EscapeString(e) + "</a>"
	}

	privacy := `<p>BookDan helps you find and book hotels in Ethiopia. This page explains what we collect, why, and the choices you have.</p>
<h2>What we collect</h2>
<ul>
<li><b>Sign-in details:</b> your phone number or email address, to send you a sign-in code.</li>
<li><b>Booking details:</b> your name, nationality and ID or passport number. Hotels in Ethiopia must record this for each guest.</li>
<li><b>Booking and payment records:</b> which hotel, dates, price and payment status. Card and mobile-money details are handled by our payment partner (Chapa); BookDan does not see or store them.</li>
<li><b>Location (only if you allow it):</b> to show hotels near you. We do not track you in the background.</li>
<li><b>Reviews and messages</b> you write, such as reviews and help requests.</li>
<li><b>Device information:</b> a device identifier and a notification token, so we can keep your sign-in safe and send booking alerts.</li>
</ul>
<h2>How we use it</h2>
<p>To make and manage your bookings, take payment and refunds, send booking messages, keep the service safe, prevent fraud, and answer help requests. We do not sell your personal data and we do not show advertising.</p>
<h2>Who sees it</h2>
<ul>
<li><b>The hotel you book</b> sees your name, contact details and ID, only after you request a room there.</li>
<li><b>Our payment partner</b> (Chapa) processes your payment.</li>
<li><b>Service providers</b> that send our messages and host our systems.</li>
<li><b>Authorities</b>, when the law requires it.</li>
</ul>
<h2>How we protect it</h2>
<p>Your ID number is stored encrypted and shown to people only in a shortened form, such as the last four characters. Each time a hotel views a full ID number, it is logged. Hotels may keep the full ID number only for a limited period after check-out, after which it is erased.</p>
<h2>How long we keep it</h2>
<p>We keep your account details while your account is open. Records of bookings and payments are kept for as long as the law and the hotels require, for accounting and dispute purposes.</p>
<h2>Your choices</h2>
<ul>
<li>See and correct your details in <b>Profile</b> in the app.</li>
<li><b>Delete your account</b> in the app (Profile, then Delete my account), or see <a href="/delete-account">how to request deletion</a>.</li>
<li>Turn location and notifications off in your phone settings at any time.</li>
</ul>
<h2>Children</h2>
<p>BookDan is for adults who can make a booking. We do not knowingly collect data from children.</p>
<h2>Contact</h2>
<p>Questions about your data? Contact ` + contact + `.</p>`

	terms := `<p>By using BookDan you agree to these terms. BookDan is a booking service: the stay itself is provided by the hotel you book.</p>
<h2>Bookings</h2>
<ul>
<li>When you request a room, we hold it for up to 15 minutes for you to pay. Some hotels let you pay at the hotel instead.</li>
<li>The hotel has 15 minutes to accept your request. If it does not accept, the booking is declined automatically and you are refunded in full.</li>
<li>All prices are in Ethiopian birr (ETB) and include the taxes and fees shown before you pay. Prices in other currencies are a guide only.</li>
</ul>
<h2>Cancellation and refunds</h2>
<ul>
<li>Cancel before the hotel answers: full refund.</li>
<li>Refundable rooms: free cancellation until 48 hours before check-in (12:00 Addis Ababa time). After that, no refund.</li>
<li>Non-refundable rooms: no refund once the hotel confirms. If the hotel cannot accept your booking, you get everything back.</li>
<li>If a hotel cannot host you after you have confirmed, we will help you find a similar stay or refund you.</li>
</ul>
<h2>Your responsibilities</h2>
<p>Give true details, bring the ID you booked with, follow the hotel's house rules, and do not misuse the app. We may suspend accounts that commit fraud or abuse the service.</p>
<h2>Reviews</h2>
<p>Reviews must be honest and about a stay you actually had. We may hide reviews that are abusive or false.</p>
<h2>Our responsibility</h2>
<p>We work to keep hotel information accurate, but hotels provide the rooms and services. To the extent the law allows, BookDan is not responsible for what happens during a stay, beyond refunds and help as described above.</p>
<h2>Changes</h2>
<p>We may update these terms. Important changes will be shown in the app.</p>
<h2>Contact</h2>
<p>Contact ` + contact + `.</p>
<p>See also our <a href="/privacy">Privacy Policy</a>.</p>`

	deletion := `<p>You can delete your BookDan account and the personal data linked to it at any time.</p>
<h2>In the app (fastest)</h2>
<ol><li>Open <b>BookDan</b> and sign in.</li><li>Go to <b>Profile</b>.</li><li>Tap <b>Delete my account</b> and confirm.</li></ol>
<h2>If you cannot use the app</h2>
<p>Contact ` + contact + ` from the email address or phone number on your account, and ask us to delete it. We will confirm the request and complete it.</p>
<h2>What is deleted, and what is kept</h2>
<ul>
<li><b>Deleted:</b> your saved details, ID number, phone numbers attached to bookings, and the text of your reviews.</li>
<li><b>Kept:</b> records of past bookings and payments, as the law and the hotels require, for accounting and disputes.</li>
</ul>
<p>An account with an unfinished booking can be deleted after that booking is cancelled or the stay is over.</p>
<p>See our <a href="/privacy">Privacy Policy</a>.</p>`

	r.Get("/privacy", legalPage("Privacy Policy", privacy))
	r.Get("/terms", legalPage("Terms of Use", terms))
	r.Get("/delete-account", legalPage("Delete your BookDan account", deletion))
}
