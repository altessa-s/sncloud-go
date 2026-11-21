// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"encoding/xml"
	"io"
	"net/http"
)

// completeStorage validates the manifest before publishing any bytes. This lets
// integration tests catch missing, reordered or incorrectly escaped parts.
func (f *Files) completeStorage(w http.ResponseWriter, r *http.Request, key, id string) {
	parts := f.uploads[id]
	if parts == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	var manifest struct {
		XMLName xml.Name `xml:"CompleteMultipartUpload"`
		Parts   []struct {
			Number int    `xml:"PartNumber"`
			ETag   string `xml:"ETag"`
		} `xml:"Part"`
	}
	decoder := xml.NewDecoder(io.LimitReader(r.Body, storageLimit))
	if err := decoder.Decode(&manifest); err != nil || len(manifest.Parts) == 0 || len(manifest.Parts) != len(parts) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var data []byte
	for i, part := range manifest.Parts {
		chunk, ok := parts[part.Number]
		if part.Number != i+1 || !ok || part.ETag != "synthetic-etag" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data = append(data, chunk...)
	}
	f.objects[key] = data
	delete(f.uploads, id)
	_, _ = io.WriteString(w, "<CompleteMultipartUploadResult/>") // A disconnected client needs no further response.
}
