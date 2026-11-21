// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package devicefiles

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// CreateFolder creates a full destination path, optionally letting the service rename it.
func (c *Client) CreateFolder(ctx context.Context, path string, autorename bool) (domain.DeviceEntry, error) {
	name, err := c.equipment()
	if err != nil {
		return domain.DeviceEntry{}, err
	}
	body := struct {
		byPath
		Autorename bool `json:"autorename"`
	}{byPath{device{name}, "/" + domain.DevicePath(path)}, autorename}
	var out struct {
		Entry *domain.DeviceEntry `json:"metadata"`
	}
	if err = c.call(ctx, "file/2/files/create_folder_v2", body, true, nil, &out); err != nil {
		return domain.DeviceEntry{}, err
	}
	if out.Entry == nil {
		return domain.DeviceEntry{}, &domain.MalformedResponseError{What: "create_folder_v2 named no folder"}
	}
	return *out.Entry, nil
}

// Delete permanently removes an entry, bypassing the recycle bin.
// The tablet endpoint performs a hard delete; web recycle semantics do not apply.
func (c *Client) Delete(ctx context.Context, id domain.FileID) error {
	name, err := c.equipment()
	if err != nil {
		return err
	}
	return c.call(ctx, "file/3/files/delete_folder_v3", byID{device{name}, id}, true, nil, nil)
}

// Move uses the full destination, including the new name.
func (c *Client) Move(ctx context.Context, id domain.FileID, path string, autorename bool) (*domain.DeviceEntry, error) {
	return c.relocate(ctx, "file/3/files/move_v3", id, path, autorename)
}

// Copy uses the full destination, including the copy's name.
func (c *Client) Copy(ctx context.Context, id domain.FileID, path string, autorename bool) (*domain.DeviceEntry, error) {
	return c.relocate(ctx, "file/3/files/copy_v3", id, path, autorename)
}

// relocate sends the entire destination, including the new name, as the tablet API requires.
func (c *Client) relocate(ctx context.Context, endpoint string, id domain.FileID, path string, autorename bool) (*domain.DeviceEntry, error) {
	name, err := c.equipment()
	if err != nil {
		return nil, err
	}
	body := struct {
		byID
		Path       string `json:"to_path"`
		Autorename bool   `json:"autorename"`
	}{byID{device{name}, id}, "/" + domain.DevicePath(path), autorename}
	var out found
	err = c.call(ctx, endpoint, body, true, nil, &out)
	return out.Entry, err
}
