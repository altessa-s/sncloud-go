// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package digests

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// List returns non-deleted digests in groupIdentifier, or across all groups
// when groupIdentifier is empty.
func (c *Client) List(ctx context.Context, groupIdentifier string) ([]domain.Digest, error) {
	extract := func(p pageAnswer[domain.Marked[domain.Digest]]) []domain.Marked[domain.Digest] {
		return p.SummaryDOList
	}
	rows, err := collectPages(c, ctx, domain.PathDigests, groupIdentifier, c.session.Pin(), extract)
	if err != nil {
		return nil, err
	}
	rows = domain.UniqueLast(rows, func(r domain.Marked[domain.Digest]) domain.DigestID { return r.Value.ID })
	out := make([]domain.Digest, 0, len(rows))
	for _, r := range rows {
		if !r.Deleted {
			out = append(out, r.Value)
		}
	}
	return out, nil
}

// ByID returns the non-deleted digests matching ids. An empty slice returns
// immediately without making a network request.
func (c *Client) ByID(ctx context.Context, ids []domain.DigestID) ([]domain.Digest, error) {
	return c.byID(ctx, ids, nil)
}

func (c *Client) byID(ctx context.Context, ids []domain.DigestID, pin *domain.CredentialPin) ([]domain.Digest, error) {
	if len(ids) == 0 {
		return []domain.Digest{}, nil
	}
	var ans digestListAnswer
	if err := c.call(ctx, domain.PathDigestsByID, methodPost, byIDsRequest{IDs: ids}, false, pin, &ans); err != nil {
		return nil, err
	}
	if ans.SummaryDOList == nil {
		return []domain.Digest{}, nil
	}
	return ans.SummaryDOList, nil
}

// Fingerprints returns lightweight sync fingerprints in groupIdentifier, or
// across all groups when groupIdentifier is empty.
func (c *Client) Fingerprints(ctx context.Context, groupIdentifier string) ([]domain.DigestFingerprint, error) {
	extract := func(p pageAnswer[domain.DigestFingerprint]) []domain.DigestFingerprint { return p.SummaryInfoVOList }
	rows, err := collectPages(c, ctx, domain.PathDigestHashes, groupIdentifier, c.session.Pin(), extract)
	if err != nil {
		return nil, err
	}
	return domain.UniqueLast(rows, func(f domain.DigestFingerprint) domain.DigestID { return f.ID }), nil
}

// Create creates a digest with tablet-compatible metadata and MD5 fingerprint.
// As on the tablet, unique_identifier and author live inside the metadata JSON
// object while the top-level columns of those names stay empty, and typed
// digests fill empty strings for unset group, sourcePath and comment fields.
func (c *Client) Create(ctx context.Context, draft domain.DigestDraft) (domain.DigestID, error) {
	if err := domain.ValidateDigestDraft(draft); err != nil {
		return 0, err
	}
	uid := draft.UniqueIdentifier
	if uid == "" {
		uid = domain.NewDigestIdentifier()
	}
	if draft.SourceType == nil {
		src := domain.TypedDigestSourceType
		draft.SourceType = &src
	}
	empty := ""
	if draft.SourcePath == nil {
		draft.SourcePath = &empty
	}
	if draft.GroupIdentifier == nil {
		draft.GroupIdentifier = &empty
	}
	if draft.Comment == nil {
		draft.Comment = &empty
	}
	mergedMeta, err := domain.MergeDigestMetadata(draft.Metadata, &uid, draft.Author)
	if err != nil {
		return 0, err
	}
	draft.Metadata = &mergedMeta
	var hwName *string
	if draft.Handwriting != nil {
		hwName = &draft.Handwriting.Name
	}
	fp := draft.MD5Hash
	if fp == nil {
		sum := domain.ComputeDigestFingerprint(domain.DigestFingerprintFields{
			Content:         draft.Content,
			Group:           draft.GroupIdentifier,
			SourceType:      draft.SourceType,
			SourcePath:      draft.SourcePath,
			Metadata:        draft.Metadata,
			Comment:         draft.Comment,
			HandwritingName: hwName,
		})
		fp = &sum
	}
	stamp := c.clock.Now().UnixMilli()
	var made madeAnswer
	err = c.call(ctx, domain.PathAddDigest, methodPost, newDigestBody(draft, nil, fp, &stamp, stamp), true, nil, &made)
	if err != nil {
		return 0, err
	}
	return made.identifier()
}

// Update updates the non-nil fields of draft on digest id under the session's
// per-digest write lock, reading the stored row first to recompute md5Hash
// whenever any fingerprinted field or author changes.
func (c *Client) Update(ctx context.Context, id domain.DigestID, draft domain.DigestDraft) error {
	if err := domain.ValidateDigestDraft(draft); err != nil {
		return err
	}
	return c.exclusively(id, func() error {
		return c.writeDigest(ctx, id, draft)
	})
}

func (c *Client) writeDigest(ctx context.Context, id domain.DigestID, draft domain.DigestDraft) error {
	pin := c.session.Pin()
	fp := draft.MD5Hash
	hashed := draft.Content != nil || draft.GroupIdentifier != nil || draft.SourceType != nil ||
		draft.SourcePath != nil || draft.Metadata != nil || draft.Comment != nil ||
		draft.Author != nil || draft.Handwriting != nil
	if hashed && (fp == nil || draft.Author != nil) {
		existing, err := c.byID(ctx, []domain.DigestID{id}, pin)
		if err != nil {
			return err
		}
		if len(existing) == 0 {
			return &domain.NotFoundError{Code: "E0340"}
		}
		cur := existing[0]
		if draft.Author != nil {
			baseMeta := cur.Metadata
			if draft.Metadata != nil {
				baseMeta = draft.Metadata
			}
			merged, err := domain.MergeDigestMetadata(baseMeta, nil, draft.Author)
			if err != nil {
				return err
			}
			draft.Metadata = &merged
		}
		if fp == nil {
			hwName := cur.HandwritingName
			if draft.Handwriting != nil {
				hwName = &draft.Handwriting.Name
			}
			sum := domain.ComputeDigestFingerprint(domain.DigestFingerprintFields{
				Content:         coalesce(draft.Content, cur.Content),
				Group:           coalesce(draft.GroupIdentifier, cur.GroupIdentifier),
				SourceType:      coalesce(draft.SourceType, cur.SourceType),
				SourcePath:      coalesce(draft.SourcePath, cur.SourcePath),
				Metadata:        coalesce(draft.Metadata, cur.Metadata),
				Comment:         coalesce(draft.Comment, cur.Comment),
				HandwritingName: hwName,
			})
			fp = &sum
		}
	}
	stamp := c.clock.Now().UnixMilli()
	return c.call(ctx, domain.PathUpdateDigest, methodPut, newDigestBody(draft, &id, fp, nil, stamp), true, pin, nil)
}

// Delete marks digest id deleted under the session's per-digest write lock.
func (c *Client) Delete(ctx context.Context, id domain.DigestID) error {
	return c.exclusively(id, func() error {
		return c.call(ctx, domain.PathDeleteDigest, methodDelete, byIDRequest{ID: id}, true, nil, nil)
	})
}

func coalesce[T any](preferred, fallback *T) *T {
	if preferred != nil {
		return preferred
	}
	return fallback
}
