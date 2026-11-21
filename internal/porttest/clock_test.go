// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package porttest

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClock(t *testing.T) {
	clock := &Clock{Time: time.Unix(42, 0)}
	if clock.Now().Unix() != 42 {
		t.Fatal(clock.Now())
	}
	if err := clock.Sleep(context.Background(), time.Second); err != nil {
		t.Fatal(err)
	}
	if len(clock.Delays()) != 1 {
		t.Fatal(clock.Delays())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(clock.Sleep(ctx, time.Second), context.Canceled) {
		t.Fatal("canceled wait recorded")
	}
}
