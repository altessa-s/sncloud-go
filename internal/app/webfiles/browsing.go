// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package webfiles

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

const (
	pageSize    = 100
	pageCeiling = 10000
)

// pageRequest carries the common one-based web paging and ordering fields.
type pageRequest struct {
	Order    domain.SortOrder `json:"order"`
	Sequence string           `json:"sequence"`
	Page     int              `json:"pageNo"`
	Size     int              `json:"pageSize"`
}

// ordering supplies the web API defaults and its textual sort direction.
func ordering(order domain.SortOrder, ascending bool, page int) pageRequest {
	if order == "" {
		order = domain.SortName
	}
	sequence := "desc"
	if ascending {
		sequence = "asc"
	}
	return pageRequest{Order: order, Sequence: sequence, Page: page, Size: pageSize}
}

// pages accepts either total convention and stops on an empty page.
// The ceiling also bounds a broken server that repeats full pages indefinitely.
func pages[T any](fetch func(int) ([]T, *domain.WireInt64, *domain.WireInt64, error)) ([]T, error) {
	var all []T
	for page := 1; page <= pageCeiling; page++ {
		entries, total, count, err := fetch(page)
		if err != nil {
			return nil, err
		}
		all = append(all, entries...)
		if len(entries) == 0 || (total != nil && int64(len(all)) >= int64(*total)) ||
			(count != nil && int64(page) >= int64(*count)) || (total == nil && count == nil && len(entries) < pageSize) {
			return all, nil
		}
	}
	return nil, &domain.MalformedResponseError{What: fmt.Sprintf("the file listing ran past %d pages", pageCeiling)}
}

// List reads every page in a folder. Empty order selects filename.
func (c *Client) List(ctx context.Context, folder domain.FileID, order domain.SortOrder, ascending bool) ([]domain.Entry, error) {
	return c.list(ctx, folder, order, ascending, c.session.Pin())
}

func (c *Client) list(
	ctx context.Context, folder domain.FileID, order domain.SortOrder, ascending bool, pin *domain.CredentialPin,
) ([]domain.Entry, error) {
	return pages(func(page int) ([]domain.Entry, *domain.WireInt64, *domain.WireInt64, error) {
		body := struct {
			pageRequest
			Folder domain.FileID `json:"directoryId"`
		}{ordering(order, ascending, page), folder}
		var out struct {
			Entries []domain.Entry    `json:"userFileVOList"`
			Total   *domain.WireInt64 `json:"total"`
			Pages   *domain.WireInt64 `json:"pages"`
		}
		err := c.call(ctx, "file/list/query", body, false, pin, &out)
		return out.Entries, out.Total, out.Pages, err
	})
}

// Search finds names throughout the tree, reading every page.
func (c *Client) Search(ctx context.Context, name string, order domain.SortOrder, ascending bool) ([]domain.Entry, error) {
	pin := c.session.Pin()
	return pages(func(page int) ([]domain.Entry, *domain.WireInt64, *domain.WireInt64, error) {
		body := struct {
			pageRequest
			Name string `json:"fileName"`
		}{ordering(order, ascending, page), name}
		var out struct {
			Entries []domain.Entry    `json:"userFileSearchVOList"`
			Total   *domain.WireInt64 `json:"total"`
		}
		err := c.call(ctx, "file/list/search", body, false, pin, &out)
		return out.Entries, out.Total, nil, err
	})
}

// Entry returns the named direct child, or nil when absent.
func (c *Client) Entry(ctx context.Context, name string, folder domain.FileID) (*domain.Entry, error) {
	entries, err := c.List(ctx, folder, domain.SortName, true)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Name == name {
			return &e, nil
		}
	}
	return nil, nil //nolint:nilnil // An absent entry is a successful lookup.
}

// Folders lists direct subfolders, excluding destinations that would contain themselves.
func (c *Client) Folders(ctx context.Context, folder domain.FileID, excluding []domain.FileID) ([]domain.Folder, error) {
	if len(excluding) == 0 {
		excluding = []domain.FileID{folder}
	}
	body := struct {
		Folder domain.FileID   `json:"directoryId"`
		IDs    []domain.FileID `json:"idList"`
	}{folder, excluding}
	var out struct {
		Folders []domain.Folder `json:"folderVOList"`
	}
	err := c.call(ctx, "file/folder/list/query", body, false, nil, &out)
	// Repeated identifiers can carry updated metadata; retain the last observation.
	return domain.UniqueLast(out.Folders, func(f domain.Folder) domain.FileID { return f.ID }), err
}

// Capacity reports used and allocated bytes.
func (c *Client) Capacity(ctx context.Context) (domain.Capacity, error) {
	var out domain.Capacity
	err := c.call(ctx, "file/capacity/query", struct{}{}, false, nil, &out)
	return out, err
}

// User reports the current account's profile.
func (c *Client) User(ctx context.Context) (domain.User, error) {
	var out domain.User
	err := c.call(ctx, "user/query", struct{}{}, false, nil, &out)
	return out, err
}

// Location reads the containing folder names and identifiers.
func (c *Client) Location(ctx context.Context, id domain.FileID) (domain.Location, error) {
	var out struct {
		Path string `json:"path"`
		IDs  string `json:"idPath"`
	}
	if err := c.call(ctx, "file/path/query", idRequest{id}, false, nil, &out); err != nil {
		return domain.Location{}, err
	}
	result := domain.Location{Names: strings.FieldsFunc(out.Path, func(r rune) bool { return r == '/' })}
	for s := range strings.SplitSeq(out.IDs, "/") {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			result.IDs = append(result.IDs, domain.FileID(n))
		}
	}
	return result, nil
}
