// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"strings"
	"time"
)

// DeviceEntry describes the stored tree, whose paths differ from the web tree.
type DeviceEntry struct {
	ID                                  FileID
	Name, Path, ParentPath, PathDisplay string
	IsFolder                            bool
	Size                                *int64
	ContentHash                         string
	Modified                            *time.Time
	IsDownloadable                      *bool
}

// DevicePath removes repeated and trailing separators without resolving names.
func DevicePath(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return r == '/' }), "/")
}

// UnmarshalJSON accepts a parent path in listings and a full path in metadata.
func (e *DeviceEntry) UnmarshalJSON(b []byte) error {
	var w struct {
		ID           *FileID        `json:"id"`
		Name         *string        `json:"name"`
		Display      string         `json:"path_display"`
		Parent       *string        `json:"parent_path"`
		Tag          string         `json:"tag"`
		Size         jsontext.Value `json:"size"`
		Hash         string         `json:"content_hash"`
		Modified     jsontext.Value `json:"lastUpdateTime"`
		Downloadable jsontext.Value `json:"is_downloadable"`
		Bare         jsontext.Value `json:"_downloadable"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	if w.ID == nil {
		return &MalformedResponseError{What: missingEntryID}
	}
	whole := DevicePath(w.Display)
	parts := strings.Split(whole, "/")
	name := parts[len(parts)-1]
	parent := strings.Join(parts[:len(parts)-1], "/")
	if w.Name != nil {
		name = *w.Name
	}
	if w.Display == "" || (w.Name != nil && parts[len(parts)-1] != name) {
		parent = whole
		whole = DevicePath(parent + "/" + name)
	} else if w.Parent != nil {
		parent = DevicePath(*w.Parent)
	}
	*e = DeviceEntry{ID: *w.ID, Name: name, Path: whole, ParentPath: parent, PathDisplay: w.Display,
		IsFolder: strings.EqualFold(w.Tag, "folder"), Size: Integer(w.Size), ContentHash: strings.ToLower(w.Hash),
		Modified: Milliseconds(w.Modified), IsDownloadable: Flag(w.Downloadable)}
	if e.IsDownloadable == nil {
		e.IsDownloadable = Flag(w.Bare)
	}
	return nil
}

// DeviceEntryListedIn resolves the ambiguous Foo entry listed inside the Foo folder.
func DeviceEntryListedIn(e DeviceEntry, folder string) DeviceEntry {
	folder = DevicePath(folder)
	if DevicePath(e.PathDisplay) == folder && e.Name != "" {
		e.ParentPath = folder
		e.Path = DevicePath(folder + "/" + e.Name)
	}
	return e
}

// SpaceUsage is the device API's storage report.
type SpaceUsage struct {
	Used      int64
	Allocated *int64
}

// UnmarshalJSON reads optional allocation without requiring it for usage.
func (s *SpaceUsage) UnmarshalJSON(b []byte) error {
	var w struct {
		Used       WireInt64      `json:"used"`
		Allocation jsontext.Value `json:"allocationVO"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	var a struct {
		Allocated jsontext.Value `json:"allocated"`
	}
	_ = json.Unmarshal(w.Allocation, &a) // Optional allocation; unsupported shapes leave the total unknown.
	*s = SpaceUsage{Used: int64(w.Used), Allocated: Integer(a.Allocated)}
	return nil
}
