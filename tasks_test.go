// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

func newAuthedFacadeClient(t *testing.T, rt http.RoundTripper) *sncloud.Client {
	t.Helper()
	store := &porttest.Secrets{}
	if err := store.Keep("synthetic-token", sncloud.AccessToken); err != nil {
		t.Fatal(err)
	}
	c, err := sncloud.New(store, sncloud.WithRoundTripper(rt))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestFacadeTasks(t *testing.T) {
	c := newAuthedFacadeClient(t, facadeTransport(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/file/schedule/group/all"):
			return jsonResponse(`{"success":true,"pageToken":null,"scheduleTaskGroup":[{"taskListId":"1","title":"Inbox","isDeleted":"N"}]}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/schedule/group/1"):
			if r.Method == http.MethodDelete {
				return jsonResponse(`{"success":true}`), nil
			}
			return jsonResponse(`{"success":true,"taskListId":"1","title":"Inbox"}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/schedule/group") && r.Method == http.MethodPost:
			return jsonResponse(`{"success":true,"taskListId":"list-new"}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/schedule/group") && r.Method == http.MethodPut:
			return jsonResponse(`{"success":true}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/schedule/task/all"):
			return jsonResponse(`{"success":true,"pageToken":null,"scheduleTask":[{"taskId":"t1","taskListId":"1","title":"Read","status":"needsAction","isDeleted":"N"}]}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/schedule/task") && r.Method == http.MethodPost:
			return jsonResponse(`{"success":true,"taskId":"t2"}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/schedule/task") && r.Method == http.MethodPut:
			return jsonResponse(`{"success":true}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/schedule/task/t2") && r.Method == http.MethodDelete:
			return jsonResponse(`{"success":true}`), nil
		default:
			return jsonResponse(`{"success":false,"errorCode":"E9999"}`), nil
		}
	}))

	ctx := context.Background()
	view := c.Tasks()
	lists, err := view.Lists(ctx)
	if err != nil || len(lists) != 1 || lists[0].ID != "1" {
		t.Fatalf("Lists = (%v, %v)", lists, err)
	}
	gotList, err := view.List(ctx, "1")
	if err != nil || gotList.Title != "Inbox" {
		t.Fatalf("List = (%v, %v)", gotList, err)
	}
	createdList, err := view.CreateList(ctx, "Work", "")
	if err != nil || createdList.ID != "list-new" {
		t.Fatalf("CreateList = (%v, %v)", createdList, err)
	}
	if err := view.RenameList(ctx, "1", "Renamed"); err != nil {
		t.Fatalf("RenameList: %v", err)
	}
	if err := view.DeleteList(ctx, "1"); err != nil {
		t.Fatalf("DeleteList: %v", err)
	}
	allTasks, err := view.All(ctx)
	if err != nil || len(allTasks) != 1 || allTasks[0].ID != "t1" {
		t.Fatalf("All = (%v, %v)", allTasks, err)
	}
	inList, err := view.InList(ctx, "1")
	if err != nil || len(inList) != 1 || inList[0].ID != "t1" {
		t.Fatalf("InList = (%v, %v)", inList, err)
	}
	createdTask, err := view.Create(ctx, sncloud.Task{Title: "Write"})
	if err != nil || createdTask.ID != "t2" {
		t.Fatalf("Create = (%v, %v)", createdTask, err)
	}
	updatedTask, err := view.Update(ctx, createdTask)
	if err != nil || updatedTask.ID != "t2" {
		t.Fatalf("Update = (%v, %v)", updatedTask, err)
	}
	if err := view.Delete(ctx, "t2"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}
