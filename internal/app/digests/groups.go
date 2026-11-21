// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package digests

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Groups returns all non-deleted digest groups, deduplicated by last arrival.
func (c *Client) Groups(ctx context.Context) ([]domain.DigestGroup, error) {
	return c.groups(ctx, c.session.Pin())
}

func (c *Client) groups(ctx context.Context, pin *domain.CredentialPin) ([]domain.DigestGroup, error) {
	extract := func(p pageAnswer[domain.Marked[domain.DigestGroup]]) []domain.Marked[domain.DigestGroup] {
		return p.SummaryDOList
	}
	rows, err := collectPages(c, ctx, domain.PathDigestGroups, "", pin, extract)
	if err != nil {
		return nil, err
	}
	rows = domain.UniqueLast(rows, func(r domain.Marked[domain.DigestGroup]) domain.DigestID { return r.Value.ID })
	groups := make([]domain.DigestGroup, 0, len(rows))
	for _, r := range rows {
		if !r.Deleted {
			groups = append(groups, r.Value)
		}
	}
	return groups, nil
}

// CreateGroup creates a digest group with the tablet's md5(uniqueIdentifier+name)
// fingerprint. When uniqueIdentifier is empty, a fresh 32-hex identifier is minted.
func (c *Client) CreateGroup(ctx context.Context, name string, description *string, uniqueIdentifier string) (domain.DigestID, error) {
	uid := uniqueIdentifier
	if uid == "" {
		uid = domain.NewDigestIdentifier()
	}
	stamp := c.clock.Now().UnixMilli()
	var made madeAnswer
	err := c.call(ctx, domain.PathAddDigestGroup, methodPost, newGroupRequest{
		UniqueIdentifier: uid,
		Name:             name,
		Description:      description,
		MD5Hash:          domain.GroupFingerprint(uid, name),
		CreationTime:     stamp,
		LastModifiedTime: stamp,
	}, true, nil, &made)
	if err != nil {
		return 0, err
	}
	return made.identifier()
}

// UpdateGroup renames a group or updates its description. Because the update
// payload carries id rather than uniqueIdentifier, the group is looked up first
// so md5(uniqueIdentifier+name) stays consistent with tablet sync.
func (c *Client) UpdateGroup(ctx context.Context, id domain.DigestID, name string, description *string) error {
	pin := c.session.Pin()
	groups, err := c.groups(ctx, pin)
	if err != nil {
		return err
	}
	var uid string
	found := false
	for _, g := range groups {
		if g.ID == id {
			uid = g.UniqueIdentifier
			found = true
			break
		}
	}
	if !found {
		return &domain.NotFoundError{Code: "E0339"}
	}
	return c.call(ctx, domain.PathUpdateDigestGroup, methodPut, groupChangeRequest{
		ID:               id,
		Name:             name,
		Description:      description,
		MD5Hash:          domain.GroupFingerprint(uid, name),
		LastModifiedTime: c.clock.Now().UnixMilli(),
	}, true, pin, nil)
}

// DeleteGroup marks a digest group deleted; digests inside it remain untouched.
func (c *Client) DeleteGroup(ctx context.Context, id domain.DigestID) error {
	return c.call(ctx, domain.PathDeleteDigestGroup, methodDelete, byIDRequest{ID: id}, true, nil, nil)
}
