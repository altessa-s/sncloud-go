// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package webfiles

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// CreateFolder creates a folder and resolves its identifier from a fresh listing.
func (c *Client) CreateFolder(ctx context.Context, name string, parent domain.FileID) (domain.Entry, error) {
	pin := c.session.Pin()
	body := struct {
		Name   string        `json:"fileName"`
		Parent domain.FileID `json:"directoryId"`
	}{name, parent}
	if err := c.call(ctx, "file/folder/add", body, true, pin, nil); err != nil {
		return domain.Entry{}, err
	}
	entries, err := c.list(ctx, parent, domain.SortName, true, pin)
	if err != nil {
		return domain.Entry{}, err
	}
	for _, e := range entries {
		if e.Name == name && e.IsFolder {
			return e, nil
		}
	}
	return domain.Entry{}, &domain.MalformedResponseError{What: "new folder is absent from its parent"}
}

// Delete moves entries from one parent to the recycle bin.
func (c *Client) Delete(ctx context.Context, ids []domain.FileID, parent domain.FileID) error {
	if len(ids) == 0 {
		return nil
	}
	body := struct {
		IDs    []domain.FileID `json:"idList"`
		Parent domain.FileID   `json:"directoryId"`
	}{ids, parent}
	return c.call(ctx, "file/delete", body, true, nil, nil)
}

// Move relocates entries between folders.
func (c *Client) Move(ctx context.Context, ids []domain.FileID, parent, destination domain.FileID) error {
	return c.relocate(ctx, "file/move", ids, parent, destination)
}

// Copy copies entries between folders.
func (c *Client) Copy(ctx context.Context, ids []domain.FileID, parent, destination domain.FileID) error {
	return c.relocate(ctx, "file/copy", ids, parent, destination)
}

// relocate shares the move/copy body and avoids a request for an empty selection.
func (c *Client) relocate(ctx context.Context, path string, ids []domain.FileID, parent, destination domain.FileID) error {
	if len(ids) == 0 {
		return nil
	}
	body := struct {
		IDs         []domain.FileID `json:"idList"`
		Parent      domain.FileID   `json:"directoryId"`
		Destination domain.FileID   `json:"goDirectoryId"`
	}{ids, parent, destination}
	return c.call(ctx, path, body, true, nil, nil)
}

// Rename changes the entry's name within its current folder.
func (c *Client) Rename(ctx context.Context, id domain.FileID, name string) error {
	body := struct {
		ID   domain.FileID `json:"id"`
		Name string        `json:"newName"`
	}{id, name}
	return c.call(ctx, "file/rename", body, true, nil, nil)
}
