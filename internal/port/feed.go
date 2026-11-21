// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package port

import (
	"context"
	"iter"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// ChangeFeed exposes the Socket.IO change-feed use cases to the facade.
type ChangeFeed interface {
	Changes(context.Context) iter.Seq2[domain.Change, error]
	Watch(context.Context) iter.Seq2[domain.Change, error]
}
