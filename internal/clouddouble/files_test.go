// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fileFixture(t *testing.T, flavor Flavor) *Files {
	t.Helper()
	c := New(flavor)
	t.Cleanup(c.Close)
	c.tokens["synthetic-token"] = true
	return c.EnableFiles()
}

func TestWebFileState(t *testing.T) {
	f := fileFixture(t, Private)
	data := []byte("synthetic")
	folder := f.Seed(0, "notes", "notes", true, nil)
	id := f.Seed(folder, "first", "notes/first", false, data)
	data[0] = 'X'
	if string(f.entries[id].Data) != "synthetic" {
		t.Fatal("Seed retained caller bytes")
	}
	other := f.Seed(0, "other", "other", true, nil)
	call := func(path, body string) map[string]any {
		t.Helper()
		w, out := exchange(t, f.web, path, body)
		if w.Code != 200 {
			t.Fatal(path, w.Code)
		}
		return out
	}
	list := func(path, body, key string, want int) {
		t.Helper()
		out := call(path, body)
		requireSuccess(t, out)
		rows, _ := out[key].([]any)
		if len(rows) != want {
			t.Fatalf("%s: %v", path, out)
		}
	}
	list("file/list/query", fmt.Sprintf(`{"directoryId":%d}`, folder), "userFileVOList", 1)
	list(searchPath, `{"fileName":"first"}`, "userFileSearchVOList", 1)
	list(foldersPath, fmt.Sprintf(`{"idList":[%d]}`, other), "folderVOList", 1)
	list("file/list/query", `{"pageNo":2,"pageSize":1}`, "userFileVOList", 1)
	list("file/list/query", `{"pageNo":99,"pageSize":1}`, "userFileVOList", 0)
	requireSuccess(t, call("file/folder/add", fmt.Sprintf(`{"directoryId":%d,"fileName":"child"}`, folder)))
	requireCode(t, call("file/folder/add", fmt.Sprintf(`{"directoryId":%d,"fileName":"child"}`, folder)), "E0007")
	requireSuccess(t, call("file/rename", fmt.Sprintf(`{"id":%d,"newName":"renamed"}`, id)))
	if f.entries[id].Path != "notes/renamed" {
		t.Fatal(f.entries[id])
	}
	requireSuccess(t, call("file/copy", fmt.Sprintf(`{"idList":[%d],"goDirectoryId":%d}`, id, other)))
	copied := f.find("other/renamed")
	if copied == nil || copied.ID == id || string(copied.Data) != "synthetic" {
		t.Fatal(copied)
	}
	requireSuccess(t, call("file/move", fmt.Sprintf(`{"idList":[%d],"goDirectoryId":%d}`, id, other)))
	if f.entries[id].Parent != other || f.entries[id].Path != "other/renamed" {
		t.Fatal(f.entries[id])
	}
	ids := fmt.Sprintf(`{"idList":[%d]}`, id)
	requireCode(t, call("file/delete", `{"idList":[-1]}`), "E0311")
	requireSuccess(t, call("file/delete", ids))
	if !f.entries[id].Deleted {
		t.Fatal("not deleted")
	}
	list(recycleListPath, `{}`, "recycleFileVOList", 1)
	list(searchPath, `{"fileName":"renamed"}`, "userFileSearchVOList", 1)
	requireSuccess(t, call("file/recycle/revert", ids))
	if f.entries[id].Deleted {
		t.Fatal("not restored")
	}
	requireSuccess(t, call("file/recycle/delete", ids))
	if f.entries[id] != nil {
		t.Fatal("not removed")
	}
	requireSuccess(t, call("file/delete", fmt.Sprintf(`{"idList":[%d]}`, copied.ID)))
	requireSuccess(t, call("file/recycle/clear", `{}`))
	if f.entries[copied.ID] != nil || f.entries[folder] == nil {
		t.Fatal("clear removed wrong entries")
	}
	for _, tc := range []struct {
		path, key string
		want      any
	}{
		{"file/capacity/query", "usedCapacity", "42"}, {"user/query", "userName", "Synthetic"}, {"file/path/query", "path", "Note/Meetings/"},
	} {
		out := call(tc.path, `{}`)
		requireSuccess(t, out)
		if out[tc.key] != tc.want {
			t.Fatal(out)
		}
	}
	f.objects["object"] = []byte("uploaded")
	requireSuccess(t, call("file/upload/finish", fmt.Sprintf(`{"directoryId":%d,"fileName":"upload","innerName":"object"}`, folder)))
	if e := f.find("notes/upload"); e == nil || string(e.Data) != "uploaded" {
		t.Fatal(e)
	}
	out := call("file/note/to/png", `{}`)
	requireSuccess(t, out)
	if len(out["pngPageVOList"].([]any)) != 1 {
		t.Fatal(out)
	}
	for _, path := range []string{"file/download/url", "file/note/to/pdf"} {
		out := call(path, fmt.Sprintf(`{"id":%d}`, folder))
		requireSuccess(t, out)
		if out["md5"] != f.entries[folder].Hash || !strings.Contains(out["url"].(string), "?id="+folder.String()) {
			t.Fatal(out)
		}
		requireCode(t, call(path, `{"id":-1}`), "E0311")
	}
}

func TestDeviceFileState(t *testing.T) {
	for _, flavor := range []Flavor{Official, Private} {
		t.Run(string(flavor), func(t *testing.T) {
			f := fileFixture(t, flavor)
			folder := f.Seed(0, "notes", "notes", true, nil)
			id := f.Seed(folder, "one", "notes/one", false, []byte("one"))
			deleted := f.Seed(folder, "deleted", "notes/deleted", false, nil)
			f.entries[deleted].Deleted = true
			call := func(path, body string) map[string]any {
				t.Helper()
				w, out := exchange(t, f.device, path, body)
				if w.Code != 200 {
					t.Fatal(path, w.Code)
				}
				return out
			}
			for _, tc := range []struct {
				path, body string
				count      int
			}{
				{"file/2/files/list_folder", `{"path":"notes"}`, 1}, {deviceListIDPath, fmt.Sprintf(`{"id":%d}`, folder), 1}, {"file/2/files/list_folder", `{"recursive":true}`, 2},
			} {
				out := call(tc.path, tc.body)
				requireSuccess(t, out)
				if rows := out["entries"].([]any); len(rows) != tc.count {
					t.Fatal(out)
				}
			}
			for _, tc := range []struct{ path, body string }{{"file/3/files/query_v3", fmt.Sprintf(`{"id":%d}`, id)}, {"file/3/files/query/by/path_v3", `{"path":"/notes/one"}`}} {
				out := call(tc.path, tc.body)
				requireSuccess(t, out)
				if out[entriesField].(map[string]any)["id"] != id.String() {
					t.Fatal(out)
				}
			}
			out := call("file/3/files/query/by/path_v3", `{"path":"notes/deleted"}`)
			if out[entriesField] != nil {
				t.Fatal(out)
			}
			requireSuccess(t, call("file/2/files/create_folder_v2", `{"path":"new"}`))
			requireCode(t, call("file/2/files/create_folder_v2", `{"path":"new"}`), "E0007")
			requireCode(t, call(deviceCopyPath, `{"id":-1}`), "E0311")
			requireSuccess(t, call(deviceCopyPath, fmt.Sprintf(`{"id":%d,"to_path":"copied"}`, id)))
			if e := f.find("copied"); e == nil || string(e.Data) != "one" {
				t.Fatal(e)
			}
			requireSuccess(t, call("file/3/files/move_v3", fmt.Sprintf(`{"id":%d,"to_path":"moved"}`, id)))
			if f.find("notes/one") != nil || f.find("moved") == nil {
				t.Fatal("move did not relocate entry")
			}
			out = call("file/3/files/download_v3", fmt.Sprintf(`{"id":%d}`, id))
			requireSuccess(t, out)
			if out["content_hash"] != f.entries[id].Hash {
				t.Fatal(out)
			}
			requireSuccess(t, call("file/3/files/delete_folder_v3", fmt.Sprintf(`{"id":%d}`, id)))
			if f.entries[id] != nil {
				t.Fatal("delete retained entry")
			}
			f.objects["object"] = []byte("upload")
			out = call("file/2/files/upload/finish", `{"path":"notes/","fileName":"upload","innerName":"object"}`)
			requireSuccess(t, out)
			if e := f.find("notes/upload"); e == nil || string(e.Data) != "upload" {
				t.Fatal(e)
			}
			out = call("file/2/users/get_space_usage", `{}`)
			if out["used"] != float64(syntheticUsed) {
				t.Fatal(out)
			}
			out = call("file/s3/auth", `{}`)
			if out["s3Authorization"] != "synthetic-signature" {
				t.Fatal(out)
			}
			body := `{"equipmentNo":"owner"}`
			out = call("file/2/files/synchronous/start", body)
			requireSuccess(t, out)
			if out["synType"] != true {
				t.Fatal(out)
			}
			requireCode(t, call("file/2/files/synchronous/start", body), "E0078")
			requireCode(t, call("file/2/files/synchronous/end", `{"equipmentNo":"other"}`), "E0078")
			requireSuccess(t, call("file/2/files/synchronous/end", body))
			if f.lock != "" {
				t.Fatal("lock retained")
			}
		})
	}
}

func TestFileRequestValidation(t *testing.T) {
	f := fileFixture(t, Private)
	for _, h := range []http.HandlerFunc{f.web, f.device} {
		for _, tc := range []struct {
			method, body, token string
			status              int
		}{{"POST", "{}", "", 200}, {"GET", "{}", "synthetic-token", 400}, {"POST", "bad", "synthetic-token", 400}} {
			r := httptest.NewRequest(tc.method, "/api/file/list/query", strings.NewReader(tc.body))
			r.Header.Set("x-access-token", tc.token)
			w := httptest.NewRecorder()
			h(w, r)
			if w.Code != tc.status {
				t.Fatal(w.Code, tc)
			}
			if tc.token == "" && !strings.Contains(w.Body.String(), "E0712") {
				t.Fatal(w.Body)
			}
		}
	}
	r := httptest.NewRequest("POST", "/api/file/2/files/list_folder", strings.NewReader(`{}`))
	r.Header.Set("x-access-token", "synthetic-token")
	w := httptest.NewRecorder()
	f.device(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
