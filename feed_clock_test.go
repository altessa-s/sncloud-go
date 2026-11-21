//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"net/http"

	"github.com/altessa-s/sncloud-go/internal/adapter/ws"
	"github.com/altessa-s/sncloud-go/internal/app/feed"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// SetFeedClockForTest installs a feed with a controlled heartbeat clock.
func SetFeedClockForTest(c *Client, endpoints Endpoints, clock port.Clock) {
	c.feed = feed.New(endpoints, c.session.(feed.Session), ws.New(ws.Config{RoundTripper: http.DefaultTransport}), clock, DefaultRetryPolicy())
}
