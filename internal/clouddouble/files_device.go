// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"net/http"
	"path"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func (f *Files) device(w http.ResponseWriter, r *http.Request) {
	b, ok := f.request(w, r)
	if !ok {
		return
	}
	if r.Header.Get("equipmentNo") == "" || r.Header.Get("channel") == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p := strings.TrimPrefix(r.URL.Path, "/api/")
	switch p {
	case "file/2/files/list_folder", deviceListIDPath:
		f.listDevice(w, b, p)
	case "file/3/files/query_v3":
		Reply(w, map[string]any{entriesField: deviceRow(f.entries[b.ID])})
	case "file/3/files/query/by/path_v3":
		Reply(w, map[string]any{entriesField: deviceRow(f.find(b.Path))})
	case "file/2/files/create_folder_v2":
		if f.find(b.Path) != nil {
			Refuse(w, "E0007")
			return
		}
		e := f.add(0, path.Base(b.Path), b.Path, true, nil)
		Reply(w, map[string]any{"metadata": deviceRow(e)})
	case "file/3/files/delete_folder_v3":
		delete(f.entries, b.ID)
		Reply(w, nil)
	case "file/3/files/move_v3", deviceCopyPath:
		e := f.entries[b.ID]
		if e == nil {
			Refuse(w, "E0311")
			return
		}
		if p == deviceCopyPath {
			e = f.add(0, path.Base(b.ToPath), b.ToPath, e.Folder, e.Data)
		} else {
			e.Name = path.Base(b.ToPath)
			e.Path = domain.DevicePath(b.ToPath)
		}
		Reply(w, map[string]any{entriesField: deviceRow(e)})
	case "file/2/users/get_space_usage":
		Reply(w, map[string]any{"used": syntheticUsed, "allocationVO": map[string]any{"allocated": syntheticAllocated}})
	case "file/3/files/download_v3":
		f.download(w, b.ID, true)
	case "file/3/files/upload/apply":
		f.apply(w)
	case "file/2/files/upload/finish":
		e := f.add(0, b.Name, b.Path+b.Name, false, f.objects[b.Inner])
		Reply(w, deviceRow(e))
	case "file/s3/auth":
		Reply(w, map[string]any{"s3Authorization": "synthetic-signature", "xamzDate": "20260101T000000Z"})
	case "file/2/files/synchronous/start":
		if f.lock != "" {
			Refuse(w, "E0078")
			return
		}
		f.lock = b.Equipment
		Reply(w, map[string]any{"synType": len(f.entries) > 0})
	case "file/2/files/synchronous/end":
		if f.lock != b.Equipment {
			Refuse(w, "E0078")
			return
		}
		f.lock = ""
		Reply(w, nil)
	}
}

func (f *Files) listDevice(w http.ResponseWriter, b fileRequest, p string) {
	folder := domain.DevicePath(b.Path)
	if p == deviceListIDPath {
		if e := f.entries[b.ID]; e != nil {
			folder = e.Path
		}
	}
	var rows []map[string]any
	for _, e := range f.ordered() {
		if e.Deleted {
			continue
		}
		parent := path.Dir(e.Path)
		if parent == "." {
			parent = ""
		}
		matches := parent == folder
		if b.Recursive {
			matches = folder == "" || strings.HasPrefix(e.Path, folder+"/")
		}
		if matches {
			row := deviceRow(e)
			if f.cloud.flavor == Official {
				row["path_display"] = parent
			}
			rows = append(rows, row)
		}
	}
	Reply(w, map[string]any{"entries": rows})
}
