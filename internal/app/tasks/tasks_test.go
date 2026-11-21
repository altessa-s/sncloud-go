// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package tasks

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/session"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

const (
	testClientID = "aabbccddeeff00112233445566778899"
)

func syntheticToken() string {
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":4102444800}`))
	return hdr + "." + claims + ".sig"
}

func newTestTaskClient(tr *porttest.Transport) (*Client, *porttest.Secrets) {
	clk := &porttest.Clock{Time: time.Unix(1780000000, 0)}
	secrets := &porttest.Secrets{}
	_ = secrets.Keep(syntheticToken(), domain.AccessToken)
	_ = secrets.Keep("synthetic@example.com", domain.Account)
	_ = secrets.Keep(testClientID, domain.ClientID)
	ch := channel.New(domain.Official(), tr, clk, domain.DefaultRetryPolicy())
	sess := session.New(ch, secrets, clk)
	return New(ch, sess, clk), secrets
}

func decodeJSONMap(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("decode JSON %s: %v", raw, err)
	}
	return m
}

const openTaskRow = `{"taskId":"9b2c1f0e5d4a3b2c1d0e9f8a7b6c5d4e","taskListId":"1","title":"Call the bank","detail":"",` +
	`"lastModified":1773292688000,"recurrence":"","isReminderOn":"N","status":"needsAction","importance":"",` +
	`"dueTime":0,"completedTime":1773292600000,"links":null,"isDeleted":"N","sort":3,"planerSort":1,` +
	`"sortTime":1773292688000,"scheduleRecurTask":[]}`

const doneTaskRow = `{"taskId":"b1","taskListId":"work","title":"Ship it","detail":"before Friday","lastModified":"1773292690000",` +
	`"isReminderOn":"Y","status":"completed","dueTime":1773300000000,"completedTime":1773292690000,` +
	`"links":"eyJhcHBOYW1lIjoibm90ZSJ9","isDeleted":"N"}`

func TestListsTasksAcrossPagesAndFiltersByList(t *testing.T) {
	tr := &porttest.Transport{
		Steps: []porttest.Step{
			porttest.OK(`"nextPageToken":"2","nextSyncToken":null,"scheduleTask":[` + openTaskRow + `]`),
			porttest.OK(`"nextPageToken":null,"nextSyncToken":1773300000000,"scheduleTask":[` + doneTaskRow + `,{"taskId":"gone","title":"x","isDeleted":"Y"}]`),
			porttest.OK(`"nextPageToken":null,"scheduleTask":[` + openTaskRow + `,` + doneTaskRow + `]`),
		},
	}
	client, _ := newTestTaskClient(tr)
	got, err := client.All(context.Background())
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	calls := tr.Calls()
	if calls[0].URL != "https://viewer.supernote.com/api/file/schedule/task/all" || calls[0].Method != "POST" {
		t.Fatalf("call 0 = %s %s", calls[0].Method, calls[0].URL)
	}
	b0 := decodeJSONMap(t, calls[0].Body)
	if b0["maxResults"] != "500" {
		t.Fatalf("maxResults = %v", b0["maxResults"])
	}
	if _, has := b0["nextPageTokens"]; has {
		t.Fatal("unexpected nextPageTokens on page 1")
	}
	if calls[0].Header.Get(domain.AccessTokenHeader) != syntheticToken() ||
		calls[0].Header.Get("version") != domain.ProtocolVersion ||
		calls[0].Header.Get(domain.XSRFHeader) != "" {
		t.Fatalf("unexpected headers: %+v", calls[0].Header)
	}
	b1 := decodeJSONMap(t, calls[1].Body)
	if b1["nextPageTokens"] != "2" {
		t.Fatalf("nextPageTokens = %v", b1["nextPageTokens"])
	}

	if len(got) != 2 || got[0].ID != "9b2c1f0e5d4a3b2c1d0e9f8a7b6c5d4e" || got[1].ID != "b1" {
		t.Fatalf("got IDs: %+v", got)
	}
	if got[0].IsCompleted || got[0].Due != nil || got[0].Detail != nil || *got[0].Order.Sort != 3 || *got[0].Order.PlannerSort != 1 {
		t.Fatalf("unexpected open task: %+v", got[0])
	}
	if !got[1].IsCompleted || !got[1].IsReminderOn || got[1].Links == nil || *got[1].Links != "eyJhcHBOYW1lIjoibm90ZSJ9" {
		t.Fatalf("unexpected done task: %+v", got[1])
	}

	inWork, err := client.InList(context.Background(), "work")
	if err != nil || len(inWork) != 1 || inWork[0].ID != "b1" {
		t.Fatalf("InList = (%+v, %v)", inWork, err)
	}
}

func TestRepeatedAndRunawayPageTokens(t *testing.T) {
	tr := &porttest.Transport{
		Steps: []porttest.Step{
			porttest.OK(`"nextPageToken":"2","scheduleTask":[]`),
			porttest.OK(`"nextPageToken":"2","scheduleTask":[]`),
		},
	}
	client, _ := newTestTaskClient(tr)
	if _, err := client.All(context.Background()); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse on repeated page token, got %v", err)
	}
	if len(tr.Calls()) != 2 {
		t.Fatalf("calls = %d, want 2", len(tr.Calls()))
	}

	runawayTr := &porttest.Transport{
		Steps: []porttest.Step{
			porttest.OK(`"pageToken":"2","scheduleTaskGroup":[]`),
			porttest.OK(`"pageToken":"3","scheduleTaskGroup":[]`),
		},
	}
	limited, _ := newTestTaskClient(runawayTr)
	limited.pageMax = 2
	if _, err := limited.Lists(context.Background()); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse past pageMax, got %v", err)
	}
}

func TestCreatesUpdatesAndDeletesTasks(t *testing.T) {
	tr := &porttest.Transport{
		Steps: []porttest.Step{
			porttest.OK(`"taskId":"c0ffee"`),
			porttest.OK(`"nextPageToken":null,"scheduleTask":[` + doneTaskRow + `]`),
			porttest.OK(`"taskId":null`),
			porttest.OK(`"taskId":null`),
			porttest.OK(""),
			porttest.OK(""),
		},
	}
	client, _ := newTestTaskClient(tr)
	listOne := "1"
	made, err := client.Create(context.Background(), domain.Task{ListID: &listOne, Title: "Water the plants"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if made.ID != "c0ffee" || made.Modified == nil || made.Modified.UnixMilli() != 1780000000000 {
		t.Fatalf("made = %+v", made)
	}
	createBody := decodeJSONMap(t, tr.Calls()[0].Body)
	if _, has := createBody["taskId"]; has {
		t.Fatal("unexpected taskId on new task")
	}
	if createBody["status"] != "needsAction" || createBody["isReminderOn"] != "N" || createBody["dueTime"] != float64(0) {
		t.Fatalf("unexpected createBody: %+v", createBody)
	}

	listed, err := client.All(context.Background())
	if err != nil || len(listed) != 1 || listed[0].ID != "b1" {
		t.Fatalf("All: %v", err)
	}
	task := listed[0]
	task.IsCompleted = false
	task.Completed = nil
	updated, err := client.Update(context.Background(), task)
	if err != nil || updated.Modified == nil {
		t.Fatalf("Update: %v", err)
	}
	updCall := tr.Calls()[2]
	updBody := decodeJSONMap(t, updCall.Body)
	if updCall.Method != "PUT" || updBody["taskId"] != "b1" || updBody["detail"] != "before Friday" || updBody["status"] != "needsAction" {
		t.Fatalf("unexpected update call: %s %+v", updCall.Method, updBody)
	}
	if _, has := updBody["completedTime"]; has {
		t.Fatal("expected completedTime omitted when nil")
	}

	// Clearing recurrence sends empty string.
	workList := "work"
	if _, err := client.Update(context.Background(), domain.Task{ID: "b1", ListID: &workList, Title: "Ship it"}); err != nil {
		t.Fatalf("Update empty recurrence: %v", err)
	}
	recBody := decodeJSONMap(t, tr.Calls()[3].Body)
	if rec, ok := recBody["recurrence"].(string); !ok || rec != "" {
		t.Fatalf("expected empty recurrence string, got %#v", recBody["recurrence"])
	}

	if err := client.Delete(context.Background(), "c0ffee"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := client.DeleteList(context.Background(), "work"); err != nil {
		t.Fatalf("DeleteList: %v", err)
	}
	delTask := tr.Calls()[4]
	if delTask.Method != "DELETE" || !strings.HasSuffix(delTask.URL, "/api/file/schedule/task/c0ffee") || len(delTask.Body) != 0 {
		t.Fatalf("unexpected Delete call: %+v", delTask)
	}
	chHdr := delTask.Header.Get("channel")
	if !strings.HasPrefix(chHdr, syntheticToken()+"_") || !strings.Contains(chHdr, testClientID) {
		t.Fatalf("missing or malformed channel header: %q", chHdr)
	}
	delList := tr.Calls()[5]
	if delList.Method != "DELETE" || !strings.HasSuffix(delList.URL, "/api/file/schedule/group/work") {
		t.Fatalf("unexpected DeleteList call: %+v", delList)
	}
}

func TestListsCRUDDeduplicationOccurrencesAndErrors(t *testing.T) {
	inboxRow := `{"taskListId":"1","userId":5,"title":"Inbox","lastModified":0,"isDeleted":"N","createTime":1773000000000}`
	movedTask := strings.ReplaceAll(openTaskRow, "Call the bank", "Call the bank today")
	recurringRow := `{"taskId":"r1","taskListId":"work","title":"Water the plants","recurrence":"RRULE:FREQ=WEEKLY","status":"needsAction",` +
		`"dueTime":1773300000000,"isDeleted":"N","scheduleRecurTask":[` +
		`{"taskId":"o1","recurrenceId":"r1","taskListId":"work","lastModified":1773292690000,"dueTime":1773300000000,"completedTime":1773292690000,"status":"completed","isDeleted":"N","sort":2},` +
		`{"taskId":"o2","recurrenceId":"r1","status":"needsAction","isDeleted":"Y"}]}`

	tr := &porttest.Transport{
		Steps: []porttest.Step{
			porttest.OK(`"pageToken":"2","scheduleTaskGroup":[` + inboxRow + `]`),
			porttest.OK(`"pageToken":null,"scheduleTaskGroup":[{"taskListId":"work","title":"Work","lastModified":1773292688000,"isDeleted":"N"},{"taskListId":"del","title":"Gone","isDeleted":"Y"}]`),
			porttest.OK(`"taskListId":"work0"`),
			porttest.OK(""),
			porttest.OK(`"taskListId":"work0","title":"Office","lastModified":1780000000000,"isDeleted":"N","createTime":1780000000000`),
			porttest.OK(`"nextPageToken":"2","scheduleTask":[` + openTaskRow + `]`),
			porttest.OK(`"nextPageToken":null,"scheduleTask":[` + doneTaskRow + `,` + movedTask + `]`),
			porttest.OK(`"nextPageToken":null,"scheduleTask":[` + recurringRow + `]`),
			porttest.Refused("E0328"),
			{Response: port.Response{Status: http.StatusServiceUnavailable}},
			porttest.OK(`"taskListId":""`),
			porttest.OK(`"taskId":""`),
			porttest.Refused("E0712"),
		},
	}
	client, secrets := newTestTaskClient(tr)
	ctx := context.Background()

	lists, err := client.Lists(ctx)
	if err != nil || len(lists) != 2 || lists[0].Title != "Inbox" || lists[1].Title != "Work" || lists[0].Modified != nil {
		t.Fatalf("Lists = (%+v, %v)", lists, err)
	}
	madeList, err := client.CreateList(ctx, "Work", "work")
	if err != nil || madeList.ID != "work0" {
		t.Fatalf("CreateList = (%+v, %v)", madeList, err)
	}
	if err := client.RenameList(ctx, "work0", "Office"); err != nil {
		t.Fatalf("RenameList: %v", err)
	}
	readList, err := client.List(ctx, "work0")
	if err != nil || readList.Title != "Office" || tr.Calls()[4].Method != "GET" || len(tr.Calls()[4].Body) != 0 {
		t.Fatalf("List = (%+v, %v)", readList, err)
	}

	deduped, err := client.All(ctx)
	if err != nil || len(deduped) != 2 || deduped[0].ID != "b1" || deduped[1].Title != "Call the bank today" {
		t.Fatalf("deduped = (%+v, %v)", deduped, err)
	}

	recTasks, err := client.All(ctx)
	if err != nil || len(recTasks) != 1 || len(recTasks[0].Occurrences) != 1 || recTasks[0].Occurrences[0].ID != "o1" {
		t.Fatalf("occurrences = (%+v, %v)", recTasks, err)
	}

	nope := "nope"
	var nf *domain.NotFoundError
	if _, err := client.Create(ctx, domain.Task{ListID: &nope, Title: "x"}); !errors.As(err, &nf) || nf.Code != "E0328" {
		t.Fatalf("expected NotFoundError E0328, got %v", err)
	}

	// Mutating delete is not retried on HTTP 503.
	callsBefore := len(tr.Calls())
	if err := client.Delete(ctx, "c0ffee"); !errors.Is(err, domain.ErrHTTP) {
		t.Fatalf("expected ErrHTTP on 503, got %v", err)
	}
	if len(tr.Calls()) != callsBefore+1 {
		t.Fatal("delete was unexpectedly retried")
	}

	// Missing server IDs in create responses fail as malformed.
	if _, err := client.CreateList(ctx, "Empty", ""); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse on empty taskListId, got %v", err)
	}
	if _, err := client.Create(ctx, domain.Task{Title: "Empty"}); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse on empty taskId, got %v", err)
	}

	// Path-traversal IDs fail locally without touching transport.
	beforeValidation := len(tr.Calls())
	for _, err := range []error{
		func() error { _, e := client.List(ctx, "../1"); return e }(),
		client.RenameList(ctx, "../1", "x"),
		client.DeleteList(ctx, "../1"),
		func() error { _, e := client.Create(ctx, domain.Task{ID: "../1", Title: "x"}); return e }(),
		func() error { _, e := client.Update(ctx, domain.Task{ID: "", Title: "x"}); return e }(),
		client.Delete(ctx, "../group/1"),
		client.Delete(ctx, ""),
	} {
		if !errors.Is(err, domain.ErrMalformedResponse) {
			t.Fatalf("expected ErrMalformedResponse, got %v", err)
		}
	}
	if len(tr.Calls()) != beforeValidation {
		t.Fatal("invalid ID reached transport")
	}

	// E0712 invalidates the session token, and subsequent calls fail as ErrNotLoggedIn.
	if _, err := client.All(ctx); !errors.Is(err, domain.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
	if tok, _ := secrets.Secret(domain.AccessToken); tok != "" {
		t.Fatalf("expected refused token cleared, got %q", tok)
	}
	if _, err := client.InList(ctx, "1"); !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Fatalf("expected ErrNotLoggedIn after invalidation, got %v", err)
	}
}
