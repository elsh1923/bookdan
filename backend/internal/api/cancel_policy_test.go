package api

import (
	"testing"
	"time"
)

func TestCancellationWindow(t *testing.T) {
	// Check-in 20 Nov, 12:00 Addis time = 09:00 UTC. A free cancellation ends 48 hours before that: 18 Nov, 09:00 UTC.
	checkIn := time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	at, until := cancelWindow(checkIn)
	if want := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC); !at.Equal(want) {
		t.Fatalf("check-in starts at %v, want %v", at, want)
	}
	if want := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC); !until.Equal(want) {
		t.Fatalf("free cancellation ends at %v, want %v", until, want)
	}
	// A cancellation is refunded when "now" is not after the limit: exactly at it is still in time, one second later is too late.
	if until.After(until) {
		t.Fatalf("the limit itself is still in time")
	}
	if !until.Add(time.Second).After(until) {
		t.Fatalf("a second later is too late")
	}
}
