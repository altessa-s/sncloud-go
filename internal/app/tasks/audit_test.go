// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package tasks

import (
	"testing"

	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestLaterTaskDeletionWins(t *testing.T) {
	for _, lists := range []bool{false, true} {
		field, token, id := "scheduleTask", "nextPageToken", "taskId"
		if lists {
			field, token, id = "scheduleTaskGroup", "pageToken", "taskListId"
		}
		tr := &porttest.Transport{Steps: []porttest.Step{
			porttest.OK(`"` + token + `":"2","` + field + `":[{"` + id + `":"1","isDeleted":"N"},{"` + id + `":"2","isDeleted":"Y"}]`),
			porttest.OK(`"` + field + `":[{"` + id + `":"1","isDeleted":"Y"},{"` + id + `":"2","isDeleted":"N"}]`),
		}}
		c, _ := newTestTaskClient(tr)
		if lists {
			got, err := c.Lists(t.Context())
			if err != nil || len(got) != 1 || got[0].ID != "2" {
				t.Fatalf("lists=%+v, %v", got, err)
			}
		} else {
			got, err := c.All(t.Context())
			if err != nil || len(got) != 1 || got[0].ID != "2" {
				t.Fatalf("tasks=%+v, %v", got, err)
			}
		}
	}
}
