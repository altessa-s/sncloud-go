// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package retry drives the pure retry policy with a cancellable clock.
package retry

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"math"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Do returns the original final failure; cancellation is never retried.
func Do(ctx context.Context, p domain.RetryPolicy, clock port.Clock, changes bool, op func() error) error {
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := op()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt >= max(1, p.MaxAttempts) || !p.ShouldRetry(err, changes) {
			return err
		}
		var after *float64
		if rate, ok := errors.AsType[*domain.RateLimitedError](err); ok && rate.HasRetryAfter {
			after = &rate.RetryAfter
		}
		if err := clock.Sleep(ctx, p.Delay(attempt+1, after, jitter())); err != nil {
			return err
		}
	}
}

// jitter maps cryptographic entropy to the policy's symmetric unit interval.
func jitter() float64 {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read cannot fail.
	return float64(binary.LittleEndian.Uint64(b[:]))/float64(math.MaxUint64)*2 - 1
}
