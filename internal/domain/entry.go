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

const missingEntryID = "entry has no identifier"

// Entry describes a web file or folder, identified independently of its name.
type Entry struct {
	ID                FileID
	ParentID          FileID
	Name              string
	IsFolder          bool
	Size              *int64
	MD5               string
	Created, Modified *time.Time
	ParentName        string
}

// UnmarshalJSON accepts the listing and search variants of UserFileVO.
func (e *Entry) UnmarshalJSON(b []byte) error {
	var w struct {
		ID         *FileID        `json:"id"`
		ParentID   *FileID        `json:"directoryId"`
		Name       *string        `json:"fileName"`
		Folder     *WireFlag      `json:"isFolder"`
		Size       jsontext.Value `json:"size"`
		MD5        string         `json:"md5"`
		Created    jsontext.Value `json:"createTime"`
		Modified   jsontext.Value `json:"updateTime"`
		ParentName string         `json:"directoryName"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	if w.ID == nil {
		return &MalformedResponseError{What: missingEntryID}
	}
	if w.Name == nil {
		return &MalformedResponseError{What: "entry has no filename"}
	}
	*e = Entry{ID: *w.ID, Name: *w.Name, Size: Integer(w.Size),
		MD5: strings.ToLower(w.MD5), Created: Milliseconds(w.Created), Modified: Milliseconds(w.Modified), ParentName: w.ParentName}
	if w.ParentID != nil {
		e.ParentID = *w.ParentID
	}
	if w.Folder != nil {
		e.IsFolder = bool(*w.Folder)
	}
	return nil
}

// Folder is the folder-only listing; HasSubfolders says nothing about files.
type Folder struct {
	ID, ParentID  FileID
	Name          string
	HasSubfolders bool
}

// UnmarshalJSON inverts the service's folder-only empty flag.
func (f *Folder) UnmarshalJSON(b []byte) error {
	var w struct {
		ID     *FileID        `json:"id"`
		Parent jsontext.Value `json:"directoryId"`
		Name   jsontext.Value `json:"fileName"`
		Empty  jsontext.Value `json:"empty"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	if w.ID == nil {
		return &MalformedResponseError{What: missingEntryID}
	}
	*f = Folder{ID: *w.ID}
	if n := Integer(w.Parent); n != nil {
		f.ParentID = FileID(*n)
	}
	if n := Text(w.Name); n != nil {
		f.Name = *n
	}
	if n := Flag(w.Empty); n != nil {
		f.HasSubfolders = !*n
	}
	return nil
}

// SortOrder names the service's required ordering field.
type SortOrder string

const (
	// SortName orders by filename.
	SortName SortOrder = "filename"
	// SortTime orders by modification time.
	SortTime SortOrder = "time"
	// SortSize orders by byte count.
	SortSize SortOrder = "size"
)

// Capacity describes account storage in bytes.
type Capacity struct {
	Used  int64 `json:"usedCapacity"`
	Total int64 `json:"totalCapacity"`
}

// Location gives the containing folders from the root down.
type Location struct {
	Names []string
	IDs   []FileID
}

// User describes the current account without retaining credentials.
type User struct {
	ID                                         *int64
	Name, Email, Phone, CountryCode, AvatarURL string
}

// UnmarshalJSON tolerates the numeric and textual country code variants.
func (u *User) UnmarshalJSON(b []byte) error {
	var w struct {
		ID      jsontext.Value `json:"userId"`
		Name    string         `json:"userName"`
		Email   string         `json:"email"`
		Phone   string         `json:"telephone"`
		Country jsontext.Value `json:"countryCode"`
		Avatar  string         `json:"avatarsUrl"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*u = User{ID: Integer(w.ID), Name: w.Name, Email: w.Email, Phone: w.Phone, AvatarURL: w.Avatar}
	if s := Text(w.Country); s != nil {
		u.CountryCode = *s
	} else if n := Integer(w.Country); n != nil {
		u.CountryCode = FileID(*n).String()
	}
	return nil
}
