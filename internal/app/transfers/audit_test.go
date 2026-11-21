// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package transfers

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"os"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestFormFilenameBackslash(t *testing.T) {
	for _, name := range []string{`report\`, `a\b.pdf`} {
		o := source(t, []byte("hello"))
		o.Name = name
		file, ct, _, sum, err := multipartFile(o, 1, 3)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(file) })
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		part, err := multipart.NewReader(strings.NewReader(string(data)), strings.TrimPrefix(ct, "multipart/form-data; boundary=")).NextPart()
		if err != nil {
			t.Fatal(err)
		}
		_, params, err := mime.ParseMediaType(part.Header.Get("Content-Disposition"))
		if err != nil || params["filename"] != name {
			t.Fatalf("filename=%q, %v; want %q", params["filename"], err, name)
		}
		payload, err := io.ReadAll(part)
		if err != nil || string(payload) != "ell" || sum != "3123059c1c816471780539f6b6b738dc" {
			t.Fatalf("payload=%q hash=%s err=%v", payload, sum, err)
		}
	}
}
func TestS3PartsUseSourceSections(t *testing.T) {
	o := source(t, []byte("synthetic"))
	tr, _ := transfer(porttest.Step{Run: func(_ context.Context, r port.Request) (port.Response, error) {
		if r.File != o.File || r.Section == nil || r.Section.Offset != 0 || r.Section.Length != o.Size || string(r.Body) != "synthetic" {
			t.Fatalf("part was not streamed from source: %+v", r)
		}
		return port.Response{Status: 200, Header: port.Header{"ETag": {"etag"}}}, nil
	}}, porttest.Step{Response: port.Response{Status: 200, Body: []byte("<CompleteMultipartUploadResult/>")}})
	err := tr.parts(t.Context(), o, "https://storage.invalid/object", "key", "upload", func(context.Context, SigningRequest) (Signature, error) { return Signature{}, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrivatePartChecksumUsesSentBytes(t *testing.T) {
	o := source(t, make([]byte, ChunkSize+1))
	last := source(t, []byte{0})
	tr, _ := transfer(porttest.OK(""), porttest.Step{Run: func(context.Context, port.Request) (port.Response, error) {
		if err := os.Remove(o.File); err != nil {
			t.Fatal(err)
		}
		return porttest.OK(`"chunkMd5":"` + last.MD5 + `"`).Response, nil
	}})
	if _, err := tr.form(t.Context(), o, UploadSlot{PartURL: "https://storage.invalid/part"}, nil, nil); err != nil {
		t.Fatal(err)
	}
}
