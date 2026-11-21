// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package port

import (
	"context"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// TaskService exposes to-do list and task use cases to the facade.
type TaskService interface {
	Lists(context.Context) ([]domain.TaskList, error)
	List(context.Context, string) (domain.TaskList, error)
	CreateList(ctx context.Context, title, id string) (domain.TaskList, error)
	RenameList(ctx context.Context, id, title string) error
	DeleteList(ctx context.Context, id string) error
	All(context.Context) ([]domain.Task, error)
	InList(ctx context.Context, listID string) ([]domain.Task, error)
	Create(context.Context, domain.Task) (domain.Task, error)
	Update(context.Context, domain.Task) (domain.Task, error)
	Delete(ctx context.Context, id string) error
}
