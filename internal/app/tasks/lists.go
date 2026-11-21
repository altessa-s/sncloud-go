// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package tasks

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Lists returns every non-deleted to-do list, read whole from the first page
// and deduplicated by last arrival.
func (c *Client) Lists(ctx context.Context) ([]domain.TaskList, error) {
	return fetchPaged[domain.TaskList, listsAnswer](c, ctx, domain.PathTaskLists)
}

// List fetches a single to-do list by identifier. The identifier is validated
// before reaching the URL path so malformed strings cannot traverse path segments.
func (c *Client) List(ctx context.Context, id string) (domain.TaskList, error) {
	if err := domain.ValidateTaskID(id); err != nil {
		return domain.TaskList{}, err
	}
	var out domain.TaskList
	if err := c.call(ctx, domain.PathTaskList+"/"+id, methodGet, nil, false, &out); err != nil {
		return domain.TaskList{}, err
	}
	return out, nil
}

// CreateList creates a to-do list and returns it with the server-assigned
// identifier. An empty id lets the server derive one; a colliding id is not
// refused by the server but receives a trailing zero until free, so callers
// must keep the returned TaskList.ID.
func (c *Client) CreateList(ctx context.Context, title, id string) (domain.TaskList, error) {
	if id != "" {
		if err := domain.ValidateTaskID(id); err != nil {
			return domain.TaskList{}, err
		}
	}
	stamp := c.clock.Now()
	ms := stamp.UnixMilli()
	var ans listCreatedAnswer
	err := c.call(ctx, domain.PathTaskList, methodPost, newListRequest{
		TaskListID:   id,
		Title:        title,
		LastModified: ms,
		CreateTime:   ms,
	}, true, &ans)
	if err != nil {
		return domain.TaskList{}, err
	}
	if ans.TaskListID == nil || *ans.TaskListID == "" {
		return domain.TaskList{}, &domain.MalformedResponseError{What: "schedule/group named no list"}
	}
	return domain.TaskList{
		ID:       *ans.TaskListID,
		Title:    title,
		Modified: &stamp,
		Created:  &stamp,
	}, nil
}

// RenameList updates a list's title.
func (c *Client) RenameList(ctx context.Context, id, title string) error {
	if err := domain.ValidateTaskID(id); err != nil {
		return err
	}
	return c.call(ctx, domain.PathTaskList, methodPut, renamedListRequest{
		TaskListID:   id,
		Title:        title,
		LastModified: c.clock.Now().UnixMilli(),
	}, true, nil)
}

// DeleteList permanently removes a list and every task inside it. The identifier
// is validated before reaching the URL path.
func (c *Client) DeleteList(ctx context.Context, id string) error {
	if err := domain.ValidateTaskID(id); err != nil {
		return err
	}
	return c.call(ctx, domain.PathTaskList+"/"+id, methodDelete, nil, true, nil)
}
