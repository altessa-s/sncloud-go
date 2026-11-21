// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package retry

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func TestVirtualBackoffAndCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := domain.DefaultRetryPolicy()
		p.Jitter = 0
		start := time.Now()
		calls := 0
		last := &domain.HTTPError{Status: 503}
		err := Do(context.Background(), p, port.SystemClock{}, false, func() error {
			calls++
			return last
		})
		if err != last || calls != 3 || time.Since(start) != 1200*time.Millisecond {
			t.Fatal(calls, err, time.Since(start))
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- Do(ctx, p, port.SystemClock{}, false, func() error { return last }) }()
		synctest.Wait()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}
func TestRateLimitWaitOnChanges(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := domain.DefaultRetryPolicy()
		start := time.Now()
		calls := 0
		err := Do(context.Background(), p, port.SystemClock{}, true, func() error {
			calls++
			if calls == 1 {
				return &domain.RateLimitedError{RetryAfter: 3, HasRetryAfter: true}
			}
			return nil
		})
		if err != nil || calls != 2 || time.Since(start) != 3*time.Second {
			t.Fatal(err, calls, time.Since(start))
		}
	})
}

func TestCancellationBeforeAndDuringAttempt(t *testing.T) {
	p := domain.DefaultRetryPolicy()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Do(ctx, p, port.SystemClock{}, false, func() error {
		t.Fatal("called after cancellation")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	if err := Do(ctx, p, port.SystemClock{}, false, func() error {
		cancel()
		return &domain.TransportError{}
	}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
