// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package port

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// DigestService exposes digest, group, tag and handwriting use cases to the facade.
type DigestService interface {
	Groups(context.Context) ([]domain.DigestGroup, error)
	CreateGroup(ctx context.Context, name string, description *string, uniqueIdentifier string) (domain.DigestID, error)
	UpdateGroup(ctx context.Context, id domain.DigestID, name string, description *string) error
	DeleteGroup(ctx context.Context, id domain.DigestID) error
	List(ctx context.Context, groupIdentifier string) ([]domain.Digest, error)
	ByID(ctx context.Context, ids []domain.DigestID) ([]domain.Digest, error)
	Fingerprints(ctx context.Context, groupIdentifier string) ([]domain.DigestFingerprint, error)
	Create(context.Context, domain.DigestDraft) (domain.DigestID, error)
	Update(context.Context, domain.DigestID, domain.DigestDraft) error
	Delete(context.Context, domain.DigestID) error
	Tags(context.Context) ([]domain.DigestTag, error)
	CreateTag(ctx context.Context, name string) (domain.DigestID, error)
	RenameTag(ctx context.Context, id domain.DigestID, name string) error
	DeleteTag(ctx context.Context, id domain.DigestID) error
	UploadHandwriting(ctx context.Context, file, name string, p Progress) (domain.DigestHandwriting, error)
	DownloadHandwriting(ctx context.Context, digest domain.Digest, p Progress) (string, error)
}
