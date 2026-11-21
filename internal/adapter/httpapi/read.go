// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"io"
	"net/http"

	"github.com/altessa-s/sncloud-go/internal/adapter/neterr"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

func (t *Transport) read(ctx context.Context, req *http.Request, resp *http.Response, cancel context.CancelCauseFunc) (port.Response, error) {
	out := response(resp)
	failed := resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices
	if resp.ContentLength > BodyLimit {
		if failed {
			return out, nil
		}
		return out, &domain.OversizedResponseError{Bytes: resp.ContentLength, Limit: BodyLimit}
	}
	watch := watchIdle(resp.Body, t.config.IdleReadTimeout, cancel)
	defer watch.stop()
	b, err := io.ReadAll(io.LimitReader(watch, BodyLimit+1))
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	// HTTP status is authoritative on failure. An oversized, truncated or stalled
	// error document must not hide a refusal or change its retry classification.
	if failed {
		if err == nil && int64(len(b)) <= BodyLimit && !watch.expired.Load() {
			out.Body = b
		}
		t.log(ctx, "receive", req, resp.StatusCode, resp.Header, out.Body)
		return out, nil
	}
	if watch.expired.Load() {
		return out, &domain.TransportError{Reason: domain.ReasonTimedOut}
	}
	if err != nil {
		return out, neterr.Classify(ctx, err)
	}
	if int64(len(b)) > BodyLimit {
		return out, &domain.OversizedResponseError{Bytes: int64(len(b)), Limit: BodyLimit}
	}
	out.Body = b
	t.log(ctx, "receive", req, resp.StatusCode, resp.Header, b)
	return out, nil
}
