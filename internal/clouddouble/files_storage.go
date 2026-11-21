// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"crypto/md5" // #nosec G501 -- Synthetic protocol checksums.
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func fileHash(data []byte) string { return fmt.Sprintf("%x", md5.Sum(data)) } // #nosec G401 -- Protocol checksum only.

func (f *Files) apply(w http.ResponseWriter) {
	f.next++
	key := f.next.String()
	if f.cloud.flavor == Private {
		Reply(w, map[string]any{"apiServer": "private_cloud", "innerName": key,
			"fullUploadUrl": f.cloud.app.URL + "/api/oss/upload?key=" + key, "partUploadUrl": f.cloud.app.URL + "/api/oss/part?key=" + key})
		return
	}
	// The key occupies the URL's last component in the actual S3 protocol.
	// Register a separate object path for this upload through the shared registry.
	f.cloud.Register(false, "oss/"+key, f.storage)
	Reply(w, map[string]any{"apiServer": "1", urlField: f.cloud.app.URL + "/api/oss/" + key,
		"s3Authorization": "synthetic-signature", "xamzDate": "20260101T000000Z", "fileServer": 1})
}

func (f *Files) download(w http.ResponseWriter, id domain.FileID, device bool) {
	e := f.entries[id]
	if e == nil {
		Refuse(w, "E0311")
		return
	}
	row := map[string]any{urlField: f.downloadURL() + "?id=" + id.String(), sizeField: len(e.Data)}
	if device {
		row["content_hash"] = e.Hash
	} else {
		row["md5"] = e.Hash
	}
	Reply(w, row)
}

func (f *Files) storage(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := r.URL.Query()
	if r.Method == "GET" {
		f.storageDownload(w, r)
		return
	}
	key := q.Get("key")
	if key == "" {
		pieces := strings.Split(r.URL.Path, "/")
		key = pieces[len(pieces)-1]
	}
	if strings.HasSuffix(r.URL.Path, "/upload") || strings.HasSuffix(r.URL.Path, "/part") {
		f.formStorage(w, r, key)
		return
	}
	if r.Header.Get("Authorization") == "" || r.Header.Get("x-access-token") != "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	id := q.Get("uploadId")
	switch r.Method {
	case "DELETE":
		if f.uploads[id] == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		delete(f.uploads, id)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		if q.Has("uploads") {
			f.uploads[key] = make(map[int][]byte)
			w.Header().Set("Content-Type", "application/xml")
			// A disconnected test client cannot receive a replacement error response.
			_ = xml.NewEncoder(w).Encode(struct {
				XMLName xml.Name `xml:"InitiateMultipartUploadResult"`
				ID      string   `xml:"UploadId"`
			}{ID: key})
			return
		}
		f.completeStorage(w, r, key, id)
	case "PUT":
		data, err := io.ReadAll(io.LimitReader(r.Body, storageLimit))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if id != "" {
			if f.uploads[id] == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			n, err := strconv.Atoi(q.Get("partNumber"))
			if err != nil || n <= 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.uploads[id][n] = data
		} else {
			f.objects[key] = data
		}
		w.Header().Set("ETag", "synthetic-etag")
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

const storageLimit = 16 * 1024 * 1024

func (f *Files) formStorage(w http.ResponseWriter, r *http.Request, key string) {
	if r.Header.Get("x-access-token") == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	part, err := reader.NextPart()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(io.LimitReader(part, storageLimit))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if strings.HasSuffix(r.URL.Path, "/part") {
		if !f.storeFormPart(w, r, key, data) {
			return
		}

		Reply(w, map[string]any{"chunkMd5": fileHash(data)})
		return
	}
	f.objects[key] = data
	Reply(w, map[string]any{"innerName": key, "md5": fileHash(data)})
}

func (f *Files) storageDownload(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("x-access-token") != "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	var data []byte
	if r.URL.Query().Get("png") != "" {
		data = []byte("\x89PNG\r\n\x1a\nsynthetic")
	} else {
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil || f.entries[domain.FileID(id)] == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		data = f.entries[domain.FileID(id)].Data
	}
	if span := r.Header.Get("Range"); span != "" {
		var first, last int64
		if _, err := fmt.Sscanf(span, "bytes=%d-%d", &first, &last); err != nil || first < 0 || last < first || first >= int64(len(data)) {
			w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		last = min(last, int64(len(data))-1)
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", first, last, len(data)))
		w.WriteHeader(http.StatusPartialContent)
		data = data[first : last+1]
	}
	_, _ = w.Write(data) // A disconnected client needs no further response.
}

func (f *Files) downloadURL() string {
	if f.cloud.flavor == Official {
		return f.cloud.app.URL + "/api/storage/download"
	}
	return f.cloud.app.URL + "/api/oss/download"
}
