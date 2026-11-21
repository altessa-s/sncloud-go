// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package devicefiles

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/session"
	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func client(t *testing.T, steps ...porttest.Step) (*Client, *porttest.Transport, *session.Session) {
	t.Helper()
	tr := &porttest.Transport{Steps: steps}
	clock := &porttest.Clock{}
	policy := domain.DefaultRetryPolicy()
	ch := channel.New(domain.PrivateCloud("https://synthetic.invalid", false), tr, clock, policy)
	store := &porttest.Secrets{}
	if err := store.Keep("synthetic-token", domain.AccessToken); err != nil {
		t.Fatal(err)
	}
	s := session.New(ch, store, clock)
	return New(ch, s, transfers.New(tr, clock, policy), "https://synthetic.invalid"), tr, s
}

func TestDeviceBrowsing(t *testing.T) {
	c, tr, _ := client(t, porttest.OK(`"entries":[{"id":"9007199254740993","name":"Foo","path_display":"Foo","tag":"folder"}]`),
		porttest.OK(`"entries":[]`), porttest.OK(`"entriesVO":null`), porttest.OK(`"entriesVO":{"id":1,"name":"a","path_display":"NOTE/Note/a"}`),
		porttest.OK(`"used":"42","allocationVO":{"allocated":"1000"}`), porttest.OK(`"entries":[]`))
	rows, e := c.List(t.Context(), "//Foo//", false)
	if e != nil || len(rows) != 1 || rows[0].Path != "Foo/Foo" {
		t.Fatal(rows, e)
	}
	if _, e = c.ListFolder(t.Context(), 9007199254740993, true); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(tr.Calls()[1].Body), `"id":9007199254740993`) {
		t.Fatal(string(tr.Calls()[1].Body))
	}
	if row, e := c.Entry(t.Context(), 1); e != nil || row != nil {
		t.Fatal(row, e)
	}
	if row, e := c.EntryAt(t.Context(), "//NOTE/Note/a//"); e != nil || row == nil || row.Path != "NOTE/Note/a" {
		t.Fatal(row, e)
	}
	if usage, e := c.SpaceUsage(t.Context()); e != nil || usage.Used != 42 || *usage.Allocated != 1000 {
		t.Fatal(usage, e)
	}
	if _, e = c.List(t.Context(), "/", true); e != nil {
		t.Fatal(e)
	}
	for _, r := range tr.Calls() {
		if r.Header.Get("channel") == "" || r.Header.Get("equipmentNo") == "" {
			t.Fatal(r.Header)
		}
	}
}

func TestDeviceChanges(t *testing.T) {
	c, tr, _ := client(t, porttest.OK(`"metadata":{"id":1,"tag":"folder","name":"F","path_display":"F"}`),
		porttest.OK(`"entriesVO":{"id":1,"name":"new","path_display":"F/new"}`), porttest.OK(`"entriesVO":null`), porttest.OK(""))
	if row, e := c.CreateFolder(t.Context(), "//F//", false); e != nil || row.ID != 1 {
		t.Fatal(row, e)
	}
	if row, e := c.Move(t.Context(), 1, "//F/new//", false); e != nil || row == nil {
		t.Fatal(row, e)
	}
	if !strings.Contains(string(tr.Calls()[1].Body), `"to_path":"/F/new"`) {
		t.Fatal(string(tr.Calls()[1].Body))
	}
	if _, e := c.Copy(t.Context(), 1, "/F/copy", true); e != nil {
		t.Fatal(e)
	}
	if e := c.Delete(t.Context(), 1); e != nil {
		t.Fatal(e)
	}
	c, _, _ = client(t, porttest.OK(""))
	if _, e := c.CreateFolder(t.Context(), "/F", false); !errors.Is(e, domain.ErrMalformedResponse) {
		t.Fatal(e)
	}
	c, _, _ = client(t, porttest.Refused("E0311"))
	if _, e := c.CreateFolder(t.Context(), "/F", false); e == nil {
		t.Fatal("accepted refusal")
	}
}

func TestDeviceBytes(t *testing.T) {
	c, tr, _ := client(t, porttest.OK(`"fullUploadUrl":"http://internal.invalid/api/oss/upload","innerName":"stored"`),
		porttest.OK(`"md5":null`), porttest.OK(`"id":"9007199254740993","name":"x","path_display":"NOTE/Note/x"`),
		porttest.OK(`"url":"http://internal.invalid/api/oss/download","content_hash":"5d41402abc4b2a76b9719d911017c592","size":5`),
		porttest.Step{Response: port.Response{Status: 200, Body: []byte("hello")}},
		porttest.OK(`"url":"https://storage.invalid/x","size":5`),
		porttest.Step{Response: port.Response{Status: 206, Header: port.Header{"Content-Range": {"bytes 1-3/5"}}, Body: []byte("ell")}})
	row, e := c.UploadBytes(t.Context(), []byte("hello"), "//NOTE/Note/x")
	if e != nil || row.ID != 9007199254740993 {
		t.Fatal(row, e)
	}
	if !strings.Contains(string(tr.Calls()[0].Body), `"path":"NOTE/Note/"`) {
		t.Fatal(string(tr.Calls()[0].Body))
	}
	f, e := c.Download(t.Context(), row.ID, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(f)
	f, e = c.Download(t.Context(), row.ID, &transfers.ByteRange{First: 1, Last: 3}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(f)
	if tr.Calls()[4].URL != "https://synthetic.invalid/api/oss/download" {
		t.Fatal(tr.Calls()[4])
	}
}

func TestDeviceUploadFailures(t *testing.T) {
	c, _, _ := client(t)
	if _, e := c.Upload(t.Context(), "/absent", "/", nil); e == nil {
		t.Fatal("missing name")
	}
	if _, e := c.Upload(t.Context(), "/absent", "/x", nil); e == nil {
		t.Fatal("missing source")
	}
	file, e := transfers.Scratch([]byte("x"))
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(file)
	for _, steps := range [][]porttest.Step{{porttest.Refused("E0311")}, {porttest.OK(`"apiServer":"0"`)}} {
		c, _, _ = client(t, steps...)
		if _, e = c.Upload(t.Context(), file, "/x", nil); e == nil {
			t.Fatal("failure ignored")
		}
	}
	c, tr, s := client(t)
	tr.Steps = []porttest.Step{{Run: func(context.Context, port.Request) (port.Response, error) {
		if e := s.Forget(); e != nil {
			t.Fatal(e)
		}
		return porttest.OK(`"url":"https://storage.invalid/x"`).Response, nil
	}}}
	if _, e = c.Upload(t.Context(), file, "/x", nil); !errors.Is(e, domain.ErrNotLoggedIn) {
		t.Fatal(e)
	}
	c, _, _ = client(t, porttest.Refused("E0712"))
	if _, e = c.Download(t.Context(), 1, nil, nil); !errors.Is(e, domain.ErrSessionExpired) {
		t.Fatal(e)
	}
}

func TestSyncLockCleanup(t *testing.T) {
	for _, fail := range []bool{false, true} {
		c, tr, _ := client(t, porttest.OK(`"synType":false`), porttest.OK(""))
		ctx, cancel := context.WithCancel(t.Context())
		sentinel := errors.New("body failed")
		e := c.WithSyncLock(ctx, func(ctx context.Context, hasData bool) error {
			if hasData {
				t.Fatal("empty server reported data")
			}
			if nested := c.WithSyncLock(ctx, func(context.Context, bool) error { t.Fatal("nested body ran"); return nil }); !errors.Is(nested, domain.ErrSyncLocked) {
				t.Fatal(nested)
			}
			if fail {
				cancel()
				return sentinel
			}
			return nil
		})
		cancel()
		if fail && !errors.Is(e, sentinel) || !fail && e != nil {
			t.Fatal(e)
		}
		flag := `"flag":"Y"`
		if fail {
			flag = `"flag":"N"`
		}
		if !strings.Contains(string(tr.Calls()[1].Body), flag) {
			t.Fatal(string(tr.Calls()[1].Body))
		}
	}
	for _, bodyErr := range []error{nil, errors.New("original")} {
		c, _, _ := client(t, porttest.OK(""), porttest.Refused("E0078"))
		e := c.WithSyncLock(t.Context(), func(context.Context, bool) error { return bodyErr })
		if e == nil || bodyErr != nil && e != bodyErr {
			t.Fatal(e)
		}
	}
	c, _, _ := client(t, porttest.Refused("E0078"))
	if e := c.WithSyncLock(t.Context(), func(context.Context, bool) error { t.Fatal("held lock ran body"); return nil }); !errors.Is(e, domain.ErrSyncLocked) {
		t.Fatal(e)
	}
}

func TestDeviceIdentityFailures(t *testing.T) {
	c, _, _ := client(t)
	c.session = failedIdentity{}
	calls := []func() error{
		func() error { _, e := c.List(t.Context(), "/", false); return e }, func() error { _, e := c.ListFolder(t.Context(), 0, false); return e },
		func() error { _, e := c.Entry(t.Context(), 0); return e }, func() error { _, e := c.EntryAt(t.Context(), "/"); return e },
		func() error { _, e := c.SpaceUsage(t.Context()); return e }, func() error { _, e := c.CreateFolder(t.Context(), "/F", false); return e },
		func() error { return c.Delete(t.Context(), 1) }, func() error { _, e := c.Move(t.Context(), 1, "/F", false); return e },
		func() error { _, e := c.Download(t.Context(), 1, nil, nil); return e },
		func() error { return c.WithSyncLock(t.Context(), func(context.Context, bool) error { return nil }) },
	}
	for _, call := range calls {
		if call() == nil {
			t.Fatal("identity failure ignored")
		}
	}
	file, e := transfers.Scratch(nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(file)
	if _, e = c.Upload(t.Context(), file, "/x", nil); e == nil {
		t.Fatal("identity failure ignored")
	}
}

type failedIdentity struct{ port.FileSession }

func (failedIdentity) Identity() (domain.DeviceIdentity, error) {
	return domain.DeviceIdentity{}, errors.New("store unavailable")
}

func TestSyncLockSharedAcrossViews(t *testing.T) {
	c, tr, s := client(t, porttest.OK(`"synType":false`), porttest.OK(""), porttest.OK(`"synType":false`), porttest.OK(""))
	other := New(c.channel, s, c.bytes, c.host)
	if err := c.WithSyncLock(t.Context(), func(ctx context.Context, _ bool) error {
		err := other.WithSyncLock(ctx, func(context.Context, bool) error { t.Error("overlapping body ran"); return nil })
		if !errors.Is(err, domain.ErrSyncLocked) {
			t.Errorf("nested: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := other.WithSyncLock(t.Context(), func(context.Context, bool) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if len(tr.Calls()) != 4 {
		t.Fatal(tr.Calls())
	}
}

func TestMissingDownloadLinkFailsBeforeStorage(t *testing.T) {
	for _, fields := range []string{"", `"url":null`, `"url":""`} {
		c, tr, _ := client(t, porttest.OK(fields))
		_, err := c.Download(t.Context(), 1, nil, nil)
		if !errors.Is(err, domain.ErrMalformedResponse) || len(tr.Calls()) != 1 {
			t.Errorf("%s: %v", fields, err)
		}
	}
}
