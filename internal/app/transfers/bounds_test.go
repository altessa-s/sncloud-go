// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package transfers

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestCompletionRequiresCompleteSuccessDocument(t *testing.T) {
	for _, body := range []string{"garbage<CompleteMultipartUploadResult/>", "", " ", "<Error", "<html/>", "<CompleteMultipartUploadResult/ trailing", "<CompleteMultipartUploadResult/><other/>", "<CompleteMultipartUploadResult/>garbage"} {
		t.Run(body, func(t *testing.T) {
			tr, _ := transfer(porttest.Step{Response: port.Response{Status: 200, Body: []byte(body)}})
			err := tr.parts(t.Context(), Original{}, "https://storage.invalid/key", "key", "id", func(context.Context, SigningRequest) (Signature, error) { return Signature{}, nil }, nil)
			if !errors.Is(err, domain.ErrMalformedResponse) {
				t.Fatalf("accepted %q: %v", body, err)
			}
		})
	}
}
func TestPrivateChunkCountDoesNotOverflow(t *testing.T) {
	tr, record := transfer()
	_, err := tr.form(t.Context(), Original{Size: math.MaxInt64, File: "missing-synthetic-file"}, UploadSlot{PartURL: "https://storage.invalid/part"}, nil, nil)
	if err == nil {
		t.Fatal("overflow skipped every part and reported success")
	}
	if len(record.Calls()) != 0 {
		t.Fatal("missing source reached storage")
	}
}

func TestInitiationRequiresCompleteDocument(t *testing.T) {
	for _, body := range []string{"<Wrong><UploadId>id</UploadId></Wrong>", "<InitiateMultipartUploadResult><UploadId>id</UploadId>", "<InitiateMultipartUploadResult><UploadId>id</UploadId></InitiateMultipartUploadResult><extra/>"} {
		tr, record := transfer(porttest.Step{Response: port.Response{Status: 200, Body: []byte(body)}}, porttest.Step{Response: port.Response{Status: 200, Body: []byte("<CompleteMultipartUploadResult/>")}})
		err := tr.multipart(t.Context(), Original{}, "https://storage.invalid/key", "key", func(context.Context, SigningRequest) (Signature, error) { return Signature{}, nil }, nil)
		if !errors.Is(err, domain.ErrMalformedResponse) || len(record.Calls()) != 1 {
			t.Errorf("invalid initiation accepted: %v, calls=%d", err, len(record.Calls()))
		}
	}
}

func TestStorageXMLDocuments(t *testing.T) {
	for _, tc := range []struct {
		body string
		want error
	}{
		{`<?xml version="1.0"?><CompleteMultipartUploadResult xmlns="urn:synthetic"/> <!--done-->`, nil},
		{`<Error xmlns="urn:synthetic"/>`, domain.ErrRejected},
		{`<CompleteMultipartUploadResult/> <?xml version="1.0"?>`, domain.ErrMalformedResponse},
		{`<!DOCTYPE synthetic><CompleteMultipartUploadResult/>`, domain.ErrMalformedResponse},
		{`<CompleteMultipartUploadResult/><!--`, domain.ErrMalformedResponse},
	} {
		if err := storageCompletion([]byte(tc.body)); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v want %v", tc.body, err, tc.want)
		}
	}
}
