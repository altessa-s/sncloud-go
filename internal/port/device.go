// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package port

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// DeviceFiles exposes file use cases to the facade.
type DeviceFiles interface {
	List(ctx context.Context, path string, recursive bool) ([]domain.DeviceEntry, error)
	ListFolder(ctx context.Context, id domain.FileID, recursive bool) ([]domain.DeviceEntry, error)
	Entry(ctx context.Context, id domain.FileID) (*domain.DeviceEntry, error)
	EntryAt(ctx context.Context, path string) (*domain.DeviceEntry, error)
	SpaceUsage(ctx context.Context) (domain.SpaceUsage, error)
	Download(ctx context.Context, id domain.FileID, span *domain.ByteRange, p Progress) (string, error)
	Upload(ctx context.Context, file, path string, p Progress) (domain.DeviceEntry, error)
	UploadBytes(ctx context.Context, data []byte, path string) (domain.DeviceEntry, error)
	CreateFolder(ctx context.Context, path string, autorename bool) (domain.DeviceEntry, error)
	Delete(ctx context.Context, id domain.FileID) error
	Move(ctx context.Context, id domain.FileID, path string, autorename bool) (*domain.DeviceEntry, error)
	Copy(ctx context.Context, id domain.FileID, path string, autorename bool) (*domain.DeviceEntry, error)
	WithSyncLock(ctx context.Context, body func(context.Context, bool) error) (err error)
}
