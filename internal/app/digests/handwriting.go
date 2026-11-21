// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package digests

import (
	"context"
	"net/url"

	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// UploadHandwriting measures file, requests an upload slot pinned to the
// initiating credential, transfers the bytes through the shared byte
// path, and returns the DigestHandwriting reference to attach to a DigestDraft.
// An empty name mints a tablet-shaped <32 hex>.mark filename.
func (c *Client) UploadHandwriting(
	ctx context.Context,
	file, name string,
	p port.Progress,
) (domain.DigestHandwriting, error) {
	if name == "" {
		name = domain.NewDigestIdentifier() + ".mark"
	}
	o, err := transfers.Measure(file, name)
	if err != nil {
		return domain.DigestHandwriting{}, err
	}
	// Apply, storage signing and form credentials must stay pinned to one login
	// even if another goroutine logs out or switches accounts mid-transfer.
	pin := c.session.Pin()
	identity, err := c.session.Identity()
	if err != nil {
		return domain.DigestHandwriting{}, err
	}
	var slot transfers.UploadSlot
	err = c.call(ctx, domain.PathApplyDigestHandwriting, methodPost, handwritingApplyRequest{
		FileName:    o.Name,
		EquipmentNo: domain.EquipmentNo(identity),
	}, false, pin, &slot)
	if err != nil {
		return domain.DigestHandwriting{}, err
	}
	_, err = c.session.AccessToken(pin)
	if err != nil {
		return domain.DigestHandwriting{}, err
	}
	stored, err := c.bytes.Send(
		ctx,
		o,
		slot,
		transfers.NewSigner(c.channel, c.session, slot.FileServer, pin),
		&transfers.FormServer{Host: c.endpoints.AppHost, Token: func() (string, error) { return c.session.AccessToken(pin) }},
		p,
	)
	if err != nil {
		return domain.DigestHandwriting{}, err
	}
	inner := slot.InnerName
	if inner == "" {
		inner = stored
	}
	return domain.DigestHandwriting{
		Name:      o.Name,
		InnerName: inner,
		MD5:       o.MD5,
	}, nil
}

// DownloadHandwriting downloads the handwritten comment file of digest into a
// verified temporary file owned by the caller, returning an empty path without
// network I/O when digest carries no handwritten comment.
func (c *Client) DownloadHandwriting(
	ctx context.Context,
	digest domain.Digest,
	p port.Progress,
) (string, error) {
	if digest.HandwritingInnerName == nil || *digest.HandwritingInnerName == "" {
		return "", nil
	}
	var link handwritingLinkAnswer
	if err := c.call(ctx, domain.PathDownloadDigestHandwriting, methodPost, byIDRequest{ID: digest.ID}, false, c.session.Pin(), &link); err != nil {
		return "", err
	}
	if link.URL == nil || *link.URL == "" {
		return "", &domain.MalformedResponseError{What: "download/summary named no link"}
	}
	if u, err := url.Parse(*link.URL); err != nil || u.Scheme == "" || u.Host == "" {
		return "", &domain.MalformedResponseError{What: "download/summary named no link"}
	}
	var md5 string
	if digest.HandwritingMD5 != nil {
		md5 = *digest.HandwritingMD5
	}
	return c.bytes.Fetch(
		ctx,
		transfers.ServedFrom(*link.URL, c.endpoints.AppHost),
		transfers.DownloadLimit,
		md5,
		nil,
		p,
	)
}
