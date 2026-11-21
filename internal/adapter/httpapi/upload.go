// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/altessa-s/sncloud-go/internal/port"
)

type progressBody struct {
	progressReader
	file *os.File
}

// Close releases the handle owned by this replay attempt.
func (b *progressBody) Close() error { return b.file.Close() }

// Upload streams a file and bounds its response like a normal protocol call.
func (t *Transport) Upload(ctx context.Context, r port.Request) (port.Response, error) {
	f, err := os.Open(r.File)
	if err != nil {
		return port.Response{}, err
	}
	// The source is read-only; transfer errors are reported by the reader.
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return port.Response{}, err
	}
	offset, length := int64(0), st.Size()
	if r.Section != nil {
		offset, length = r.Section.Offset, r.Section.Length
		if offset < 0 || length < 0 {
			return port.Response{}, fmt.Errorf("invalid upload section %d+%d: %w", offset, length, os.ErrInvalid)
		}
		if offset > st.Size() || length > st.Size()-offset {
			return port.Response{}, fmt.Errorf("upload source shorter than section %d+%d: %w", offset, length, io.ErrUnexpectedEOF)
		}
	}
	child, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := t.request(child, r, &progressReader{src: io.NewSectionReader(f, offset, length), total: length, report: r.Progress})
	if err != nil {
		return port.Response{}, err
	}
	req.ContentLength = length
	// A same-origin 307/308 may replay the file. Each attempt owns its handle;
	// a cross-origin replay is still refused by redirect before sending.
	req.GetBody = func() (io.ReadCloser, error) {
		next, openErr := os.Open(r.File)
		if openErr != nil {
			return nil, openErr
		}
		return &progressBody{
			src: io.NewSectionReader(next, offset, length), total: length, report: r.Progress,
			file: next,
		}, nil
	}
	resp, err := t.do(ctx, t.upload, req, nil)
	if err != nil {
		return port.Response{}, err
	}
	// Reading reports body errors; closing only releases the connection.
	defer func() { _ = resp.Body.Close() }()
	return t.read(ctx, req, resp, cancel)
}
