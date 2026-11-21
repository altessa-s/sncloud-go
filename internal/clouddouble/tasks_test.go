// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func serviceFixture(t *testing.T, flavor Flavor) (*Cloud, http.HandlerFunc) {
	t.Helper()
	c := New(flavor)
	t.Cleanup(c.Close)
	c.tokens["synthetic-token"] = true
	return c, func(w http.ResponseWriter, r *http.Request) { c.serve(w, r, false) }
}

func exchangeMethod(t *testing.T, h http.HandlerFunc, method, path, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(method, "/api/"+path, strings.NewReader(body))
	r.Header.Set("version", domain.ProtocolVersion)
	r.Header.Set("equipmentNo", "synthetic-device")
	r.Header.Set("channel", "synthetic-channel")
	r.Header.Set(domain.AccessTokenHeader, "synthetic-token")
	w := httptest.NewRecorder()
	h(w, r)
	out := make(map[string]any)
	if w.Code == http.StatusOK {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: %v: %s", method, path, err, w.Body)
		}
	}
	return w, out
}

func TestTaskListAndTaskState(t *testing.T) {
	for _, flavor := range []Flavor{Official, Private} {
		t.Run(string(flavor), func(t *testing.T) {
			_, h := serviceFixture(t, flavor)

			_, listsOut := exchange(t, h, domain.PathTaskLists, `{}`)
			requireSuccess(t, listsOut)
			if listsOut["pageToken"] != nil {
				t.Fatalf("unexpected pageToken: %v", listsOut)
			}
			initialGroups := listsOut["scheduleTaskGroup"].([]any)
			if len(initialGroups) != 1 || initialGroups[0].(map[string]any)[keyTaskListID] != "1" {
				t.Fatalf("seeded Inbox missing: %v", listsOut)
			}

			_, genOut := exchange(t, h, domain.PathTaskList, `{"title":"Projects","lastModified":10,"createTime":10}`)
			requireSuccess(t, genOut)
			genID, _ := genOut[keyTaskListID].(string)
			if genID == "" {
				t.Fatal(genOut)
			}

			_, customOut := exchange(t, h, domain.PathTaskList, `{"taskListId":"work","title":"Work","lastModified":11,"createTime":11}`)
			requireSuccess(t, customOut)
			_, collidedOut := exchange(t, h, domain.PathTaskList, `{"taskListId":"work","title":"Work Copy","lastModified":12,"createTime":12}`)
			requireSuccess(t, collidedOut)
			if customOut[keyTaskListID] != "work" || collidedOut[keyTaskListID] != "work0" {
				t.Fatalf("list IDs: %v %v", customOut, collidedOut)
			}

			_, renameOut := exchangeMethod(t, h, http.MethodPut, domain.PathTaskList, `{"taskListId":"work","title":"Active Work","lastModified":20}`)
			requireSuccess(t, renameOut)
			_, renameMissing := exchangeMethod(t, h, http.MethodPut, domain.PathTaskList, `{"taskListId":"unknown","title":"Ignored","lastModified":20}`)
			requireSuccess(t, renameMissing)

			_, gotList := exchangeMethod(t, h, http.MethodGet, domain.PathTaskList+"/work", "")
			requireSuccess(t, gotList)
			if gotList["title"] != "Active Work" || gotList[keyIsDeleted] != flagUndeleted {
				t.Fatalf("renamed list: %v", gotList)
			}

			_, badListTask := exchange(t, h, domain.PathTask, `{"taskListId":"missing","title":"Orphan"}`)
			requireCode(t, badListTask, "E0328")

			_, autoTask := exchange(t, h, domain.PathTask, `{"taskListId":"work","title":"Generated"}`)
			requireSuccess(t, autoTask)
			if autoID, _ := autoTask[keyTaskID].(string); autoID == "" {
				t.Fatal(autoTask)
			}
			_, taskOne := exchange(t, h, domain.PathTask, `{"taskId":"t-a","taskListId":"work","title":"Draft"}`)
			requireSuccess(t, taskOne)
			_, taskDup := exchange(t, h, domain.PathTask, `{"taskId":"t-a","taskListId":"work0","title":"Duplicate ID"}`)
			requireSuccess(t, taskDup)
			if taskOne[keyTaskID] != "t-a" || taskDup[keyTaskID] != "t-a0" {
				t.Fatalf("task IDs: %v %v", taskOne, taskDup)
			}

			_, updMissing := exchangeMethod(t, h, http.MethodPut, domain.PathTask, `{"taskId":"missing","title":"None"}`)
			requireCode(t, updMissing, "E0329")
			_, updOut := exchangeMethod(t, h, http.MethodPut, domain.PathTask, `{"taskId":"t-a","taskListId":"work","title":"Published","status":"completed"}`)
			requireSuccess(t, updOut)
			if updOut[keyTaskID] != nil {
				t.Fatal(updOut)
			}

			_, delTaskOut := exchangeMethod(t, h, http.MethodDelete, domain.PathTask+"/t-a0", "")
			requireSuccess(t, delTaskOut)
			_, delTaskAgain := exchangeMethod(t, h, http.MethodDelete, domain.PathTask+"/t-a0", "")
			requireCode(t, delTaskAgain, "E0329")
			_, updDeleted := exchangeMethod(t, h, http.MethodPut, domain.PathTask, `{"taskId":"t-a0","taskListId":"work0","title":"Revived"}`)
			requireCode(t, updDeleted, "E0329")

			_, allTasksOut := exchange(t, h, domain.PathTasks, `{}`)
			requireSuccess(t, allTasksOut)
			if allTasksOut["nextPageToken"] != nil {
				t.Fatal(allTasksOut)
			}
			taskRows := allTasksOut["scheduleTask"].([]any)
			if len(taskRows) != 3 {
				t.Fatalf("expected 3 stored rows (including tombstone), got %v", taskRows)
			}
			if taskRows[1].(map[string]any)["title"] != "Published" || taskRows[2].(map[string]any)[keyIsDeleted] != flagDeleted {
				t.Fatalf("unexpected task rows: %v", taskRows)
			}

			// Deleting a list tombstones both the list and every task inside it.
			_, delListOut := exchangeMethod(t, h, http.MethodDelete, domain.PathTaskList+"/work", "")
			requireSuccess(t, delListOut)
			_, delListAgain := exchangeMethod(t, h, http.MethodDelete, domain.PathTaskList+"/work", "")
			requireCode(t, delListAgain, "E0328")
			_, getDeletedList := exchangeMethod(t, h, http.MethodGet, domain.PathTaskList+"/work", "")
			requireCode(t, getDeletedList, "E0328")
			_, addIntoDeleted := exchange(t, h, domain.PathTask, `{"taskListId":"work","title":"Late"}`)
			requireCode(t, addIntoDeleted, "E0328")
			_, updCascadeDeleted := exchangeMethod(t, h, http.MethodPut, domain.PathTask, `{"taskId":"t-a","taskListId":"work","title":"Stale"}`)
			requireCode(t, updCascadeDeleted, "E0329")

			_, finalLists := exchange(t, h, domain.PathTaskLists, `{}`)
			requireSuccess(t, finalLists)
			groups := finalLists["scheduleTaskGroup"].([]any)
			if len(groups) != 4 || groups[2].(map[string]any)[keyIsDeleted] != flagDeleted {
				t.Fatalf("expected deleted tombstone in group listing: %v", groups)
			}
		})
	}
}

func TestTaskRequestValidation(t *testing.T) {
	c, h := serviceFixture(t, Official)

	// Task endpoints live on the app host only; official web host returns 404.
	webReq := httptest.NewRequest(http.MethodPost, "/api/"+domain.PathTaskLists, strings.NewReader(`{}`))
	webReq.AddCookie(&http.Cookie{Name: domain.XSRFCookie, Value: "synthetic-xsrf"})
	webReq.Header.Set(domain.XSRFHeader, "synthetic-xsrf")
	webRec := httptest.NewRecorder()
	c.serve(webRec, webReq, true)
	if webRec.Code != http.StatusNotFound {
		t.Fatalf("official web host status = %d, want 404", webRec.Code)
	}

	// Missing protocol headers and invalid tokens are rejected before routing.
	noHeaders := httptest.NewRequest(http.MethodPost, "/api/"+domain.PathTaskLists, strings.NewReader(`{}`))
	noHeadersRec := httptest.NewRecorder()
	h(noHeadersRec, noHeaders)
	if noHeadersRec.Code != http.StatusBadRequest {
		t.Fatalf("missing headers status = %d, want 400", noHeadersRec.Code)
	}

	badToken := httptest.NewRequest(http.MethodPost, "/api/"+domain.PathTaskLists, strings.NewReader(`{}`))
	badToken.Header.Set("version", domain.ProtocolVersion)
	badToken.Header.Set("equipmentNo", "synthetic-device")
	badToken.Header.Set(domain.AccessTokenHeader, "revoked")
	badTokenRec := httptest.NewRecorder()
	h(badTokenRec, badToken)
	if !strings.Contains(badTokenRec.Body.String(), "E0712") {
		t.Fatalf("bad token body = %s", badTokenRec.Body)
	}

	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPost, domain.PathTaskLists, "bad", http.StatusBadRequest},
		{http.MethodPost, domain.PathTaskList, "bad", http.StatusBadRequest},
		{http.MethodPut, domain.PathTaskList, "bad", http.StatusBadRequest},
		{http.MethodPost, domain.PathTasks, "bad", http.StatusBadRequest},
		{http.MethodPost, domain.PathTask, "bad", http.StatusBadRequest},
		{http.MethodPut, domain.PathTask, "bad", http.StatusBadRequest},
		{http.MethodPatch, domain.PathTaskList + "/1", "", http.StatusMethodNotAllowed},
		{http.MethodGet, domain.PathTask + "/t-a", "", http.StatusNotFound},
		{http.MethodPost, "file/schedule/unknown", `{}`, http.StatusNotFound},
	} {
		w, _ := exchangeMethod(t, h, tc.method, tc.path, tc.body)
		if w.Code != tc.status {
			t.Fatalf("%s %s: status %d, want %d", tc.method, tc.path, w.Code, tc.status)
		}
	}
}
