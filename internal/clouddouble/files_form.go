// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"net/http"
	"strconv"
)

type formUpload struct {
	key   string
	count int
	parts map[int][]byte
}

// storeFormPart replaces retries by number and publishes only complete objects.
// Keep completed parts too: a lost final response must be safe to replay.
func (f *Files) storeFormPart(w http.ResponseWriter, r *http.Request, key string, data []byte) bool {
	q := r.URL.Query()
	id := q.Get("uploadId")
	number, err := strconv.Atoi(q.Get("partNumber"))
	count, countErr := strconv.Atoi(q.Get("totalChunks"))
	if id == "" || key == "" || err != nil || countErr != nil || count < 1 || number < 1 || number > count {
		w.WriteHeader(http.StatusBadRequest)
		return false
	}
	upload := f.formUploads[id]
	if upload == nil {
		upload = &formUpload{key: key, count: count, parts: make(map[int][]byte)}
		f.formUploads[id] = upload
	} else if upload.key != key || upload.count != count {
		w.WriteHeader(http.StatusBadRequest)
		return false
	}
	upload.parts[number] = data
	if len(upload.parts) == count {
		var assembled []byte
		for n := 1; n <= count; n++ {
			assembled = append(assembled, upload.parts[n]...)
		}
		f.objects[key] = assembled
	}
	return true
}
