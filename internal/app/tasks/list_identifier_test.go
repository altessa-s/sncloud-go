// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package tasks

import (
	"testing"

	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestCreateListRejectsUnusableIdentifier(t *testing.T) {
	tr := &porttest.Transport{Steps: []porttest.Step{porttest.OK(`"taskListId":"reading/list"`)}}
	c, _ := newTestTaskClient(tr)
	if _, err := c.CreateList(t.Context(), "Reading", "reading/list"); err == nil {
		t.Error("accepted unusable identifier")
	}
	if len(tr.Calls()) != 0 {
		t.Fatal("invalid identifier reached server")
	}
}
