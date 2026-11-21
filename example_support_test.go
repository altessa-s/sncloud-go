// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"sync"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/clouddouble"
	"github.com/altessa-s/sncloud-go/internal/testutil"

	sncloud "github.com/altessa-s/sncloud-go"
)

// The emulator sends Socket.IO connect before registering its subscriber.
// Observe its first ratta_ping reply before making the example's peer mutation.
// This is fixture synchronization, not a requirement on application callers.
type exampleFeedTransport struct {
	base    http.RoundTripper
	onReady func() error
	once    sync.Once
}

func (t *exampleFeedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(r)
	if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		return resp, err
	}
	body, ok := resp.Body.(io.ReadWriteCloser)
	if !ok {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("example upgrade has no bidirectional body")
	}
	resp.Body = &exampleFeedBody{ReadWriteCloser: body, transport: t}
	return resp, nil
}

type exampleFeedBody struct {
	io.ReadWriteCloser
	transport *exampleFeedTransport
	prefix    []byte
	ready     bool
}

func (b *exampleFeedBody) Read(p []byte) (int, error) {
	n, err := b.ReadWriteCloser.Read(p)
	if !b.ready {
		// Server text frames are unmasked; retain split reads of the fixed reply.
		b.prefix = append(b.prefix, p[:n]...)
		if bytes.Contains(b.prefix, []byte(`42["ratta_ping","Received"]`)) {
			b.ready = true
			b.prefix = nil
			b.transport.once.Do(func() { err = b.transport.onReady() })
		}
	}
	return n, err
}

// TestMain keeps emulator wiring out of the consumer examples.
func TestMain(m *testing.M) {
	var cleanups []func()
	setup := func(verification, feed bool) []sncloud.Option {
		cloud := clouddouble.New(clouddouble.Official)
		cloud.EnableFiles()
		cloud.RequireVerification(verification)
		cleanups = append(cleanups, cloud.Close)
		opts := []sncloud.Option{sncloud.WithEndpoints(cloud.Endpoints()), sncloud.WithRoundTripper(http.DefaultTransport)}
		if feed {
			peer, err := sncloud.New(&memoryStore{}, opts...)
			if err != nil {
				panic(err)
			}
			cleanups = append(cleanups, func() { _ = peer.Close() })
			transport := &exampleFeedTransport{base: http.DefaultTransport}
			transport.onReady = func() error {
				ctx := context.Background()
				if _, err := peer.LogIn(ctx, "reader@example.invalid", "synthetic-password", nil); err != nil {
					return err
				}
				_, err := peer.Tasks().CreateList(ctx, "Shared", "")
				return err
			}
			opts[1] = sncloud.WithRoundTripper(transport)
		}
		return opts
	}
	exampleOptions = func() []sncloud.Option { return setup(false, false) }
	verificationOptions = func() []sncloud.Option { return setup(true, false) }
	feedOptions = func() []sncloud.Option { return setup(false, true) }
	testutil.Main(m, func() {
		for _, cleanup := range slices.Backward(cleanups) {
			cleanup()
		}
	})
}
