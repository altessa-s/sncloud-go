//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/clouddouble"
	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

func TestFilesBothCloudFlavors(t *testing.T) {
	for _, flavor := range []clouddouble.Flavor{clouddouble.Official, clouddouble.Private} {
		t.Run(string(flavor), func(t *testing.T) {
			cloud := clouddouble.New(flavor)
			defer cloud.Close()
			state := cloud.EnableFiles()
			note := state.Seed(0, "Note", "NOTE/Note", true, nil)
			seeded := make(map[sncloud.FileID]string)
			for i := range 101 {
				id := state.Seed(note, fmt.Sprintf("item-%03d", i), fmt.Sprintf("NOTE/Note/item-%03d", i), false, []byte("synthetic"))
				seeded[id] = fmt.Sprintf("item-%03d", i)
			}
			c, e := sncloud.New(&porttest.Secrets{}, sncloud.WithEndpoints(cloud.Endpoints()), sncloud.WithRoundTripper(http.DefaultTransport))
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			if _, e = c.LogIn(t.Context(), "test@example.invalid", "synthetic-password", nil); e != nil {
				t.Fatal(e)
			}
			rows, e := c.Files().List(t.Context(), note, sncloud.SortName, true)
			if e != nil || len(rows) != 101 {
				t.Fatal(len(rows), e)
			}
			checkSeededRows(t, rows, seeded)
			rows, e = c.Files().Search(t.Context(), "item-", sncloud.SortName, true)
			if e != nil || len(rows) != 101 {
				t.Fatal(len(rows), e)
			}
			checkSeededRows(t, rows, seeded)
			folder, e := c.Files().CreateFolder(t.Context(), "new", note)
			if e != nil {
				t.Fatal(e)
			}
			folders, e := c.Files().Folders(t.Context(), note, nil)
			if e != nil || len(folders) != 1 || folders[0].ID != folder.ID {
				t.Fatal(folders, e)
			}
			if e = c.Files().Rename(t.Context(), folder.ID, "renamed"); e != nil {
				t.Fatal(e)
			}
			if e = c.Files().Copy(t.Context(), []sncloud.FileID{rows[0].ID}, note, folder.ID); e != nil {
				t.Fatal(e)
			}
			if e = c.Files().Move(t.Context(), []sncloud.FileID{rows[1].ID}, note, folder.ID); e != nil {
				t.Fatal(e)
			}
			if e = c.Files().Delete(t.Context(), []sncloud.FileID{rows[0].ID}, note); e != nil {
				t.Fatal(e)
			}
			bin, e := c.Files().RecycleBin(t.Context(), sncloud.SortTime, false)
			if e != nil || len(bin) != 1 || bin[0].ID != rows[0].ID {
				t.Fatal(bin, e)
			}
			if e = c.Files().Restore(t.Context(), []sncloud.FileID{rows[0].ID, rows[0].ID}); e != nil {
				t.Fatal(e)
			}
			if e = c.Files().Delete(t.Context(), []sncloud.FileID{rows[0].ID}, note); e != nil {
				t.Fatal(e)
			}
			if e = c.Files().DeleteForever(t.Context(), []sncloud.FileID{rows[0].ID}); e != nil {
				t.Fatal(e)
			}
			if e = c.Files().EmptyRecycleBin(t.Context()); e != nil {
				t.Fatal(e)
			}
			if cap, e := c.Files().Capacity(t.Context()); e != nil || cap.Total != 1000000 || cap.Used != 42 {
				t.Fatal(cap, e)
			}
			if user, e := c.Files().User(t.Context()); e != nil || user.Name != "Synthetic" {
				t.Fatal(user, e)
			}
			if loc, e := c.Files().Location(t.Context(), note); e != nil || len(loc.IDs) != 2 || loc.IDs[0] != 1 || loc.IDs[1] != 9007199254740993 {
				t.Fatal(loc, e)
			}
			for _, size := range []int{5, 8*1024*1024 + 1} {
				data := bytes.Repeat([]byte("x"), size)
				name := fmt.Sprintf("uploaded-%d", size)
				if e = c.Files().UploadBytes(t.Context(), data, name, note); e != nil {
					t.Fatal(e)
				}
				row, e := c.Files().Entry(t.Context(), name, note)
				if e != nil || row == nil {
					t.Fatal(row, e)
				}
				file, e := c.Files().Download(t.Context(), row.ID, row.Size, nil)
				if e != nil {
					t.Fatal(e)
				}
				downloaded, e := os.ReadFile(file)
				os.Remove(file)
				if e != nil || !bytes.Equal(downloaded, data) {
					t.Fatal("web round trip", size, e)
				}
				device, e := c.Device().UploadBytes(t.Context(), data, "EXPORT/"+name)
				if e != nil {
					t.Fatal(e)
				}
				file, e = c.Device().Download(t.Context(), device.ID, nil, nil)
				if e != nil {
					t.Fatal(e)
				}
				downloaded, e = os.ReadFile(file)
				os.Remove(file)
				if e != nil || !bytes.Equal(downloaded, data) {
					t.Fatal("device round trip", size, e)
				}
				file, e = c.Device().Download(t.Context(), device.ID, &sncloud.ByteRange{First: 1, Last: 3}, nil)
				if e != nil {
					t.Fatal(e)
				}
				downloaded, e = os.ReadFile(file)
				os.Remove(file)
				if e != nil || string(downloaded) != "xxx" {
					t.Fatal(string(downloaded), e)
				}
			}
			pngs, e := c.Files().PNGs(t.Context(), rows[2].ID, nil)
			if e != nil || len(pngs) != 1 {
				t.Fatal(pngs, e)
			}
			for _, file := range pngs {
				data, err := os.ReadFile(file)
				if err != nil || string(data) != "\x89PNG\r\n\x1a\nsynthetic" {
					t.Fatalf("PNG = %q, %v", data, err)
				}
				os.Remove(file)
			}
			pdf, e := c.Files().PDF(t.Context(), rows[2].ID, nil, nil)
			if e != nil {
				t.Fatal(e)
			}
			pdfBytes, err := os.ReadFile(pdf)
			if err != nil || string(pdfBytes) != "synthetic" {
				t.Fatalf("PDF = %q, %v", pdfBytes, err)
			}
			os.Remove(pdf)
			if entries, e := c.Device().List(t.Context(), "/NOTE/Note", false); e != nil || !hasDeviceEntry(entries, rows[2].ID) {
				t.Fatal(entries, e)
			}
			if entries, e := c.Device().ListFolder(t.Context(), note, true); e != nil || !hasDeviceEntry(entries, rows[2].ID) {
				t.Fatal(entries, e)
			}
			if entry, e := c.Device().Entry(t.Context(), -1); e != nil || entry != nil {
				t.Fatal(entry, e)
			}
			if entry, e := c.Device().EntryAt(t.Context(), "NOTE/Note"); e != nil || entry == nil || entry.ID != note {
				t.Fatal(entry, e)
			}
			if usage, e := c.Device().SpaceUsage(t.Context()); e != nil || usage.Used != 42 || usage.Allocated == nil || *usage.Allocated != 1000000 {
				t.Fatal(usage, e)
			}
			device, e := c.Device().CreateFolder(t.Context(), "EXPORT/folder", false)
			if e != nil {
				t.Fatal(e)
			}
			moved, e := c.Device().Move(t.Context(), device.ID, "EXPORT/moved", false)
			if e != nil || moved == nil || moved.ID != device.ID || moved.Path != "EXPORT/moved" {
				t.Fatal(moved, e)
			}
			copied, e := c.Device().Copy(t.Context(), device.ID, "EXPORT/copy", false)
			if e != nil || copied == nil || copied.Path != "EXPORT/copy" {
				t.Fatal(copied, e)
			}
			if e = c.Device().Delete(t.Context(), copied.ID); e != nil {
				t.Fatal(e)
			}
			sentinel := errors.New("synthetic failure")
			ctx, cancel := context.WithCancel(t.Context())
			e = c.Device().WithSyncLock(ctx, func(context.Context, bool) error { cancel(); return sentinel })
			if !errors.Is(e, sentinel) {
				t.Fatal(e)
			}
			if e = c.Device().WithSyncLock(t.Context(), func(context.Context, bool) error { return nil }); e != nil {
				t.Fatal(e)
			}
		})
	}
}

func checkSeededRows(t *testing.T, rows []sncloud.Entry, seeded map[sncloud.FileID]string) {
	t.Helper()
	seen := make(map[sncloud.FileID]bool)
	for _, row := range rows {
		if name, ok := seeded[row.ID]; !ok || row.Name != name || seen[row.ID] {
			t.Fatalf("unexpected or repeated entry: %+v", row)
		}
		seen[row.ID] = true
	}
}

func hasDeviceEntry(entries []sncloud.DeviceEntry, id sncloud.FileID) bool {
	for _, entry := range entries {
		if entry.ID == id {
			return true
		}
	}
	return false
}
