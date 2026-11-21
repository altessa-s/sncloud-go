// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"math"
	"testing"
	"time"
)

func TestRetriesTransientFailures(t *testing.T) {
	p := DefaultRetryPolicy()
	for _, tc := range []struct {
		err          error
		read, change bool
	}{
		{&TransportError{}, true, false},
		{&HTTPError{Status: 503}, true, false},
		{&RateLimitedError{}, true, true},
		{ErrInvalidCredentials, false, false},
		{&RejectedError{}, false, false},
		{&HTTPError{Status: 400}, false, false},
	} {
		if p.ShouldRetry(tc.err, false) != tc.read || p.ShouldRetry(tc.err, true) != tc.change {
			t.Fatal(tc)
		}
	}
}
func TestHonoursRetryAfter(t *testing.T) {
	p := DefaultRetryPolicy()
	for _, tc := range []struct {
		n    float64
		want time.Duration
	}{
		{3, 3 * time.Second},
		{3600, p.MaxDelay},
		{1e300, p.MaxDelay},
	} {
		if got := p.Delay(2, &tc.n, 0); got != tc.want {
			t.Fatal(got)
		}
	}
	if p.Delay(3, nil, 0) != 900*time.Millisecond {
		t.Fatal("backoff")
	}
}
func TestClampsAnAbsurdRetryAfter(t *testing.T) {
	p := DefaultRetryPolicy()
	inf := math.Inf(1)
	if p.Delay(2, &inf, 0) > p.MaxDelay || p.Delay(60, nil, 1) > p.MaxDelay {
		t.Fatal("unbounded wait")
	}
	e := FromHTTP(429, "/", "inf", time.Time{}).(*RateLimitedError)
	if e.HasRetryAfter {
		t.Fatal(e)
	}
}

func TestNonfiniteJitter(t *testing.T) {
	p := DefaultRetryPolicy()
	p.Jitter = math.NaN()
	if p.Delay(2, nil, 0) != p.MaxDelay {
		t.Fatal("NaN delay")
	}

}
