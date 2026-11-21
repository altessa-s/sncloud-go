// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"io"
	"net/http"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func newTestTransport(t *testing.T, c Config) *Transport {
	t.Helper()
	c.PrivateCloud = true
	tr := New(c)
	t.Cleanup(tr.CloseIdleConnections)
	return tr
}

type zeroReader struct{}

func (zeroReader) Read(b []byte) (int, error) {
	clear(b)
	return len(b), nil
}

type trackBody struct {
	io.Reader
	closed *bool
}

func (b *trackBody) Close() error {
	*b.closed = true
	return nil
}

type countReader struct{ n *int }

func (r *countReader) Read(p []byte) (int, error) {
	*r.n += len(p)
	clear(p)
	return len(p), nil
}

type contextBody struct{ ctx context.Context }

func (b contextBody) Read([]byte) (int, error) {
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}
func (b contextBody) Close() error { return nil }

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
