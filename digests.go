// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// DigestID is a signed 64-bit digest, group or tag identifier. Unlike [FileID],
// it marshals as a JSON number; it accepts numbers or strings when decoding.
type DigestID = domain.DigestID

// DigestGroup is a digest library. Digests reference a group by UniqueIdentifier.
type DigestGroup = domain.DigestGroup

// Digest is a passage captured from a document or entered directly, along with
// its source metadata, tags, typed comment, and handwritten comment reference.
type Digest = domain.Digest

// DigestFingerprint carries the summary fields returned by the hash endpoint
// for lightweight synchronization checks.
type DigestFingerprint = domain.DigestFingerprint

// DigestTag is a named tag that can be attached to digests.
type DigestTag = domain.DigestTag

// DigestDraft describes a new digest or a partial update. Nil fields are left
// unchanged on [Digests.Update]; UniqueIdentifier and FileID apply only on
// [Digests.Create].
type DigestDraft = domain.DigestDraft

// DigestHandwriting references an uploaded handwritten comment .mark file.
type DigestHandwriting = domain.DigestHandwriting

// Protocol constants for digests.
const (
	// TypedDigestSourceType (4) is the default SourceType recorded for a typed
	// digest not extracted from a document.
	TypedDigestSourceType = domain.TypedDigestSourceType

	// DigestBusyCode is the [RejectedError] code for overlapping Update or
	// Delete calls on the same digest through one [Client].
	DigestBusyCode = domain.DigestBusyCode
)

// NewDigestIdentifier generates a 32-character lowercase hexadecimal identifier.
func NewDigestIdentifier() string { return domain.NewDigestIdentifier() }

// Groups lists every non-deleted digest group in the account.
func (c *Digests) Groups(ctx context.Context) ([]DigestGroup, error) {
	return c.digests.Groups(ctx)
}

// CreateGroup creates a digest group with a tablet-compatible MD5
// fingerprint. An empty uniqueIdentifier mints a fresh 32-hex identifier.
func (c *Digests) CreateGroup(
	ctx context.Context,
	name string,
	description *string,
	uniqueIdentifier string,
) (DigestID, error) {
	return c.digests.CreateGroup(ctx, name, description, uniqueIdentifier)
}

// UpdateGroup updates a digest group's name or description, fetching its
// existing uniqueIdentifier first so the stored MD5 fingerprint stays accurate.
func (c *Digests) UpdateGroup(ctx context.Context, id DigestID, name string, description *string) error {
	return c.digests.UpdateGroup(ctx, id, name, description)
}

// DeleteGroup soft-deletes a digest group by numeric identifier, leaving any
// digests inside that group intact.
func (c *Digests) DeleteGroup(ctx context.Context, id DigestID) error {
	return c.digests.DeleteGroup(ctx, id)
}

// List reads non-deleted digests across all groups when groupIdentifier is
// empty, or within a single group's UniqueIdentifier when non-empty.
func (c *Digests) List(ctx context.Context, groupIdentifier string) ([]Digest, error) {
	return c.digests.List(ctx, groupIdentifier)
}

// ByID fetches digests matching the supplied numeric identifiers.
func (c *Digests) ByID(ctx context.Context, ids []DigestID) ([]Digest, error) {
	return c.digests.ByID(ctx, ids)
}

// Fingerprints lists lightweight digest fingerprints across all groups
// when groupIdentifier is empty, or within a single group when non-empty.
func (c *Digests) Fingerprints(ctx context.Context, groupIdentifier string) ([]DigestFingerprint, error) {
	return c.digests.Fingerprints(ctx, groupIdentifier)
}

// Create merges JSON metadata and computes a tablet-compatible fingerprint
// unless MD5Hash is supplied. An empty UniqueIdentifier is generated and a nil
// SourceType defaults to [TypedDigestSourceType]. Tags containing commas fail
// locally with [MalformedResponseError].
func (c *Digests) Create(ctx context.Context, draft DigestDraft) (DigestID, error) {
	return c.digests.Create(ctx, draft)
}

// Update applies non-nil draft fields. It reads the stored row when needed
// to recompute a changed fingerprint or merge Author into metadata; a supplied
// MD5Hash overrides computation. Tags containing commas fail locally with
// [MalformedResponseError]. Overlapping Update or Delete calls for the same
// id through one [Client] fail with [DigestBusyCode].
func (c *Digests) Update(ctx context.Context, id DigestID, draft DigestDraft) error {
	return c.digests.Update(ctx, id, draft)
}

// Delete soft-deletes a digest by numeric identifier.
func (c *Digests) Delete(ctx context.Context, id DigestID) error {
	return c.digests.Delete(ctx, id)
}

// Tags lists all digest tags in the account.
func (c *Digests) Tags(ctx context.Context) ([]DigestTag, error) {
	return c.digests.Tags(ctx)
}

// CreateTag creates a named digest tag; names containing commas fail locally
// with [MalformedResponseError].
func (c *Digests) CreateTag(ctx context.Context, name string) (DigestID, error) {
	return c.digests.CreateTag(ctx, name)
}

// RenameTag renames an existing digest tag; names containing commas fail
// locally with [MalformedResponseError].
func (c *Digests) RenameTag(ctx context.Context, id DigestID, name string) error {
	return c.digests.RenameTag(ctx, id, name)
}

// DeleteTag deletes a digest tag by numeric identifier.
func (c *Digests) DeleteTag(ctx context.Context, id DigestID) error {
	return c.digests.DeleteTag(ctx, id)
}

// UploadHandwriting uploads a handwritten comment .mark file and returns
// the reference to attach to DigestDraft.Handwriting. An empty name mints a
// fresh <identifier>.mark filename.
func (c *Digests) UploadHandwriting(
	ctx context.Context,
	file, name string,
	p Progress,
) (DigestHandwriting, error) {
	return c.digests.UploadHandwriting(ctx, file, name, p)
}

// DownloadHandwriting returns a temporary file the caller must remove, checking
// HandwritingMD5 when supplied. An absent or empty HandwritingInnerName returns
// ("", nil) without a request.
func (c *Digests) DownloadHandwriting(
	ctx context.Context,
	digest Digest,
	p Progress,
) (string, error) {
	return c.digests.DownloadHandwriting(ctx, digest, p)
}

// Digests is a view of the digest API over the client's shared session.
type Digests struct {
	digests port.DigestService
}

// Digests returns a cheap view sharing the session, channel and byte-transfer services.
func (c *Client) Digests() *Digests {
	return &Digests{digests: c.digests}
}
