// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package httpapi implements bounded HTTP exchanges, redirects and wire logging.
package httpapi

import (
	"bytes"
	"context"
	"crypto/x509"
	"log/slog"
	"net/http"
	"time"

	"github.com/altessa-s/go-tlsutils"

	"github.com/altessa-s/go-atlas-net/http/client"

	"github.com/altessa-s/sncloud-go/internal/port"
)

// Metadata response-header budget.
const defaultHeaderTimeout = 60 * time.Second

// Maximum silence between response body reads.
const defaultIdleReadTimeout = 45 * time.Second

// HTTP/2 silence and ping-response budgets.
const defaultPingTimeout = 15 * time.Second

// Server processing budget after an upload.
const defaultUploadHeaderTimeout = 5 * time.Minute

// BodyLimit bounds protocol answers in memory, including chunked responses.
const BodyLimit int64 = 32 << 20

// Config supplies transport overrides. Zero bounds use production defaults.
// The built-in transport guards against private and loopback addresses unless
// PrivateCloud is set; a supplied RoundTripper takes over that dial policy.
type Config struct {
	// RootCAs replaces the system roots when a host supplies its own trust store.
	RootCAs *x509.CertPool
	// RoundTripper replaces the built-in transport, whose guard blocks private
	// and loopback addresses unless PrivateCloud is set. A supplied transport
	// takes over that dial policy and connection bounds.
	RoundTripper http.RoundTripper
	// PrivateCloud disables the dial guard for an explicitly selected private cloud.
	PrivateCloud bool
	// Logger receives sanitized wire diagnostics.
	Logger port.Logger
	// WireLevel chooses how much sanitized request data to record.
	WireLevel WireLevel
	// HeaderTimeout bounds the wait after a metadata or download request is sent.
	HeaderTimeout time.Duration
	// UploadHeaderTimeout leaves the server time to process a completed upload.
	UploadHeaderTimeout time.Duration
	// IdleReadTimeout bounds silence while reading an in-memory response.
	IdleReadTimeout time.Duration
	// PingAfter is the HTTP/2 silence before a health ping.
	PingAfter time.Duration
	// PingTimeout is the wait for the peer to answer that ping.
	PingTimeout time.Duration
}

// Transport owns an isolated client with no cache or global cookie jar.
type Transport struct {
	client                  *http.Client
	memBase, upBase, dlBase *http.Transport
	upload                  *http.Client
	download                *http.Client
	config                  Config
	logger                  port.Logger
}

// New builds three atlas clients with pooled TLS transports and a circuit breaker
// per host. Transport errors redact URLs. The dial guard rejects private addresses
// unless this is an explicitly selected private cloud. A host-supplied round tripper
// owns its dials, which the guard cannot inspect.
// Retries belong to the use cases: an ambiguous write must not be repeated here.
func New(c Config) *Transport {
	if c.HeaderTimeout <= 0 {
		c.HeaderTimeout = defaultHeaderTimeout
	}
	if c.IdleReadTimeout <= 0 {
		c.IdleReadTimeout = defaultIdleReadTimeout
	}
	if c.PingAfter <= 0 {
		c.PingAfter = defaultPingTimeout
	}
	if c.PingTimeout <= 0 {
		c.PingTimeout = defaultPingTimeout
	}
	if c.Logger == nil {
		c.Logger = slog.Default()
	}
	if c.UploadHeaderTimeout <= 0 {
		c.UploadHeaderTimeout = defaultUploadHeaderTimeout
	}
	t := &Transport{config: c, logger: c.Logger}
	t.memBase = t.bounded(c.HeaderTimeout)
	t.upBase = t.bounded(c.UploadHeaderTimeout)
	t.dlBase = t.bounded(c.HeaderTimeout)
	t.dlBase.MaxConnsPerHost = 6
	t.client = t.newClient(t.memBase)
	t.upload = t.newClient(t.upBase)
	t.download = t.newClient(t.dlBase)
	return t
}

// bounded limits a stalled exchange, not the total time a transfer may take.
// The header timer starts after the request body is sent; HTTP/2 health pings
// fail all calls on a dead connection while leaving a responsive slow peer alone.
func (t *Transport) bounded(headerTimeout time.Duration) *http.Transport {
	tr := client.DefaultPooledTransport()
	tr.TLSClientConfig = tlsutils.DefaultClientTLSConfig("")
	tr.TLSClientConfig.RootCAs = t.config.RootCAs
	tr.ResponseHeaderTimeout = headerTimeout
	tr.HTTP2 = &http.HTTP2Config{SendPingTimeout: t.config.PingAfter, PingTimeout: t.config.PingTimeout}
	return tr
}

func (t *Transport) newClient(base *http.Transport) *http.Client {
	opts := []client.Option{client.WithoutRetry()}
	if logger, ok := t.logger.(*slog.Logger); ok {
		opts = append(opts, client.WithLogger(logger))
	}
	switch {
	case t.config.RoundTripper != nil:
		opts = append(opts, client.WithClient(&http.Client{Transport: guard{t.config.RoundTripper}}),
			client.WithoutSSRFProtection())
	case t.config.PrivateCloud:
		opts = append(opts, client.WithTransport(base), client.WithoutSSRFProtection())
	default:
		opts = append(opts, client.WithTransport(base))
	}
	c := client.New(opts...)
	c.CheckRedirect = t.redirect
	return c
}

// RoundTrip reads a bounded JSON response, with a resettable idle timeout.
func (t *Transport) RoundTrip(ctx context.Context, r port.Request) (port.Response, error) {
	child, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := t.request(child, r, bytes.NewReader(r.Body))
	if err != nil {
		return port.Response{}, err
	}
	resp, err := t.do(ctx, t.metadataClient(r.LongRunning), req, r.Body)
	if err != nil {
		return port.Response{}, err
	}
	// Reading reports body errors; closing only releases the connection.
	defer func() { _ = resp.Body.Close() }()
	return t.read(ctx, req, resp, cancel)
}

var _ port.Transport = (*Transport)(nil)
