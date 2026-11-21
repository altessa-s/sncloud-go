// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/altessa-s/go-atlas-net/http/client"

	"github.com/altessa-s/sncloud-go/internal/adapter/neterr"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

type guard struct{ rt http.RoundTripper }

// CloseIdleConnections forwards cleanup when the supplied transport supports it.
func (g guard) CloseIdleConnections() {
	if closer, ok := g.rt.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}

// CloseIdleConnections releases pooled connections without interrupting calls.
func (t *Transport) CloseIdleConnections() {
	t.client.CloseIdleConnections()
	t.upload.CloseIdleConnections()
	t.download.CloseIdleConnections()
}
func (t *Transport) request(ctx context.Context, r port.Request, body io.Reader) (*http.Request, error) {
	method := r.Method
	if method == "" {
		method = "GET"
	}
	req, err := http.NewRequestWithContext(ctx, method, r.URL, body)
	if err != nil {
		return nil, &domain.MalformedResponseError{What: "invalid request URL or method"}
	}
	req.Header = make(http.Header, len(r.Header))
	for k, values := range r.Header {
		for _, value := range values {
			req.Header.Add(k, value)
		}
	}
	return req, nil
}
func (t *Transport) do(ctx context.Context, c *http.Client, r *http.Request, body []byte) (*http.Response, error) {
	t.log(ctx, "send", r, 0, r.Header, body)
	resp, err := c.Do(r)
	if err != nil {
		// net/http adds a fresh URL wrapper outside atlas; redact that layer too.
		err = neterr.Classify(ctx, client.RedactError(err))
		if t.config.WireLevel != WireOff {
			attrs := []any{"kind", domain.KindOf(err)}
			if transport, ok := errors.AsType[*domain.TransportError](err); ok {
				attrs = append(attrs, "reason", transport.Reason)
			}
			t.logger.Log(ctx, slog.LevelDebug, "request failed", attrs...)
		}
		return nil, err
	}
	return resp, nil
}
func response(r *http.Response) port.Response {
	out := port.Response{Status: r.StatusCode, Header: port.Header(r.Header.Clone())}
	for _, c := range r.Cookies() {
		out.Cookies = append(out.Cookies, port.Cookie{Name: c.Name, Value: c.Value, Expires: c.Expires, Delete: c.MaxAge < 0})
	}
	return out
}

// RoundTrip gives a nil response a stable protocol error before net/http sees it.
func (g guard) RoundTrip(r *http.Request) (*http.Response, error) {
	resp, err := g.rt.RoundTrip(r)
	if resp == nil && err == nil {
		return nil, &domain.MalformedResponseError{What: "response is not HTTP"}
	}
	return resp, err
}
