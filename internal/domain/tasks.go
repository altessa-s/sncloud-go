// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"time"
)

// To-do endpoint paths under /api.
const (
	// PathTaskLists queries paged task groups.
	PathTaskLists = "file/schedule/group/all"
	// PathTaskList addresses a single task group for create, rename, get or delete.
	PathTaskList = "file/schedule/group"
	// PathTasks queries paged tasks across all lists.
	PathTasks = "file/schedule/task/all"
	// PathTask addresses a single task for create, update or delete.
	PathTask = "file/schedule/task"
)

// Task wire status values.
const (
	// TaskStatusCompleted marks a completed task or occurrence.
	TaskStatusCompleted = "completed"
	// TaskStatusNeedsAction marks an open task or occurrence.
	TaskStatusNeedsAction = "needsAction"
)

// TaskList is a to-do list, called a task group on the wire. List identifiers
// are strings rather than numeric snowflakes; the default list is "1".
type TaskList struct {
	ID       string
	Title    string
	Modified *time.Time
	Created  *time.Time
}

// UnmarshalJSON decodes a task group row from its protocol wire representation.
func (l *TaskList) UnmarshalJSON(b []byte) error {
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	id, err := requiredString(row, "taskListId")
	if err != nil {
		return err
	}
	title, err := optionalString(row, "title")
	if err != nil {
		return err
	}
	*l = TaskList{
		ID:       id,
		Title:    title,
		Modified: WireTime(row["lastModified"]),
		Created:  WireTime(row["createTime"]),
	}
	return nil
}

// Task is one item in a to-do list. The server's update endpoint overwrites
// title, detail, status, importance, due and completion timestamps, links and
// list whether omitted or present, so callers mutate a Task read from a listing
// and send the complete value back.
type Task struct {
	ID           string
	ListID       *string
	Title        string
	Detail       *string
	IsCompleted  bool
	Due          *time.Time
	Completed    *time.Time
	Modified     *time.Time
	Recurrence   *string
	IsReminderOn bool
	Importance   *string
	Links        *string
	Order        TaskOrder
	Occurrences  []TaskOccurrence
}

// UnmarshalJSON decodes a task row and filters out deleted recurrence occurrences.
func (t *Task) UnmarshalJSON(b []byte) error {
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	id, err := optionalString(row, "taskId")
	if err != nil {
		return err
	}
	title, err := optionalString(row, "title")
	if err != nil {
		return err
	}
	status := Text(row["status"])
	rem := Flag(row["isReminderOn"])
	var occ []TaskOccurrence
	if raw, ok := row["scheduleRecurTask"]; ok && len(raw) > 0 {
		var marked []Marked[TaskOccurrence]
		if json.Unmarshal(raw, &marked) == nil {
			occ = make([]TaskOccurrence, 0, len(marked))
			for _, m := range marked {
				if !m.Deleted {
					occ = append(occ, m.Value)
				}
			}
		}
	}
	*t = Task{
		ID:           id,
		ListID:       Text(row["taskListId"]),
		Title:        title,
		Detail:       Text(row["detail"]),
		IsCompleted:  status != nil && *status == TaskStatusCompleted,
		Due:          WireTime(row["dueTime"]),
		Completed:    WireTime(row["completedTime"]),
		Modified:     WireTime(row["lastModified"]),
		Recurrence:   Text(row["recurrence"]),
		IsReminderOn: rem != nil && *rem,
		Importance:   Text(row["importance"]),
		Links:        Text(row["links"]),
		Order:        decodeTaskOrder(row),
		Occurrences:  occ,
	}
	return nil
}

// TaskOrder holds the opaque sort positions and timestamps across tablet views
// so updates preserve ordering across open, completed, planner and all-tasks views.
type TaskOrder struct {
	Sort             *int
	SortCompleted    *int
	PlannerSort      *int
	AllSort          *int
	AllSortCompleted *int
	SortTime         *int64
	PlannerSortTime  *int64
	AllSortTime      *int64
}

func decodeTaskOrder(row map[string]jsontext.Value) TaskOrder {
	pos := func(key string) *int {
		n := Integer(row[key])
		if n == nil {
			return nil
		}
		v := int(*n)
		return &v
	}
	// planerSort and planerSortTime are the protocol's spellings.
	return TaskOrder{
		Sort:             pos("sort"),
		SortCompleted:    pos("sortCompleted"),
		PlannerSort:      pos("planerSort"),
		AllSort:          pos("allSort"),
		AllSortCompleted: pos("allSortCompleted"),
		SortTime:         Integer(row["sortTime"]),
		PlannerSortTime:  Integer(row["planerSortTime"]),
		AllSortTime:      Integer(row["allSortTime"]),
	}
}

// TaskOccurrence is a single detached or completed occurrence of a recurring task.
type TaskOccurrence struct {
	ID          string
	TaskID      string
	ListID      *string
	IsCompleted bool
	Due         *time.Time
	Completed   *time.Time
	Modified    *time.Time
	Order       TaskOrder
}

// UnmarshalJSON decodes a detached recurrence occurrence row.
func (o *TaskOccurrence) UnmarshalJSON(b []byte) error {
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	id, err := optionalString(row, "taskId")
	if err != nil {
		return err
	}
	var taskID string
	if rec := Text(row["recurrenceId"]); rec != nil {
		taskID = *rec
	}
	status := Text(row["status"])
	*o = TaskOccurrence{
		ID:          id,
		TaskID:      taskID,
		ListID:      Text(row["taskListId"]),
		IsCompleted: status != nil && *status == TaskStatusCompleted,
		Due:         WireTime(row["dueTime"]),
		Completed:   WireTime(row["completedTime"]),
		Modified:    WireTime(row["lastModified"]),
		Order:       decodeTaskOrder(row),
	}
	return nil
}

// ValidateTaskID verifies that id is safe to interpolate as the final segment
// of a REST path. List GET/DELETE and task DELETE place the caller's string
// directly into the URL rather than a JSON body; server-issued identifiers are
// hex digests or "1", so rejecting anything outside non-empty ASCII letters,
// digits, hyphens and underscores blocks path traversal, extra segments and
// query or fragment injection before any request is built.
func ValidateTaskID(id string) error {
	if id == "" {
		return &MalformedResponseError{What: fmt.Sprintf("not a to-do identifier: %q", id)}
	}
	for i := range len(id) {
		c := id[i]
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_'
		if !ok {
			return &MalformedResponseError{What: fmt.Sprintf("not a to-do identifier: %q", id)}
		}
	}
	return nil
}

func requiredString(row map[string]jsontext.Value, key string) (string, error) {
	raw, ok := row[key]
	if !ok || raw.Kind() == 'n' {
		return "", fmt.Errorf("missing %s", key)
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	return s, nil
}

func optionalString(row map[string]jsontext.Value, key string) (string, error) {
	raw, ok := row[key]
	if !ok || len(raw) == 0 || raw.Kind() == 'n' {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", err
	}
	return s, nil
}
