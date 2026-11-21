// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package porttest

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Socket is a concurrency-safe port.Socket double. Frames are delivered in FIFO
// order; when HoldOpen is true, Read blocks after the last frame until Push,
// Close or context cancellation wakes it.
type Socket struct {
	mu       sync.Mutex
	frames   [][]byte
	sent     []string
	holdOpen bool
	readErr  error
	writeErr error
	closed   bool
	notify   chan struct{}
	closedCh chan struct{}
}

// NewSocket creates a scripted socket that returns io.EOF once frames are drained.
func NewSocket(frames ...string) *Socket {
	s := &Socket{
		frames:   make([][]byte, 0, len(frames)),
		notify:   make(chan struct{}),
		closedCh: make(chan struct{}),
	}
	for _, f := range frames {
		s.frames = append(s.frames, []byte(f))
	}
	return s
}

// NewQuietSocket creates a socket that stays open after delivering frames until
// Push, Close or context cancellation wakes a waiting Read.
func NewQuietSocket(frames ...string) *Socket {
	s := NewSocket(frames...)
	s.holdOpen = true
	return s
}

// Push appends a frame and wakes any waiting Read.
func (s *Socket) Push(frame string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.frames = append(s.frames, []byte(frame))
	close(s.notify)
	s.notify = make(chan struct{})
}

// SetWriteError configures subsequent Write calls to fail with err.
func (s *Socket) SetWriteError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writeErr = err
}

// SetReadError configures Read to return err once queued frames are drained.
func (s *Socket) SetReadError(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.readErr = err
	close(s.notify)
	s.notify = make(chan struct{})
}

// Read returns the next scripted frame, io.EOF when finished, or blocks while HoldOpen is set.
func (s *Socket) Read(ctx context.Context) ([]byte, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return nil, &domain.TransportError{Reason: domain.ReasonConnectionLost}
		}
		if len(s.frames) > 0 {
			f := append([]byte(nil), s.frames[0]...)
			s.frames = s.frames[1:]
			s.mu.Unlock()
			return f, nil
		}
		if s.readErr != nil {
			err := s.readErr
			s.mu.Unlock()
			return nil, err
		}
		if !s.holdOpen {
			s.mu.Unlock()
			return nil, io.EOF
		}
		wait := s.notify
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-wait:
		}
	}
}

// Write records the outgoing text frame unless closed or configured to fail.
func (s *Socket) Write(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return &domain.TransportError{Reason: domain.ReasonConnectionLost}
	}
	if s.writeErr != nil {
		return s.writeErr
	}
	s.sent = append(s.sent, string(data))
	return nil
}

// Close marks the socket closed and wakes any blocked Read.
func (s *Socket) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.notify)
		s.notify = make(chan struct{})
		close(s.closedCh)
	}
	return nil
}

// Closed reports whether Close was called.
func (s *Socket) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// ClosedChan returns a channel closed when Close runs.
func (s *Socket) ClosedChan() <-chan struct{} { return s.closedCh }

// Sent returns a detached snapshot of frames written to the socket.
func (s *Socket) Sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sent...)
}

// DialCall records one SocketDialer.Dial invocation.
type DialCall struct {
	URL    string
	Header port.Header
}

// DialStep scripts one SocketDialer.Dial outcome.
type DialStep struct {
	Socket port.Socket
	Err    error
	Run    func(context.Context, string, port.Header) (port.Socket, error)
}

// SocketDialer records dial attempts and returns scripted sockets.
type SocketDialer struct {
	mu    sync.Mutex
	Steps []DialStep
	calls []DialCall
}

// Dial consumes the next scripted step under one lock.
func (d *SocketDialer) Dial(ctx context.Context, url string, h port.Header) (port.Socket, error) {
	d.mu.Lock()
	d.calls = append(d.calls, DialCall{URL: url, Header: h.Clone()})
	if len(d.Steps) == 0 {
		d.mu.Unlock()
		return nil, fmt.Errorf("porttest: unexpected socket dial %s", safeURL(url))
	}
	step := d.Steps[0]
	d.Steps = d.Steps[1:]
	d.mu.Unlock()
	if step.Run != nil {
		return step.Run(ctx, url, h)
	}
	return step.Socket, step.Err
}

// Calls returns a detached snapshot of dial calls.
func (d *SocketDialer) Calls() []DialCall {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make([]DialCall, len(d.calls))
	for i, c := range d.calls {
		out[i] = DialCall{URL: c.URL, Header: c.Header.Clone()}
	}
	return out
}

var (
	_ port.Socket       = (*Socket)(nil)
	_ port.SocketDialer = (*SocketDialer)(nil)
)
