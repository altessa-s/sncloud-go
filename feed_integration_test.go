//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/clouddouble"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

func TestFeedBothCloudFlavors(t *testing.T) {
	for _, flavor := range []clouddouble.Flavor{clouddouble.Official, clouddouble.Private} {
		t.Run(string(flavor), func(t *testing.T) {
			cloud := clouddouble.New(flavor)
			defer cloud.Close()

			watcherClient, _ := loginTestClient(t, cloud)
			clock := feedStepClock{ticks: make(chan struct{})}
			sncloud.SetFeedClockForTest(watcherClient, cloud.Endpoints(), clock)
			actorClient, _ := loginTestClient(t, cloud)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			type feedResult struct {
				change sncloud.Change
				err    error
			}
			events := make(chan feedResult, 16)
			feedCtx, stopFeed := context.WithCancel(ctx)
			defer stopFeed()

			go func() {
				for ch, err := range watcherClient.Feed().Changes(feedCtx) {
					events <- feedResult{change: ch, err: err}
					if err != nil {
						return
					}
				}
			}()

			// 1. Every stream starts with ChangeConnected so callers know to re-list.
			first := <-events
			if first.err != nil || first.change.Type != sncloud.ChangeConnected {
				t.Fatalf("initial event = (%+v, %v), want ChangeConnected", first.change, first.err)
			}

			// 2. Self-originated mutations carry watcherClient's channel header and
			// do not echo back on watcherClient's own feed.
			selfList, err := watcherClient.Tasks().CreateList(ctx, "Self-Muted", "")
			if err != nil {
				t.Fatalf("watcher CreateList: %v", err)
			}
			if err := watcherClient.Tasks().DeleteList(ctx, selfList.ID); err != nil {
				t.Fatalf("watcher DeleteList: %v", err)
			}

			var delivered []sncloud.Change
			readNextType := func(want sncloud.ChangeType) sncloud.Change {
				t.Helper()
				select {
				case clock.ticks <- struct{}{}:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				for {
					var ev feedResult
					select {
					case ev = <-events:
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
					if ev.err != nil {
						t.Fatal(ev.err)
					}
					for _, tc := range ev.change.Tasks {
						if tc.ListID != nil && *tc.ListID == selfList.ID {
							t.Fatalf("self-originated list echoed: %+v", ev.change)
						}
					}
					// The initial flush and a heartbeat can both precede the acknowledgement.
					// The feed permits repeats; ignore only exact, previously validated batches.
					// This cannot distinguish server redelivery from client duplicate yields.
					// TestReadsAndAcknowledgesAnnouncementsAndHandlesRefusals in internal/app/feed
					// checks exact output cardinality against a controlled sequence of frames.
					duplicate := false
					for _, previous := range delivered {
						if reflect.DeepEqual(previous, ev.change) {
							duplicate = true
							break
						}
					}
					if duplicate {
						continue
					}
					if ev.change.Type != want {
						t.Fatalf("want %v, got %+v", want, ev.change)
					}
					delivered = append(delivered, ev.change)
					return ev.change
				}

			}

			// 3. Cross-device task, digest and file announcements arrive in order.
			workList, err := actorClient.Tasks().CreateList(ctx, "Shared", "")
			if err != nil {
				t.Fatalf("actor CreateList: %v", err)
			}
			taskCh := readNextType(sncloud.ChangeTasks)
			if len(taskCh.Tasks) != 1 || taskCh.Tasks[0].ListID == nil || *taskCh.Tasks[0].ListID != workList.ID {
				t.Fatalf("expected actor task announcement %q, got %+v", workList.ID, taskCh)
			}

			groupID, err := actorClient.Digests().CreateGroup(ctx, "Shared Notes", nil, "")
			if err != nil || groupID == 0 {
				t.Fatalf("actor CreateGroup = (%v, %v)", groupID, err)
			}
			digestCh := readNextType(sncloud.ChangeDigests)
			if len(digestCh.Digests) != 1 || digestCh.Digests[0].Item != sncloud.DigestItemGroup {
				t.Fatalf("expected actor digest announcement, got %+v", digestCh)
			}

			cloud.Announce("REMOTE_TABLET", "ServerMessage", map[string]any{
				"messageType":  "DOWNLOADFILE",
				"fileType":     "FILE",
				"id":           "42",
				"originalName": "Note.note",
				"md5":          "ABCDEF0123456789ABCDEF0123456789",
			})
			fileCh := readNextType(sncloud.ChangeFiles)
			if len(fileCh.Files) != 1 || fileCh.Files[0].Kind != sncloud.ChangeFileAdded {
				t.Fatalf("file announcement = %+v", fileCh)
			}
			stopFeed()

			// 4. Socket.IO 403 token refusal surfaces ErrSessionExpired on Watch
			// without retrying and drops the stale token from SecretStore.
			badStore := &porttest.Secrets{}
			if err := badStore.Keep("revoked-token", sncloud.AccessToken); err != nil {
				t.Fatal(err)
			}
			badClient, err := sncloud.New(
				badStore,
				sncloud.WithEndpoints(cloud.Endpoints()),
				sncloud.WithRoundTripper(http.DefaultTransport),
			)
			if err != nil {
				t.Fatal(err)
			}
			defer badClient.Close()

			var feedErr error
			for _, ferr := range badClient.Feed().Watch(ctx) {
				if ferr != nil {
					feedErr = ferr
				}
			}
			if !errors.Is(feedErr, sncloud.ErrSessionExpired) {
				t.Fatalf("revoked token Watch = %v, want ErrSessionExpired", feedErr)
			}
			if loggedIn, _ := badClient.IsLoggedIn(); loggedIn {
				t.Fatal("session remained logged in after Socket.IO 403 token refusal")
			}
		})
	}
}

// feedStepClock releases one heartbeat after each completed mutation.
type feedStepClock struct{ ticks chan struct{} }

func (feedStepClock) Now() time.Time { return port.SystemClock{}.Now() }
func (c feedStepClock) Sleep(ctx context.Context, _ time.Duration) error {
	select {
	case <-c.ticks:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
