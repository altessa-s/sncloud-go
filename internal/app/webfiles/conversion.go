// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package webfiles

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"time"

	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

const (
	conversionTimeout       = 300 * time.Second
	conversionPoll          = 5 * time.Second
	pageShare         int64 = 1000
)

// PDF downloads the rendered note; an empty page list selects every page.
func (c *Client) PDF(ctx context.Context, id domain.FileID, pages []int, p port.Progress) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, conversionTimeout)
	defer cancel()
	body := struct {
		ID    domain.FileID `json:"id"`
		Pages []int         `json:"pageNoList"`
		Type  int           `json:"type"`
	}{ID: id, Pages: pages}
	var out link
	if err := c.call(ctx, "file/note/to/pdf", body, false, c.session.Pin(), &out); err != nil {
		return "", err
	}
	return c.bytes.Fetch(ctx, transfers.ServedFrom(out.URL, c.host), transfers.DownloadLimit, out.MD5, nil, p)
}

// pngPage retains the service page number because response order is not guaranteed.
type pngPage struct {
	Page int    `json:"pageNo"`
	URL  string `json:"url"`
}

// convertedPages polls only the conversion-in-progress refusal.
// The injected clock makes waits cancellable and testable. Elapsed time counts
// slow requests; accumulated waits still bound clocks that do not advance on Sleep.
func (c *Client) convertedPages(ctx context.Context, id domain.FileID) ([]pngPage, error) {
	pin := c.session.Pin()
	started := c.clock.Now()
	for waited := time.Duration(0); ; waited += conversionPoll {
		var out struct {
			Pages []pngPage `json:"pngPageVOList"`
		}
		err := c.call(ctx, "file/note/to/png", idRequest{id}, false, pin, &out)
		if err == nil {
			return out.Pages, nil
		}
		e, ok := errors.AsType[*domain.RejectedError](err)
		if !ok || e.Code != "E0316" || waited >= conversionTimeout || c.clock.Now().Sub(started) >= conversionTimeout {
			return nil, err
		}
		if err = c.clock.Sleep(ctx, conversionPoll); err != nil {
			return nil, err
		}
	}
}

// PNGs returns page-ordered temporary PNG files. Any failure removes all pages.
func (c *Client) PNGs(ctx context.Context, id domain.FileID, p port.Progress) (files []string, err error) {
	pages, err := c.convertedPages(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(pages) == 0 {
		return nil, &domain.MalformedResponseError{What: "note/to/png named no pages"}
	}
	slices.SortStableFunc(pages, func(a, b pngPage) int {
		if a.Page < b.Page {
			return -1
		}
		if a.Page > b.Page {
			return 1
		}
		return 0
	})
	defer func() {
		if err != nil {
			for _, file := range files {
				_ = os.Remove(file) // Best-effort cleanup; preserve the transfer or integrity result.
			}
			files = nil
		}
	}()
	files = make([]string, 0, len(pages))
	for i, page := range pages {
		var file string
		file, err = c.bytes.Fetch(ctx, transfers.ServedFrom(page.URL, c.host), transfers.DownloadLimit, "", nil, func(done, total int64) {
			within := int64(0)
			if total > 0 {
				within = min(done, total) * pageShare / total
			}
			if p != nil {
				p(int64(i)*pageShare+within, int64(len(pages))*pageShare)
			}
		})
		if err != nil {
			return files, err
		}
		files = append(files, file)
		if err = checkPNG(file); err != nil {
			return files, err
		}
	}
	return files, nil
}

// checkPNG rejects an expired link's XML error even when storage answers HTTP 200.
func checkPNG(file string) error {
	// #nosec G304 -- Fetch created this private temporary file; no server filename is used.
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }() // Read-only handle; closing cannot lose buffered output.
	var head [8]byte
	_, err = io.ReadFull(f, head[:])
	if err != nil || !bytes.Equal(head[:], []byte("\x89PNG\r\n\x1a\n")) {
		return &domain.MalformedResponseError{What: "page is not a PNG"}
	}
	return nil
}
