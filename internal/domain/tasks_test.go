// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json/v2"
	"errors"
	"testing"
	"time"
)

func TestTaskListDecoding(t *testing.T) {
	raw := `{"taskListId":"1","title":"Inbox","lastModified":1773292688000,"createTime":0}`
	var list TaskList
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		t.Fatalf("Unmarshal TaskList: %v", err)
	}
	wantMod := time.UnixMilli(1773292688000)
	if list.ID != "1" || list.Title != "Inbox" || list.Modified == nil || !list.Modified.Equal(wantMod) || list.Created != nil {
		t.Fatalf("unexpected TaskList: %+v", list)
	}

	var missingTitle TaskList
	if err := json.Unmarshal([]byte(`{"taskListId":"2","title":null}`), &missingTitle); err != nil || missingTitle.Title != "" {
		t.Fatalf("null title: %+v, err=%v", missingTitle, err)
	}

	for _, bad := range []string{`{}`, `{"taskListId":null}`, `{"taskListId":123}`, `{"taskListId":"1","title":99}`, `[]`} {
		var badList TaskList
		if err := json.Unmarshal([]byte(bad), &badList); err == nil {
			t.Fatalf("expected error for %s", bad)
		}
	}
}

func TestTaskAndOccurrencesDecoding(t *testing.T) {
	raw := `{
		"taskId":"t-1","taskListId":"1","title":"Review","detail":"Notes",
		"status":"completed","dueTime":1773379200000,"completedTime":1773380000000,
		"lastModified":1773380100000,"recurrence":"FREQ=DAILY","isReminderOn":"Y",
		"importance":"high","links":"eyJwIjoxfQ==",
		"sort":3,"sortCompleted":4,"planerSort":2,"allSort":5,"allSortCompleted":6,
		"sortTime":100,"planerSortTime":200,"allSortTime":300,
		"scheduleRecurTask":[
			{"taskId":"o-1","recurrenceId":"t-1","taskListId":"1","status":"completed","dueTime":1773379200000,"completedTime":1773380000000,"lastModified":1773380000000,"isDeleted":"N","sort":1},
			{"taskId":"o-2","recurrenceId":"t-1","status":"needsAction","dueTime":0,"isDeleted":"Y"}
		]
	}`
	var task Task
	if err := json.Unmarshal([]byte(raw), &task); err != nil {
		t.Fatalf("Unmarshal Task: %v", err)
	}
	if task.ID != "t-1" || task.Title != "Review" || !task.IsCompleted || !task.IsReminderOn {
		t.Fatalf("unexpected Task: %+v", task)
	}
	if task.Order.PlannerSort == nil || *task.Order.PlannerSort != 2 || task.Order.PlannerSortTime == nil || *task.Order.PlannerSortTime != 200 {
		t.Fatalf("unexpected Order: %+v", task.Order)
	}
	if len(task.Occurrences) != 1 || task.Occurrences[0].ID != "o-1" || task.Occurrences[0].TaskID != "t-1" || !task.Occurrences[0].IsCompleted {
		t.Fatalf("unexpected Occurrences: %+v", task.Occurrences)
	}

	// Open task with zero dueTime and malformed optional scheduleRecurTask survives.
	var open Task
	if err := json.Unmarshal([]byte(`{"taskId":"t-2","title":"Open","status":"needsAction","dueTime":0,"completedTime":1773380000000,"scheduleRecurTask":"bad"}`), &open); err != nil {
		t.Fatalf("open task: %v", err)
	}
	if open.IsCompleted || open.Due != nil || open.Completed == nil {
		t.Fatalf("status alone governs IsCompleted: %+v", open)
	}

	for _, bad := range []string{`[]`, `{"taskId":123}`, `{"title":123}`} {
		var badTask Task
		if err := json.Unmarshal([]byte(bad), &badTask); err == nil {
			t.Fatalf("expected error for %s", bad)
		}
	}
	for _, badOcc := range []string{`[]`, `{"taskId":123}`} {
		var occ TaskOccurrence
		if err := json.Unmarshal([]byte(badOcc), &occ); err == nil {
			t.Fatalf("expected occurrence error for %s", badOcc)
		}
	}
}

func TestValidateTaskID(t *testing.T) {
	for _, valid := range []string{"1", "abc_DEF-09"} {
		if err := ValidateTaskID(valid); err != nil {
			t.Fatalf("ValidateTaskID(%q): %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "../other", "a/b", "a?b=1", "space id"} {
		err := ValidateTaskID(invalid)
		if _, ok := errors.AsType[*MalformedResponseError](err); !ok {
			t.Fatalf("ValidateTaskID(%q) = %v, want *MalformedResponseError", invalid, err)
		}
	}
}
