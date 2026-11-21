// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

// Every file operation must use the client's session. An unauthenticated
// operation must never reach the network.
func TestFileViewsRequireSession(t *testing.T) {
	c, err := sncloud.New(&porttest.Secrets{}, sncloud.WithRoundTripper(facadeTransport(func(*http.Request) (*http.Response, error) {
		t.Error("unauthenticated request reached transport")
		return nil, errors.New("unexpected request")
	})))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := c.Close(); err != nil {
			t.Error(err)
		}
	})
	file := filepath.Join(t.TempDir(), "synthetic.txt")
	if err := os.WriteFile(file, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	f, d := c.Files(), c.Device()
	ctx := t.Context()
	ids := []sncloud.FileID{42}
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"web list", func() error { _, e := f.List(ctx, 42, sncloud.SortName, true); return e }},
		{"web search", func() error { _, e := f.Search(ctx, "note", sncloud.SortName, true); return e }},
		{"web entry", func() error { _, e := f.Entry(ctx, "note", 42); return e }},
		{"web folders", func() error { _, e := f.Folders(ctx, 42, ids); return e }},
		{"web capacity", func() error { _, e := f.Capacity(ctx); return e }},
		{"web user", func() error { _, e := f.User(ctx); return e }},
		{"web location", func() error { _, e := f.Location(ctx, 42); return e }},
		{"web download", func() error { _, e := f.Download(ctx, 42, nil, nil); return e }},
		{"web upload", func() error { return f.Upload(ctx, file, "note", 42, nil) }},
		{"web bytes", func() error { return f.UploadBytes(ctx, []byte("synthetic"), "note", 42) }},
		{"web create", func() error { _, e := f.CreateFolder(ctx, "folder", 42); return e }},
		{"web delete", func() error { return f.Delete(ctx, ids, 42) }},
		{"web move", func() error { return f.Move(ctx, ids, 42, 43) }},
		{"web copy", func() error { return f.Copy(ctx, ids, 42, 43) }},
		{"web rename", func() error { return f.Rename(ctx, 42, "renamed") }},
		{"web pdf", func() error { _, e := f.PDF(ctx, 42, nil, nil); return e }},
		{"web png", func() error { _, e := f.PNGs(ctx, 42, nil); return e }},
		{"web recycle", func() error { _, e := f.RecycleBin(ctx, sncloud.SortTime, false); return e }},
		{"web restore", func() error { return f.Restore(ctx, ids) }},
		{"web delete forever", func() error { return f.DeleteForever(ctx, ids) }},
		{"web empty", func() error { return f.EmptyRecycleBin(ctx) }},
		{"device list", func() error { _, e := d.List(ctx, "notes", true); return e }},
		{"device folder", func() error { _, e := d.ListFolder(ctx, 42, true); return e }},
		{"device entry", func() error { _, e := d.Entry(ctx, 42); return e }},
		{"device path", func() error { _, e := d.EntryAt(ctx, "notes"); return e }},
		{"device usage", func() error { _, e := d.SpaceUsage(ctx); return e }},
		{"device download", func() error { _, e := d.Download(ctx, 42, nil, nil); return e }},
		{"device upload", func() error { _, e := d.Upload(ctx, file, "notes/note", nil); return e }},
		{"device bytes", func() error { _, e := d.UploadBytes(ctx, []byte("synthetic"), "notes/note"); return e }},
		{"device create", func() error { _, e := d.CreateFolder(ctx, "folder", false); return e }},
		{"device delete", func() error { return d.Delete(ctx, 42) }},
		{"device move", func() error { _, e := d.Move(ctx, 42, "moved", false); return e }},
		{"device copy", func() error { _, e := d.Copy(ctx, 42, "copied", false); return e }},
		{"device lock", func() error {
			return d.WithSyncLock(ctx, func(context.Context, bool) error { t.Error("callback ran without session"); return nil })
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, sncloud.ErrNotLoggedIn) {
				t.Fatalf("want ErrNotLoggedIn, got %v", err)
			}
		})
	}
}
