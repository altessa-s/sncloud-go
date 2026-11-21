//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/clouddouble"
	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

func loginTestClient(t *testing.T, cloud *clouddouble.Cloud) (*sncloud.Client, *porttest.Secrets) {
	t.Helper()
	store := &porttest.Secrets{}
	c, err := sncloud.New(
		store,
		sncloud.WithEndpoints(cloud.Endpoints()),
		sncloud.WithRoundTripper(http.DefaultTransport),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	out, err := c.LogIn(context.Background(), "synthetic@example.com", "synthetic-password", nil)
	if err != nil || out != sncloud.LoggedIn {
		t.Fatalf("LogIn = (%v, %v)", out, err)
	}
	return c, store
}

func TestTasksBothCloudFlavors(t *testing.T) {
	for _, flavor := range []clouddouble.Flavor{clouddouble.Official, clouddouble.Private} {
		t.Run(string(flavor), func(t *testing.T) {
			cloud := clouddouble.New(flavor)
			defer cloud.Close()

			c, _ := loginTestClient(t, cloud)
			ctx := t.Context()
			view := c.Tasks()

			initialLists, err := view.Lists(ctx)
			if err != nil || len(initialLists) != 1 || initialLists[0].ID != "1" {
				t.Fatalf("initial Lists = (%+v, %v)", initialLists, err)
			}

			workList, err := view.CreateList(ctx, "Projects", "")
			if err != nil || workList.ID == "" {
				t.Fatalf("CreateList = (%+v, %v)", workList, err)
			}
			if err := view.RenameList(ctx, workList.ID, "Active Projects"); err != nil {
				t.Fatalf("RenameList: %v", err)
			}
			fetchedList, err := view.List(ctx, workList.ID)
			if err != nil || fetchedList.Title != "Active Projects" {
				t.Fatalf("List = (%+v, %v)", fetchedList, err)
			}

			detail := "Outline chapter one"
			recurrence := "FREQ=WEEKLY;BYDAY=MO"
			createdTask, err := view.Create(ctx, sncloud.Task{
				ListID:     &workList.ID,
				Title:      "Draft outline",
				Detail:     &detail,
				Recurrence: &recurrence,
			})
			if err != nil || createdTask.ID == "" {
				t.Fatalf("Create = (%+v, %v)", createdTask, err)
			}

			createdTask.IsCompleted = true
			updatedTask, err := view.Update(ctx, createdTask)
			if err != nil || !updatedTask.IsCompleted {
				t.Fatalf("Update = (%+v, %v)", updatedTask, err)
			}

			inList, err := view.InList(ctx, workList.ID)
			if err != nil || len(inList) != 1 || !inList[0].IsCompleted || inList[0].Recurrence == nil || *inList[0].Recurrence != recurrence {
				t.Fatalf("InList = (%+v, %v)", inList, err)
			}
			all, err := view.All(ctx)
			if err != nil || len(all) != 1 || all[0].ID != createdTask.ID {
				t.Fatalf("All = (%+v, %v)", all, err)
			}

			if err := view.Delete(ctx, createdTask.ID); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if err := view.Delete(ctx, createdTask.ID); !errors.Is(err, sncloud.ErrNotFound) {
				t.Fatalf("Delete missing = %v, want ErrNotFound", err)
			}
			if err := view.DeleteList(ctx, workList.ID); err != nil {
				t.Fatalf("DeleteList: %v", err)
			}
			if _, err := view.List(ctx, workList.ID); !errors.Is(err, sncloud.ErrNotFound) {
				t.Fatalf("List deleted = %v, want ErrNotFound", err)
			}
		})
	}
}
