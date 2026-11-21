// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package transfers

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/app/retry"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// ByteRange is inclusive at both ends.
type ByteRange = domain.ByteRange

// Fetch downloads without account credentials, bounds the body and verifies its hash.
// Expired signatures can answer HTTP 200 with XML instead of file bytes.
// Enforce the size ceiling and declared MD5 before handing the file to the caller.
// A range is verified against Content-Range instead of the whole-file checksum.
func (t *Transfers) Fetch(ctx context.Context, target string, limit int64, md5 string, span *ByteRange, progress port.Progress) (string, error) {
	if _, err := storageURL(target); err != nil {
		return "", err
	}
	if limit < 0 || limit == math.MaxInt64 {
		return "", malformed("unsupported download ceiling")
	}
	h := make(port.Header)
	if span != nil {
		if span.First < 0 || span.Last < span.First || span.Last == math.MaxInt64 {
			return "", malformed("invalid byte range")
		}
		h.Set("Range", fmt.Sprintf("bytes=%d-%d", span.First, span.Last))
		limit = min(limit, span.Last-span.First+1)
	}
	var file string
	err := retry.Do(ctx, t.policy, t.clock, false, func() error {
		// The adapter treats zero as unspecified; enforce an explicitly empty file
		// after reading at most one byte.
		r, err := t.transport.Download(ctx, port.Request{Method: "GET", URL: target, Header: h, Limit: max(1, limit), Progress: progress})
		if err != nil {
			return err
		}
		err = accepted(r, target, t.clock.Now())
		if err == nil {
			err = checkSize(r.File, limit)
		}
		if err == nil && span != nil {
			err = served(*span, r)
		}
		if err != nil {
			_ = os.Remove(r.File) // Best-effort cleanup; preserve the transfer or integrity result.
			return err
		}
		file = r.File
		return nil
	})
	if err != nil {
		return "", err
	}
	if span == nil && md5 != "" {
		actual, e := Measure(file, "")
		if e != nil {
			_ = os.Remove(file) // Best-effort cleanup; preserve the transfer or integrity result.
			return "", e
		}
		if actual.MD5 != strings.ToLower(md5) {
			_ = os.Remove(file) // Best-effort cleanup; preserve the transfer or integrity result.
			return "", &domain.ChecksumMismatchError{Expected: strings.ToLower(md5), Actual: actual.MD5}
		}
	}
	return file, nil
}

// checkSize also enforces a declared zero size, which the transport treats as unspecified.
func checkSize(file string, limit int64) error {
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	if info.Size() > limit {
		return &domain.OversizedResponseError{Bytes: info.Size(), Limit: limit}
	}
	return nil
}

// served rejects ignored ranges and short responses except at the declared end of file.
func served(want ByteRange, r port.Response) error {
	if r.Status != statusPartialContent {
		return malformed("range request did not answer 206")
	}
	first, last, total, ok := contentRange(r.Header.Get("Content-Range"))
	if !ok {
		return malformed("missing or invalid Content-Range")
	}
	info, err := os.Stat(r.File)
	if err != nil {
		return err
	}
	if first != want.First || last > want.Last || (last != want.Last && last != total-1) || info.Size() != last-first+1 {
		return malformed("served range does not match requested bytes")
	}
	return nil
}

// contentRange parses inclusive byte bounds, retaining -1 for an unknown total.
func contentRange(s string) (first, last, total int64, ok bool) {
	unit, spec, found := strings.Cut(strings.TrimSpace(s), " ")
	if !found || !strings.EqualFold(unit, "bytes") {
		return
	}
	ends, size, found := strings.Cut(spec, "/")
	if !found {
		return
	}
	a, b, found := strings.Cut(ends, "-")
	if !found {
		return
	}
	var err error
	first, err = strconv.ParseInt(a, 10, 64)
	if err != nil || first < 0 {
		return
	}
	last, err = strconv.ParseInt(b, 10, 64)
	if err != nil || last < first {
		return
	}
	total = -1
	if size != "*" {
		total, err = strconv.ParseInt(size, 10, 64)
		if err != nil || total <= last {
			return
		}
	}
	ok = true
	return
}
