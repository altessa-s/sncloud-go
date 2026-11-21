// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package transfers

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// storageCompletion requires one complete success document. HTTP 200 alone is
// ambiguous: storage can report an error after it has already sent the headers.
func storageCompletion(data []byte) error {
	var result struct {
		XMLName xml.Name
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	if err := decodeStorageXML(data, &result); err != nil {
		return err
	}
	if result.Code != "" || result.XMLName.Local == "Error" {
		if result.Code == "" {
			result.Code = "S3"
		}
		return &domain.RejectedError{Code: result.Code, Message: result.Message}
	}
	if result.XMLName.Local != "CompleteMultipartUploadResult" {
		return malformed("unexpected S3 completion root")
	}
	return nil
}

// decodeStorageXML consumes the whole document, rejecting trailing roots or text.
func decodeStorageXML(data []byte, out any) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	seen := false
	for {
		token, err := decoder.Token()
		if err == io.EOF && seen {
			return nil
		}
		if err != nil {
			return malformed("invalid S3 response document")
		}
		switch value := token.(type) {
		case xml.StartElement:
			if seen {
				return malformed("multiple S3 response roots")
			}
			if err := decoder.DecodeElement(out, &value); err != nil {
				return malformed("invalid S3 response root")
			}
			seen = true
		case xml.CharData:
			if strings.TrimSpace(string(value)) != "" {
				return malformed("unexpected S3 response text")
			}
		case xml.Comment:
		case xml.ProcInst:
			if seen {
				return malformed("unexpected S3 response instruction")
			}
		default:
			return malformed("unexpected S3 response document")
		}
	}
}
