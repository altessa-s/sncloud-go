// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/clouddouble"
	"github.com/altessa-s/sncloud-go/internal/porttest"

	sncloud "github.com/altessa-s/sncloud-go"
)

var sinkBenchEntries []sncloud.Entry

// benchClient keeps login and the first XSRF exchange outside the timed loop.
// All requests stay on the emulator's loopback servers.
func benchClient(b *testing.B, flavor clouddouble.Flavor) (*sncloud.Client, *clouddouble.Files) {
	b.Helper()
	cloud := clouddouble.New(flavor)
	b.Cleanup(cloud.Close)
	files := cloud.EnableFiles()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	b.Cleanup(transport.CloseIdleConnections)
	c, err := sncloud.New(&porttest.Secrets{},
		sncloud.WithEndpoints(cloud.Endpoints()), sncloud.WithRoundTripper(transport))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = c.Close() })
	if _, err := c.LogIn(b.Context(), "reader@example.invalid", "synthetic-password", nil); err != nil {
		b.Fatal(err)
	}
	if _, err := c.Files().List(b.Context(), sncloud.RootFileID, sncloud.SortName, true); err != nil {
		b.Fatal(err)
	}
	return c, files
}

func BenchmarkClientFilesList(b *testing.B) {
	for _, flavor := range []clouddouble.Flavor{clouddouble.Official, clouddouble.Private} {
		for _, count := range []int{10, 1000} {
			b.Run(fmt.Sprintf("%s/entries=%d", flavor, count), func(b *testing.B) {
				c, files := benchClient(b, flavor)
				for i := range count {
					name := fmt.Sprintf("synthetic-%04d.note", i)
					files.Seed(sncloud.RootFileID, name, name, false, nil)
				}
				ctx := b.Context()
				view := c.Files()
				b.ReportAllocs()
				for b.Loop() {
					entries, err := view.List(ctx, sncloud.RootFileID, sncloud.SortName, true)
					if err != nil {
						b.Fatal(err)
					}
					if len(entries) != count {
						b.Fatalf("listed %d entries, want %d", len(entries), count)
					}
					sinkBenchEntries = entries
				}
			})
		}
	}
}

func BenchmarkClientDownload(b *testing.B) {
	for _, flavor := range []clouddouble.Flavor{clouddouble.Official, clouddouble.Private} {
		for _, size := range []int64{1 << 10, 1 << 20} {
			b.Run(fmt.Sprintf("%s/bytes=%d", flavor, size), func(b *testing.B) {
				c, files := benchClient(b, flavor)
				id := files.Seed(sncloud.RootFileID, "synthetic.note", "synthetic.note", false, bytes.Repeat([]byte("x"), int(size)))
				ctx := b.Context()
				view := c.Files()
				b.ReportAllocs()
				b.SetBytes(size)
				for b.Loop() {
					file, err := view.Download(ctx, id, &size, nil)
					if err != nil {
						b.Fatal(err)
					}
					// Include removal: each operation owns a new verified temporary file.
					if err := os.Remove(file); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
