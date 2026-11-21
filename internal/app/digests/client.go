// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package digests implements the digest, group, tag, fingerprint and
// handwritten-comment protocol use cases.
//
// Three server behaviors shape the client:
//   - The server stores md5Hash blindly without computing or checking it, while
//     tablets compare md5Hash during sync to detect edits. Groups hash
//     uniqueIdentifier+name (omitting description because tablets do not set
//     one), and digests hash content, group, sourceType, sourcePath, metadata,
//     commentStr and commentHandwriteName in that order with unique_identifier
//     and author stored inside the metadata JSON object.
//   - Recomputing md5Hash on a partial update reads the stored digest first,
//     and the service offers no conditional write. Each session therefore
//     allows only one in-flight write or delete per digest ID, refusing
//     overlapping calls on the same session with SNCloud.busy so two concurrent
//     callers cannot compute fingerprints from pre-write snapshots that
//     disagree with the combined row.
//   - Digest tags are stored on the server in one comma-separated string
//     column, so tags containing commas are rejected locally before sending.
package digests

import (
	"context"
	"fmt"

	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

const (
	pageSize       = 100
	defaultPageMax = 10000
	methodPost     = "POST"
	methodPut      = "PUT"
	methodGet      = "GET"
	methodDelete   = "DELETE"
)

// Session supplies operation-pinned credentials, headers, device identity and
// per-digest write exclusion without coupling to the concrete session type.
type Session interface {
	port.FileSession
	ClaimDigestWrite(id domain.FileID) bool
	ReleaseDigestWrite(id domain.FileID)
}

// Client operates on digests, groups, tags and handwritten comments.
type Client struct {
	endpoints domain.Endpoints
	channel   port.Channel
	session   Session
	bytes     *transfers.Transfers
	clock     port.Clock
	pageMax   int
}

// New constructs a digest client sharing the session, channel and byte transfers.
func New(e domain.Endpoints, ch port.Channel, s Session, b *transfers.Transfers, clock port.Clock) *Client {
	return &Client{
		endpoints: e,
		channel:   ch,
		session:   s,
		bytes:     b,
		clock:     clock,
		pageMax:   defaultPageMax,
	}
}

// call routes digest requests over the app host with the installation's channel
// header attached (through Authorized) so self-originated changes are suppressed
// on this installation's Socket.IO change feed.
func (c *Client) call(ctx context.Context, path, method string, body any, changes bool, pin *domain.CredentialPin, out any) error {
	req := port.Call{Path: path, Method: method, Changes: changes, Body: body}
	return transfers.Authorized(ctx, c.channel, c.session, req, pin, out)
}

// exclusively serializes writes and deletes to one digest within a session.
// Because a hashed update reads the existing row first and the server has no
// conditional write, overlapping writes from the same session would leave an
// md5Hash computed from stale fields; concurrent writes from another device
// still cannot be prevented by the protocol.
func (c *Client) exclusively(id domain.DigestID, fn func() error) error {
	key := domain.FileID(id)
	if !c.session.ClaimDigestWrite(key) {
		return &domain.RejectedError{
			Code:    domain.DigestBusyCode,
			Message: fmt.Sprintf("another write to digest %s is running on this session", id),
		}
	}
	defer c.session.ReleaseDigestWrite(key)
	return fn()
}

func collectPages[T any](
	c *Client,
	ctx context.Context,
	path string,
	parent string,
	pin *domain.CredentialPin,
	extract func(pageAnswer[T]) []T,
) ([]T, error) {
	var all []T
	for page := 1; page <= c.pageMax; page++ {
		var ans pageAnswer[T]
		err := c.call(ctx, path, methodPost, pageQuery{
			Page:                   page,
			Size:                   pageSize,
			ParentUniqueIdentifier: parent,
		}, false, pin, &ans)
		if err != nil {
			return nil, err
		}
		rows := extract(ans)
		all = append(all, rows...)
		// Without a server-reported page count, a short page marks the end.
		var last bool
		if ans.TotalPages != nil {
			last = int64(page) >= int64(*ans.TotalPages)
		} else {
			last = len(rows) < pageSize
		}
		if len(rows) == 0 || last {
			return all, nil
		}
	}
	return nil, &domain.MalformedResponseError{What: fmt.Sprintf("the digest listing ran past %d pages", c.pageMax)}
}

var _ port.DigestService = (*Client)(nil)
