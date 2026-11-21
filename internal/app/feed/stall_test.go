// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package feed

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

type stalledWriteSocket struct{ *porttest.Socket }

func (s stalledWriteSocket) Write(ctx context.Context, data []byte) error {
	if string(data) == "2" {
		<-ctx.Done()
		return ctx.Err()
	}
	return s.Socket.Write(ctx, data)
}

func TestStalledOpenAndHeartbeatWrite(t *testing.T) {
	for _, opening := range []bool{true, false} {
		synctest.Test(t, func(t *testing.T) {
			quiet := porttest.NewQuietSocket()
			var sock port.Socket = quiet
			if !opening {
				quiet.Push(openFrame)
				quiet.Push("40")
				sock = stalledWriteSocket{quiet}
			}
			dialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: sock}, {Err: domain.ErrSessionExpired}}}
			f, _ := newTestFeed(domain.Official(), dialer, port.SystemClock{})
			changes, err := collectChanges(f.Changes(t.Context()))
			var transport *domain.TransportError
			if !errors.As(err, &transport) || transport.Reason != domain.ReasonTimedOut {
				t.Fatalf("timeout = %v", err)
			}
			want := 1
			if opening {
				want = 0
			}
			if len(changes) != want || !quiet.Closed() {
				t.Fatalf("changes=%v closed=%v", changes, quiet.Closed())
			}
		})
	}
}

func TestWatchRetriesStalledOpen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		quiet := porttest.NewQuietSocket()
		dialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: quiet}, {Err: domain.ErrSessionExpired}}}
		f, _ := newTestFeed(domain.Official(), dialer, port.SystemClock{})
		_, err := collectChanges(f.Watch(t.Context()))
		if !errors.Is(err, domain.ErrSessionExpired) || len(dialer.Calls()) != 2 {
			t.Fatalf("Watch = %v, calls=%d", err, len(dialer.Calls()))
		}
	})
}
