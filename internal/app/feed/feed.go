// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package feed implements the Socket.IO / Engine.IO 3 change-feed stream.
//
// Announcements on this socket are a "look again" hint rather than a durable
// log or complete record: the server buffers events only for the life of a
// single socket, drops anything unacknowledged when that socket closes, and
// omits bodies altogether for tasks (sending only QUERY or SORT) and digests.
// Because a reconnect may have missed arbitrary mutations while disconnected,
// every fresh Socket.IO session begins by emitting [domain.ChangeConnected] so
// the caller knows to reconcile state over REST before acting on incremental
// batches.
//
// Two distinct keepalive frames share the connection. Engine.IO frame "2" keeps
// the WebSocket transport alive and measures server liveness against
// pingInterval + pingTimeout, while the application-level `42["ratta_ping","ping"]`
// packet is what actually prompts the cloud to flush queued `ServerMessage`,
// `to-do` and `digest` batches; replying `42["<event>","Received"]` clears the
// delivered batch from the connection queue.
//
// Breaking out of the iterator or canceling its context closes the underlying
// socket immediately without waiting on a WebSocket close handshake, unblocking
// any in-flight read on an idle or half-open connection at once.
package feed

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"iter"
	"math"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// handshakeTimeout bounds a peer that upgrades without sending its Engine.IO open packet.
const handshakeTimeout = 30 * time.Second

// A brief successful handshake does not establish a healthy subscription.
const stableConnection = time.Minute

// Session supplies Socket.IO handshake credentials and invalidates refused tokens.
type Session interface {
	Pin() *domain.CredentialPin
	SocketCredentials(expected *domain.CredentialPin) (token, kind, random string, err error)
	Invalidate(refused string)
}

// Feed opens and reads the Socket.IO change feed.
type Feed struct {
	endpoints domain.Endpoints
	session   Session
	dialer    port.SocketDialer
	clock     port.Clock
	policy    domain.RetryPolicy
}

// New constructs a change-feed client.
func New(
	endpoints domain.Endpoints,
	s Session,
	dialer port.SocketDialer,
	clock port.Clock,
	policy domain.RetryPolicy,
) *Feed {
	return &Feed{
		endpoints: endpoints,
		session:   s,
		dialer:    dialer,
		clock:     clock,
		policy:    policy,
	}
}

// Changes opens a single Socket.IO connection and yields ChangeConnected followed
// by file, task and digest batches until the server closes the connection, an
// error occurs, the caller breaks out of iteration or ctx is canceled.
func (f *Feed) Changes(ctx context.Context) iter.Seq2[domain.Change, error] {
	return func(yield func(domain.Change, error) bool) {
		pin := f.session.Pin()
		stopped, err := f.runSingle(ctx, pin, yield)
		if stopped || err == nil {
			return
		}
		yield(domain.Change{}, err)
	}
}

// Watch maintains a persistent subscription across clean server disconnects
// and transient transport or HTTP failures, sleeping according to the client's
// retry backoff between reconnects and emitting ChangeConnected on every fresh
// connection so the caller re-lists state missed during the gap. Non-retryable
// errors such as ErrNotLoggedIn, ErrSessionExpired, RejectedError or
// MalformedResponseError end the stream immediately.
func (f *Feed) Watch(ctx context.Context) iter.Seq2[domain.Change, error] {
	return func(yield func(domain.Change, error) bool) {
		pin := f.session.Pin()
		var failures int
		for {
			if err := ctx.Err(); err != nil {
				yield(domain.Change{}, err)
				return
			}
			var connected time.Time
			stopped, err := f.runSingle(ctx, pin, func(ch domain.Change, chErr error) bool {
				if ch.Type == domain.ChangeConnected {
					connected = f.clock.Now()
				}
				return yield(ch, chErr)
			})
			if stopped {
				return
			}
			if ctx.Err() != nil {
				yield(domain.Change{}, ctx.Err())
				return
			}
			if !connected.IsZero() && f.clock.Now().Sub(connected) >= stableConnection {
				failures = 0
			}
			if err != nil && !f.policy.ShouldRetry(err, false) {
				yield(domain.Change{}, err)
				return
			}
			failures++
			var after *float64
			if rate, ok := errors.AsType[*domain.RateLimitedError](err); ok && rate.HasRetryAfter {
				after = &rate.RetryAfter
			}
			if sleepErr := f.clock.Sleep(ctx, f.policy.Delay(failures+1, after, feedJitter())); sleepErr != nil {
				yield(domain.Change{}, sleepErr)
				return
			}
		}
	}
}

func (f *Feed) runSingle(ctx context.Context, pin *domain.CredentialPin, yield func(domain.Change, error) bool) (stopped bool, err error) {
	token, kind, random, err := f.session.SocketCredentials(pin)
	if err != nil {
		return false, err
	}
	addr, err := handshakeURL(f.endpoints.SocketHost, token, kind, random)
	if err != nil {
		return false, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	sock, err := f.dialer.Dial(runCtx, addr, port.Header{
		"User-Agent": {domain.AppUserAgent},
	})
	if err != nil {
		if errors.Is(err, domain.ErrSessionExpired) {
			f.session.Invalidate(token)
		}
		return false, err
	}

	// Close the socket immediately when runCtx ends—either because the caller's
	// context was canceled or because yield returned false on break—so a goroutine
	// parked inside sock.Read wakes at once instead of waiting for the next server
	// frame or close handshake.
	stopAfter := context.AfterFunc(runCtx, func() {
		_ = sock.Close()
	})
	hb := newHeartbeat(f.clock)
	defer func() {
		hb.stop()
		stopAfter()
		_ = sock.Close()
	}()

	stopped, readErr := f.readLoop(runCtx, sock, hb, yield)
	// Tear down the socket before returning on break or completion rather than
	// relying solely on deferred cleanup or a peer close frame.
	hb.stop()
	_ = sock.Close()
	if stopped {
		return true, nil
	}
	if hbErr := hb.failure(); hbErr != nil {
		readErr = hbErr
	}
	if readErr != nil && ctx.Err() != nil {
		readErr = ctx.Err()
	}
	if errors.Is(readErr, domain.ErrSessionExpired) {
		f.session.Invalidate(token)
	}
	return false, readErr
}

func (f *Feed) readLoop(
	ctx context.Context,
	sock port.Socket,
	hb *heartbeat,
	yield func(domain.Change, error) bool,
) (stopped bool, err error) {
	opened := false
	readCtx, cancelOpen := context.WithTimeout(ctx, handshakeTimeout)
	defer cancelOpen()
	for {
		raw, rerr := sock.Read(readCtx)
		if errors.Is(rerr, context.DeadlineExceeded) && ctx.Err() == nil {
			return false, &domain.TransportError{Reason: domain.ReasonTimedOut}
		}
		if errors.Is(rerr, io.EOF) {
			return false, nil
		}
		if rerr != nil {
			return false, rerr
		}
		if cerr := ctx.Err(); cerr != nil {
			return false, cerr
		}
		hb.heard()
		frame, perr := parseFrame(string(raw))
		if perr != nil {
			return false, perr
		}
		if frame.kind == frameOpen {
			cancelOpen()
			readCtx = ctx
		}
		done, stopped, ferr := dispatchFrame(ctx, sock, hb, frame, &opened, yield)
		if done || stopped || ferr != nil {
			return stopped, ferr
		}
	}
}

func dispatchFrame(
	ctx context.Context,
	sock port.Socket,
	hb *heartbeat,
	frame parsedFrame,
	opened *bool,
	yield func(domain.Change, error) bool,
) (done, stopped bool, err error) {
	switch frame.kind {
	case frameOpen:
		hb.start(ctx, sock, frame.interval, frame.timeout)
		return false, false, nil
	case frameConnected:
		if *opened {
			return false, false, nil
		}
		*opened = true
		if !yield(domain.Change{Type: domain.ChangeConnected}, nil) {
			return false, true, nil
		}
		// Request queued server announcements immediately rather than waiting
		// for the first heartbeat interval to elapse.
		return false, false, sendEventPacket(ctx, sock, eventRattaPing, argPing)
	case frameClosed:
		return true, false, nil
	case frameEvent:
		stopped, err := handleEventFrame(ctx, sock, frame, yield)
		return false, stopped, err
	case frameIgnored:
		return false, false, nil
	}
	return false, false, nil
}

func handleEventFrame(
	ctx context.Context,
	sock port.Socket,
	frame parsedFrame,
	yield func(domain.Change, error) bool,
) (bool, error) {
	change, err := parseAnnouncement(frame.eventName, frame.eventArg)
	if err != nil || change == nil {
		return false, err
	}
	if err := sendEventPacket(ctx, sock, frame.eventName, argReceived); err != nil {
		return false, err
	}
	if !yield(*change, nil) {
		return true, nil
	}
	return false, nil
}

func sendEventPacket(ctx context.Context, sock port.Socket, name, arg string) error {
	pkt, err := formatEventPacket(name, arg)
	if err != nil {
		return err
	}
	return writePacket(ctx, sock, pkt, handshakeTimeout)
}

func feedJitter() float64 {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read cannot fail.
	return float64(binary.LittleEndian.Uint64(b[:]))/float64(math.MaxUint64)*2 - 1
}

var _ port.ChangeFeed = (*Feed)(nil)
