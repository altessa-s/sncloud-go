// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"context"
	"iter"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// ChangeType discriminates what a Change carries on the change feed.
type ChangeType = domain.ChangeType

// Change types emitted by the Socket.IO change feed.
const (
	// ChangeConnected marks a newly established change-feed session. Callers
	// should re-read remote state on every ChangeConnected because announcements
	// while disconnected are not replayed.
	ChangeConnected = domain.ChangeConnected
	// ChangeFiles carries file and folder announcements.
	ChangeFiles = domain.ChangeFiles
	// ChangeTasks carries to-do list and task announcements.
	ChangeTasks = domain.ChangeTasks
	// ChangeDigests carries digest, digest group and digest tag announcements.
	ChangeDigests = domain.ChangeDigests
)

// Change is one event from the change feed. Type selects which batch slice is
// populated; for [ChangeConnected] all slices are nil. Announcements are hints
// to re-read the corresponding REST view, and an announcement may arrive more
// than once.
type Change = domain.Change

// ChangeKind is the wire messageType of an announcement; kept as an open string
// type so newly introduced server message types survive decoding.
type ChangeKind = domain.ChangeKind

// Known change-feed messageType values.
const (
	// ChangeFileAdded announces a newly uploaded file (DOWNLOADFILE on the wire).
	ChangeFileAdded = domain.ChangeFileAdded
	// ChangeFolderAdded announces a newly created folder.
	ChangeFolderAdded = domain.ChangeFolderAdded
	// ChangeFileModified announces a modified file.
	ChangeFileModified = domain.ChangeFileModified
	// ChangeFolderModified announces a modified folder.
	ChangeFolderModified = domain.ChangeFolderModified
	// ChangeFileDeleted announces a deleted file.
	ChangeFileDeleted = domain.ChangeFileDeleted
	// ChangeFolderDeleted announces a deleted folder.
	ChangeFolderDeleted = domain.ChangeFolderDeleted
	// ChangeFileCopied announces a copied file.
	ChangeFileCopied = domain.ChangeFileCopied
	// ChangeFolderCopied announces a copied folder.
	ChangeFolderCopied = domain.ChangeFolderCopied
	// ChangeFileMoved announces a moved file.
	ChangeFileMoved = domain.ChangeFileMoved
	// ChangeFolderMoved announces a moved folder.
	ChangeFolderMoved = domain.ChangeFolderMoved
	// ChangeWaiting announces that another device holds the sync lock.
	ChangeWaiting = domain.ChangeWaiting
	// ChangeSyncStarted announces that synchronization started.
	ChangeSyncStarted = domain.ChangeSyncStarted

	// ChangeTasksChanged announces a task or task-list mutation.
	ChangeTasksChanged = domain.ChangeTasksChanged
	// ChangeTasksSorted announces a task ordering change.
	ChangeTasksSorted = domain.ChangeTasksSorted

	// ChangeDigestAdded announces a newly created digest, group or tag.
	ChangeDigestAdded = domain.ChangeDigestAdded
	// ChangeDigestUpdated announces an updated digest, group or tag.
	ChangeDigestUpdated = domain.ChangeDigestUpdated
	// ChangeDigestDeleted announces a deleted digest, group or tag.
	ChangeDigestDeleted = domain.ChangeDigestDeleted
)

// FileChange describes a file or folder announcement on ServerMessage.
type FileChange = domain.FileChange

// TaskChange describes a to-do announcement on the to-do event.
type TaskChange = domain.TaskChange

// DigestChangeItem identifies which kind of digest entity
// changed: [DigestItemGroup], [DigestItemDigest], or [DigestItemTag].
type DigestChangeItem = domain.DigestChangeItem

// Known DigestChangeItem wire values.
const (
	// DigestItemGroup indicates a digest group ("1").
	DigestItemGroup = domain.DigestItemGroup
	// DigestItemDigest indicates a digest ("2").
	DigestItemDigest = domain.DigestItemDigest
	// DigestItemTag indicates a digest tag ("3").
	DigestItemTag = domain.DigestItemTag
)

// DigestChange describes an announcement on the digest event.
type DigestChange = domain.DigestChange

// Changes opens a single Socket.IO change-feed connection, emitting
// ChangeConnected followed by batches of file, task and digest announcements
// until the server closes cleanly, ctx is canceled, the caller breaks out of
// iteration, or a protocol or transport error occurs.
func (c *Feed) Changes(ctx context.Context) iter.Seq2[Change, error] {
	return c.feed.Changes(ctx)
}

// Watch maintains a reconnecting Socket.IO change-feed stream, re-opening
// the connection with RetryPolicy backoff after clean disconnects or retryable
// transport and HTTP failures, and emitting ChangeConnected on every fresh
// connection. Reconnection is not capped by RetryPolicy.MaxAttempts. Terminal
// errors such as ErrNotLoggedIn or ErrSessionExpired end the iterator.
func (c *Feed) Watch(ctx context.Context) iter.Seq2[Change, error] {
	return c.feed.Watch(ctx)
}

// Feed is a view of the Socket.IO change feed over the client's shared session.
type Feed struct {
	feed port.ChangeFeed
}

// Feed returns a cheap view sharing the session and socket dialer.
func (c *Client) Feed() *Feed {
	return &Feed{feed: c.feed}
}
