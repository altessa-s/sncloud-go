// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package ws opens framed WebSocket connections for the Socket.IO change feed.
package ws

import (
	"context"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coder/websocket"

	"github.com/altessa-s/go-tlsutils"

	"github.com/altessa-s/go-atlas-net/http/client"

	"github.com/altessa-s/sncloud-go/internal/adapter/neterr"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

const (
	// readLimit bounds a single Socket.IO frame to 4 MiB; digest announcements
	// carry full passage text rather than short identifiers alone.
	readLimit int64 = 4 << 20

	// defaultUpgradeTimeout bounds how long the HTTP upgrade header wait may stall.
	defaultUpgradeTimeout = 60 * time.Second

	// defaultDialTimeout bounds TCP connection establishment and keep-alive probes.
	defaultDialTimeout = 30 * time.Second
)

// Config configures a WebSocket Dialer.
// The built-in transport guards against private and loopback addresses unless
// PrivateCloud is set; a supplied RoundTripper takes over that dial policy.
type Config struct {
	RootCAs *x509.CertPool
	// RoundTripper replaces the built-in transport, whose guard blocks private
	// and loopback addresses unless PrivateCloud is set. A supplied transport
	// takes over that dial policy and connection bounds.
	RoundTripper  http.RoundTripper
	PrivateCloud  bool
	HeaderTimeout time.Duration
	Clock         port.Clock
}

// Dialer opens WebSocket connections without following HTTP redirects or sharing
// any cookie jar, keeping upgrade query credentials on the target origin.
type Dialer struct {
	transport http.RoundTripper
	base      *http.Transport
	clock     port.Clock
}

// New constructs a Dialer with a pooled TLS transport unless RoundTripper is set.
// Private and loopback addresses are rejected unless PrivateCloud is enabled.
func New(c Config) *Dialer {
	if c.Clock == nil {
		c.Clock = port.SystemClock{}
	}
	if c.RoundTripper != nil {
		return &Dialer{transport: c.RoundTripper, clock: c.Clock}
	}
	timeout := c.HeaderTimeout
	if timeout <= 0 {
		timeout = defaultUpgradeTimeout
	}
	base := client.DefaultPooledTransport()
	base.TLSClientConfig = tlsutils.DefaultClientTLSConfig("")
	base.TLSClientConfig.RootCAs = c.RootCAs
	base.ResponseHeaderTimeout = timeout
	if !c.PrivateCloud {
		d := &net.Dialer{
			Timeout:   defaultDialTimeout,
			KeepAlive: defaultDialTimeout,
			Control:   client.SSRFDialControl(),
		}
		base.DialContext = d.DialContext
	}
	return &Dialer{transport: base, base: base, clock: c.Clock}
}

// CloseIdleConnections releases pooled idle upgrade connections.
func (d *Dialer) CloseIdleConnections() {
	if d.base != nil {
		d.base.CloseIdleConnections()
	}
}

// Dial performs the HTTP-to-WebSocket upgrade and configures the frame read ceiling.
func (d *Dialer) Dial(ctx context.Context, rawURL string, header port.Header) (port.Socket, error) {
	target, err := normalizeSocketURL(rawURL)
	if err != nil {
		return nil, err
	}
	hdr := copyHeaders(header)
	hc := &http.Client{
		Transport: d.transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	// coder/websocket owns the response body across both successful and failed upgrades.
	conn, resp, err := websocket.Dial(ctx, target.String(), &websocket.DialOptions{ //nolint:bodyclose // owned by websocket.Dial
		HTTPClient: hc,
		HTTPHeader: hdr,
	})
	if err != nil {
		if resp != nil && resp.StatusCode != http.StatusSwitchingProtocols {
			if mapped := domain.FromHTTP(resp.StatusCode, target.Path, resp.Header.Get("Retry-After"), d.clock.Now()); mapped != nil {
				return nil, mapped
			}
			return nil, &domain.HTTPError{Status: resp.StatusCode, Path: target.Path}
		}
		return nil, neterr.Classify(ctx, err)
	}
	conn.SetReadLimit(readLimit)
	return &socket{conn: conn}, nil
}

func normalizeSocketURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, &domain.MalformedResponseError{What: "invalid socket URL"}
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "ws":
		u.Scheme = "ws"
	case "https", "wss":
		u.Scheme = "wss"
	default:
		return nil, &domain.MalformedResponseError{What: fmt.Sprintf("unsupported socket scheme %q", u.Scheme)}
	}
	return u, nil
}

func copyHeaders(header port.Header) http.Header {
	out := make(http.Header, len(header)+1)
	hasUA := false
	for k, vals := range header {
		if strings.EqualFold(k, "User-Agent") {
			hasUA = true
		}
		out[k] = append([]string(nil), vals...)
	}
	if !hasUA {
		out["User-Agent"] = []string{""}
	}
	return out
}

var (
	_ port.SocketDialer = (*Dialer)(nil)
	_ port.IdleCloser   = (*Dialer)(nil)
)
