// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package webfiles

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// RecycleBin reads all pages and keeps the latest repeated entry.
func (c *Client) RecycleBin(ctx context.Context, order domain.SortOrder, ascending bool) ([]domain.RecycledEntry, error) {
	if order == "" {
		order = domain.SortTime
	}
	pin := c.session.Pin()
	all, err := pages(func(page int) ([]domain.RecycledEntry, *domain.WireInt64, *domain.WireInt64, error) {
		var out struct {
			Entries []domain.RecycledEntry `json:"recycleFileVOList"`
			Total   *domain.WireInt64      `json:"total"`
		}
		err := c.call(ctx, "file/recycle/list/query", ordering(order, ascending, page), false, pin, &out)
		return out.Entries, out.Total, nil, err
	})
	// Repeated identifiers can carry updated metadata; retain the last observation.
	return domain.UniqueLast(all, func(e domain.RecycledEntry) domain.FileID { return e.ID }), err
}

// Restore restores entries to their former parents.
func (c *Client) Restore(ctx context.Context, ids []domain.FileID) error {
	return c.recycle(ctx, "file/recycle/revert", ids)
}

// DeleteForever permanently removes the named recycled entries.
func (c *Client) DeleteForever(ctx context.Context, ids []domain.FileID) error {
	return c.recycle(ctx, "file/recycle/delete", ids)
}

// recycle sends each identifier once so duplicate selections cannot repeat a change.
func (c *Client) recycle(ctx context.Context, path string, ids []domain.FileID) error {
	if len(ids) == 0 {
		return nil
	}
	body := struct {
		IDs []domain.FileID `json:"idList"`
	}{domain.UniqueLast(ids, func(id domain.FileID) domain.FileID { return id })}
	return c.call(ctx, path, body, true, nil, nil)
}

// EmptyRecycleBin permanently removes every recycled entry.
func (c *Client) EmptyRecycleBin(ctx context.Context) error {
	return c.call(ctx, "file/recycle/clear", struct{}{}, true, nil, nil)
}
