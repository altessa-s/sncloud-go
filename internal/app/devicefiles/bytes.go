// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package devicefiles

import (
	"context"
	"os"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Download returns a temporary file, verifying the full hash or the requested range.
func (c *Client) Download(ctx context.Context, id domain.FileID, span *transfers.ByteRange, p port.Progress) (string, error) {
	name, err := c.equipment()
	if err != nil {
		return "", err
	}
	var out struct {
		URL  string            `json:"url"`
		Hash string            `json:"content_hash"`
		Size *domain.WireInt64 `json:"size"`
	}
	if err = c.call(ctx, "file/3/files/download_v3", byID{device{name}, id}, false, c.session.Pin(), &out); err != nil {
		return "", err
	}
	limit := transfers.DownloadLimit
	if out.Size != nil {
		limit = int64(*out.Size)
	}
	return c.bytes.Fetch(ctx, transfers.ServedFrom(out.URL, c.host), limit, out.Hash, span, p)
}

// Upload sends a local file to a full destination and returns its metadata.
func (c *Client) Upload(ctx context.Context, file, path string, p port.Progress) (domain.DeviceEntry, error) {
	destination := domain.DevicePath(path)
	parts := strings.Split(destination, "/")
	filename := parts[len(parts)-1]
	if filename == "" {
		return domain.DeviceEntry{}, &domain.MalformedResponseError{What: "upload needs a file name"}
	}
	folder := strings.Join(parts[:len(parts)-1], "/")
	if folder != "" {
		folder += "/"
	}
	o, err := transfers.Measure(file, filename)
	if err != nil {
		return domain.DeviceEntry{}, err
	}
	name, err := c.equipment()
	if err != nil {
		return domain.DeviceEntry{}, err
	}
	// Apply, storage signing and finish must belong to one login, even if
	// another goroutine logs out or switches accounts during the byte transfer.
	pin := c.session.Pin()
	apply := struct {
		byPath
		Name string `json:"fileName"`
		Size int64  `json:"size"`
	}{byPath{device{name}, folder}, o.Name, o.Size}
	var slot transfers.UploadSlot
	if err = c.call(ctx, "file/3/files/upload/apply", apply, false, pin, &slot); err != nil {
		return domain.DeviceEntry{}, err
	}
	_, err = c.session.AccessToken(pin)
	if err != nil {
		return domain.DeviceEntry{}, err
	}
	stored, err := c.bytes.Send(ctx, o, slot, transfers.NewSigner(c.channel, c.session, slot.FileServer, pin),
		&transfers.FormServer{Host: c.host, Token: func() (string, error) { return c.session.AccessToken(pin) }}, p)
	if err != nil {
		return domain.DeviceEntry{}, err
	}
	finish := struct {
		byPath
		Name  string `json:"fileName"`
		Hash  string `json:"content_hash"`
		Inner string `json:"innerName"`
		Size  int64  `json:"size"`
	}{byPath{device{name}, folder}, o.Name, o.MD5, stored, o.Size}
	var out domain.DeviceEntry
	err = c.call(ctx, "file/2/files/upload/finish", finish, true, pin, &out)
	return out, err
}

// UploadBytes sends in-memory bytes through the same file upload path.
func (c *Client) UploadBytes(ctx context.Context, data []byte, path string) (domain.DeviceEntry, error) {
	file, err := transfers.Scratch(data)
	if err != nil {
		return domain.DeviceEntry{}, err
	}
	defer func() { _ = os.Remove(file) }() // Best-effort scratch cleanup cannot change the transfer result.
	return c.Upload(ctx, file, path, nil)
}
