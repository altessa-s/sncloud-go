// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// DeviceEntry describes a file or folder in the path-addressed tablet storage tree.
//
// Device listings normalize Path automatically when PathDisplay names the
// containing folder. The remaining fields describe the entry and its metadata.
type DeviceEntry = domain.DeviceEntry

// SpaceUsage reports used and optionally allocated bytes from the device API.
type SpaceUsage = domain.SpaceUsage

// ByteRange selects a zero-based byte span inclusive at both ends for ranged
// [Device.Download] calls.
type ByteRange = domain.ByteRange

// List reads entries under path ("/" or "" for the root), optionally
// descending into all subfolders when recursive is true.
func (c *Device) List(ctx context.Context, path string, recursive bool) ([]DeviceEntry, error) {
	return c.device.List(ctx, path, recursive)
}

// ListFolder reads entries under folder id; this endpoint uniquely encodes the
// identifier as a JSON number rather than a string.
func (c *Device) ListFolder(ctx context.Context, id FileID, recursive bool) ([]DeviceEntry, error) {
	return c.device.ListFolder(ctx, id, recursive)
}

// Entry looks up id and returns (nil, nil) when the service reports entriesVO:null.
func (c *Device) Entry(ctx context.Context, id FileID) (*DeviceEntry, error) {
	return c.device.Entry(ctx, id)
}

// EntryAt looks up a normalized full path, returning (nil, nil) when absent.
func (c *Device) EntryAt(ctx context.Context, path string) (*DeviceEntry, error) {
	return c.device.EntryAt(ctx, path)
}

// SpaceUsage reads used and optionally allocated storage bytes.
func (c *Device) SpaceUsage(ctx context.Context) (SpaceUsage, error) {
	return c.device.SpaceUsage(ctx)
}

// Download writes id to a temporary file the caller must remove. Full downloads
// check MD5 when the server supplies it; ranged downloads check HTTP 206,
// Content-Range and the byte count instead of the whole-file hash.
func (c *Device) Download(ctx context.Context, id FileID, span *ByteRange, p Progress) (string, error) {
	return c.device.Download(ctx, id, span, p)
}

// Upload streams the local path file to destination path (including filename)
// and returns the resulting [DeviceEntry].
func (c *Device) Upload(ctx context.Context, file, path string, p Progress) (DeviceEntry, error) {
	return c.device.Upload(ctx, file, path, p)
}

// UploadBytes writes in-memory bytes to destination path through the same
// upload path as [Device.Upload].
func (c *Device) UploadBytes(ctx context.Context, data []byte, path string) (DeviceEntry, error) {
	return c.device.UploadBytes(ctx, data, path)
}

// CreateFolder creates a folder at full destination path, optionally letting
// the service rename collisions when autorename is true.
func (c *Device) CreateFolder(ctx context.Context, path string, autorename bool) (DeviceEntry, error) {
	return c.device.CreateFolder(ctx, path, autorename)
}

// Delete permanently removes id, bypassing the recycle bin.
func (c *Device) Delete(ctx context.Context, id FileID) error {
	return c.device.Delete(ctx, id)
}

// Move relocates id to full destination path (including the new filename).
func (c *Device) Move(ctx context.Context, id FileID, path string, autorename bool) (*DeviceEntry, error) {
	return c.device.Move(ctx, id, path, autorename)
}

// Copy copies id to full destination path (including the copy's filename).
func (c *Device) Copy(ctx context.Context, id FileID, path string, autorename bool) (*DeviceEntry, error) {
	return c.device.Copy(ctx, id, path, autorename)
}

// WithSyncLock acquires the account sync lock, invokes body(ctx, hasRemoteData)
// where hasRemoteData defaults to true when the server omits synType. After
// acquisition, release is attempted even on cancellation, with a separate
// five-second deadline and the acquiring login. A release error is returned
// only if body succeeded. Overlapping calls on the same [Client] and device
// return [ErrSyncLocked].
func (c *Device) WithSyncLock(ctx context.Context, body func(context.Context, bool) error) (err error) {
	return c.device.WithSyncLock(ctx, body)
}

// Device is a view of the path-based device API over the client's shared session.
type Device struct {
	device port.DeviceFiles
}

// Device returns a cheap view sharing the session and network services.
func (c *Client) Device() *Device {
	return &Device{device: c.device}
}
