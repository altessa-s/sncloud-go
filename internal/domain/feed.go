// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"strconv"
	"strings"
	"time"
)

// The Socket.IO handshake uses the same HMAC key across accounts.
const feedSigningKey = "K+5xFzxbnB1iSZWqmu3Etw=="

// FeedSignature computes the Socket.IO handshake signature for data:
// HMAC-SHA256 under the protocol key, standard base64 encoded with all
// non-alphanumeric ASCII characters stripped.
func FeedSignature(data string) string {
	mac := hmac.New(sha256.New, []byte(feedSigningKey))
	_, _ = mac.Write([]byte(data))
	encoded := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	var b strings.Builder
	b.Grow(len(encoded))
	for i := range len(encoded) {
		c := encoded[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// ChangeType discriminates what a Change carries on the change feed.
type ChangeType string

// Change types emitted by the Socket.IO change feed.
const (
	// ChangeConnected marks a newly established change-feed session. The server
	// holds queued announcements per connection and discards them when a socket
	// drops, so callers must re-read current state over REST on every
	// ChangeConnected before trusting subsequent incremental hints.
	ChangeConnected ChangeType = "connected"
	// ChangeFiles carries file and folder announcements.
	ChangeFiles ChangeType = "files"
	// ChangeTasks carries to-do list and task announcements.
	ChangeTasks ChangeType = "tasks"
	// ChangeDigests carries digest, digest group and digest tag announcements.
	ChangeDigests ChangeType = "digests"
)

// Change is one event from the change feed. Type selects which batch slice is
// populated; for ChangeConnected all slices are nil.
//
// An announcement is a "look again" signal rather than the authoritative
// record: task notifications carry only QUERY or SORT without task bodies,
// digest notifications carry identifiers without content, and any disconnection
// drops whatever was queued on the server. Callers reconcile against the REST
// views rather than applying announcements blindly as mutations.
type Change struct {
	Type    ChangeType
	Files   []FileChange
	Tasks   []TaskChange
	Digests []DigestChange
}

// ChangeKind is the wire messageType of an announcement; kept as an open string
// type so newly introduced server message types survive decoding.
type ChangeKind string

// String returns the wire spelling of k.
func (k ChangeKind) String() string { return string(k) }

// File and folder change-feed messageType values.
const (
	// ChangeFileAdded announces a newly uploaded file (DOWNLOADFILE on the wire).
	ChangeFileAdded ChangeKind = "DOWNLOADFILE"
	// ChangeFolderAdded announces a newly created folder.
	ChangeFolderAdded ChangeKind = "ADDFOLDER"
	// ChangeFileModified announces a modified file.
	ChangeFileModified ChangeKind = "MODIFYFILE"
	// ChangeFolderModified announces a modified folder.
	ChangeFolderModified ChangeKind = "MODIFYFOLDER"
	// ChangeFileDeleted announces a deleted file.
	ChangeFileDeleted ChangeKind = "DELETEFILE"
	// ChangeFolderDeleted announces a deleted folder.
	ChangeFolderDeleted ChangeKind = "DELETEFOLDER"
	// ChangeFileCopied announces a copied file.
	ChangeFileCopied ChangeKind = "COPYFILE"
	// ChangeFolderCopied announces a copied folder.
	ChangeFolderCopied ChangeKind = "COPYFOLDER"
	// ChangeFileMoved announces a moved file.
	ChangeFileMoved ChangeKind = "MOVEFILE"
	// ChangeFolderMoved announces a moved folder.
	ChangeFolderMoved ChangeKind = "MOVEFOLDER"
	// ChangeWaiting announces that another device holds the sync lock.
	ChangeWaiting ChangeKind = "WAITING"
	// ChangeSyncStarted announces that synchronization started.
	ChangeSyncStarted ChangeKind = "STARTSYNC"
)

// To-do change-feed messageType values.
const (
	// ChangeTasksChanged announces a task or task-list mutation.
	ChangeTasksChanged ChangeKind = "QUERY"
	// ChangeTasksSorted announces a task ordering change.
	ChangeTasksSorted ChangeKind = "SORT"
)

// Digest change-feed messageType values.
const (
	// ChangeDigestAdded announces a newly created digest, group or tag.
	ChangeDigestAdded ChangeKind = "ADD_DIGEST"
	// ChangeDigestUpdated announces an updated digest, group or tag.
	ChangeDigestUpdated ChangeKind = "UPDATE_DIGEST"
	// ChangeDigestDeleted announces a deleted digest, group or tag.
	ChangeDigestDeleted ChangeKind = "DELETE_DIGEST"
)

// FileChange describes a file or folder announcement on ServerMessage.
type FileChange struct {
	Kind          ChangeKind
	IsFolder      bool
	ID            *FileID
	NewID         *FileID
	Name          *string
	NewName       *string
	MD5           *string
	Size          *int64
	FolderID      *FileID
	DestinationID *FileID
	Path          *string
	NewPath       *string
	Device        *string
	Time          *time.Time
}

// UnmarshalJSON decodes a ServerMessage item, accepting the server's FOLDE
// prefix for folders and lowercasing MD5 sums.
func (c *FileChange) UnmarshalJSON(b []byte) error {
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	var kind ChangeKind
	if s := Text(row["messageType"]); s != nil {
		kind = ChangeKind(*s)
	}
	var isFolder bool
	if ft := Text(row["fileType"]); ft != nil {
		isFolder = strings.HasPrefix(strings.ToUpper(*ft), "FOLD")
	}
	fileID := func(key string) *FileID {
		n := Integer(row[key])
		if n == nil {
			return nil
		}
		v := FileID(*n)
		return &v
	}
	var md5Sum *string
	if s := Text(row["md5"]); s != nil {
		lower := strings.ToLower(*s)
		md5Sum = &lower
	}
	*c = FileChange{
		Kind:          kind,
		IsFolder:      isFolder,
		ID:            fileID("id"),
		NewID:         fileID("newId"),
		Name:          Text(row["originalName"]),
		NewName:       Text(row["newName"]),
		MD5:           md5Sum,
		Size:          Integer(row["size"]),
		FolderID:      fileID("directoryId"),
		DestinationID: fileID("goDirectoryId"),
		Path:          Text(row["originalPath"]),
		NewPath:       Text(row["newPath"]),
		Device:        Text(row["equipmentNo"]),
		Time:          WireTime(row["timestamp"]),
	}
	return nil
}

// TaskChange describes a to-do announcement on the to-do event.
type TaskChange struct {
	Kind   ChangeKind
	ListID *string
	TaskID *string
	Device *string
	Time   *time.Time
}

// UnmarshalJSON decodes a to-do announcement item.
func (c *TaskChange) UnmarshalJSON(b []byte) error {
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	var kind ChangeKind
	if s := Text(row["messageType"]); s != nil {
		kind = ChangeKind(*s)
	}
	*c = TaskChange{
		Kind:   kind,
		ListID: Text(row["taskListId"]),
		TaskID: Text(row["taskId"]),
		Device: Text(row["equipmentNo"]),
		Time:   WireTime(row["timestamp"]),
	}
	return nil
}

// DigestChangeItem identifies which kind of digest entity changed.
type DigestChangeItem string

// String returns the wire spelling of i.
func (i DigestChangeItem) String() string { return string(i) }

// Known DigestChangeItem wire values.
const (
	// DigestItemGroup indicates a digest group ("1").
	DigestItemGroup DigestChangeItem = "1"
	// DigestItemDigest indicates a digest ("2").
	DigestItemDigest DigestChangeItem = "2"
	// DigestItemTag indicates a digest tag ("3").
	DigestItemTag DigestChangeItem = "3"
)

// DigestChange describes an announcement on the digest event.
type DigestChange struct {
	Kind             ChangeKind
	Item             DigestChangeItem
	ID               *DigestID
	GroupIdentifier  *string
	UniqueIdentifier *string
	Device           *string
	Time             *time.Time
}

// UnmarshalJSON decodes a digest announcement item, accepting dataType as
// either a string or an integer.
func (c *DigestChange) UnmarshalJSON(b []byte) error {
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	var kind ChangeKind
	if s := Text(row["messageType"]); s != nil {
		kind = ChangeKind(*s)
	}
	var item DigestChangeItem
	if s := Text(row["dataType"]); s != nil {
		item = DigestChangeItem(*s)
	} else if n := Integer(row["dataType"]); n != nil {
		item = DigestChangeItem(strconv.FormatInt(*n, 10))
	}
	var id *DigestID
	if n := Integer(row["id"]); n != nil {
		v := DigestID(*n)
		id = &v
	}
	*c = DigestChange{
		Kind:             kind,
		Item:             item,
		ID:               id,
		GroupIdentifier:  Text(row["parentUniqueIdentifier"]),
		UniqueIdentifier: Text(row["uniqueIdentifier"]),
		Device:           Text(row["equipmentNo"]),
		Time:             WireTime(row["timestamp"]),
	}
	return nil
}
