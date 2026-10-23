package api

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// fcmPusher sends alerts through Firebase Cloud Messaging (HTTP v1 API) using a service-account key. Nothing but the Go
// standard library is needed: it signs a short-lived Google sign-in token itself, trades it for an access token, and
// caches that until shortly before it expires.
type fcmPusher struct {
	projectID string
	email     string
	key       *rsa.PrivateKey
	tokenURI  string // where the access token comes from
	sendURL   string // https://fcm.googleapis.com/v1/projects/<id>/messages:send
	client    *http.Client

	mu      sync.Mutex
	access  string
	expires time.Time
}

type serviceAccount struct {
	ProjectID   string `json:"project_id"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// newFCMPusher reads the service-account JSON (the file Firebase gives you), given as base64 so it fits in one
// environment variable.
func newFCMPusher(serviceAccountB64 string) (*fcmPusher, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(serviceAccountB64))
	if err != nil {
		return nil, errors.New("FCM_SERVICE_ACCOUNT_B64 must be the service-account JSON file, base64 encoded")
	}
	var sa serviceAccount
	if err := json.Unmarshal(raw, &sa); err != nil || sa.ProjectID == "" || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("FCM_SERVICE_ACCOUNT_B64 is not a Firebase service-account key (project_id, client_email, private_key)")
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return nil, errors.New("the service-account private_key is not valid PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("the service-account private_key cannot be read: %w", err)
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("the service-account private_key is not an RSA key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = "https://oauth2.googleapis.com/token"
	}
	return &fcmPusher{
		projectID: sa.ProjectID, email: sa.ClientEmail, key: key, tokenURI: sa.TokenURI,
		sendURL: "https://fcm.googleapis.com/v1/projects/" + sa.ProjectID + "/messages:send",
		client:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (p *fcmPusher) accessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.access != "" && time.Now().Before(p.expires.Add(-time.Minute)) {
		return p.access, nil
	}
	now := time.Now()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{
		"iss": p.email, "scope": "https://www.googleapis.com/auth/firebase.messaging", "aud": p.tokenURI,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	signing := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {signing + "." + base64.RawURLEncoding.EncodeToString(sig)}}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURI, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if res.StatusCode != http.StatusOK || json.Unmarshal(body, &out) != nil || out.AccessToken == "" {
		return "", fmt.Errorf("google sign-in for FCM failed (%d)", res.StatusCode)
	}
	p.access, p.expires = out.AccessToken, now.Add(time.Duration(out.ExpiresIn)*time.Second)
	return p.access, nil
}

// Notify sends one message per phone. A phone whose token Firebase no longer knows is returned in invalid, so the server
// can forget it. data["channel"], if present, names the Android channel (the hotel app's loud one, for example).
func (p *fcmPusher) Notify(ctx context.Context, tokens []string, title, body string, data map[string]string) (invalid []string, err error) {
	access, err := p.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	var firstErr error
	for _, token := range tokens {
		android := map[string]any{"priority": "HIGH"}
		if ch := data["channel"]; ch != "" {
			android["notification"] = map[string]any{"channel_id": ch}
		}
		payload, _ := json.Marshal(map[string]any{"message": map[string]any{
			"token":        token,
			"notification": map[string]string{"title": title, "body": body},
			"data":         data,
			"android":      android,
		}})
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.sendURL, bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+access)
		req.Header.Set("Content-Type", "application/json")
		res, err := p.client.Do(req)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		resp, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
		res.Body.Close()
		switch {
		case res.StatusCode == http.StatusOK:
		case res.StatusCode == http.StatusNotFound || (res.StatusCode == http.StatusBadRequest && bytes.Contains(resp, []byte("registration token"))):
			invalid = append(invalid, token) // uninstalled app or rotated token
		default:
			if firstErr == nil {
				firstErr = fmt.Errorf("fcm send failed (%d)", res.StatusCode)
			}
		}
	}
	return invalid, firstErr
}
