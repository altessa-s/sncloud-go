// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"testing"
	"time"
)

func TestRetryAfterHTTPDates(t *testing.T) {
	now := time.Date(2026, time.November, 18, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		header  string
		seconds float64
		valid   bool
	}{
		{"Wed, 18 Nov 2026 10:00:12 GMT", 12, true},
		{"Wednesday, 18-Nov-26 10:00:12 GMT", 12, true},
		{"Wed Nov 18 10:00:12 2026", 12, true},
		{"Wed, 18 Nov 2026 09:59:00 GMT", 0, true},
		{" 1.5 ", 1.5, true}, {"invalid", 0, false}, {"NaN", 0, false}, {"+Inf", 0, false}, {"-1", 0, false},
	} {
		e := FromHTTP(429, "/", tc.header, now).(*RateLimitedError)
		if e.HasRetryAfter != tc.valid || (tc.valid && e.RetryAfter != tc.seconds) {
			t.Errorf("%q: %+v", tc.header, e)
		}
	}
}
