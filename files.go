// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Entry describes a file or folder in the identifier-based web API.
type Entry = domain.Entry

// Folder describes a subfolder returned by [Files.Folders]. HasSubfolders
// reports whether the folder contains child folders, independent of files.
type Folder = domain.Folder

// SortOrder names the ordering field for web file listings: [SortName],
// [SortTime], or [SortSize].
type SortOrder = domain.SortOrder

// Capacity reports account storage in bytes.
type Capacity = domain.Capacity

// Location lists the containing folders of an entry from the root down.
type Location = domain.Location

// User describes the current account's profile without retaining credentials.
type User = domain.User

// RecycledEntry describes a deleted file or folder in the recycle bin.
type RecycledEntry = domain.RecycledEntry

// Progress reports transfer progress; a negative total means the length is
// unknown. PNG export uses 1000 units per page rather than bytes. Retries can
// restart progress for the repeated transfer.
type Progress = port.Progress

// File sort orders.
const (
	// SortName ("filename") orders entries by filename.
	SortName = domain.SortName
	// SortTime ("time") orders entries by modification or deletion time.
	SortTime = domain.SortTime
	// SortSize ("size") orders entries by byte size.
	SortSize = domain.SortSize
)

// List reads every page in folder. An empty order defaults to [SortName].
func (c *Files) List(ctx context.Context, folder FileID, order SortOrder, ascending bool) ([]Entry, error) {
	return c.files.List(ctx, folder, order, ascending)
}

// Search finds entries whose names match name throughout the tree, reading
// every page. An empty order defaults to [SortName].
func (c *Files) Search(ctx context.Context, name string, order SortOrder, ascending bool) ([]Entry, error) {
	return c.files.Search(ctx, name, order, ascending)
}

// Entry returns the named direct child of folder, or (nil, nil) when absent.
func (c *Files) Entry(ctx context.Context, name string, folder FileID) (*Entry, error) {
	return c.files.Entry(ctx, name, folder)
}

// Folders lists direct subfolders of folder, omitting any identifier in
// excluding (defaulting to folder itself when excluding is empty).
func (c *Files) Folders(ctx context.Context, folder FileID, excluding []FileID) ([]Folder, error) {
	return c.files.Folders(ctx, folder, excluding)
}

// Capacity reports used and total storage bytes for the current account.
func (c *Files) Capacity(ctx context.Context) (Capacity, error) {
	return c.files.Capacity(ctx)
}

// User reports the current account's profile.
func (c *Files) User(ctx context.Context) (User, error) {
	return c.files.User(ctx)
}

// Location reads the containing folder names and identifiers for id.
func (c *Files) Location(ctx context.Context, id FileID) (Location, error) {
	return c.files.Location(ctx, id)
}

// Download writes id to a temporary file the caller must remove. Size is a
// byte ceiling, defaulting to [DownloadLimit] when nil. The HTTP content length
// is checked when present, and MD5 is checked when the service supplies it.
func (c *Files) Download(ctx context.Context, id FileID, size *int64, p Progress) (string, error) {
	return c.files.Download(ctx, id, size, p)
}

// Upload streams the local path file into folder under name (defaulting to
// filepath.Base(file) when name is empty), pinned to the starting login.
func (c *Files) Upload(ctx context.Context, file, name string, folder FileID, p Progress) error {
	return c.files.Upload(ctx, file, name, folder, p)
}

// UploadBytes writes an in-memory payload into folder under name through the
// same upload path as [Files.Upload].
func (c *Files) UploadBytes(ctx context.Context, data []byte, name string, folder FileID) error {
	return c.files.UploadBytes(ctx, data, name, folder)
}

// CreateFolder creates name inside parent and resolves the created [Entry] from
// a fresh folder listing.
func (c *Files) CreateFolder(ctx context.Context, name string, parent FileID) (Entry, error) {
	return c.files.CreateFolder(ctx, name, parent)
}

// Delete moves ids from parent to the recycle bin.
func (c *Files) Delete(ctx context.Context, ids []FileID, parent FileID) error {
	return c.files.Delete(ctx, ids, parent)
}

// Move relocates ids from parent into destination.
func (c *Files) Move(ctx context.Context, ids []FileID, parent, destination FileID) error {
	return c.files.Move(ctx, ids, parent, destination)
}

// Copy copies ids from parent into destination.
func (c *Files) Copy(ctx context.Context, ids []FileID, parent, destination FileID) error {
	return c.files.Copy(ctx, ids, parent, destination)
}

// Rename changes id's filename within its current folder.
func (c *Files) Rename(ctx context.Context, id FileID, name string) error {
	return c.files.Rename(ctx, id, name)
}

// PDF renders the note id to PDF and downloads it to a caller-owned temporary
// file; an empty pages slice exports every page.
func (c *Files) PDF(ctx context.Context, id FileID, pages []int, p Progress) (string, error) {
	return c.files.PDF(ctx, id, pages, p)
}

// PNGs renders the note id to page-ordered PNGs (polling while conversion runs)
// and returns caller-owned temporary files. Any failure removes all pages.
func (c *Files) PNGs(ctx context.Context, id FileID, p Progress) (files []string, err error) {
	return c.files.PNGs(ctx, id, p)
}

// RecycleBin reads all recycle-bin pages and deduplicates repeated identifiers,
// keeping the latest entry. An empty order defaults to [SortTime].
func (c *Files) RecycleBin(ctx context.Context, order SortOrder, ascending bool) ([]RecycledEntry, error) {
	return c.files.RecycleBin(ctx, order, ascending)
}

// Restore restores recycled ids to their former parent folders.
func (c *Files) Restore(ctx context.Context, ids []FileID) error {
	return c.files.Restore(ctx, ids)
}

// DeleteForever permanently removes the named recycled entries.
func (c *Files) DeleteForever(ctx context.Context, ids []FileID) error {
	return c.files.DeleteForever(ctx, ids)
}

// EmptyRecycleBin permanently removes every entry in the recycle bin.
func (c *Files) EmptyRecycleBin(ctx context.Context) error {
	return c.files.EmptyRecycleBin(ctx)
}

// DownloadLimit (2 GiB) bounds downloads when no byte size is declared.
const DownloadLimit = domain.DownloadLimit

// Files is a view of the identifier-based web API over the client's shared session.
type Files struct {
	files port.WebFiles
}

// Files returns a cheap view sharing the session and network services.
func (c *Client) Files() *Files {
	return &Files{files: c.files}
}
