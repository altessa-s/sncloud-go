// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"io"

	"github.com/altessa-s/sncloud-go/internal/port"
)

type progressReader struct {
	src         io.Reader
	done, total int64
	report      port.Progress
}

// Read reports cumulative bytes after each successful read.
func (r *progressReader) Read(p []byte) (int, error) {
	n, e := r.src.Read(p)
	r.done += int64(n)
	if n > 0 && r.report != nil {
		r.report(r.done, r.total)
	}
	return n, e
}
