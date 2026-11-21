// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// HTTP status codes used by the protocol without importing the HTTP adapter stack.
const (
	statusUnauthorized    = 401
	statusTooManyRequests = 429
)

// FromEnvelope maps the service's codes, preserving unknown refusals.
func FromEnvelope(code, message string) error {
	code = strings.TrimSpace(code)
	switch code {
	case "E0712", "E0085":
		return ErrSessionExpired
	case "E0019", "E0708", "E0711":
		return ErrInvalidCredentials
	case "E0709", "E0710", "E0061":
		return &AccountLockedError{Code: code}
	case "E1760":
		return ErrVerificationRequired
	case "E1756", "E0101":
		return ErrInvalidVerificationCode
	case "E0322", "E0335", "E0337", "E0338":
		return ErrAlreadyExists
	case "E0081", "E0308", "E0311", "E0318", "E0320", "E0321", "E0328", "E0329", "E0336", "E0339", "E0340":
		return &NotFoundError{Code: code}
	case "E0078", "E0079", "E0301":
		return ErrSyncLocked
	default:
		return &RejectedError{Code: code, Message: message}
	}
}

// FromHTTP interprets status; envelope handling may refine a generic 4xx.
// now comes from the caller's clock so HTTP-date retry hints remain deterministic.
func FromHTTP(status int, path, retryAfter string, now time.Time) error {
	switch {
	case status >= 200 && status < 300:
		return nil
	case status == statusUnauthorized:
		return ErrSessionExpired
	case status == statusTooManyRequests:
		retryAfter = strings.TrimSpace(retryAfter)
		n, err := strconv.ParseFloat(retryAfter, 64)
		if err != nil {
			// Accept the three HTTP date forms without coupling the domain to net/http.
			for _, layout := range []string{time.RFC1123, time.RFC850, time.ANSIC} {
				if date, parseErr := time.Parse(layout, retryAfter); parseErr == nil {
					n, err = max(0, date.Sub(now).Seconds()), nil
					break
				}
			}
		}
		return &RateLimitedError{
			RetryAfter:    n,
			HasRetryAfter: err == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0,
		}
	default:
		return &HTTPError{Status: status, Path: path}
	}
}
