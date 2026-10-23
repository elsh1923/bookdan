package api

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

func testServiceAccount(t *testing.T, tokenURI string) (string, *rsa.PublicKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	raw, _ := json.Marshal(map[string]string{"project_id": "bookdan-test", "client_email": "push@bookdan-test.iam.gserviceaccount.com", "private_key": pemKey, "token_uri": tokenURI})
	return base64.StdEncoding.EncodeToString(raw), &key.PublicKey
}

func TestFCMSignsInOncePerHourAndSendsTheMessage(t *testing.T) {
	var mu sync.Mutex
	var tokenCalls int
	var sent []map[string]any
	var pub *rsa.PublicKey
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/token":
			tokenCalls++
			// The sign-in request must carry a Google assertion signed with the service account's key.
			form, _ := url.ParseQuery(string(body))
			parts := strings.Split(form.Get("assertion"), ".")
			if len(parts) != 3 {
				http.Error(w, "bad assertion", http.StatusBadRequest)
				return
			}
			sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
			digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			if rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) != nil {
				http.Error(w, "bad signature", http.StatusBadRequest)
				return
			}
			claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
			if !strings.Contains(string(claims), "firebase.messaging") {
				http.Error(w, "wrong scope", http.StatusBadRequest)
				return
			}
			io.WriteString(w, `{"access_token":"ya29.test","expires_in":3600}`)
		case "/v1/projects/bookdan-test/messages:send":
			if r.Header.Get("Authorization") != "Bearer ya29.test" {
				http.Error(w, "no auth", http.StatusUnauthorized)
				return
			}
			var m map[string]map[string]any
			json.Unmarshal(body, &m)
			sent = append(sent, m["message"])
			if m["message"]["token"] == "gone" {
				w.WriteHeader(http.StatusNotFound)
				io.WriteString(w, `{"error":{"status":"NOT_FOUND","details":[{"errorCode":"UNREGISTERED"}]}}`)
				return
			}
			io.WriteString(w, `{"name":"projects/bookdan-test/messages/1"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	b64, key := testServiceAccount(t, srv.URL+"/token")
	pub = key
	p, err := newFCMPusher(b64)
	if err != nil {
		t.Fatal(err)
	}
	p.sendURL = srv.URL + "/v1/projects/bookdan-test/messages:send"

	invalid, err := p.Notify(context.Background(), []string{"phone-1", "gone"}, "New booking request", "Hanna · BD-ABC123", map[string]string{"booking_id": "b1", "channel": "booking_alerts"})
	if err != nil {
		t.Fatal(err)
	}
	if len(invalid) != 1 || invalid[0] != "gone" {
		t.Fatalf("a token Firebase no longer knows must be reported, got %v", invalid)
	}
	if _, err := p.Notify(context.Background(), []string{"phone-1"}, "x", "y", nil); err != nil {
		t.Fatal(err)
	}
	if tokenCalls != 1 {
		t.Fatalf("the access token must be reused, signed in %d times", tokenCalls)
	}
	first := sent[0]
	n := first["notification"].(map[string]any)
	d := first["data"].(map[string]any)
	a := first["android"].(map[string]any)
	if n["title"] != "New booking request" || d["booking_id"] != "b1" || a["priority"] != "HIGH" || a["notification"].(map[string]any)["channel_id"] != "booking_alerts" {
		t.Fatalf("unexpected message: %v", first)
	}
}

func TestFCMRefusesAKeyThatIsNotAServiceAccount(t *testing.T) {
	for name, v := range map[string]string{
		"not base64":   "%%%",
		"not json":     base64.StdEncoding.EncodeToString([]byte("hello")),
		"missing keys": base64.StdEncoding.EncodeToString([]byte(`{"project_id":"x"}`)),
		"bad pem":      base64.StdEncoding.EncodeToString([]byte(`{"project_id":"x","client_email":"a@b","private_key":"nope"}`)),
	} {
		if _, err := newFCMPusher(v); err == nil {
			t.Errorf("%s should be refused", name)
		}
	}
}
