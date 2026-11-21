// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"time"
)

// RecycledEntry retains the identifier used to restore a deleted entry.
type RecycledEntry struct {
	ID       FileID
	Name     string
	IsFolder bool
	Size     *int64
	Deleted  *time.Time
}

// UnmarshalJSON accepts the recycle bin's date-string timestamps as well as milliseconds.
func (e *RecycledEntry) UnmarshalJSON(b []byte) error {
	var w struct {
		ID      *FileID        `json:"fileId"`
		Name    jsontext.Value `json:"fileName"`
		Folder  jsontext.Value `json:"isFolder"`
		Size    jsontext.Value `json:"size"`
		Deleted jsontext.Value `json:"updateTime"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	if w.ID == nil {
		return &MalformedResponseError{What: missingEntryID}
	}
	*e = RecycledEntry{ID: *w.ID, Deleted: Instant(w.Deleted)}
	if v := Text(w.Name); v != nil {
		e.Name = *v
	}
	if v := Flag(w.Folder); v != nil {
		e.IsFolder = *v
	}
	if !e.IsFolder {
		e.Size = Integer(w.Size)
	}
	return nil
}
