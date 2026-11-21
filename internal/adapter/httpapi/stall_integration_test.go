//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

const stallSlack = 5 * time.Second

// stallServer is a loopback TLS server speaking HTTP/1.1 or HTTP/2, and a
// channel that is closed before the server is, so a handler parked on it
// lets the server close.
func stallServer(t *testing.T, major int, h func(w http.ResponseWriter, r *http.Request, release <-chan struct{})) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != major {
			t.Errorf("the request arrived over HTTP/%d, want HTTP/%d", r.ProtoMajor, major)
		}
		h(w, r, release)
	}))
	srv.EnableHTTP2 = major == 2
	srv.StartTLS()
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return srv
}

func stallTransport(t *testing.T, srv *httptest.Server, bound time.Duration) *Transport {
	t.Helper()
	roots := x509.NewCertPool()
	roots.AddCert(srv.Certificate())
	tr := New(Config{RootCAs: roots, PrivateCloud: true, PingAfter: bound, PingTimeout: bound})
	t.Cleanup(tr.CloseIdleConnections)
	return tr
}
func requireRepeated(t *testing.T, err error, reason domain.TransportReason) {
	t.Helper()
	var failure *domain.TransportError
	if !errors.As(err, &failure) || failure.Reason != reason {
		t.Fatalf("got %v, want %s", err, reason)
	}
	if !domain.DefaultRetryPolicy().ShouldRetry(err, false) {
		t.Fatal("read failure not retryable")
	}
}
func requireBetween(t *testing.T, start time.Time, floor time.Duration) {
	t.Helper()
	if elapsed := time.Since(start); elapsed < floor || elapsed > floor+stallSlack {
		t.Fatal("bound elapsed", elapsed)
	}
}
func flush(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	if err := http.NewResponseController(w).Flush(); err != nil {
		t.Error(err)
	}
}

// blackhole is a TCP relay in front of a server that can stop relaying
// without closing anything: what a network that silently drops an
// established connection looks like from the client.
type blackhole struct {
	ln     net.Listener
	target string
	silent atomic.Bool
	done   chan struct{}
	wg     sync.WaitGroup
}

func newBlackhole(t *testing.T, target string) *blackhole {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &blackhole{ln: ln, target: target, done: make(chan struct{})}
	b.wg.Go(b.accept)
	t.Cleanup(func() {
		close(b.done)
		if err := ln.Close(); err != nil {
			t.Fatal(err)
		}
		b.wg.Wait()
	})
	return b
}

func (b *blackhole) accept() {
	for {
		client, err := b.ln.Accept()
		if err != nil {
			return
		}
		server, err := net.Dial("tcp", b.target)
		if err != nil {
			// Closing a relay during teardown is best effort.
			_ = client.Close()
			continue
		}
		b.wg.Go(func() { b.relay(server, client) })
		b.wg.Go(func() { b.relay(client, server) })
		b.wg.Go(func() {
			<-b.done
			// Closing a relay during teardown is best effort.
			_ = client.Close()
			// Closing a relay during teardown is best effort.
			_ = server.Close()
		})
	}
}

// relay copies src to dst until the relay goes silent; from then on what
// arrives is dropped and nothing is closed.
func (b *blackhole) relay(dst, src net.Conn) {
	buf := make([]byte, 32<<10)
	for {
		n, err := src.Read(buf)
		if n > 0 && !b.silent.Load() {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// The wording net/http gives a connection closed by an unanswered ping is
// what package neterr recognizes it by. This test meets the real failure,
// so a Go release that words it differently fails here.
func TestDeadHTTP2ConnectionIsFoundByItsPing(t *testing.T) {
	t.Parallel()
	const bound = 150 * time.Millisecond

	started := make(chan struct{}, 2)
	srv := stallServer(t, 2, func(w http.ResponseWriter, r *http.Request, release <-chan struct{}) {
		w.Header().Set("Content-Length", "1048576")
		// A canceled client may close the connection; the response needs no retry.
		_, _ = w.Write([]byte("first bytes"))
		flush(t, w)
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
		}
	})
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	calls := map[string]func(tr *Transport, via string) (port.Response, error){
		// A file transfer has no bound but the ping.
		"download": func(tr *Transport, via string) (port.Response, error) {
			return tr.Download(t.Context(), port.Request{Method: http.MethodGet, URL: via + "/file", Dest: filepath.Join(dir, "partial")})
		},
		// An in-memory read whose idle bound is further away than the ping.
		"round trip": func(tr *Transport, via string) (port.Response, error) {
			return tr.RoundTrip(t.Context(), port.Request{Method: http.MethodGet, URL: via + "/memory"})
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			hole := newBlackhole(t, target.Host)
			tr := stallTransport(t, srv, bound)

			errs := make(chan error, 1)
			go func() {
				_, err := call(tr, "https://"+hole.ln.Addr().String())
				errs <- err
			}()
			select {
			case <-started:
			case <-time.After(stallSlack):
				t.Fatal("the request never reached the server")
			}
			hole.silent.Store(true)
			start := time.Now()
			select {
			case err := <-errs:
				requireRepeated(t, err, domain.ReasonConnectionLost)
				// One silence before the ping and one waiting for its answer,
				// less what had already passed when the relay went silent.
				requireBetween(t, start, bound)
			case <-time.After(2*bound + stallSlack):
				t.Fatal("a dead connection was never given up on")
			}
		})
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("a lost download left %d files", len(entries))
	}
}

func TestSSRFGuardSelection(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(204)
	}))
	defer srv.Close()
	for _, e := range []domain.Endpoints{
		domain.Official(),
		domain.EndpointsFromEnvironment(map[string]string{"SNCLOUD_HOST": srv.URL}),
		domain.PrivateCloud(srv.URL, false),
	} {
		tr := New(Config{PrivateCloud: e.PrivateCloud})
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		_, err := tr.RoundTrip(ctx, port.Request{URL: srv.URL})
		cancel()
		tr.CloseIdleConnections()
		if (err == nil) != e.PrivateCloud {
			t.Fatalf("private=%v: %v", e.PrivateCloud, err)
		}
	}
	if hits.Load() != 1 {
		t.Fatal("guarded calls reached loopback", hits.Load())
	}
}
