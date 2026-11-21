// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package channel owns the JSON envelope and browser conversation.
package channel

import (
	"sync"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Channel is safe for concurrent calls; no lock is held across network I/O.
type Channel struct {
	endpoints domain.Endpoints
	transport port.RoundTripper
	clock     port.Clock
	policy    domain.RetryPolicy
	mu        sync.Mutex
	epoch     uint64
	cookies   map[string]string
	xsrf      string
	fetching  *flight
}

// New starts an empty browser conversation.
func New(e domain.Endpoints, t port.RoundTripper, c port.Clock, p domain.RetryPolicy) *Channel {
	return &Channel{endpoints: e, transport: t, clock: c, policy: p, cookies: make(map[string]string)}
}

// Epoch returns the current conversation number.
func (c *Channel) Epoch() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoch
}

// Forget clears the browser and returns its new epoch in the same step.
func (c *Channel) Forget() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epoch++
	c.cookies = make(map[string]string)
	c.xsrf = ""
	c.fetching = nil
	return c.epoch
}
