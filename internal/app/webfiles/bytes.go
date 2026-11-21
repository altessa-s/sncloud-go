// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package webfiles

import (
	"context"
	"os"

	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// link carries the storage URL and integrity claim independently of account credentials.
type link struct {
	URL string `json:"url"`
	MD5 string `json:"md5"`
}

// Download returns a verified temporary file owned by the caller; nil size uses the default ceiling.
func (c *Client) Download(ctx context.Context, id domain.FileID, size *int64, p port.Progress) (string, error) {
	body := struct {
		ID   domain.FileID `json:"id"`
		Type int           `json:"type"`
	}{ID: id}
	var out link
	if err := c.call(ctx, "file/download/url", body, false, c.session.Pin(), &out); err != nil {
		return "", err
	}
	limit := transfers.DownloadLimit
	if size != nil {
		limit = *size
	}
	return c.bytes.Fetch(ctx, transfers.ServedFrom(out.URL, c.host), limit, out.MD5, nil, p)
}

// Upload sends a local file under name (or its base name) into a web folder.
func (c *Client) Upload(ctx context.Context, file, name string, folder domain.FileID, p port.Progress) error {
	o, err := transfers.Measure(file, name)
	if err != nil {
		return err
	}
	// Apply, storage signing and finish must belong to one login, even if
	// another goroutine logs out or switches accounts during the byte transfer.
	pin := c.session.Pin()
	apply := struct {
		Folder domain.FileID `json:"directoryId"`
		Name   string        `json:"fileName"`
		MD5    string        `json:"md5"`
		Size   int64         `json:"size"`
	}{folder, o.Name, o.MD5, o.Size}
	var slot transfers.UploadSlot
	if err = c.call(ctx, "file/upload/apply", apply, false, pin, &slot); err != nil {
		return err
	}
	_, err = c.session.AccessToken(pin)
	if err != nil {
		return err
	}
	stored, err := c.bytes.Send(ctx, o, slot, transfers.NewSigner(c.channel, c.session, slot.FileServer, pin),
		&transfers.FormServer{Host: c.host, Token: func() (string, error) { return c.session.AccessToken(pin) }}, p)
	if err != nil {
		return err
	}
	finish := struct {
		Folder domain.FileID `json:"directoryId"`
		Name   string        `json:"fileName"`
		Size   int64         `json:"fileSize"`
		MD5    string        `json:"md5"`
		Inner  string        `json:"innerName"`
		Type   string        `json:"type"`
	}{folder, o.Name, o.Size, o.MD5, stored, "2"}
	return c.call(ctx, "file/upload/finish", finish, true, pin, nil)
}

// UploadBytes writes an in-memory payload through the same upload path.
func (c *Client) UploadBytes(ctx context.Context, data []byte, name string, folder domain.FileID) error {
	file, err := transfers.Scratch(data)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(file) }() // Best-effort scratch cleanup cannot change the transfer result.
	return c.Upload(ctx, file, name, folder, nil)
}
