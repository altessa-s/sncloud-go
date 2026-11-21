// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json/v2"
	"testing"
)

func TestEntryWireFormats(t *testing.T) {
	for _, raw := range []string{
		`{"id":"9007199254740993","directoryId":2,"fileName":"a","isFolder":"Y","size":"42","md5":"ABC","createTime":"1234567890000","updateTime":1234567891000,"directoryName":"parent"}`,
		`{"id":9007199254740993,"directoryId":"2","fileName":"a","isFolder":true,"size":42,"md5":"abc"}`,
	} {
		var e Entry
		if err := json.Unmarshal([]byte(raw), &e); err != nil || e.ID != 9007199254740993 || !e.IsFolder || *e.Size != 42 || e.MD5 != "abc" {
			t.Fatal(e, err)
		}
	}
	var e Entry
	if err := json.Unmarshal([]byte(`{"id":1,"fileName":"a","size":"bad","createTime":"bad"}`), &e); err != nil || e.Size != nil || e.Created != nil {
		t.Fatal(e, err)
	}
	if json.Unmarshal([]byte(`{"id":{}}`), &e) == nil {
		t.Fatal("bad id")
	}
	var f Folder
	if err := json.Unmarshal([]byte(`{"id":1,"directoryId":"2","fileName":"F","empty":"N"}`), &f); err != nil || !f.HasSubfolders || f.ParentID != 2 || f.Name != "F" {
		t.Fatal(f, err)
	}
	if err := json.Unmarshal([]byte(`{"id":1,"directoryId":{},"fileName":null}`), &f); err != nil || f.HasSubfolders || f.ParentID != 0 {
		t.Fatal(f, err)
	}
	if json.Unmarshal([]byte(`{"id":{}}`), &f) == nil {
		t.Fatal("bad folder id")
	}
	var u User
	for _, country := range []string{`"44"`, `44`} {
		if err := json.Unmarshal([]byte(`{"userId":"9007199254740993","userName":"name","email":"a@example.invalid","countryCode":`+country+`}`), &u); err != nil || *u.ID != 9007199254740993 || u.CountryCode != "44" {
			t.Fatal(u, err)
		}
	}
	if json.Unmarshal([]byte(`{"userName":3}`), &u) == nil {
		t.Fatal("bad user name")
	}
	var r RecycledEntry
	if err := json.Unmarshal([]byte(`{"fileId":"1","fileName":"f","isFolder":"Y","size":3,"updateTime":"2026-01-01 12:00:00"}`), &r); err != nil || !r.IsFolder || r.Size != nil || r.Deleted == nil {
		t.Fatal(r, err)
	}
	if err := json.Unmarshal([]byte(`{"fileId":"1","size":"3"}`), &r); err != nil || *r.Size != 3 {
		t.Fatal(r, err)
	}
	if json.Unmarshal([]byte(`{"fileId":{}}`), &r) == nil {
		t.Fatal("bad recycled id")
	}
}

func TestDeviceEntryShapes(t *testing.T) {
	cases := []struct{ raw, path, parent, name string }{
		{`{"id":1,"name":"a","path_display":"/NOTE/Note"}`, "NOTE/Note/a", "NOTE/Note", "a"},
		{`{"id":1,"name":"a","path_display":"NOTE/Note/a"}`, "NOTE/Note/a", "NOTE/Note", "a"},
		{`{"id":1,"path_display":"NOTE/Note/a"}`, "NOTE/Note/a", "NOTE/Note", "a"},
		{`{"id":1,"name":"a","path_display":""}`, "a", "", "a"},
		{`{"id":1,"name":"a","path_display":"a","parent_path":"/F/"}`, "a", "F", "a"},
		{`{"id":1}`, "", "", ""},
	}
	for _, tc := range cases {
		var e DeviceEntry
		if err := json.Unmarshal([]byte(tc.raw), &e); err != nil || e.Path != tc.path || e.ParentPath != tc.parent || e.Name != tc.name {
			t.Fatal(e, err, tc)
		}
	}
	var e DeviceEntry
	if err := json.Unmarshal([]byte(`{"id":1,"name":"Foo","path_display":"Foo","tag":"FOLDER","size":"42","content_hash":"ABC","lastUpdateTime":1234567890000,"_downloadable":"N"}`), &e); err != nil {
		t.Fatal(err)
	}
	if e = DeviceEntryListedIn(e, "//Foo//"); e.Path != "Foo/Foo" || !e.IsFolder || e.IsDownloadable == nil || *e.IsDownloadable || *e.Size != 42 || e.ContentHash != "abc" || e.Modified == nil {
		t.Fatal(e)
	}
	if got := DeviceEntryListedIn(e, "other"); got.Path != e.Path {
		t.Fatal(got)
	}
	if err := json.Unmarshal([]byte(`{"id":1,"is_downloadable":true,"_downloadable":false}`), &e); err != nil || !*e.IsDownloadable {
		t.Fatal(e, err)
	}
	if json.Unmarshal([]byte(`{"id":{}}`), &e) == nil {
		t.Fatal("bad device id")
	}
	var s SpaceUsage
	for _, raw := range []string{`{"used":"42","allocationVO":{"allocated":"100"}}`, `{"used":42,"allocationVO":null}`} {
		if err := json.Unmarshal([]byte(raw), &s); err != nil || s.Used != 42 {
			t.Fatal(s, err)
		}
	}
	if json.Unmarshal([]byte(`{"used":{}}`), &s) == nil {
		t.Fatal("bad used count")
	}
}

func TestEntriesRequireIdentifiersAndWebNames(t *testing.T) {
	for _, out := range []any{&Entry{}, &Folder{}, &DeviceEntry{}, &RecycledEntry{}} {
		if err := json.Unmarshal([]byte(`{}`), out); err == nil {
			t.Fatalf("%T accepted missing identifier", out)
		}
	}
	var entry Entry
	if err := json.Unmarshal([]byte(`{"id":1}`), &entry); err == nil {
		t.Fatal("missing filename accepted")
	}
}
