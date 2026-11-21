// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package digests

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Tags lists every digest tag on the account.
func (c *Client) Tags(ctx context.Context) ([]domain.DigestTag, error) {
	var ans tagListAnswer
	if err := c.call(ctx, domain.PathDigestTags, methodGet, nil, false, nil, &ans); err != nil {
		return nil, err
	}
	if ans.SummaryTagDOList == nil {
		return []domain.DigestTag{}, nil
	}
	return ans.SummaryTagDOList, nil
}

// CreateTag creates a named digest tag and returns its identifier. A taken name
// returns ErrAlreadyExists.
func (c *Client) CreateTag(ctx context.Context, name string) (domain.DigestID, error) {
	if err := domain.ValidateDigestTag(name); err != nil {
		return 0, err
	}
	var made madeAnswer
	if err := c.call(ctx, domain.PathAddDigestTag, methodPost, tagNameRequest{Name: name}, true, nil, &made); err != nil {
		return 0, err
	}
	return made.identifier()
}

// RenameTag renames an existing digest tag. A taken name returns ErrAlreadyExists
// and an unknown identifier returns ErrNotFound.
func (c *Client) RenameTag(ctx context.Context, id domain.DigestID, name string) error {
	if err := domain.ValidateDigestTag(name); err != nil {
		return err
	}
	return c.call(ctx, domain.PathUpdateDigestTag, methodPut, tagChangeRequest{ID: id, Name: name}, true, nil, nil)
}

// DeleteTag permanently deletes a digest tag by identifier; digests already
// carrying its name keep it.
func (c *Client) DeleteTag(ctx context.Context, id domain.DigestID) error {
	return c.call(ctx, domain.PathDeleteDigestTag, methodDelete, byIDRequest{ID: id}, true, nil, nil)
}
