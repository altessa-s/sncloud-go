// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/digests"
	"github.com/altessa-s/sncloud-go/internal/app/feed"
	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/app/webfiles"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestFirstIdentityDoesNotInvalidateOperations(t *testing.T) {
	for _, operation := range []string{"handwriting", "feed", "web-signing", "concurrent-identity"} {
		t.Run(operation, func(t *testing.T) {
			s, tr, store := setup()
			_ = store.Discard(domain.ClientID)
			_ = store.Keep("account-a", domain.AccessToken)
			policy := domain.DefaultRetryPolicy()
			s.channel = channel.New(domain.PrivateCloud("https://synthetic.invalid", false), tr, s.clock, policy)
			bytes := transfers.New(tr, s.clock, policy)
			slot := porttest.OK(`"url":"https://storage.invalid/object","s3Authorization":"synthetic","xamzDate":"date"`)
			storage := porttest.Step{Response: port.Response{Status: 200, Header: port.Header{"ETag": {"tag"}}}}
			var err error
			switch operation {
			case "handwriting":
				tr.Steps = []porttest.Step{slot, storage}
				file := filepath.Join(t.TempDir(), "synthetic.mark")
				if e := os.WriteFile(file, []byte("strokes"), 0600); e != nil {
					t.Fatal(e)
				}
				c := digests.New(domain.Official(), s.channel, s, bytes, s.clock)
				_, err = c.UploadHandwriting(t.Context(), file, "synthetic.mark", nil)
			case "feed":
				open := `0{"sid":"synthetic","pingInterval":5000,"pingTimeout":25000}`
				dialer := &porttest.SocketDialer{Steps: []porttest.DialStep{
					{Socket: porttest.NewSocket(open, "40", "41")},
					{Socket: porttest.NewSocket(open, "40", "41")},
				}}
				f := feed.New(domain.Official(), s, dialer, &porttest.Clock{}, policy)
				connected := 0
				for change, e := range f.Watch(t.Context()) {
					if e != nil {
						err = e
						break
					}
					if change.Type == domain.ChangeConnected {
						connected++
					}
					if connected == 2 {
						break
					}
				}
				if connected != 2 || len(dialer.Calls()) != 2 {
					t.Errorf("connections=%d dials=%d", connected, len(dialer.Calls()))
				}
			case "web-signing":
				signature := porttest.OK(`"s3Authorization":"synthetic","xamzDate":"date"`)
				tr.Steps = []porttest.Step{slot, signature,
					{Response: port.Response{Status: 200, Body: []byte("<InitiateMultipartUploadResult><UploadId>id</UploadId></InitiateMultipartUploadResult>")}},
					signature, storage, signature, storage, signature,
					{Response: port.Response{Status: 200, Body: []byte("<CompleteMultipartUploadResult/>")}}, porttest.OK("")}
				c := webfiles.New(s.channel, s, bytes, "https://synthetic.invalid", s.clock)
				err = c.UploadBytes(t.Context(), make([]byte, transfers.ChunkSize+1), "synthetic.bin", 0)
				signs := 0
				for _, r := range tr.Calls() {
					if strings.HasSuffix(r.URL, "/file/s3/auth") {
						signs++
					}
				}
				if signs != 4 {
					t.Errorf("signatures=%d", signs)
				}
			case "concurrent-identity":
				fired := false
				tr.Steps = []porttest.Step{slot, {Run: func(context.Context, port.Request) (port.Response, error) {
					fired = true
					done := make(chan error, 1)
					go func() { _, e := s.Identity(); done <- e }()
					if e := <-done; e != nil {
						return port.Response{}, e
					}
					return storage.Response, nil
				}}, porttest.OK("")}
				c := webfiles.New(s.channel, s, bytes, "https://synthetic.invalid", s.clock)
				err = c.UploadBytes(t.Context(), []byte("data"), "synthetic.bin", 0)
				if !fired {
					t.Error("identity boundary not reached")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if id, e := store.Secret(domain.ClientID); e != nil || id == "" {
				t.Fatal("identity not persisted", e)
			}
		})
	}
}
