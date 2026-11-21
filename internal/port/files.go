// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package port

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// WebFiles exposes file use cases to the facade.
type WebFiles interface {
	List(ctx context.Context, folder domain.FileID, order domain.SortOrder, ascending bool) ([]domain.Entry, error)
	Search(ctx context.Context, name string, order domain.SortOrder, ascending bool) ([]domain.Entry, error)
	Entry(ctx context.Context, name string, folder domain.FileID) (*domain.Entry, error)
	Folders(ctx context.Context, folder domain.FileID, excluding []domain.FileID) ([]domain.Folder, error)
	Capacity(ctx context.Context) (domain.Capacity, error)
	User(ctx context.Context) (domain.User, error)
	Location(ctx context.Context, id domain.FileID) (domain.Location, error)
	Download(ctx context.Context, id domain.FileID, size *int64, p Progress) (string, error)
	Upload(ctx context.Context, file, name string, folder domain.FileID, p Progress) error
	UploadBytes(ctx context.Context, data []byte, name string, folder domain.FileID) error
	CreateFolder(ctx context.Context, name string, parent domain.FileID) (domain.Entry, error)
	Delete(ctx context.Context, ids []domain.FileID, parent domain.FileID) error
	Move(ctx context.Context, ids []domain.FileID, parent, destination domain.FileID) error
	Copy(ctx context.Context, ids []domain.FileID, parent, destination domain.FileID) error
	Rename(ctx context.Context, id domain.FileID, name string) error
	PDF(ctx context.Context, id domain.FileID, pages []int, p Progress) (string, error)
	PNGs(ctx context.Context, id domain.FileID, p Progress) (files []string, err error)
	RecycleBin(ctx context.Context, order domain.SortOrder, ascending bool) ([]domain.RecycledEntry, error)
	Restore(ctx context.Context, ids []domain.FileID) error
	DeleteForever(ctx context.Context, ids []domain.FileID) error
	EmptyRecycleBin(ctx context.Context) error
}
