package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SMS sign-in codes. Two providers are built behind the OTPSender interface, chosen with OTP_PROVIDER:
//   - africastalking: Africa's Talking (https://developers.africastalking.com), a common route to Ethiopian numbers;
//   - twilio: Twilio Programmable Messaging.
// "off" turns phone sign-in off (a launch with email sign-in only); "log" only prints the code and is for development.
// Both senders only send to an Ethiopian mobile number in international form (+2519... or +2517...) and never log the code or the message.
// They are written against the providers' documented HTTP APIs and tested against fakes of them; a first real SMS to your own phone, with
// real credentials, is the final check (see docs/sms-setup.md).

var errPhoneSignInOff = errors.New("phone sign-in is switched off")

// smsText is the whole message: the code, how long it lasts, and a warning. Short enough for one SMS segment.
func smsText(code string) string {
	return "BookDan: your code is " + code + ". It expires in 5 minutes. Never share it."
}

func checkSMS(channel, to string) error {
	if channel != "sms" {
		return fmt.Errorf("the SMS sender cannot send a %s message", channel)
	}
	if !phoneRE.MatchString(to) {
		return errors.New("refusing to send an SMS to anything but an Ethiopian mobile number (+2519... or +2517...)")
	}
	return nil
}

// offSender is what phone sign-in becomes when OTP_PROVIDER=off.
type offSender struct{}

func (offSender) Send(context.Context, string, string, string) error { return errPhoneSignInOff }

// ---- Africa's Talking

type atSender struct {
	username, apiKey, from, url string
	client                      *http.Client
}

const (
	atLiveURL    = "https://api.africastalking.com/version1/messaging"
	atSandboxURL = "https://api.sandbox.africastalking.com/version1/messaging"
)

// newAfricasTalking builds the sender. from is an optional approved sender id; sandbox uses Africa's Talking's test environment (username "sandbox").
func newAfricasTalking(username, apiKey, from string, sandbox bool) (*atSender, error) {
	if username == "" || apiKey == "" {
		return nil, errors.New("OTP_PROVIDER=africastalking needs AT_USERNAME and AT_API_KEY")
	}
	u := atLiveURL
	if sandbox {
		u = atSandboxURL
	}
	return &atSender{username: username, apiKey: apiKey, from: from, url: u, client: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (a *atSender) Send(ctx context.Context, channel, to, code string) error {
	if err := checkSMS(channel, to); err != nil {
		return err
	}
	form := url.Values{"username": {a.username}, "to": {to}, "message": {smsText(code)}}
	if a.from != "" {
		form.Set("from", a.from)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("apiKey", a.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to Africa's Talking: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<14))
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("Africa's Talking refused the SMS (%d): %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		SMSMessageData struct {
			Recipients []struct {
				StatusCode int    `json:"statusCode"`
				Status     string `json:"status"`
			} `json:"Recipients"`
			Message string `json:"Message"`
		} `json:"SMSMessageData"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return fmt.Errorf("Africa's Talking answered something unexpected: %w", err)
	}
	if len(out.SMSMessageData.Recipients) == 0 {
		return fmt.Errorf("Africa's Talking accepted no recipient: %s", out.SMSMessageData.Message)
	}
	// 100 processed, 101 sent, 102 queued: the message is on its way. Anything else (invalid number, no balance, blacklisted) is a failure.
	if c := out.SMSMessageData.Recipients[0].StatusCode; c < 100 || c > 102 {
		return fmt.Errorf("Africa's Talking did not send the SMS (%d %s)", c, out.SMSMessageData.Recipients[0].Status)
	}
	return nil
}

// ---- Twilio

type twilioSender struct {
	sid, token, from, url string
	client                *http.Client
}

// newTwilio builds the sender. from is a Twilio number in international form, an approved alphanumeric sender id, or a Messaging Service id (starts with MG).
func newTwilio(sid, token, from string) (*twilioSender, error) {
	if sid == "" || token == "" || from == "" {
		return nil, errors.New("OTP_PROVIDER=twilio needs TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN and SMS_FROM")
	}
	return &twilioSender{sid: sid, token: token, from: from, url: "https://api.twilio.com/2010-04-01/Accounts/" + url.PathEscape(sid) + "/Messages.json", client: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (t *twilioSender) Send(ctx context.Context, channel, to, code string) error {
	if err := checkSMS(channel, to); err != nil {
		return err
	}
	form := url.Values{"To": {to}, "Body": {smsText(code)}}
	if strings.HasPrefix(t.from, "MG") {
		form.Set("MessagingServiceSid", t.from)
	} else {
		form.Set("From", t.from)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(t.sid, t.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to Twilio: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<14))
	if res.StatusCode/100 != 2 {
		var e struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &e)
		return fmt.Errorf("Twilio refused the SMS (%d, code %d): %s", res.StatusCode, e.Code, e.Message)
	}
	return nil
}
