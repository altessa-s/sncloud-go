// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package feed

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

const enginePingFrame = "2"

type heartbeat struct {
	clock     port.Clock
	mu        sync.Mutex
	lastHeard time.Time
	err       error
	cancel    context.CancelFunc
	done      chan struct{}
}

func newHeartbeat(clock port.Clock) *heartbeat {
	return &heartbeat{
		clock:     clock,
		lastHeard: clock.Now(),
	}
}

func (h *heartbeat) heard() {
	h.mu.Lock()
	h.lastHeard = h.clock.Now()
	h.mu.Unlock()
}

func (h *heartbeat) silence() time.Duration {
	h.mu.Lock()
	last := h.lastHeard
	h.mu.Unlock()
	return h.clock.Now().Sub(last)
}

func (h *heartbeat) fail(err error) {
	h.mu.Lock()
	if h.err == nil {
		h.err = err
	}
	h.mu.Unlock()
}

func (h *heartbeat) failure() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

func (h *heartbeat) start(parent context.Context, sock port.Socket, interval, timeout time.Duration) {
	h.stop()
	hbCtx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	h.mu.Lock()
	h.cancel = cancel
	h.done = done
	h.mu.Unlock()
	go func() {
		defer close(done)
		h.beat(hbCtx, sock, interval, timeout)
	}()
}

func (h *heartbeat) stop() {
	h.mu.Lock()
	cancel, done := h.cancel, h.done
	h.cancel = nil
	h.done = nil
	h.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (h *heartbeat) beat(ctx context.Context, sock port.Socket, interval, timeout time.Duration) {
	// Engine.IO "2" alone only keeps the WebSocket open and elicits "3" pongs;
	// the server withholds queued ServerMessage, to-do and digest batches until
	// it receives a Socket.IO ratta_ping event, so every beat sends both frames.
	rattaPkt, err := formatEventPacket(eventRattaPing, argPing)
	if err != nil {
		return
	}
	for {
		if err := h.clock.Sleep(ctx, interval); err != nil {
			return
		}
		if h.silence() > interval+timeout {
			h.fail(&domain.TransportError{Reason: domain.ReasonTimedOut})
			_ = sock.Close()
			return
		}
		if err := writePacket(ctx, sock, []byte(enginePingFrame), timeout); err != nil {
			h.recordSendFailure(ctx, sock, err)
			return
		}
		if err := writePacket(ctx, sock, rattaPkt, timeout); err != nil {
			h.recordSendFailure(ctx, sock, err)
			return
		}
	}
}

func (h *heartbeat) recordSendFailure(ctx context.Context, sock port.Socket, err error) {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return
	}
	if _, ok := errors.AsType[domain.Error](err); ok {
		h.fail(err)
	} else {
		h.fail(&domain.TransportError{Reason: domain.ReasonConnectionLost})
	}
	_ = sock.Close()
}

// writePacket bounds stalled writes, including acknowledgements and initial pings.
func writePacket(ctx context.Context, sock port.Socket, packet []byte, timeout time.Duration) error {
	writeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	err := sock.Write(writeCtx, packet)
	if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		return &domain.TransportError{Reason: domain.ReasonTimedOut}
	}
	return err
}
