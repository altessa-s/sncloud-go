// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"context"
	"errors"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/digests"
	"github.com/altessa-s/sncloud-go/internal/app/tasks"
	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/app/webfiles"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestMultiStepReadersRejectExternalReplacement(t *testing.T) {
	for _, operation := range []string{"list", "search", "recycle", "folder", "conversion", "task-lists", "tasks", "digests", "fingerprints", "groups", "digest-update", "group-update"} {
		t.Run(operation, func(t *testing.T) {
			s, tr, store := setup()
			_ = store.Keep("account-a", domain.AccessToken)
			policy := domain.DefaultRetryPolicy()
			s.channel = channel.New(domain.PrivateCloud("https://synthetic.invalid", false), tr, s.clock, policy)
			bytes := transfers.New(tr, s.clock, policy)
			web := webfiles.New(s.channel, s, bytes, "https://synthetic.invalid", s.clock)
			digest := digests.New(domain.Official(), s.channel, s, bytes, s.clock)
			task := tasks.New(s.channel, s, s.clock)
			var reply porttest.Step
			switch operation {
			case "list":
				reply = porttest.OK(`"total":2,"userFileVOList":[{"id":1,"fileName":"synthetic"}]`)
			case "search":
				reply = porttest.OK(`"total":2,"userFileSearchVOList":[{"id":1,"fileName":"synthetic"}]`)
			case "recycle":
				reply = porttest.OK(`"total":2,"recycleFileVOList":[{"fileId":1,"fileName":"synthetic"}]`)
			case "folder":
				reply = porttest.OK("")
			case "conversion":
				reply = porttest.Refused("E0316")
			case "task-lists":
				reply = porttest.OK(`"pageToken":"next"`)
			case "tasks":
				reply = porttest.OK(`"nextPageToken":"next"`)
			case "digests", "groups":
				reply = porttest.OK(`"totalPages":2,"summaryDOList":[{"id":1,"fileName":"synthetic"}]`)
			case "fingerprints":
				reply = porttest.OK(`"totalPages":2,"summaryInfoVOList":[{"id":1,"fileName":"synthetic"}]`)
			case "digest-update", "group-update":
				reply = porttest.OK(`"summaryDOList":[{"id":1,"uniqueIdentifier":"synthetic"}]`)
			}
			tr.Steps = []porttest.Step{{Run: func(context.Context, port.Request) (port.Response, error) {
				_ = store.Keep("account-b", domain.AccessToken)
				return reply.Response, nil
			}}}
			var err error
			switch operation {
			case "list":
				_, err = web.List(t.Context(), 0, "", true)
			case "search":
				_, err = web.Search(t.Context(), "name", "", true)
			case "recycle":
				_, err = web.RecycleBin(t.Context(), "", true)
			case "folder":
				_, err = web.CreateFolder(t.Context(), "name", 0)
			case "conversion":
				_, err = web.PNGs(t.Context(), 1, nil)
			case "task-lists":
				_, err = task.Lists(t.Context())
			case "tasks":
				_, err = task.All(t.Context())
			case "digests":
				_, err = digest.List(t.Context(), "")
			case "fingerprints":
				_, err = digest.Fingerprints(t.Context(), "")
			case "groups":
				_, err = digest.Groups(t.Context())
			case "digest-update":
				content := "replacement"
				err = digest.Update(t.Context(), 1, domain.DigestDraft{Content: &content})
			case "group-update":
				err = digest.UpdateGroup(t.Context(), 1, "replacement", nil)
			}
			if !errors.Is(err, domain.ErrNotLoggedIn) {
				t.Fatalf("continued after replacement: %v", err)
			}
			calls := tr.Calls()
			if len(calls) != 1 || calls[0].Header.Get(domain.AccessTokenHeader) != "account-a" {
				t.Fatal("unexpected authenticated requests")
			}
		})
	}
}
