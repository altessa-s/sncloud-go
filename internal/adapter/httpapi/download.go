// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"io"
	"math"
	"net/http"
	"os"

	"github.com/altessa-s/sncloud-go/internal/adapter/neterr"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// Default ceiling for downloads with no caller limit.
const defaultDownloadLimit = 1 << 30

// Downloaded files are readable only by their owner.
const privateFileMode = 0600

// Download streams bytes to Dest, or to a temporary file when Dest is empty.
// A failed download removes the partial file. Limit is enforced before and
// during the read; nonpositive limits use a 1 GiB ceiling.
func (t *Transport) Download(ctx context.Context, r port.Request) (out port.Response, err error) {
	if r.Limit == math.MaxInt64 {
		return out, &domain.MalformedResponseError{What: "unsupported download ceiling"}
	}
	child, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := t.request(child, r, nil)
	if err != nil {
		return out, err
	}
	resp, err := t.do(ctx, t.download, req, nil)
	if err != nil {
		return out, err
	}
	// Reading reports body errors; closing only releases the connection.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return t.read(ctx, req, resp, cancel)
	}
	out = response(resp)
	limit := r.Limit
	if limit <= 0 {
		limit = defaultDownloadLimit
	}
	if resp.ContentLength > limit {
		return out, &domain.OversizedResponseError{Bytes: resp.ContentLength, Limit: limit}
	}
	var f *os.File
	if r.Dest == "" {
		f, err = os.CreateTemp("", "sncloud-download-*")
	} else {
		f, err = os.OpenFile(r.Dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateFileMode)
	}
	if err != nil {
		return out, err
	}
	defer func() {
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			// Cleanup is best effort; preserve the transfer or close failure.
			_ = os.Remove(f.Name())
		}
	}()
	reader := &progressReader{
		src:    io.LimitReader(resp.Body, limit+1),
		total:  resp.ContentLength,
		report: r.Progress,
	}
	n, err := io.Copy(f, reader)
	if err != nil {
		return out, neterr.Classify(ctx, err)
	}
	if n > limit {
		return out, &domain.OversizedResponseError{Bytes: n, Limit: limit}
	}
	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return out, neterr.Classify(ctx, io.ErrUnexpectedEOF)
	}
	out.File = f.Name()
	t.log(ctx, "receive file", req, resp.StatusCode, resp.Header, nil)
	return out, nil
}
