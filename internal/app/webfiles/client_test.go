// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package webfiles

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"testing"
	"testing/synctest"

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
	return New(ch, s, transfers.New(tr, clock, policy), "https://synthetic.invalid", clock), tr, s
}

func TestListsEveryPage(t *testing.T) {
	c, tr, _ := client(t, porttest.OK(`"total":"2","userFileVOList":[{"id":"9007199254740993","fileName":"one"}]`),
		porttest.OK(`"total":2,"userFileVOList":[{"id":2,"fileName":"two"}]`))
	entries, err := c.List(t.Context(), 0, "", true)
	if err != nil || len(entries) != 2 || entries[0].ID != 9007199254740993 {
		t.Fatal(entries, err)
	}
	for i, r := range tr.Calls() {
		var b struct {
			Page  int    `json:"pageNo"`
			Order string `json:"order"`
		}
		if err = json.Unmarshal(r.Body, &b); err != nil || b.Page != i+1 || b.Order != "filename" {
			t.Fatal(b, err)
		}
	}
}

func TestBrowsingAndChanges(t *testing.T) {
	c, _, _ := client(t, porttest.OK(`"userFileVOList":[]`), porttest.OK(`"userFileSearchVOList":[{"id":1,"fileName":"match"}]`),
		porttest.OK(`"userFileVOList":[{"id":1,"fileName":"match"}]`), porttest.OK(`"userFileVOList":[]`),
		porttest.OK(`"folderVOList":[{"id":1},{"id":1,"fileName":"latest","empty":false}]`),
		porttest.OK(`"usedCapacity":"2","totalCapacity":3`), porttest.OK(`"userName":"test"`), porttest.OK(`"path":"Note/Meetings/","idPath":"1/bad/9007199254740993/"`),
		porttest.OK(""), porttest.OK(`"userFileVOList":[{"id":2,"fileName":"new","isFolder":"Y"}]`),
		porttest.OK(""), porttest.OK(""), porttest.OK(""), porttest.OK(""))
	if rows, e := c.List(t.Context(), 0, domain.SortTime, false); e != nil || len(rows) != 0 {
		t.Fatal(rows, e)
	}
	if rows, e := c.Search(t.Context(), "match", domain.SortSize, false); e != nil || len(rows) != 1 {
		t.Fatal(rows, e)
	}
	if row, e := c.Entry(t.Context(), "match", 0); e != nil || row == nil {
		t.Fatal(row, e)
	}
	if row, e := c.Entry(t.Context(), "absent", 0); e != nil || row != nil {
		t.Fatal(row, e)
	}
	if rows, e := c.Folders(t.Context(), 0, nil); e != nil || len(rows) != 1 || rows[0].Name != "latest" {
		t.Fatal(rows, e)
	}
	if cap, e := c.Capacity(t.Context()); e != nil || cap.Used != 2 {
		t.Fatal(cap, e)
	}
	if user, e := c.User(t.Context()); e != nil || user.Name != "test" {
		t.Fatal(user, e)
	}
	if loc, e := c.Location(t.Context(), 1); e != nil || len(loc.IDs) != 2 || len(loc.Names) != 2 {
		t.Fatal(loc, e)
	}
	if folder, e := c.CreateFolder(t.Context(), "new", 0); e != nil || folder.ID != 2 {
		t.Fatal(folder, e)
	}
	for _, e := range []error{c.Delete(t.Context(), []domain.FileID{2}, 0), c.Move(t.Context(), []domain.FileID{2}, 0, 3),
		c.Copy(t.Context(), []domain.FileID{2}, 0, 3), c.Rename(t.Context(), 2, "renamed"), c.Delete(t.Context(), nil, 0),
		c.Move(t.Context(), nil, 0, 1), c.Copy(t.Context(), nil, 0, 1)} {
		if e != nil {
			t.Fatal(e)
		}
	}
}

func TestRecycleBin(t *testing.T) {
	c, tr, _ := client(t, porttest.OK(`"total":2,"recycleFileVOList":[{"fileId":1,"fileName":"old"}]`),
		porttest.OK(`"total":2,"recycleFileVOList":[{"fileId":1,"fileName":"new"}]`), porttest.OK(""), porttest.OK(""), porttest.OK(""))
	rows, err := c.RecycleBin(t.Context(), "", false)
	if err != nil || len(rows) != 1 || rows[0].Name != "new" {
		t.Fatal(rows, err)
	}
	for _, e := range []error{c.Restore(t.Context(), []domain.FileID{1, 1}), c.DeleteForever(t.Context(), []domain.FileID{1, 1}),
		c.EmptyRecycleBin(t.Context()), c.Restore(t.Context(), nil), c.DeleteForever(t.Context(), nil)} {
		if e != nil {
			t.Fatal(e)
		}
	}
	for _, r := range tr.Calls()[2:4] {
		if string(r.Body) != `{"idList":["1"]}` {
			t.Fatal(string(r.Body))
		}
	}
}

func TestRefusalAndReadRetries(t *testing.T) {
	for _, code := range []string{"E0712", "E0001"} {
		c, _, s := client(t, porttest.Refused(code))
		_, err := c.List(t.Context(), 0, "", true)
		if err == nil {
			t.Fatal("accepted refusal")
		}
		if code == "E0712" {
			if _, err = s.AccessToken(nil); !errors.Is(err, domain.ErrNotLoggedIn) {
				t.Fatal(err)
			}
		}
	}
	c, tr, _ := client(t, porttest.Step{Response: port.Response{Status: 503}}, porttest.OK(`"userFileVOList":[]`),
		porttest.Step{Response: port.Response{Status: 503}}, porttest.Step{Response: port.Response{Status: 429}}, porttest.OK(""))
	if _, err := c.List(t.Context(), 0, "", true); err != nil {
		t.Fatal(err)
	}
	if err := c.Rename(t.Context(), 1, "x"); err == nil {
		t.Fatal("change retried")
	}
	if err := c.Rename(t.Context(), 1, "x"); err != nil {
		t.Fatal(err)
	}
	if len(tr.Calls()) != 5 {
		t.Fatal(tr.Calls())
	}
}

func TestBrowsingFailures(t *testing.T) {
	calls := []func(*Client) error{
		func(c *Client) error { _, e := c.List(t.Context(), 0, "", true); return e },
		func(c *Client) error { _, e := c.Entry(t.Context(), "x", 0); return e },
		func(c *Client) error { _, e := c.Location(t.Context(), 1); return e },
		func(c *Client) error { _, e := c.CreateFolder(t.Context(), "x", 0); return e },
	}
	for _, call := range calls {
		c, _, _ := client(t, porttest.Refused("E0311"))
		if call(c) == nil {
			t.Fatal("accepted failure")
		}
	}
	c, _, _ := client(t, porttest.OK(""), porttest.OK(`"userFileVOList":[]`))
	if _, e := c.CreateFolder(t.Context(), "x", 0); !errors.Is(e, domain.ErrMalformedResponse) {
		t.Fatal(e)
	}
	c, _, _ = client(t, porttest.OK(""), porttest.Refused("E0311"))
	if _, e := c.CreateFolder(t.Context(), "x", 0); e == nil {
		t.Fatal("ignored list failure")
	}
}

func TestDownloadsAndUploads(t *testing.T) {
	data := []byte("hello")
	file, err := transfers.Scratch(data)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(file)
	c, tr, _ := client(t, porttest.OK(`"url":"https://storage.invalid/file","md5":"5d41402abc4b2a76b9719d911017c592"`),
		porttest.Step{Response: port.Response{Status: 200, Body: data}},
		porttest.OK(`"url":"https://storage.invalid/stored","s3Authorization":"signature","xamzDate":"date"`),
		porttest.Step{Response: port.Response{Status: 200}}, porttest.OK(""),
		porttest.OK(`"fullUploadUrl":"http://unreachable.invalid/api/oss/upload","innerName":"stored"`), porttest.OK(`"md5":null`), porttest.OK(""))
	size := int64(5)
	downloaded, e := c.Download(t.Context(), 1, &size, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(downloaded)
	if e = c.Upload(t.Context(), file, "hello", 0, nil); e != nil {
		t.Fatal(e)
	}
	if e = c.UploadBytes(t.Context(), data, "hello", 0); e != nil {
		t.Fatal(e)
	}
	if tr.Calls()[1].Header.Get(domain.AccessTokenHeader) != "" || tr.Calls()[6].URL != "https://synthetic.invalid/api/oss/upload" {
		t.Fatal(tr.Calls())
	}
}

func TestUploadFailuresAndGeneration(t *testing.T) {
	file, e := transfers.Scratch([]byte("x"))
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(file)
	c, _, _ := client(t)
	if c.Upload(t.Context(), "/absent/file", "x", 0, nil) == nil {
		t.Fatal("missing source accepted")
	}
	for _, steps := range [][]porttest.Step{
		{porttest.Refused("E0311")},
		{porttest.OK(`"apiServer":"0"`)},
	} {
		c, _, _ = client(t, steps...)
		if c.Upload(t.Context(), file, "x", 0, nil) == nil {
			t.Fatal("failure accepted")
		}
	}
	c, tr, s := client(t)
	tr.Steps = []porttest.Step{{Run: func(context.Context, port.Request) (port.Response, error) {
		if err := s.Forget(); err != nil {
			t.Fatal(err)
		}
		return porttest.OK(`"url":"https://storage.invalid/object","s3Authorization":"signature"`).Response, nil
	}}}
	if e = c.Upload(t.Context(), file, "x", 0, nil); !errors.Is(e, domain.ErrNotLoggedIn) {
		t.Fatal(e)
	}
}

func TestConversions(t *testing.T) {
	c, _, _ := client(t, porttest.OK(`"url":"https://storage.invalid/pdf"`), porttest.Step{Response: port.Response{Status: 200, Body: []byte("pdf")}},
		porttest.Refused("E0316"), porttest.OK(`"pngPageVOList":[{"pageNo":2,"url":"https://storage.invalid/2"},{"pageNo":1,"url":"https://storage.invalid/1"}]`),
		porttest.Step{Response: port.Response{Status: 200, Body: []byte("\x89PNG\r\n\x1a\n1")}}, porttest.Step{Response: port.Response{Status: 200, Body: []byte("\x89PNG\r\n\x1a\n2")}})
	f, e := c.PDF(t.Context(), 1, nil, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(f)
	var progress []int64
	files, e := c.PNGs(t.Context(), 1, func(n, _ int64) { progress = append(progress, n) })
	if e != nil || len(files) != 2 || len(progress) != 2 || progress[1] != 2000 {
		t.Fatal(files, progress, e)
	}
	for i, f := range files {
		b, _ := os.ReadFile(f)
		if string(b[len(b)-1:]) != fmt.Sprint(i+1) {
			t.Fatal(string(b))
		}
		os.Remove(f)
	}
}

func TestConversionFailures(t *testing.T) {
	for _, steps := range [][]porttest.Step{
		{porttest.OK(`"pngPageVOList":[]`)},
		{porttest.Refused("E0315")},
		{porttest.OK(`"pngPageVOList":[{"url":"https://storage.invalid/a"}]`), {Response: port.Response{Status: 200, Body: []byte("not png")}}},
		{porttest.OK(`"pngPageVOList":[{"url":"https://storage.invalid/a"}]`), {Err: context.Canceled}},
	} {
		c, _, _ := client(t, steps...)
		if _, e := c.PNGs(t.Context(), 1, nil); e == nil {
			t.Fatal("accepted invalid export")
		}
	}
	c, _, _ := client(t, porttest.Refused("E0315"))
	if _, e := c.PDF(t.Context(), 1, nil, nil); e == nil {
		t.Fatal("accepted failed PDF")
	}
	if checkPNG("/absent/png") == nil {
		t.Fatal("accepted missing PNG")
	}
	synctest.Test(t, func(t *testing.T) {
		steps := make([]porttest.Step, 61)
		for i := range steps {
			steps[i] = porttest.Refused("E0316")
		}
		c, _, _ := client(t, steps...)
		c.clock = port.SystemClock{}
		if _, e := c.PNGs(t.Context(), 1, nil); e == nil {
			t.Fatal("endless conversion accepted")
		}
	})
	c, _, _ = client(t, porttest.Refused("E0316"))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e := c.PNGs(ctx, 1, nil); e == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestRefreshesStaleXSRFToken(t *testing.T) {
	for _, refusedAgain := range []bool{false, true} {
		c, tr, _ := client(t)
		c.channel = channel.New(domain.Official(), tr, &porttest.Clock{}, domain.DefaultRetryPolicy())
		tr.Steps = []porttest.Step{
			{Response: port.Response{Status: 200, Header: port.Header{domain.XSRFHeader: {"stale"}}, Cookies: []port.Cookie{{Name: domain.XSRFCookie, Value: "stale"}}}},
			{Response: port.Response{Status: 403}},
			{Response: port.Response{Status: 200, Header: port.Header{domain.XSRFHeader: {"fresh"}}, Cookies: []port.Cookie{{Name: domain.XSRFCookie, Value: "fresh"}}}},
			porttest.OK(`"usedCapacity":1,"totalCapacity":2`),
		}
		if refusedAgain {
			tr.Steps[3] = porttest.Step{Response: port.Response{Status: 403}}
		}
		_, err := c.Capacity(t.Context())
		if refusedAgain && !errors.Is(err, domain.ErrHTTP) || !refusedAgain && err != nil {
			t.Fatal(err)
		}
		calls := tr.Calls()
		if len(calls) != 4 || calls[1].Header.Get(domain.XSRFHeader) != "stale" || calls[3].Header.Get(domain.XSRFHeader) != "fresh" {
			t.Fatal(calls)
		}
	}
}

func TestStorageRefusalKeepsLogin(t *testing.T) {
	c, tr, s := client(t, porttest.OK(`"url":"https://storage.invalid/file"`), porttest.Step{Response: port.Response{Status: 401}})
	if _, e := c.Download(t.Context(), 1, nil, nil); !errors.Is(e, domain.ErrHTTP) {
		t.Fatal(e)
	}
	if _, e := s.AccessToken(nil); e != nil {
		t.Fatal("storage dropped login", e)
	}
	if len(tr.Calls()) != 2 {
		t.Fatal("storage refusal retried")
	}
}

func TestPNGFailureRemovesEarlierPages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TMPDIR", dir)
	c, tr, _ := client(t, porttest.OK(`"pngPageVOList":[{"pageNo":1,"url":"https://storage.invalid/1"},{"pageNo":2,"url":"https://storage.invalid/2"}]`),
		porttest.Step{Response: port.Response{Status: 200, Body: []byte("\x89PNG\r\n\x1a\n")}},
		porttest.Step{Response: port.Response{Status: 200, Body: []byte("<Error/>")}})
	if files, e := c.PNGs(t.Context(), 1, nil); e == nil || len(files) != 0 {
		t.Fatal(files, e)
	}
	if !tr.Calls()[0].LongRunning {
		t.Fatal("conversion lost processing budget")
	}
	files, e := os.ReadDir(dir)
	if e != nil || len(files) != 0 {
		t.Fatal(files, e)
	}
}

func TestRecycleRefusalsAndFolderExclusions(t *testing.T) {
	c, tr, _ := client(t, porttest.Refused("E0311"), porttest.Refused("E0301"), porttest.OK(`"folderVOList":[]`))
	if e := c.Restore(t.Context(), []domain.FileID{1}); !errors.Is(e, domain.ErrNotFound) {
		t.Fatal(e)
	}
	if e := c.Restore(t.Context(), []domain.FileID{1}); !errors.Is(e, domain.ErrSyncLocked) {
		t.Fatal(e)
	}
	if _, e := c.Folders(t.Context(), 1, []domain.FileID{2}); e != nil {
		t.Fatal(e)
	}
	if string(tr.Calls()[2].Body) != `{"directoryId":"1","idList":["2"]}` {
		t.Fatal(string(tr.Calls()[2].Body))
	}
}

func TestMissingDownloadLinksFailBeforeStorage(t *testing.T) {
	for _, fields := range []string{"", `"url":null`, `"url":""`} {
		for _, method := range []string{"download", "pdf", "png"} {
			payload := fields
			if method == "png" {
				payload = `"pngPageVOList":[{` + fields + `}]`
			}
			c, tr, _ := client(t, porttest.OK(payload))
			var err error
			switch method {
			case "download":
				_, err = c.Download(t.Context(), 1, nil, nil)
			case "pdf":
				_, err = c.PDF(t.Context(), 1, nil, nil)
			case "png":
				_, err = c.PNGs(t.Context(), 1, nil)
			}
			if !errors.Is(err, domain.ErrMalformedResponse) || len(tr.Calls()) != 1 {
				t.Errorf("%s %s: %v", method, fields, err)
			}
		}
	}
}
