// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package devicefiles

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// List reads a path, optionally including every descendant.
func (c *Client) List(ctx context.Context, path string, recursive bool) ([]domain.DeviceEntry, error) {
	name, err := c.equipment()
	if err != nil {
		return nil, err
	}
	body := struct {
		byPath
		Recursive bool `json:"recursive"`
	}{byPath{device{name}, "/" + domain.DevicePath(path)}, recursive}
	var out struct {
		Entries []domain.DeviceEntry `json:"entries"`
	}
	err = c.call(ctx, "file/2/files/list_folder", body, false, nil, &out)
	if !recursive {
		for i := range out.Entries {
			out.Entries[i] = domain.DeviceEntryListedIn(out.Entries[i], path)
		}
	}
	return out.Entries, err
}

// ListFolder reads by identifier; this endpoint uniquely requires a JSON number.
func (c *Client) ListFolder(ctx context.Context, id domain.FileID, recursive bool) ([]domain.DeviceEntry, error) {
	name, err := c.equipment()
	if err != nil {
		return nil, err
	}
	body := struct {
		device
		ID        int64 `json:"id"`
		Recursive bool  `json:"recursive"`
	}{device{name}, int64(id), recursive}
	var out struct {
		Entries []domain.DeviceEntry `json:"entries"`
	}
	err = c.call(ctx, "file/3/files/list_folder_v3", body, false, nil, &out)
	return out.Entries, err
}

// Entry returns nil for the service's entriesVO:null answer.
func (c *Client) Entry(ctx context.Context, id domain.FileID) (*domain.DeviceEntry, error) {
	name, err := c.equipment()
	if err != nil {
		return nil, err
	}
	var out found
	err = c.call(ctx, "file/3/files/query_v3", byID{device{name}, id}, false, nil, &out)
	return out.Entry, err
}

// EntryAt looks up a normalized full path.
func (c *Client) EntryAt(ctx context.Context, path string) (*domain.DeviceEntry, error) {
	name, err := c.equipment()
	if err != nil {
		return nil, err
	}
	var out found
	err = c.call(ctx, "file/3/files/query/by/path_v3", byPath{device{name}, "/" + domain.DevicePath(path)}, false, nil, &out)
	return out.Entry, err
}

// SpaceUsage reads used and optionally allocated storage.
func (c *Client) SpaceUsage(ctx context.Context) (domain.SpaceUsage, error) {
	name, err := c.equipment()
	if err != nil {
		return domain.SpaceUsage{}, err
	}
	var out domain.SpaceUsage
	err = c.call(ctx, "file/2/users/get_space_usage", device{name}, false, nil, &out)
	return out, err
}
