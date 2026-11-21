// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package devicefiles implements the tablet's path-based file API.
package devicefiles

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Client uses the session to coordinate sync-lock scopes across device views.
type Client struct {
	channel port.Channel
	session port.FileSession
	bytes   *transfers.Transfers
	host    string
}

// New constructs the single device client for a session.
func New(ch port.Channel, s port.FileSession, b *transfers.Transfers, host string) *Client {
	return &Client{channel: ch, session: s, bytes: b, host: host}
}

// call routes tablet metadata through the shared credential-pinned authorization path.
func (c *Client) call(ctx context.Context, path string, body any, changes bool, pin *domain.CredentialPin, out any) error {
	return transfers.Authorized(ctx, c.channel, c.session, port.Call{Path: path, Changes: changes, Body: body}, pin, out)
}

// equipment uses the persistent installation identity required by tablet requests.
func (c *Client) equipment() (string, error) {
	id, err := c.session.Identity()
	return domain.EquipmentNo(id), err
}

// device supplies the installation identity embedded in device request bodies.
type device struct {
	Equipment string `json:"equipmentNo"`
}

// byID identifies an entry with the usual string-encoded snowflake.
type byID struct {
	device
	ID domain.FileID `json:"id"`
}

// byPath addresses a normalized tablet path rather than a web folder identifier.
type byPath struct {
	device
	Path string `json:"path"`
}

// found preserves null metadata as a successful absent lookup.
type found struct {
	Entry *domain.DeviceEntry `json:"entriesVO"`
}
