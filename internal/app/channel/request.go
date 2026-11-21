// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package channel

import (
	"context"
	"encoding/json/v2"
	"maps"
	"net/url"

	"github.com/altessa-s/sncloud-go/internal/app/retry"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Call sends a flat JSON API request and decodes its payload beside the envelope.
func (c *Channel) Call(ctx context.Context, call port.Call, out any) error {
	var body []byte
	var err error
	if call.Body != nil {
		body, err = json.Marshal(call.Body)
		if err != nil {
			return err
		}
	}
	target := c.endpoints.App(call.Path)
	if call.Web {
		target = c.endpoints.Web(call.Path)
	}
	method := call.Method
	if method == "" {
		method = "POST"
	}
	h := port.Header{
		"Content-Type": {"application/json"},
		"Accept":       {"application/json, text/plain, */*"},
		"User-Agent":   {domain.WebUserAgent},
	}
	for k, v := range call.Header {
		h[k] = append([]string(nil), v...)
	}
	if call.Token != "" {
		h.Set(domain.AccessTokenHeader, call.Token)
	}
	epoch := c.Epoch()
	if call.Epoch != nil {
		epoch = *call.Epoch
	}
	return retry.Do(ctx, c.policy, c.clock, call.Changes, func() error {
		request := port.Request{URL: target, Method: method, Header: h, Body: body, LongRunning: call.LongRunning}
		data, err := c.exchange(ctx, request, call.Web, epoch)
		if err != nil {
			return err
		}
		return Open(data, out)
	})
}
func (c *Channel) exchange(ctx context.Context, r port.Request, web bool, epoch uint64) ([]byte, error) {
	for refreshed := false; ; refreshed = true {
		h, err := c.prepare(ctx, web, epoch)
		if err != nil {
			return nil, err
		}
		req := r
		req.Header = r.Header.Clone()
		maps.Copy(req.Header, h)
		resp, err := c.transport.RoundTrip(ctx, req)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		// Reject stale answers before absorbing cookies into the current browser.
		if c.epoch != epoch {
			c.mu.Unlock()
			return nil, domain.ErrNotLoggedIn
		}
		if web {
			c.absorb(resp)
		}
		if resp.Status == 403 && web && c.endpoints.WebUsesXSRF && !refreshed {
			// Refresh the cookie/token pair once: repeated 403s must reach the
			// caller rather than causing an unbounded handshake loop.
			c.xsrf = ""
			delete(c.cookies, domain.XSRFCookie)
			c.mu.Unlock()
			continue
		}
		c.mu.Unlock()
		u, parseErr := url.Parse(r.URL)
		if parseErr != nil {
			return nil, &domain.MalformedResponseError{What: "invalid request URL"}
		}
		err = domain.FromHTTP(resp.Status, u.Path, resp.Header.Get("Retry-After"), c.clock.Now())
		if err != nil {
			return nil, explained(err, resp.Body)
		}
		return resp.Body, nil
	}
}
func (c *Channel) prepare(ctx context.Context, web bool, epoch uint64) (port.Header, error) {
	if web && c.endpoints.WebUsesXSRF {
		if err := c.ensureXSRF(ctx, epoch); err != nil {
			return nil, err
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// A handshake may have waited while Forget started a new conversation.
	if epoch != c.epoch {
		return nil, domain.ErrNotLoggedIn
	}
	h := make(port.Header)
	if !web {
		return h, nil
	}
	if c.endpoints.WebUsesXSRF {
		h.Set(domain.XSRFHeader, c.xsrf)
	}
	if cookie := c.cookieHeader(); cookie != "" {
		h.Set("Cookie", cookie)
	}
	return h, nil
}
