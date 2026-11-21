// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package porttest

import (
	"context"
	"sync"
	"time"
)

// Clock records waits without advancing wall time.
type Clock struct {
	mu     sync.Mutex
	Time   time.Time
	delays []time.Duration
}

// Now returns the configured instant.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Time
}

// Sleep records a wait unless the context was canceled.
func (c *Clock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delays = append(c.delays, d)
	return nil
}

// Delays returns the waits recorded so far.
func (c *Clock) Delays() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.delays...)
}
