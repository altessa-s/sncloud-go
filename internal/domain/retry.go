// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"errors"
	"math"
	"time"
)

// Total attempts in the official client policy.
const defaultAttempts = 3

// Initial retry wait.
const defaultBaseDelay = 300 * time.Millisecond

// Maximum retry wait, including server hints.
const defaultMaxDelay = 20 * time.Second

// Exponential backoff factor.
const defaultMultiplier = 3

// Symmetric twenty-percent jitter.
const defaultJitter = .2

// The second attempt is the first retry and uses the base delay.
const firstRetryAttempt = 2

// RetryPolicy controls total attempts and capped exponential backoff.
type RetryPolicy struct {
	MaxAttempts         int
	BaseDelay, MaxDelay time.Duration
	Multiplier, Jitter  float64
}

// DefaultRetryPolicy matches the official client's port policy.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: defaultAttempts,
		BaseDelay:   defaultBaseDelay,
		MaxDelay:    defaultMaxDelay,
		Multiplier:  defaultMultiplier,
		Jitter:      defaultJitter,
	}
}

// ShouldRetry lets callers apply the client policy to their own protocol operations.
// It permits only transport failures, 5xx responses and explicit waits; changes
// marks a mutation, for which only explicit rate limits permit another attempt.
func (p RetryPolicy) ShouldRetry(err error, changes bool) bool {
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	if changes {
		return false
	}
	e, ok := errors.AsType[*HTTPError](err)
	return errors.Is(err, ErrTransport) || ok && e.Status >= 500 && e.Status <= 599
}

// Delay computes a wait for callers coordinating retries or feed reconnections.
// Attempt is the one-based attempt number; random is in [-1,1].
// Server waits get no jitter and are clamped before duration conversion.
func (p RetryPolicy) Delay(attempt int, after *float64, random float64) time.Duration {
	ceiling := math.Max(0, float64(p.MaxDelay))
	if after != nil && *after > 0 && !math.IsInf(*after, 0) && !math.IsNaN(*after) {
		return time.Duration(math.Min(*after*float64(time.Second), ceiling))
	}
	n := math.Min(float64(p.BaseDelay)*math.Pow(p.Multiplier, float64(max(0, attempt-firstRetryAttempt))), ceiling)
	n += n * p.Jitter * random
	if math.IsNaN(n) || math.IsInf(n, 0) {
		n = ceiling
	}
	return time.Duration(math.Min(math.Max(n, 0), ceiling))
}
