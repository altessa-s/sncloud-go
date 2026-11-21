// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package webfiles implements the identifier-based web file API.
package webfiles

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Client shares its session, channel and byte transport with the other APIs.
type Client struct {
	channel port.Channel
	session port.FileSession
	bytes   *transfers.Transfers
	host    string
	clock   port.Clock
}

// New assembles the web protocol without network I/O.
func New(ch port.Channel, s port.FileSession, b *transfers.Transfers, host string, clock port.Clock) *Client {
	return &Client{channel: ch, session: s, bytes: b, host: host, clock: clock}
}

// call keeps authorization and conversion processing budgets on the shared channel.
func (c *Client) call(ctx context.Context, path string, body any, changes bool, pin *domain.CredentialPin, out any) error {
	call := port.Call{Path: path, Web: true, Changes: changes, Body: body,
		LongRunning: path == "file/note/to/pdf" || path == "file/note/to/png"}
	return transfers.Authorized(ctx, c.channel, c.session, call, pin, out)
}

// idRequest preserves the string encoding of snowflake identifiers.
type idRequest struct {
	ID domain.FileID `json:"id"`
}
