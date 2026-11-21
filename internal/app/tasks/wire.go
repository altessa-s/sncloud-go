// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package tasks

import (
	"github.com/altessa-s/sncloud-go/internal/domain"
)

const (
	flagYes = "Y"
	flagNo  = "N"
)

type listPageRequest struct {
	MaxResults string `json:"maxResults"`
	PageToken  string `json:"pageToken,omitzero"`
}

// nextPageTokens is plural in the request and singular in the response on the wire.
type taskPageRequest struct {
	MaxResults     string `json:"maxResults"`
	NextPageTokens string `json:"nextPageTokens,omitzero"`
}

type listsAnswer struct {
	PageToken         *string                          `json:"pageToken"`
	ScheduleTaskGroup []domain.Marked[domain.TaskList] `json:"scheduleTaskGroup"`
}

func (a listsAnswer) rows() ([]domain.Marked[domain.TaskList], *string) {
	return a.ScheduleTaskGroup, a.PageToken
}

func (listsAnswer) pageRequest(tok string) any {
	return listPageRequest{MaxResults: pageSize, PageToken: tok}
}

func (listsAnswer) itemID(l domain.TaskList) string { return l.ID }

type tasksAnswer struct {
	NextPageToken *string                      `json:"nextPageToken"`
	ScheduleTask  []domain.Marked[domain.Task] `json:"scheduleTask"`
}

func (a tasksAnswer) rows() ([]domain.Marked[domain.Task], *string) {
	return a.ScheduleTask, a.NextPageToken
}

func (tasksAnswer) pageRequest(tok string) any {
	return taskPageRequest{MaxResults: pageSize, NextPageTokens: tok}
}

func (tasksAnswer) itemID(t domain.Task) string { return t.ID }

type newListRequest struct {
	TaskListID   string `json:"taskListId,omitzero"`
	Title        string `json:"title"`
	LastModified int64  `json:"lastModified"`
	CreateTime   int64  `json:"createTime"`
}

type renamedListRequest struct {
	TaskListID   string `json:"taskListId"`
	Title        string `json:"title"`
	LastModified int64  `json:"lastModified"`
}

type listCreatedAnswer struct {
	TaskListID *string `json:"taskListId"`
}

type taskCreatedAnswer struct {
	TaskID *string `json:"taskId"`
}

// taskBody encodes every field the server overwrites on create and update.
// Recurrence and Detail are sent as empty strings when nil so updates can clear
// an existing rule or note; DueTime is sent as 0 when nil because the server's
// column is non-nullable.
type taskBody struct {
	TaskID           string  `json:"taskId,omitzero"`
	TaskListID       *string `json:"taskListId,omitzero"`
	Title            string  `json:"title"`
	Detail           string  `json:"detail"`
	Status           string  `json:"status"`
	DueTime          int64   `json:"dueTime"`
	CompletedTime    *int64  `json:"completedTime,omitzero"`
	LastModified     int64   `json:"lastModified"`
	Recurrence       string  `json:"recurrence"`
	IsReminderOn     string  `json:"isReminderOn"`
	Importance       *string `json:"importance,omitzero"`
	Links            *string `json:"links,omitzero"`
	Sort             *int    `json:"sort,omitzero"`
	SortCompleted    *int    `json:"sortCompleted,omitzero"`
	PlanerSort       *int    `json:"planerSort,omitzero"`
	AllSort          *int    `json:"allSort,omitzero"`
	AllSortCompleted *int    `json:"allSortCompleted,omitzero"`
	SortTime         *int64  `json:"sortTime,omitzero"`
	PlanerSortTime   *int64  `json:"planerSortTime,omitzero"`
	AllSortTime      *int64  `json:"allSortTime,omitzero"`
}

func newTaskBody(t domain.Task, lastModified int64) taskBody {
	var detail string
	if t.Detail != nil {
		detail = *t.Detail
	}
	status := domain.TaskStatusNeedsAction
	if t.IsCompleted {
		status = domain.TaskStatusCompleted
	}
	var due int64
	if t.Due != nil {
		due = t.Due.UnixMilli()
	}
	var completed *int64
	if t.Completed != nil {
		ms := t.Completed.UnixMilli()
		completed = &ms
	}
	var recurrence string
	if t.Recurrence != nil {
		recurrence = *t.Recurrence
	}
	reminder := flagNo
	if t.IsReminderOn {
		reminder = flagYes
	}
	return taskBody{
		TaskID:           t.ID,
		TaskListID:       t.ListID,
		Title:            t.Title,
		Detail:           detail,
		Status:           status,
		DueTime:          due,
		CompletedTime:    completed,
		LastModified:     lastModified,
		Recurrence:       recurrence,
		IsReminderOn:     reminder,
		Importance:       t.Importance,
		Links:            t.Links,
		Sort:             t.Order.Sort,
		SortCompleted:    t.Order.SortCompleted,
		PlanerSort:       t.Order.PlannerSort,
		AllSort:          t.Order.AllSort,
		AllSortCompleted: t.Order.AllSortCompleted,
		SortTime:         t.Order.SortTime,
		PlanerSortTime:   t.Order.PlannerSortTime,
		AllSortTime:      t.Order.AllSortTime,
	}
}
