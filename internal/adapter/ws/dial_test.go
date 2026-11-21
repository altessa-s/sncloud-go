// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package ws

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"testing"

	"github.com/coder/websocket"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "pipe" }

type pipeListener struct {
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{
		conns:  make(chan net.Conn),
		closed: make(chan struct{}),
	}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

func (l *pipeListener) dialContext(ctx context.Context, _, _ string) (net.Conn, error) {
	c1, c2 := net.Pipe()
	select {
	case l.conns <- c2:
		return c1, nil
	case <-l.closed:
		_ = c1.Close()
		_ = c2.Close()
		return nil, net.ErrClosed
	case <-ctx.Done():
		_ = c1.Close()
		_ = c2.Close()
		return nil, ctx.Err()
	}
}

func newPipeServer(t *testing.T, handler http.Handler) *http.Transport {
	t.Helper()
	ln := newPipeListener()
	srv := &http.Server{Handler: handler}
	go func() { _ = srv.Serve(ln) }()
	tr := &http.Transport{DialContext: ln.dialContext}
	t.Cleanup(func() {
		tr.CloseIdleConnections()
		_ = srv.Close()
		_ = ln.Close()
	})
	return tr
}

func TestDialReadWriteAndCleanClose(t *testing.T) {
	var seenUA string
	tr := newPipeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		_, msg, err := conn.Read(r.Context())
		if err != nil {
			_ = conn.CloseNow()
			return
		}
		_ = conn.Write(r.Context(), websocket.MessageText, append([]byte("echo:"), msg...))
		_ = conn.Close(websocket.StatusNormalClosure, "bye")
	}))

	dialer := New(Config{RoundTripper: tr})
	defer dialer.CloseIdleConnections()

	sock, err := dialer.Dial(context.Background(), "http://feed.example.test/socket.io/?EIO=3&transport=websocket", port.Header{
		"User-Agent": {domain.AppUserAgent},
	})
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = sock.Close() }()

	if seenUA != domain.AppUserAgent {
		t.Fatalf("User-Agent = %q, want %q", seenUA, domain.AppUserAgent)
	}
	if err := sock.Write(context.Background(), []byte("ping")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := sock.Read(context.Background())
	if err != nil || string(got) != "echo:ping" {
		t.Fatalf("Read = (%q, %v)", got, err)
	}
	if _, err := sock.Read(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("Read after normal closure = %v, want io.EOF", err)
	}
	if err := sock.Close(); err != nil {
		t.Fatalf("Close 1: %v", err)
	}
	if err := sock.Close(); err != nil {
		t.Fatalf("Close 2 (idempotent): %v", err)
	}
}

func TestDialOversizedAndCloseWakesBlockedRead(t *testing.T) {
	serverCtx, cancelServer := context.WithCancel(t.Context())
	var emptyUA string
	var handlers sync.WaitGroup
	t.Cleanup(handlers.Wait)
	tr := newPipeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.Add(1)
		defer handlers.Done()
		emptyUA = r.Header.Get("User-Agent")
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.CloseNow() }()
		if r.URL.Query().Get("mode") == "huge" {
			_ = conn.Write(serverCtx, websocket.MessageText, bytes.Repeat([]byte("a"), int(readLimit)+16))
			_ = conn.CloseNow()
			return
		}
		for {
			if _, _, err := conn.Read(serverCtx); err != nil {
				break
			}
		}
		_ = conn.CloseNow()
	}))

	t.Cleanup(cancelServer)
	dialer := New(Config{RoundTripper: tr})
	hugeSock, err := dialer.Dial(context.Background(), "http://feed.example.test/socket.io/?mode=huge", nil)
	if err != nil {
		t.Fatalf("Dial huge: %v", err)
	}
	if emptyUA != "" {
		t.Fatalf("expected suppressed default User-Agent, got %q", emptyUA)
	}
	if _, err := hugeSock.Read(context.Background()); !errors.Is(err, domain.ErrOversizedResponse) {
		t.Fatalf("expected ErrOversizedResponse, got %v", err)
	}
	_ = hugeSock.Close()

	quietSock, err := dialer.Dial(context.Background(), "ws://feed.example.test/socket.io/?mode=quiet", nil)
	if err != nil {
		t.Fatalf("Dial quiet: %v", err)
	}
	readErrCh := make(chan error, 1)
	go func() {
		_, rerr := quietSock.Read(context.Background())
		readErrCh <- rerr
	}()
	_ = quietSock.Close()
	if rerr := <-readErrCh; !errors.Is(rerr, domain.ErrTransport) {
		t.Fatalf("Read after local Close = %v, want ErrTransport", rerr)
	}
	if werr := quietSock.Write(context.Background(), []byte("late")); !errors.Is(werr, domain.ErrTransport) {
		t.Fatalf("Write after local Close = %v, want ErrTransport", werr)
	}
}

func TestDialStatusesRedirectsAndValidation(t *testing.T) {
	pooled := New(Config{})
	pooled.CloseIdleConnections()
	private := New(Config{PrivateCloud: true})
	private.CloseIdleConnections()

	for _, tc := range []struct{ in, want string }{
		{"https://feed.example.test/socket.io/", "wss"},
		{"wss://feed.example.test/socket.io/", "wss"},
		{"http://feed.example.test/socket.io/", "ws"},
	} {
		u, err := normalizeSocketURL(tc.in)
		if err != nil || u.Scheme != tc.want {
			t.Fatalf("normalizeSocketURL(%q) = (%v, %v)", tc.in, u, err)
		}
	}
	for _, badURL := range []string{"://bad", "ws:///missing-host", "ftp://feed.example.test/socket.io/"} {
		if _, err := pooled.Dial(context.Background(), badURL, nil); !errors.Is(err, domain.ErrMalformedResponse) {
			t.Fatalf("Dial(%q) = %v, want ErrMalformedResponse", badURL, err)
		}
	}

	tr := newPipeServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/401":
			w.WriteHeader(http.StatusUnauthorized)
		case "/429":
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
		case "/redirect":
			http.Redirect(w, r, "https://other.example.test/socket.io/", http.StatusFound)
		case "/200":
			w.WriteHeader(http.StatusOK)
		}
	}))
	dialer := New(Config{RoundTripper: tr})

	if _, err := dialer.Dial(context.Background(), "ws://feed.example.test/401", nil); !errors.Is(err, domain.ErrSessionExpired) {
		t.Fatalf("401 = %v", err)
	}
	if _, err := dialer.Dial(context.Background(), "ws://feed.example.test/429", nil); !errors.Is(err, domain.ErrRateLimited) {
		t.Fatalf("429 = %v", err)
	}
	// Redirect is not followed and surfaces HTTP 302 on the original path.
	var httpErr *domain.HTTPError
	if _, err := dialer.Dial(context.Background(), "ws://feed.example.test/redirect", nil); !errors.As(err, &httpErr) || httpErr.Status != http.StatusFound {
		t.Fatalf("redirect = %v", err)
	}
	if _, err := dialer.Dial(context.Background(), "ws://feed.example.test/200", nil); !errors.As(err, &httpErr) || httpErr.Status != http.StatusOK {
		t.Fatalf("200 non-upgrade = %v", err)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dialer.Dial(canceled, "ws://feed.example.test/200", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Dial = %v", err)
	}
}
