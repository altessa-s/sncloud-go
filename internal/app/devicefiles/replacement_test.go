// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package devicefiles

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestTransfersDuringReplacementLogin(t *testing.T) {
	for _, operation := range []string{"upload", "download", "multipart"} {
		for _, failed := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "/installed", true: "/failed"}[failed], func(t *testing.T) {
				arrived, release := make(chan struct{}), make(chan struct{})
				loginDone := make(chan error, 1)
				pending := porttest.Step{Run: func(context.Context, port.Request) (port.Response, error) {
					close(arrived)
					<-release
					if failed {
						return port.Response{}, domain.ErrInvalidCredentials
					}
					return porttest.OK(`"token":"account-b"`).Response, nil
				}}
				storage := porttest.Step{Run: func(_ context.Context, r port.Request) (port.Response, error) {
					close(release)
					err := <-loginDone
					if !failed && err != nil || failed && !errors.Is(err, domain.ErrInvalidCredentials) {
						t.Errorf("login: %v", err)
					}
					return port.Response{Status: 200, Header: port.Header{"ETag": {"tag"}}, Body: []byte("account-a bytes")}, nil
				}}
				steps := []porttest.Step{porttest.OK(`"randomCode":"salt","timestamp":1`), pending}
				switch operation {
				case "download":
					steps = append(steps, porttest.OK(`"url":"https://storage.invalid/object","size":15`), storage)
				case "upload":
					steps = append(steps, porttest.OK(`"url":"https://storage.invalid/object","s3Authorization":"synthetic","xamzDate":"date"`), storage, porttest.OK(`"id":1`))
				case "multipart":
					steps = append(steps, porttest.OK(`"url":"https://storage.invalid/object"`),
						porttest.OK(`"s3Authorization":"signature","xamzDate":"date"`),
						porttest.Step{Response: port.Response{Status: 200, Body: []byte("<InitiateMultipartUploadResult><UploadId>id</UploadId></InitiateMultipartUploadResult>")}},
						porttest.OK(`"s3Authorization":"signature","xamzDate":"date"`), storage,
						porttest.OK(`"s3Authorization":"signature","xamzDate":"date"`),
						porttest.Step{Response: port.Response{Status: 200, Header: port.Header{"ETag": {"tag"}}}},
						porttest.OK(`"s3Authorization":"signature","xamzDate":"date"`),
						porttest.Step{Response: port.Response{Status: 200, Body: []byte("<CompleteMultipartUploadResult/>")}}, porttest.OK(`"id":1`))
				}
				c, tr, s := client(t, steps...)
				go func() { _, err := s.LogIn(t.Context(), "b@example.invalid", "password", nil); loginDone <- err }()
				<-arrived
				var err error
				if operation == "download" {
					var file string
					file, err = c.Download(t.Context(), 1, nil, nil)
					if err == nil {
						data, e := os.ReadFile(file)
						os.Remove(file)
						if e != nil || string(data) != "account-a bytes" {
							t.Errorf("download: %q %v", data, e)
						}
					}
				} else {
					size := 1
					if operation == "multipart" {
						size = int(transfers.ChunkSize) + 1
					}
					_, err = c.UploadBytes(t.Context(), make([]byte, size), "synthetic.bin")
				}
				if failed || operation == "download" {
					if err != nil {
						t.Fatal(err)
					}
				} else if !errors.Is(err, domain.ErrNotLoggedIn) {
					t.Fatalf("old transfer: %v", err)
				}
				for _, r := range tr.Calls() {
					if strings.Contains(r.URL, "files/") || strings.Contains(r.URL, "s3/auth") {
						if got := r.Header.Get(domain.AccessTokenHeader); got != "synthetic-token" {
							t.Errorf("cross-account request: %s %q", r.URL, got)
						}
					}
				}
			})
		}
	}
}
