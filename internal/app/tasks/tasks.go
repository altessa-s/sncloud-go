// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package tasks

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// All returns every non-deleted task across all lists. Full pagination is
// always used instead of sync-token deltas: a delta never shows a deleted
// task, and on servers older than image 26.10.09 it returns other accounts'
// rows.
func (c *Client) All(ctx context.Context) ([]domain.Task, error) {
	return fetchPaged[domain.Task, tasksAnswer](c, ctx, domain.PathTasks)
}

// InList returns every non-deleted task belonging to listID.
func (c *Client) InList(ctx context.Context, listID string) ([]domain.Task, error) {
	all, err := c.All(ctx)
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.Task, 0, len(all))
	for _, t := range all {
		if t.ListID != nil && *t.ListID == listID {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}

// Create creates a task and returns it with the server-assigned identifier and
// modification timestamp populated. A colliding caller-chosen ID receives a
// trailing zero on the server rather than a refusal.
func (c *Client) Create(ctx context.Context, task domain.Task) (domain.Task, error) {
	if task.ID != "" {
		if err := domain.ValidateTaskID(task.ID); err != nil {
			return domain.Task{}, err
		}
	}
	stamp := c.clock.Now()
	var ans taskCreatedAnswer
	if err := c.call(ctx, domain.PathTask, methodPost, newTaskBody(task, stamp.UnixMilli()), true, &ans); err != nil {
		return domain.Task{}, err
	}
	if ans.TaskID == nil || *ans.TaskID == "" {
		return domain.Task{}, &domain.MalformedResponseError{What: "schedule/task named no task"}
	}
	created := task
	created.ID = *ans.TaskID
	created.Modified = &stamp
	return created, nil
}

// Update writes the complete task back to the server and returns it with the
// updated modification timestamp. Because PUT file/schedule/task overwrites
// mutable columns even when omitted from JSON, callers must pass a full Task.
func (c *Client) Update(ctx context.Context, task domain.Task) (domain.Task, error) {
	if err := domain.ValidateTaskID(task.ID); err != nil {
		return domain.Task{}, err
	}
	stamp := c.clock.Now()
	if err := c.call(ctx, domain.PathTask, methodPut, newTaskBody(task, stamp.UnixMilli()), true, nil); err != nil {
		return domain.Task{}, err
	}
	updated := task
	updated.Modified = &stamp
	return updated, nil
}

// Delete permanently deletes a task by identifier. The identifier is validated
// before reaching the URL path.
func (c *Client) Delete(ctx context.Context, id string) error {
	if err := domain.ValidateTaskID(id); err != nil {
		return err
	}
	return c.call(ctx, domain.PathTask+"/"+id, methodDelete, nil, true, nil)
}
