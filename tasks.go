// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// TaskList is a to-do list, called a task group on the wire. List identifiers
// are strings rather than numeric snowflakes; the default inbox list is "1".
type TaskList = domain.TaskList

// Task is one item in a to-do list. Updates overwrite all mutable fields on
// the server, so callers mutate a Task read from a listing and pass the full
// value back to [Tasks.Update].
type Task = domain.Task

// TaskOrder holds opaque sort positions and timestamps across tablet views.
type TaskOrder = domain.TaskOrder

// TaskOccurrence is a single detached or completed occurrence of a recurring task.
type TaskOccurrence = domain.TaskOccurrence

// Lists reads every non-deleted to-do list in the account.
func (c *Tasks) Lists(ctx context.Context) ([]TaskList, error) {
	return c.tasks.Lists(ctx)
}

// List fetches a single to-do list by identifier. Identifiers outside
// [A-Za-z0-9_-]+ fail locally with [MalformedResponseError].
func (c *Tasks) List(ctx context.Context, id string) (TaskList, error) {
	return c.tasks.List(ctx, id)
}

// CreateList requires a nonempty caller-chosen id to match [A-Za-z0-9_-]+.
// It creates a to-do list. An empty id accepts a server-assigned
// identifier; when a caller-supplied id already exists, the server appends "0"
// until unused rather than rejecting, so inspect the returned [TaskList].ID.
func (c *Tasks) CreateList(ctx context.Context, title, id string) (TaskList, error) {
	return c.tasks.CreateList(ctx, title, id)
}

// RenameList updates a to-do list's title. Identifiers outside [A-Za-z0-9_-]+
// fail locally with [MalformedResponseError].
func (c *Tasks) RenameList(ctx context.Context, id, title string) error {
	return c.tasks.RenameList(ctx, id, title)
}

// DeleteList permanently deletes a list and its tasks. Identifiers outside
// [A-Za-z0-9_-]+ fail locally with [MalformedResponseError].
func (c *Tasks) DeleteList(ctx context.Context, id string) error {
	return c.tasks.DeleteList(ctx, id)
}

// All reads every non-deleted task across all to-do lists in the account.
func (c *Tasks) All(ctx context.Context) ([]Task, error) {
	return c.tasks.All(ctx)
}

// InList reads every non-deleted task belonging to listID.
func (c *Tasks) InList(ctx context.Context, listID string) ([]Task, error) {
	return c.tasks.InList(ctx, listID)
}

// Create creates a task and returns the input with the assigned ID and local
// Modified timestamp; it does not re-read the stored task. On an ID collision,
// the server appends "0" until unused; inspect the returned [Task].ID.
func (c *Tasks) Create(ctx context.Context, task Task) (Task, error) {
	return c.tasks.Create(ctx, task)
}

// Update overwrites all mutable fields of task on the server and returns the
// updated value. An invalid task.ID fails locally with [MalformedResponseError].
func (c *Tasks) Update(ctx context.Context, task Task) (Task, error) {
	return c.tasks.Update(ctx, task)
}

// Delete permanently deletes a task by identifier. Identifiers outside [A-Za-z0-9_-]+
// fail locally with [MalformedResponseError].
func (c *Tasks) Delete(ctx context.Context, id string) error {
	return c.tasks.Delete(ctx, id)
}

// Tasks is a view of the to-do API over the client's shared session.
type Tasks struct {
	tasks port.TaskService
}

// Tasks returns a cheap view sharing the session and network services.
func (c *Client) Tasks() *Tasks {
	return &Tasks{tasks: c.tasks}
}
