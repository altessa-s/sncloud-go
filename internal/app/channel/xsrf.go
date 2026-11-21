// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package channel

import (
	"context"
	"errors"

	"github.com/altessa-s/sncloud-go/internal/app/retry"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

type flight struct {
	done chan struct{}
	err  error
}

// ensureXSRF coalesces a cold browser's callers so they share one cookie/token
// pair. The initiator owns the request context; surviving waiters start another
// flight if that context ends, so cancellation belongs only to its caller.
// Epoch checks prevent a late handshake from reviving a forgotten browser.
func (c *Channel) ensureXSRF(ctx context.Context, epoch uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if epoch != c.epoch {
		c.mu.Unlock()
		return domain.ErrNotLoggedIn
	}
	if c.xsrf != "" {
		c.mu.Unlock()
		return nil
	}
	if f := c.fetching; f != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-f.done:
		}
		if f.err != nil && !errors.Is(f.err, context.Canceled) && !errors.Is(f.err, context.DeadlineExceeded) {
			return f.err
		}
		return c.ensureXSRF(ctx, epoch)
	}
	f := &flight{done: make(chan struct{})}
	c.fetching = f
	c.mu.Unlock()
	err := retry.Do(ctx, c.policy, c.clock, false, func() error {
		c.mu.Lock()
		// Both retries and returning responses must still belong to this browser.
		if epoch != c.epoch {
			c.mu.Unlock()
			return domain.ErrNotLoggedIn
		}
		h := port.Header{"User-Agent": {domain.WebUserAgent}}
		if cookie := c.cookieHeader(); cookie != "" {
			h.Set("Cookie", cookie)
		}
		c.mu.Unlock()
		resp, err := c.transport.RoundTrip(ctx, port.Request{Method: "GET", URL: c.endpoints.Web("csrf"), Header: h})
		if err != nil {
			return err
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		// Both retries and returning responses must still belong to this browser.
		if epoch != c.epoch {
			return domain.ErrNotLoggedIn
		}
		c.absorb(resp)
		if err := domain.FromHTTP(resp.Status, "/api/csrf", resp.Header.Get("Retry-After"), c.clock.Now()); err != nil {
			return err
		}
		if c.xsrf == "" {
			c.xsrf = resp.Header.Get(domain.XSRFHeader)
		}
		if c.xsrf == "" {
			return &domain.MalformedResponseError{What: "the XSRF handshake named no token"}
		}
		return nil
	})
	c.mu.Lock()
	f.err = err
	// Forget may already have installed a different flight; complete only ours.
	if c.fetching == f {
		c.fetching = nil
	}
	close(f.done)
	c.mu.Unlock()
	return err
}
