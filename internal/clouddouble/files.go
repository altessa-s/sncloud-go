// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

const (
	searchPath         = "file/list/search"
	syntheticCreated   = 1234567890000
	syntheticModified  = 1234567891000
	syntheticAllocated = 1000000
	foldersPath        = "file/folder/list/query"
	recycleListPath    = "file/recycle/list/query"
	deviceListIDPath   = "file/3/files/list_folder_v3"
	deviceCopyPath     = "file/3/files/copy_v3"
	sizeField          = "size"
	urlField           = "url"
	entriesField       = "entriesVO"
	firstFileID        = 9007199254740993
	syntheticCountry   = 44
	syntheticUsed      = 42
)

// Files owns synthetic file metadata and storage, independently of other API handlers.
// EnableFiles installs it on an existing cloud so each test chooses its state.
type Files struct {
	mu          sync.Mutex
	cloud       *Cloud
	next        domain.FileID
	entries     map[domain.FileID]*fileEntry
	objects     map[string][]byte
	uploads     map[string]map[int][]byte
	lock        string
	formUploads map[string]*formUpload
}

type fileEntry struct {
	ID, Parent              domain.FileID
	Name, Path, Hash, Inner string
	Folder, Deleted         bool
	Data                    []byte
}

type fileRequest struct {
	ID          domain.FileID   `json:"id"`
	IDs         []domain.FileID `json:"idList"`
	Parent      domain.FileID   `json:"directoryId"`
	Destination domain.FileID   `json:"goDirectoryId"`
	Name        string          `json:"fileName"`
	NewName     string          `json:"newName"`
	Path        string          `json:"path"`
	ToPath      string          `json:"to_path"`
	Recursive   bool            `json:"recursive"`
	Equipment   string          `json:"equipmentNo"`
	Flag        string          `json:"flag"`
	Inner       string          `json:"innerName"`
	Page        int             `json:"pageNo"`
	Size        int             `json:"pageSize"`
}

// EnableFiles registers both file APIs and a storage endpoint on the app host.
func (c *Cloud) EnableFiles() *Files {
	f := &Files{cloud: c, next: firstFileID, entries: make(map[domain.FileID]*fileEntry),
		formUploads: make(map[string]*formUpload), objects: make(map[string][]byte), uploads: make(map[string]map[int][]byte)}
	for _, p := range []string{"file/list/query", searchPath, foldersPath, "file/folder/add", "file/delete",
		"file/move", "file/copy", "file/rename", "file/capacity/query", "user/query", "file/path/query", recycleListPath,
		"file/recycle/revert", "file/recycle/delete", "file/recycle/clear", "file/download/url", "file/note/to/pdf", "file/note/to/png",
		"file/upload/apply", "file/upload/finish"} {
		c.Register(true, p, f.web)
	}
	for _, p := range []string{"file/2/files/list_folder", deviceListIDPath, "file/3/files/query_v3", "file/3/files/query/by/path_v3",
		"file/2/files/create_folder_v2", "file/3/files/delete_folder_v3", "file/3/files/move_v3", deviceCopyPath,
		"file/3/files/download_v3", "file/3/files/upload/apply", "file/2/files/upload/finish", "file/2/users/get_space_usage",
		"file/2/files/synchronous/start", "file/2/files/synchronous/end", "file/s3/auth"} {
		c.Register(false, p, f.device)
	}
	for _, p := range []string{"oss/object", "oss/upload", "oss/part", "oss/download", "storage/download"} {
		c.Register(false, p, f.storage)
	}
	return f
}

// Seed adds a synthetic entry; data is copied so callers cannot race the server.
func (f *Files) Seed(parent domain.FileID, name, path string, folder bool, data []byte) domain.FileID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.add(parent, name, path, folder, data).ID
}

func (f *Files) add(parent domain.FileID, name, path string, folder bool, data []byte) *fileEntry {
	f.next++
	e := &fileEntry{ID: f.next, Parent: parent, Name: name, Path: domain.DevicePath(path), Folder: folder,
		Data: append([]byte(nil), data...), Hash: fileHash(data)}
	f.entries[e.ID] = e
	return e
}

func (f *Files) request(w http.ResponseWriter, r *http.Request) (fileRequest, bool) {
	f.cloud.mu.Lock()
	authorized := f.cloud.tokens[r.Header.Get(domain.AccessTokenHeader)]
	f.cloud.mu.Unlock()
	var b fileRequest
	if !authorized {
		Refuse(w, "E0712")
		return b, false
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusBadRequest)
		return b, false
	}
	return decodeObject[fileRequest](w, r)
}

func (f *Files) ordered() []*fileEntry {
	all := make([]*fileEntry, 0, len(f.entries))
	for _, e := range f.entries {
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	return all
}

func webRow(e *fileEntry) map[string]any {
	return map[string]any{"id": e.ID.String(), "directoryId": e.Parent.String(), "fileName": e.Name, "isFolder": e.Folder,
		sizeField: len(e.Data), "md5": e.Hash, "createTime": syntheticCreated, "updateTime": syntheticModified}
}

func deviceRow(e *fileEntry) map[string]any {
	if e == nil {
		return nil
	}
	tag := "file"
	if e.Folder {
		tag = "folder"
	}
	return map[string]any{"id": e.ID.String(), "name": e.Name, "path_display": e.Path, "tag": tag, sizeField: len(e.Data),
		"content_hash": e.Hash, "_downloadable": !e.Folder}
}

func (f *Files) find(path string) *fileEntry {
	path = domain.DevicePath(path)
	for _, e := range f.entries {
		if !e.Deleted && e.Path == path {
			return e
		}
	}
	return nil
}

func (f *Files) parentPath(parent domain.FileID, name string) string {
	if e := f.entries[parent]; e != nil {
		return e.Path + "/" + name
	}
	return name
}

func pageRows(rows []map[string]any, b fileRequest) []map[string]any {
	if b.Page < 1 || b.Size < 1 {
		return rows
	}
	start := min((b.Page-1)*b.Size, len(rows))
	return rows[start:min(start+b.Size, len(rows))]
}

func (f *Files) web(w http.ResponseWriter, r *http.Request) {
	b, ok := f.request(w, r)
	if !ok {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p := strings.TrimPrefix(r.URL.Path, "/api/")
	switch p {
	case "file/list/query", searchPath, foldersPath, recycleListPath:
		f.listWeb(w, p, b)
	case "file/folder/add":
		for _, e := range f.entries {
			if !e.Deleted && e.Parent == b.Parent && e.Name == b.Name {
				Refuse(w, "E0007")
				return
			}
		}
		f.add(b.Parent, b.Name, f.parentPath(b.Parent, b.Name), true, nil)
		Reply(w, nil)
	case "file/capacity/query":
		Reply(w, map[string]any{"usedCapacity": "42", "totalCapacity": "1000000"})
	case "user/query":
		Reply(w, map[string]any{"userId": "9007199254740999", "userName": "Synthetic", "email": "test@example.invalid", "countryCode": syntheticCountry})
	case "file/path/query":
		Reply(w, map[string]any{"path": "Note/Meetings/", "idPath": "1/9007199254740993/"})
	case "file/download/url", "file/note/to/pdf":
		f.download(w, b.ID, false)
	case "file/note/to/png":
		Reply(w, map[string]any{"pngPageVOList": []any{map[string]any{"pageNo": 1, urlField: f.downloadURL() + "?png=1"}}})
	case "file/upload/apply":
		f.apply(w)
	case "file/upload/finish":
		f.add(b.Parent, b.Name, f.parentPath(b.Parent, b.Name), false, f.objects[b.Inner])
		Reply(w, nil)
	default:
		f.changeWeb(w, p, b)
	}
}

func (f *Files) listWeb(w http.ResponseWriter, p string, b fileRequest) {
	var rows []map[string]any
	for _, e := range f.ordered() {
		if p == recycleListPath {
			if e.Deleted {
				rows = append(rows, map[string]any{"fileId": e.ID.String(), "fileName": e.Name, "isFolder": e.Folder, sizeField: len(e.Data)})
			}
			continue
		}
		if e.Deleted {
			continue
		}
		if p == searchPath {
			if strings.Contains(e.Name, b.Name) {
				rows = append(rows, webRow(e))
			}
			continue
		}
		if e.Parent != b.Parent {
			continue
		}
		if p == foldersPath {
			excluded := false
			for _, id := range b.IDs {
				if e.ID == id {
					excluded = true
				}
			}
			if e.Folder && !excluded {
				row := webRow(e)
				row["empty"] = true
				rows = append(rows, row)
			}
		} else {
			rows = append(rows, webRow(e))
		}
	}
	key := "userFileVOList"
	switch p {
	case searchPath:
		key = "userFileSearchVOList"
	case foldersPath:
		key = "folderVOList"
	case recycleListPath:
		key = "recycleFileVOList"
	}
	Reply(w, map[string]any{key: pageRows(rows, b), "total": len(rows)})
}

func (f *Files) changeWeb(w http.ResponseWriter, p string, b fileRequest) {
	if p == "file/recycle/clear" {
		for id, e := range f.entries {
			if e.Deleted {
				delete(f.entries, id)
			}
		}
		Reply(w, nil)
		return
	}
	if p == "file/rename" {
		if e := f.entries[b.ID]; e != nil {
			e.Name = b.NewName
			e.Path = f.parentPath(e.Parent, e.Name)
		}
		Reply(w, nil)
		return
	}
	for _, id := range b.IDs {
		if f.entries[id] == nil {
			Refuse(w, "E0311")
			return
		}
	}
	for _, id := range b.IDs {
		e := f.entries[id]
		switch p {
		case "file/delete":
			e.Deleted = true
		case "file/recycle/revert":
			e.Deleted = false
		case "file/recycle/delete":
			delete(f.entries, id)
		case "file/move":
			e.Parent = b.Destination
			e.Path = f.parentPath(e.Parent, e.Name)
		case "file/copy":
			f.add(b.Destination, e.Name, f.parentPath(b.Destination, e.Name), e.Folder, e.Data)
		}
	}
	Reply(w, nil)
}
