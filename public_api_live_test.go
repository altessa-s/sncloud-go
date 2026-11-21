//go:build live

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

// TestPublicAPILive is intentionally sequential: mutations share one disposable
// namespace, allowing for the vendor's documented repeated-POST guard. No secrets
// are included in assertions; the built-in wire logger redacts credentials.
// SNCLOUD_LIVE_DESTRUCTIVE=1 enables account-wide deletion: disposable account only.
func TestPublicAPILive(t *testing.T) {
	host, account, password := os.Getenv("SNCLOUD_LIVE_HOST"), os.Getenv("SNCLOUD_LIVE_ACCOUNT"), os.Getenv("SNCLOUD_LIVE_PASSWORD")
	if host == "" || account == "" || password == "" {
		t.Skip("set SNCLOUD_LIVE_HOST, SNCLOUD_LIVE_ACCOUNT and SNCLOUD_LIVE_PASSWORD; optional SNCLOUD_LIVE_SOCKET")
	}
	t.Setenv("TMPDIR", t.TempDir())
	ep := sncloud.PrivateCloud(host, false)
	if socket := os.Getenv("SNCLOUD_LIVE_SOCKET"); socket != "" {
		ep.SocketHost = socket
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	newClient := func(store *porttest.Secrets) *sncloud.Client {
		c, err := sncloud.New(store, sncloud.WithEndpoints(ep), sncloud.WithLogger(logger))
		liveOK(t, err)
		t.Cleanup(func() { _ = c.Close() })
		return c
	}
	store := &porttest.Secrets{}
	c := newClient(store)
	ctx, cancel := context.WithTimeout(t.Context(), 25*time.Minute)
	defer cancel()
	logged := false
	run := func(name string, fn func(*testing.T)) {
		t.Run(name, func(t *testing.T) {
			if !logged {
				t.Skip("LogIn prerequisite failed")
			}
			fn(t)
		})
	}
	t.Run("Client/LogIn", func(t *testing.T) {
		out, err := c.LogIn(ctx, account, password, nil)
		liveOK(t, err)
		if out != sncloud.LoggedIn {
			t.Fatalf("outcome %v; SMTP verification unavailable", out)
		}
		logged = true
	})
	run("Client/NoXSRF", func(t *testing.T) {
		xsrf := ep
		xsrf.WebUsesXSRF = true
		probe, e := sncloud.New(store, sncloud.WithEndpoints(xsrf), sncloud.WithLogger(logger))
		liveOK(t, e)
		defer probe.Close()
		_, e = probe.Files().Capacity(ctx)
		var httpErr *sncloud.HTTPError
		if !errors.As(e, &httpErr) || httpErr.Status != 404 {
			t.Fatalf("XSRF probe: %v; expected HTTP 404", e)
		}
	})

	run("Client/Identity", func(t *testing.T) {
		a, e := c.Identity()
		liveOK(t, e)
		b, e := c.Identity()
		liveOK(t, e)
		if a != b {
			t.Fatal("identity changed")
		}
	})
	run("Client/IsLoggedIn", func(t *testing.T) {
		v, e := c.IsLoggedIn()
		liveOK(t, e)
		if !v {
			t.Fatal("not logged in")
		}
	})
	run("Client/Account", func(t *testing.T) {
		v, e := c.Account()
		liveOK(t, e)
		if v != account {
			t.Fatal("account mismatch")
		}
	})
	run("Client/Expiry", func(t *testing.T) { v, e := c.Expiry(); liveOK(t, e); t.Logf("expiry present: %v", v != nil) })
	run("Client/ReuseToken", func(t *testing.T) { second := newClient(store); _, e := second.Files().Capacity(ctx); liveOK(t, e) })
	run("Client/SendVerificationCode", func(t *testing.T) {
		if e := c.SendVerificationCode(ctx); !errors.Is(e, sncloud.ErrNoVerificationPending) {
			t.Fatalf("kind=%s; want noVerificationPending", sncloud.KindOf(e))
		}
	})
	run("Client/Verify", func(t *testing.T) {
		if e := c.Verify(ctx, "000000"); !errors.Is(e, sncloud.ErrNoVerificationPending) {
			t.Fatalf("kind=%s; want noVerificationPending", sncloud.KindOf(e))
		}
	})
	peerStore := &porttest.Secrets{}
	peer := newClient(peerStore)
	run("Client/Peer", func(t *testing.T) {
		token, e := store.Secret(sncloud.AccessToken)
		liveOK(t, e)
		liveOK(t, peerStore.Keep(token, sncloud.AccessToken))
		liveOK(t, peerStore.Keep(account, sncloud.Account))
		a, e := c.Identity()
		liveOK(t, e)
		b, e := peer.Identity()
		liveOK(t, e)
		if a == b {
			t.Fatal("peer identity equals primary")
		}
	})
	name := "live-" + sncloud.NewDigestIdentifier()
	f, d := c.Files(), c.Device()
	var root, parent sncloud.Entry
	createdIDs := make(map[sncloud.FileID]bool)
	// Cleanup uses an independent context because parent cancellation must not
	// strand uploaded data. Only this run's exact root or known IDs are removed.
	defer func() {
		if !logged {
			return
		}
		t.Run("Cleanup/Files", func(t *testing.T) {
			clean, stop := context.WithTimeout(context.Background(), 2*time.Minute)
			defer stop()
			if root.ID != 0 {
				liveOK(t, d.Delete(clean, root.ID))
			}

			bin, e := f.RecycleBin(clean, sncloud.SortTime, false)
			liveOK(t, e)
			for _, v := range bin {
				if createdIDs[v.ID] {
					liveOK(t, f.DeleteForever(clean, []sncloud.FileID{v.ID}))
				}
			}
			entry, e := f.Entry(clean, name, parent.ID)
			liveOK(t, e)
			if entry != nil {
				t.Fatal("run root remains")
			}
		})
		t.Run("Client/LogOut", func(t *testing.T) {
			clean, stop := context.WithTimeout(context.Background(), 30*time.Second)
			defer stop()
			liveOK(t, c.LogOut(clean))
			v, e := c.IsLoggedIn()
			liveOK(t, e)
			if v {
				t.Fatal("session remains")
			}
		})
		t.Run("Client/Forget", func(t *testing.T) {
			liveOK(t, peer.Forget())
			v, e := peer.IsLoggedIn()
			liveOK(t, e)
			if v {
				t.Fatal("session remains")
			}
		})
		t.Run("Client/Close", func(t *testing.T) { liveOK(t, c.Close()) })
	}()
	run("Client/Files", func(t *testing.T) {
		if f == nil {
			t.Fatal("nil view")
		}
	})
	run("Client/Device", func(t *testing.T) {
		if d == nil {
			t.Fatal("nil view")
		}
	})
	run("Client/Tasks", func(t *testing.T) {
		if c.Tasks() == nil {
			t.Fatal("nil view")
		}
	})
	run("Client/Digests", func(t *testing.T) {
		if c.Digests() == nil {
			t.Fatal("nil view")
		}
	})
	run("Client/Feed", func(t *testing.T) {
		if c.Feed() == nil {
			t.Fatal("nil view")
		}
	})
	run("Files/Capacity", func(t *testing.T) { v, e := f.Capacity(ctx); liveOK(t, e); t.Logf("used=%d total=%d", v.Used, v.Total) })
	run("Files/User", func(t *testing.T) { _, e := f.User(ctx); liveOK(t, e) })
	run("Files/Root", func(t *testing.T) {
		rows, e := f.List(ctx, sncloud.RootFileID, sncloud.SortName, true)
		liveOK(t, e)
		for _, row := range rows {
			if row.IsFolder && row.Name == "Note" {
				parent = row
				return
			}
		}
		t.Fatal("default Note storage folder unavailable")
	})
	run("Files/CreateFolder", func(t *testing.T) {
		liveID(t, parent.ID)
		var e error
		root, e = f.CreateFolder(ctx, name, parent.ID)
		liveOK(t, e)
		liveID(t, root.ID)
	})
	needRoot := func(t *testing.T) {
		t.Helper()
		if root.ID == 0 {
			t.Skip("CreateFolder prerequisite failed")
		}
	}

	run("Files/Folders", func(t *testing.T) {
		needRoot(t)
		rows, e := f.Folders(ctx, parent.ID, nil)
		liveOK(t, e)
		for _, v := range rows {
			if v.ID == root.ID {
				return
			}
		}
		t.Fatal("root missing")
	})
	small := []byte("sncloud live round-trip: 0123456789\n")
	large := bytes.Repeat([]byte("0123456789abcdef"), 524289)
	var webID sncloud.FileID
	for _, fixture := range []struct {
		name string
		data []byte
	}{{"Small", small}, {"Large", large}} {
		run("Files/Upload"+fixture.name, func(t *testing.T) {
			needRoot(t)
			file := liveFile(t, fixture.data)
			liveOK(t, f.Upload(ctx, file, name+fixture.name+".txt", root.ID, nil))
		})
		run("Files/Download"+fixture.name, func(t *testing.T) {
			needRoot(t)
			v, e := f.Entry(ctx, name+fixture.name+".txt", root.ID)
			liveOK(t, e)
			if v == nil {
				t.Skip("upload prerequisite failed")
			}
			file, e := f.Download(ctx, v.ID, nil, nil)
			liveOK(t, e)
			liveBytes(t, file, fixture.data)
		})
	}
	run("Files/UploadBytes", func(t *testing.T) { needRoot(t); liveOK(t, f.UploadBytes(ctx, small, name+"bytes.txt", root.ID)) })
	run("Files/Entry", func(t *testing.T) {
		needRoot(t)
		v, e := f.Entry(ctx, name+"bytes.txt", root.ID)
		liveOK(t, e)
		if v == nil {
			t.Fatal("uploaded entry missing")
		}
		webID = v.ID
		createdIDs[webID] = true
	})
	for _, order := range []sncloud.SortOrder{sncloud.SortName, sncloud.SortTime, sncloud.SortSize} {
		for _, asc := range []bool{true, false} {
			run(fmt.Sprintf("Files/List/%s/%t", order, asc), func(t *testing.T) { needRoot(t); _, e := f.List(ctx, root.ID, order, asc); liveOK(t, e) })
		}
	}
	run("Files/Search", func(t *testing.T) {
		needRoot(t)
		rows, e := f.Search(ctx, name, sncloud.SortName, true)
		liveOK(t, e)
		if len(rows) == 0 {
			t.Fatal("empty search")
		}
	})
	run("Files/Location", func(t *testing.T) { liveID(t, webID); _, e := f.Location(ctx, webID); liveOK(t, e) })
	run("Files/DownloadRange", func(t *testing.T) {
		t.Skip("Files.Download exposes a size ceiling, not a range; Device.DownloadRange covers ranges")
	})
	var dest sncloud.Entry
	run("Files/MoveDestination", func(t *testing.T) {
		needRoot(t)
		var e error
		dest, e = f.CreateFolder(ctx, "destination", root.ID)
		liveOK(t, e)
	})
	run("Files/Rename", func(t *testing.T) {
		liveID(t, webID)
		liveOK(t, f.Rename(ctx, webID, name+"renamed.txt"))
		row, e := f.Entry(ctx, name+"renamed.txt", root.ID)
		liveOK(t, e)
		if row == nil || row.ID != webID {
			t.Fatal("rename not persisted")
		}
	})
	run("Files/Copy", func(t *testing.T) {
		liveID(t, webID)
		liveID(t, dest.ID)
		liveOK(t, f.Copy(ctx, []sncloud.FileID{webID}, root.ID, dest.ID))
		row, e := f.Entry(ctx, name+"renamed.txt", dest.ID)
		liveOK(t, e)
		if row == nil || row.ID == webID {
			t.Fatal("copy missing or reused source ID")
		}
	})
	run("Files/Move", func(t *testing.T) {
		liveID(t, webID)
		liveID(t, dest.ID)
		liveOK(t, f.Rename(ctx, webID, name+"moved.txt"))
		liveOK(t, f.Move(ctx, []sncloud.FileID{webID}, root.ID, dest.ID))
		row, e := f.Entry(ctx, name+"moved.txt", dest.ID)
		liveOK(t, e)
		if row == nil || row.ID != webID {
			t.Fatal("move not persisted")
		}
	})
	deleted := false
	run("Files/Delete", func(t *testing.T) {
		liveID(t, webID)
		liveID(t, dest.ID)
		liveOK(t, f.Delete(ctx, []sncloud.FileID{webID}, dest.ID))
		deleted = true
	})
	run("Files/RecycleBin", func(t *testing.T) {
		rows, e := f.RecycleBin(ctx, sncloud.SortTime, false)
		liveOK(t, e)
		if !deleted {
			t.Skip("Delete prerequisite failed")
		}
		for _, v := range rows {
			if v.ID == webID {
				return
			}
		}
		t.Fatal("deleted file missing")
	})
	run("Files/Restore", func(t *testing.T) {
		if !deleted {
			t.Skip("Delete prerequisite failed")
		}
		liveOK(t, f.Restore(ctx, []sncloud.FileID{webID}))
		deleted = false
		row, e := f.Entry(ctx, name+"moved.txt", dest.ID)
		liveOK(t, e)
		if row == nil || row.ID != webID {
			t.Fatal("restore not persisted")
		}
	})
	run("Files/DeleteForever", func(t *testing.T) {
		liveID(t, webID)
		if !deleted {
			liveOK(t, f.Delete(ctx, []sncloud.FileID{webID}, dest.ID))
		}
		liveOK(t, f.DeleteForever(ctx, []sncloud.FileID{webID}))
	})
	run("Files/EmptyRecycleBin", func(t *testing.T) {
		if os.Getenv("SNCLOUD_LIVE_DESTRUCTIVE") != "1" {
			t.Skip("set SNCLOUD_LIVE_DESTRUCTIVE=1: disposable account only; empties the entire account recycle bin")
		}
		needRoot(t)
		sacrificial, e := f.CreateFolder(ctx, name+"-empty", root.ID)
		liveOK(t, e)
		createdIDs[sacrificial.ID] = true
		liveOK(t, f.Delete(ctx, []sncloud.FileID{sacrificial.ID}, root.ID))
		liveOK(t, f.EmptyRecycleBin(ctx))
		rows, e := f.RecycleBin(ctx, sncloud.SortTime, false)
		liveOK(t, e)
		if len(rows) != 0 {
			t.Fatal("bin not empty")
		}
	})
	for _, op := range []string{"PDF", "PNGs"} {
		run("Files/"+op, func(t *testing.T) {
			t.Skip("no valid synthetic .note fixture or documented writer; arbitrary bytes do not exercise note conversion")
		})
	}
	liveDevice(t, ctx, c, peer, root, name, small, large, run)
	liveTasks(t, ctx, c, name, run)
	liveDigests(t, ctx, c, name, run)
	liveFeed(t, ctx, c, peer, root, name, run)
	run("Client/WrongPassword", func(t *testing.T) {
		if os.Getenv("SNCLOUD_LIVE_DESTRUCTIVE") != "1" {
			t.Skip("SNCLOUD_LIVE_DESTRUCTIVE=1: disposable account only; changes account-wide authentication or sync state")
		}
		bad := newClient(&porttest.Secrets{})
		_, e := bad.LogIn(ctx, account, password+"-deliberately-wrong", nil)
		if !errors.Is(e, sncloud.ErrInvalidCredentials) {
			t.Fatalf("kind=%s; want invalidCredentials", sncloud.KindOf(e))
		}
	})
}

func liveOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func liveID(t *testing.T, id sncloud.FileID) {
	t.Helper()
	if id == 0 {
		t.Skip("entry prerequisite unavailable")
	}
}
func liveFile(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "payload.txt")
	liveOK(t, os.WriteFile(p, data, 0o600))
	return p
}
func liveBytes(t *testing.T, file string, want []byte) {
	t.Helper()
	defer os.Remove(file)
	got, e := os.ReadFile(file)
	liveOK(t, e)
	if !bytes.Equal(got, want) || md5.Sum(got) != md5.Sum(want) {
		t.Fatalf("download mismatch: got %d want %d bytes", len(got), len(want))
	}
	t.Logf("bytes=%d MD5=%x", len(got), md5.Sum(got))
}

type liveRun func(string, func(*testing.T))

func liveDevice(t *testing.T, ctx context.Context, c, peer *sncloud.Client, root sncloud.Entry, name string, small, large []byte, run liveRun) {
	t.Helper()
	d := c.Device()
	base := "NOTE/Note/" + name
	var folder sncloud.DeviceEntry
	run("Device/CreateFolder", func(t *testing.T) {
		liveID(t, root.ID)
		var e error
		folder, e = d.CreateFolder(ctx, base+"/device", false)
		liveOK(t, e)
	})
	run("Device/List", func(t *testing.T) { liveID(t, root.ID); _, e := d.List(ctx, base, true); liveOK(t, e) })
	run("Device/ListFolder", func(t *testing.T) { liveID(t, root.ID); _, e := d.ListFolder(ctx, root.ID, true); liveOK(t, e) })
	run("Device/Entry", func(t *testing.T) {
		liveID(t, folder.ID)
		v, e := d.Entry(ctx, folder.ID)
		liveOK(t, e)
		if v == nil || v.ID != folder.ID {
			t.Fatal("entry mismatch")
		}
	})
	run("Device/EntryAt", func(t *testing.T) {
		liveID(t, folder.ID)
		v, e := d.EntryAt(ctx, base+"/device")
		liveOK(t, e)
		if v == nil || v.ID != folder.ID {
			t.Fatal("entry mismatch")
		}
	})
	run("Device/SpaceUsage", func(t *testing.T) { _, e := d.SpaceUsage(ctx); liveOK(t, e) })
	for _, fixture := range []struct {
		name string
		data []byte
	}{{"Small", small}, {"Large", large}} {
		var id sncloud.FileID
		run("Device/Upload"+fixture.name, func(t *testing.T) {
			liveID(t, folder.ID)
			v, e := d.Upload(ctx, liveFile(t, fixture.data), base+"/device/"+fixture.name+".txt", nil)
			liveOK(t, e)
			id = v.ID
		})
		run("Device/Download"+fixture.name, func(t *testing.T) {
			liveID(t, id)
			p, e := d.Download(ctx, id, nil, nil)
			liveOK(t, e)
			liveBytes(t, p, fixture.data)
		})
		run("Device/DownloadRange"+fixture.name, func(t *testing.T) {
			liveID(t, id)
			p, e := d.Download(ctx, id, &sncloud.ByteRange{First: 2, Last: 10}, nil)
			liveOK(t, e)
			liveBytes(t, p, fixture.data[2:11])
		})
	}
	var id sncloud.FileID
	run("Device/UploadBytes", func(t *testing.T) {
		liveID(t, folder.ID)
		v, e := d.UploadBytes(ctx, small, base+"/device/bytes.txt")
		liveOK(t, e)
		id = v.ID
	})
	run("Device/Move", func(t *testing.T) {
		liveID(t, id)
		v, e := d.Move(ctx, id, base+"/device/moved.txt", false)
		liveOK(t, e)
		if v == nil {
			t.Fatal("nil moved entry")
		}
	})
	run("Device/Copy", func(t *testing.T) {
		liveID(t, id)
		v, e := d.Copy(ctx, id, base+"/device/copied.txt", false)
		liveOK(t, e)
		if v == nil {
			t.Fatal("nil copied entry")
		}
	})
	run("Device/Delete", func(t *testing.T) {
		liveID(t, id)
		liveOK(t, d.Delete(ctx, id))
		v, e := d.Entry(ctx, id)
		liveOK(t, e)
		if v != nil {
			t.Fatal("deleted entry remains")
		}
	})
	run("Device/WithSyncLock", func(t *testing.T) {
		if os.Getenv("SNCLOUD_LIVE_DESTRUCTIVE") != "1" {
			t.Skip("SNCLOUD_LIVE_DESTRUCTIVE=1: disposable account only; changes account-wide authentication or sync state")
		}
		liveOK(t, d.WithSyncLock(ctx, func(inner context.Context, _ bool) error {
			e := peer.Device().WithSyncLock(inner, func(context.Context, bool) error { return errors.New("second claim unexpectedly acquired") })
			if !errors.Is(e, sncloud.ErrSyncLocked) {
				return fmt.Errorf("second claim: %w", e)
			}
			return nil
		}))
	})
}

func liveTasks(t *testing.T, ctx context.Context, c *sncloud.Client, name string, run liveRun) {
	t.Helper()
	v := c.Tasks()
	var list sncloud.TaskList
	var task sncloud.Task
	defer func() {
		if list.ID != "" {
			run("Cleanup/Tasks", func(t *testing.T) {
				clean, stop := context.WithTimeout(context.Background(), time.Minute)
				defer stop()
				liveOK(t, v.DeleteList(clean, list.ID))
			})
		}
	}()
	run("Tasks/Lists", func(t *testing.T) { _, e := v.Lists(ctx); liveOK(t, e) })
	run("Tasks/CreateList", func(t *testing.T) {
		time.Sleep(1100 * time.Millisecond)
		var e error
		list, e = v.CreateList(ctx, name, strings.TrimPrefix(name, "live-"))
		liveOK(t, e)
	})
	need := func(t *testing.T) {
		t.Helper()
		if list.ID == "" {
			t.Skip("CreateList prerequisite failed")
		}
	}
	run("Tasks/List", func(t *testing.T) {
		need(t)
		row, e := v.List(ctx, list.ID)
		liveOK(t, e)
		if row.Title != name {
			t.Fatal("title mismatch")
		}
	})
	run("Tasks/RenameList", func(t *testing.T) {
		need(t)
		liveOK(t, v.RenameList(ctx, list.ID, name+"-renamed"))
		row, e := v.List(ctx, list.ID)
		liveOK(t, e)
		if row.Title != name+"-renamed" {
			t.Fatal("rename not persisted")
		}
	})
	run("Tasks/Create", func(t *testing.T) {
		need(t)
		time.Sleep(1100 * time.Millisecond)
		var e error
		task, e = v.Create(ctx, sncloud.Task{Title: name, ListID: &list.ID})
		liveOK(t, e)
	})
	run("Tasks/Update", func(t *testing.T) {
		if task.ID == "" {
			t.Skip("Create prerequisite failed")
		}
		rows, e := v.InList(ctx, list.ID)
		liveOK(t, e)
		if len(rows) != 1 {
			t.Fatal("task missing")
		}
		task = rows[0]
		task.Title = name + "-updated"
		task, e = v.Update(ctx, task)
		liveOK(t, e)
	})
	run("Tasks/Complete", func(t *testing.T) {
		if task.ID == "" {
			t.Skip("Create prerequisite failed")
		}
		task.IsCompleted = true
		now := time.Now()
		task.Completed = &now
		_, e := v.Update(ctx, task)
		liveOK(t, e)
	})
	run("Tasks/InList", func(t *testing.T) {
		need(t)
		rows, e := v.InList(ctx, list.ID)
		liveOK(t, e)
		if len(rows) != 1 || !rows[0].IsCompleted || rows[0].Title != name+"-updated" {
			t.Fatal("updated/completed task not persisted")
		}
	})
	run("Tasks/All", func(t *testing.T) {
		rows, e := v.All(ctx)
		liveOK(t, e)
		if task.ID == "" {
			return
		}
		for _, row := range rows {
			if row.ID == task.ID {
				return
			}
		}
		t.Fatal("task missing")
	})
	run("Tasks/Paging", func(t *testing.T) {
		need(t)
		if task.ID == "" {
			t.Skip("Create prerequisite failed")
		}
		for i := range 501 {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-time.After(1100 * time.Millisecond):
			}
			_, e := v.Create(ctx, sncloud.Task{Title: fmt.Sprintf("%s-%03d", name, i), ListID: &list.ID})
			liveOK(t, e)
		}
		rows, e := v.InList(ctx, list.ID)
		liveOK(t, e)
		if len(rows) != 502 {
			t.Fatalf("got %d tasks; want 502 across 500-row pages", len(rows))
		}
		seen := map[string]bool{}
		for _, row := range rows {
			if seen[row.ID] {
				t.Fatal("duplicate task across pages")
			}
			seen[row.ID] = true
		}
	})
	run("Tasks/Delete", func(t *testing.T) {
		if task.ID == "" {
			t.Skip("Create prerequisite failed")
		}
		liveOK(t, v.Delete(ctx, task.ID))
	})
	run("Tasks/DeleteList", func(t *testing.T) {
		need(t)
		liveOK(t, v.DeleteList(ctx, list.ID))
		deletedID := list.ID
		list.ID = ""
		rows, e := v.Lists(ctx)
		liveOK(t, e)
		for _, row := range rows {
			if row.ID == deletedID {
				t.Fatal("deleted list remains")
			}
		}

	})
}

func liveDigests(t *testing.T, ctx context.Context, c *sncloud.Client, name string, run liveRun) {
	t.Helper()
	v := c.Digests()
	uid := sncloud.NewDigestIdentifier()
	var group, tag, id sncloud.DigestID
	defer func() {
		run("Cleanup/Digests", func(t *testing.T) {
			clean, stop := context.WithTimeout(context.Background(), time.Minute)
			defer stop()
			if id != 0 {
				liveOK(t, v.Delete(clean, id))
			}
			if tag != 0 {
				liveOK(t, v.DeleteTag(clean, tag))
			}
			if group != 0 {
				liveOK(t, v.DeleteGroup(clean, group))
			}
		})
	}()
	run("Digests/CreateGroup", func(t *testing.T) {
		time.Sleep(1100 * time.Millisecond)
		var e error
		group, e = v.CreateGroup(ctx, name, nil, uid)
		liveOK(t, e)
	})
	run("Digests/Groups", func(t *testing.T) {
		rows, e := v.Groups(ctx)
		liveOK(t, e)
		if group == 0 {
			return
		}
		for _, row := range rows {
			if row.ID == group {
				return
			}
		}
		t.Fatal("group missing")
	})
	run("Digests/UpdateGroup", func(t *testing.T) {
		if group == 0 {
			t.Skip("CreateGroup prerequisite failed")
		}
		liveOK(t, v.UpdateGroup(ctx, group, name+"-updated", nil))
	})
	run("Digests/CreateTag", func(t *testing.T) { var e error; tag, e = v.CreateTag(ctx, name); liveOK(t, e) })
	run("Digests/RenameTag", func(t *testing.T) {
		if tag == 0 {
			t.Skip("CreateTag prerequisite failed")
		}
		liveOK(t, v.RenameTag(ctx, tag, name+"-tag"))
	})
	run("Digests/Tags", func(t *testing.T) { _, e := v.Tags(ctx); liveOK(t, e) })
	run("Digests/Create", func(t *testing.T) {
		if group == 0 {
			t.Skip("CreateGroup prerequisite failed")
		}
		tags := []string{name + "-tag"}
		var e error
		id, e = v.Create(ctx, sncloud.DigestDraft{Content: &name, GroupIdentifier: &uid, Tags: &tags})
		liveOK(t, e)
	})
	need := func(t *testing.T) {
		t.Helper()
		if id == 0 {
			t.Skip("Create prerequisite failed")
		}
	}
	run("Digests/ByID", func(t *testing.T) {
		need(t)
		rows, e := v.ByID(ctx, []sncloud.DigestID{id})
		liveOK(t, e)
		if len(rows) != 1 || rows[0].ID != id {
			t.Fatal("digest missing")
		}
	})
	run("Digests/List", func(t *testing.T) {
		need(t)
		rows, e := v.List(ctx, uid)
		liveOK(t, e)
		if len(rows) != 1 {
			t.Fatal("digest missing")
		}
	})
	run("Digests/Fingerprints", func(t *testing.T) {
		need(t)
		rows, e := v.Fingerprints(ctx, uid)
		liveOK(t, e)
		if len(rows) != 1 {
			t.Fatal("fingerprint missing")
		}
	})
	payload := []byte("synthetic opaque handwriting payload\n")
	var hw sncloud.DigestHandwriting
	run("Digests/UploadHandwriting", func(t *testing.T) {
		need(t)
		var e error
		hw, e = v.UploadHandwriting(ctx, liveFile(t, payload), name+".mark", nil)
		liveOK(t, e)
		if hw.MD5 != fmt.Sprintf("%x", md5.Sum(payload)) {
			t.Fatal("handwriting checksum mismatch")
		}
	})
	run("Digests/Update", func(t *testing.T) {
		need(t)
		draft := sncloud.DigestDraft{Comment: &name}
		if hw.InnerName != "" {
			draft.Handwriting = &hw
		}
		liveOK(t, v.Update(ctx, id, draft))
	})
	run("Digests/DownloadHandwriting", func(t *testing.T) {
		need(t)
		if hw.InnerName == "" {
			t.Skip("UploadHandwriting prerequisite failed")
		}
		rows, e := v.ByID(ctx, []sncloud.DigestID{id})
		liveOK(t, e)
		if len(rows) != 1 {
			t.Fatal("digest missing")
		}
		p, e := v.DownloadHandwriting(ctx, rows[0], nil)
		liveOK(t, e)
		liveBytes(t, p, payload)
	})
	run("Digests/Delete", func(t *testing.T) {
		need(t)
		liveOK(t, v.Delete(ctx, id))
		id = 0
		rows, e := v.List(ctx, uid)
		liveOK(t, e)
		if len(rows) != 0 {
			t.Fatal("deleted digest remains")
		}
	})
	run("Digests/DeleteTag", func(t *testing.T) {
		if tag == 0 {
			t.Skip("CreateTag prerequisite failed")
		}
		liveOK(t, v.DeleteTag(ctx, tag))
		tag = 0
	})
	run("Digests/DeleteGroup", func(t *testing.T) {
		if group == 0 {
			t.Skip("CreateGroup prerequisite failed")
		}
		liveOK(t, v.DeleteGroup(ctx, group))
		group = 0
	})
}

func liveFeed(t *testing.T, ctx context.Context, c, peer *sncloud.Client, root sncloud.Entry, name string, run liveRun) {
	t.Helper()
	for _, mode := range []string{"Watch", "Changes"} {
		run("Feed/"+mode, func(t *testing.T) {
			liveID(t, root.ID)
			feedCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			type result struct {
				change sncloud.Change
				err    error
			}
			events := make(chan result, 32)
			done := make(chan struct{})
			seq := c.Feed().Watch(feedCtx)
			if mode == "Changes" {
				seq = c.Feed().Changes(feedCtx)
			}
			go func() {
				defer close(done)
				for change, e := range seq {
					select {
					case events <- result{change, e}:
					case <-feedCtx.Done():
						return
					}
				}
			}()
			defer func() {
				t.Run("Cancel", func(t *testing.T) {
					cancel()
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Fatal("stream did not stop within 5s")
					}
				})
			}()
			next := func() result {
				select {
				case event := <-events:
					return event
				case <-done:
					t.Fatal("stream ended before event")
				case <-time.After(20 * time.Second):
					t.Fatal("no feed event within 20s")
				}
				return result{}
			}
			first := next()
			liveOK(t, first.err)
			if first.change.Type != sncloud.ChangeConnected {
				t.Fatal("first event is not ChangeConnected")
			}
			if mode == "Changes" {
				return
			}
			time.Sleep(time.Second)
			t.Run("Files", func(t *testing.T) { liveOK(t, peer.Files().UploadBytes(ctx, []byte("feed"), name+"feed.txt", root.ID)) })
			var list sncloud.TaskList
			var changedTask sncloud.Task
			t.Run("Tasks", func(t *testing.T) {
				time.Sleep(1100 * time.Millisecond)
				var e error
				list, e = peer.Tasks().CreateList(ctx, name+"-feed", "")
				liveOK(t, e)
				changedTask, e = peer.Tasks().Create(ctx, sncloud.Task{Title: name, ListID: &list.ID})
				liveOK(t, e)
			})
			defer func() {
				if list.ID != "" {
					clean, stop := context.WithTimeout(context.Background(), time.Minute)
					defer stop()
					if e := peer.Tasks().DeleteList(clean, list.ID); e != nil {
						t.Error(e)
					}
				}
			}()
			var id sncloud.DigestID
			t.Run("Digests", func(t *testing.T) {
				var e error
				id, e = peer.Digests().Create(ctx, sncloud.DigestDraft{Content: &name})
				liveOK(t, e)
			})
			defer func() {
				if id != 0 {
					clean, stop := context.WithTimeout(context.Background(), time.Minute)
					defer stop()
					if e := peer.Digests().Delete(clean, id); e != nil {
						t.Error(e)
					}
				}
			}()
			seen := map[sncloud.ChangeType]bool{}
			timer := time.NewTimer(20 * time.Second)
			defer timer.Stop()
		collect:
			for len(seen) < 3 {
				select {
				case event := <-events:
					liveOK(t, event.err)
					t.Logf("feed batch: type=%s files=%d tasks=%d digests=%d", event.change.Type, len(event.change.Files), len(event.change.Tasks), len(event.change.Digests))
					for _, item := range event.change.Files {
						if item.Name != nil && *item.Name == name+"feed.txt" {
							seen[sncloud.ChangeFiles] = true
						}
					}
					for _, item := range event.change.Tasks {
						if item.TaskID != nil && *item.TaskID == changedTask.ID {
							seen[sncloud.ChangeTasks] = true
						}
					}
					for _, item := range event.change.Digests {
						if item.ID != nil && *item.ID == id && item.Item == sncloud.DigestItemDigest {
							seen[sncloud.ChangeDigests] = true
						}
					}
				case <-timer.C:
					break collect
				case <-done:
					break collect
				}
			}
			for _, kind := range []sncloud.ChangeType{sncloud.ChangeFiles, sncloud.ChangeTasks, sncloud.ChangeDigests} {
				t.Run(fmt.Sprintf("Announced-%v", kind), func(t *testing.T) {
					if !seen[kind] {
						t.Fatalf("no %v announcement", kind)
					}
				})
			}
		})
	}
}
