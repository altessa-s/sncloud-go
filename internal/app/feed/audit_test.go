// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package feed

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

type reconnectClock struct {
	porttest.Clock
	elapsed atomic.Int64
}

func (c *reconnectClock) Now() time.Time { return c.Clock.Now().Add(time.Duration(c.elapsed.Load())) }
func (c *reconnectClock) Sleep(ctx context.Context, d time.Duration) error {
	if d == 5*time.Second {
		<-ctx.Done()
		return ctx.Err()
	}
	return c.Clock.Sleep(ctx, d)
}
func TestWatchBackoffAfterBriefConnections(t *testing.T) {
	clk := &reconnectClock{Time: time.Unix(1780000000, 0)}
	dialer := &porttest.SocketDialer{}
	// Heartbeat waits block separately, leaving only reconnect waits recorded.
	for range 4 {
		dialer.Steps = append(dialer.Steps, porttest.DialStep{Socket: porttest.NewSocket(openFrame, "40", "41")})
	}
	dialer.Steps = append(dialer.Steps, porttest.DialStep{Err: domain.ErrSessionExpired})
	f, _ := newTestFeed(domain.Official(), dialer, clk)
	f.policy.Jitter = 0
	connections := 0
	for ch, err := range f.Watch(t.Context()) {
		if err != nil && !errors.Is(err, domain.ErrSessionExpired) {
			t.Fatal(err)
		}
		if ch.Type == domain.ChangeConnected {
			connections++
			if connections == 4 {
				clk.elapsed.Add(int64(time.Minute))
			}
		}
	}
	want := []time.Duration{300 * time.Millisecond, 900 * time.Millisecond, 2700 * time.Millisecond, 300 * time.Millisecond}
	if got := clk.Delays(); !slices.Equal(got, want) {
		t.Fatalf("delays=%v, want %v", got, want)
	}
}
