// Package httpx holds the response conventions shared by every handler:
// every body carries server_time, and every error has one shape.
package httpx

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// Stable error codes the mobile apps map to translated messages.
const (
	CodeNotImplemented  = "NOT_IMPLEMENTED"
	CodeNotFound        = "NOT_FOUND"
	CodeUnauthenticated = "UNAUTHENTICATED"
	CodeInvalidRequest  = "INVALID_REQUEST"
	CodeInternal        = "INTERNAL"
	CodeUnavailable     = "UNAVAILABLE"
)

// Meta is embedded in response structs so server_time is always present.
// Apps use it for countdowns instead of the phone's clock.
type Meta struct {
	ServerTime time.Time `json:"server_time"`
}

func NowMeta() Meta { return Meta{ServerTime: time.Now().UTC()} }

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
	Details   any    `json:"details,omitempty"`
}

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func Error(w http.ResponseWriter, r *http.Request, status int, code, message string, details any) {
	JSON(w, status, struct {
		Meta
		Error errorBody `json:"error"`
	}{NowMeta(), errorBody{Code: code, Message: message, RequestID: middleware.GetReqID(r.Context()), Details: details}})
}
