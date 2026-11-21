// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package port

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestHeadersAreCaseInsensitiveAndDetached(t *testing.T) {
	h := Header{"x-token": {"old"}}
	h.Set("X-Token", "new")
	if len(h) != 1 || h.Get("X-TOKEN") != "new" {
		t.Fatal(h)
	}
	clone := h.Clone()
	clone["X-Token"][0] = "other"
	if h.Get("x-token") != "new" || h.Get("absent") != "" {
		t.Fatal(h)
	}
}
func TestClockCancellationAndVirtualTime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := SystemClock{}
		start := clock.Now()
		if err := clock.Sleep(context.Background(), time.Second); err != nil || clock.Now().Sub(start) != time.Second {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if !errors.Is(clock.Sleep(ctx, time.Second), context.Canceled) {
			t.Fatal("cancellation ignored")
		}
		ctx, cancel = context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- clock.Sleep(ctx, time.Hour) }()
		synctest.Wait()
		cancel()
		if !errors.Is(<-done, context.Canceled) {
			t.Fatal("wait not canceled")
		}
	})
}
