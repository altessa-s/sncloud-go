// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func summaryStorageRequest(h http.HandlerFunc, method, url, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, strings.NewReader(body))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}

func TestSummaryHandwritingApplyAuthAndDownload(t *testing.T) {
	for _, flavor := range []Flavor{Official, Private} {
		t.Run(string(flavor), func(t *testing.T) {
			c, h := serviceFixture(t, flavor)

			_, s3Auth := exchange(t, h, domain.PathS3Auth, `{}`)
			requireSuccess(t, s3Auth)
			if s3Auth[keyS3Auth] != syntheticSummaryS3Auth || s3Auth[keyXAmzDate] != syntheticSummaryAmzDate {
				t.Fatalf("unexpected s3 auth: %v", s3Auth)
			}
			badAuthReq := httptest.NewRequest(http.MethodPost, "/api/"+domain.PathS3Auth, strings.NewReader(`{}`))
			badAuthRec := httptest.NewRecorder()
			h(badAuthRec, badAuthReq)
			if badAuthRec.Code != http.StatusBadRequest {
				t.Fatalf("missing headers on s3 auth = %d, want 400", badAuthRec.Code)
			}

			_, applyOut := exchange(t, h, domain.PathApplyDigestHandwriting, `{"fileName":"comment.mark"}`)
			requireSuccess(t, applyOut)
			inner, _ := applyOut[keyInnerName].(string)
			if !strings.HasSuffix(inner, "-comment.mark") {
				t.Fatalf("unexpected innerName: %v", applyOut)
			}
			if flavor == Official {
				wantURL := c.app.URL + "/api/oss/summary/" + inner
				if applyOut[keyAPIServer] != "1" || applyOut["fileServer"] != "3" || applyOut["s3Url"] != wantURL || applyOut[keyS3Auth] != syntheticSummaryS3Auth {
					t.Fatalf("official apply slot: %v", applyOut)
				}
			} else {
				wantFull := privateInternalOrigin + "/api/oss/summary/upload?key=" + inner
				wantPart := privateInternalOrigin + "/api/oss/summary/part?key=" + inner
				if applyOut[keyAPIServer] != "private_cloud" || applyOut["fullUploadUrl"] != wantFull || applyOut["partUploadUrl"] != wantPart {
					t.Fatalf("private apply slot: %v", applyOut)
				}
			}

			_, missingDL := exchange(t, h, domain.PathDownloadDigestHandwriting, `{"id":-1}`)
			requireCode(t, missingDL, "E0340")

			createdRec, created := exchange(t, h, domain.PathAddDigest, `{"content":"No mark yet"}`)
			requireSuccess(t, created)
			digestID := responseSnowflake(t, createdRec)

			_, emptyMarkDL := exchange(t, h, domain.PathDownloadDigestHandwriting, fmt.Sprintf(`{"id":%s}`, digestID))
			requireCode(t, emptyMarkDL, "E0340")

			_, upd := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigest, fmt.Sprintf(`{"id":%s,"handwriteInnerName":%q}`, digestID, inner))
			requireSuccess(t, upd)
			_, dlOut := exchange(t, h, domain.PathDownloadDigestHandwriting, fmt.Sprintf(`{"id":%s}`, digestID))
			requireSuccess(t, dlOut)
			wantOrigin := c.app.URL
			if flavor == Private {
				wantOrigin = privateInternalOrigin
			}
			if dlOut["url"] != wantOrigin+"/api/oss/summary/"+inner {
				t.Fatalf("download url = %v", dlOut)
			}

			if w := summaryStorageRequest(h, http.MethodGet, "/api/oss/summary/"+inner, "", nil); w.Code != http.StatusNotFound {
				t.Fatalf("missing object status = %d, want 404", w.Code)
			}
			c.mu.Lock()
			c.state.hwObjects[inner] = []byte("handwritten-bytes")
			c.mu.Unlock()
			if w := summaryStorageRequest(h, http.MethodGet, "/api/oss/summary/"+inner, "", map[string]string{domain.AccessTokenHeader: "leak"}); w.Code != http.StatusForbidden {
				t.Fatalf("leaked token status = %d, want 403", w.Code)
			}
			w := summaryStorageRequest(h, http.MethodGet, "/api/oss/summary/"+inner, "", nil)
			if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/octet-stream" || w.Body.String() != "handwritten-bytes" {
				t.Fatalf("download = (%d, %q, %q)", w.Code, w.Header().Get("Content-Type"), w.Body.String())
			}
		})
	}
}

func TestSummaryS3StorageLifecycle(t *testing.T) {
	c, h := serviceFixture(t, Official)
	auth := map[string]string{"Authorization": syntheticSummaryS3Auth}

	for _, hdrs := range []map[string]string{nil, {"Authorization": syntheticSummaryS3Auth, domain.AccessTokenHeader: "leak"}} {
		if w := summaryStorageRequest(h, http.MethodPut, "/api/oss/summary/single", "data", hdrs); w.Code != http.StatusUnauthorized {
			t.Fatalf("unauthorized status = %d, want 401", w.Code)
		}
	}
	if w := summaryStorageRequest(h, http.MethodPatch, "/api/oss/summary/single", "", auth); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("method not allowed status = %d, want 405", w.Code)
	}

	brokenReq := httptest.NewRequest(http.MethodPut, "/api/oss/summary/broken", nil)
	brokenReq.Body = io.NopCloser(failingReader{})
	brokenReq.Header.Set("Authorization", syntheticSummaryS3Auth)
	brokenRec := httptest.NewRecorder()
	h(brokenRec, brokenReq)
	if brokenRec.Code != http.StatusBadRequest {
		t.Fatalf("broken body status = %d, want 400", brokenRec.Code)
	}

	w := summaryStorageRequest(h, http.MethodPut, "/api/oss/summary/single", "whole-mark", auth)
	if w.Code != http.StatusOK || w.Header().Get("ETag") == "" {
		t.Fatalf("single PUT = (%d, %v)", w.Code, w.Header())
	}
	if dl := summaryStorageRequest(h, http.MethodGet, "/api/oss/summary/single", "", nil); dl.Body.String() != "whole-mark" {
		t.Fatalf("single GET = %q", dl.Body.String())
	}

	// Out-of-order multipart upload reassembles parts by partNumber on completion.
	w = summaryStorageRequest(h, http.MethodPost, "/api/oss/summary/multi?uploads", "", auth)
	var init struct {
		ID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &init); err != nil || init.ID != "multi" {
		t.Fatalf("initiate multipart = (%+v, %v)", init, err)
	}
	for _, part := range []struct{ number, body string }{{"2", "second"}, {"1", "first"}} {
		pw := summaryStorageRequest(h, http.MethodPut, "/api/oss/summary/multi?uploadId=multi&partNumber="+part.number, part.body, auth)
		if pw.Code != http.StatusOK || pw.Header().Get("ETag") == "" {
			t.Fatalf("part %s = (%d, %v)", part.number, pw.Code, pw.Header())
		}
	}
	w = summaryStorageRequest(h, http.MethodPost, "/api/oss/summary/multi?uploadId=multi", "", auth)
	if w.Body.String() != "<CompleteMultipartUploadResult/>" || c.state.hwUploads["multi"] != nil {
		t.Fatalf("complete multipart = (%q, %v)", w.Body.String(), c.state.hwUploads)
	}
	if dl := summaryStorageRequest(h, http.MethodGet, "/api/oss/summary/multi", "", nil); dl.Body.String() != "firstsecond" {
		t.Fatalf("reassembled GET = %q, want %q", dl.Body.String(), "firstsecond")
	}

	// Unannounced uploadId allocates its part map on demand, and DELETE aborts an active upload.
	summaryStorageRequest(h, http.MethodPut, "/api/oss/summary/abort?uploadId=abort&partNumber=1", "discard", auth)
	w = summaryStorageRequest(h, http.MethodDelete, "/api/oss/summary/abort?uploadId=abort", "", auth)
	if w.Code != http.StatusNoContent || c.state.hwUploads["abort"] != nil {
		t.Fatalf("abort multipart = (%d, %v)", w.Code, c.state.hwUploads)
	}
	if dl := summaryStorageRequest(h, http.MethodGet, "/api/oss/summary/abort", "", nil); dl.Code != http.StatusNotFound {
		t.Fatalf("aborted object status = %d, want 404", dl.Code)
	}
}

func TestSummaryPrivateFormStorage(t *testing.T) {
	_, h := serviceFixture(t, Private)

	for _, tc := range []struct {
		method, url, contentType, body, token string
		want                                  int
	}{
		{http.MethodPost, "/api/oss/summary/upload?key=k", "", "", "", http.StatusUnauthorized},
		{http.MethodPut, "/api/oss/summary/upload?key=k", "", "", "synthetic-token", http.StatusUnauthorized},
		{http.MethodPost, "/api/oss/summary/upload", "", "", "synthetic-token", http.StatusBadRequest},
		{http.MethodPost, "/api/oss/summary/upload?key=k", "", "", "synthetic-token", http.StatusBadRequest},
		{http.MethodPost, "/api/oss/summary/upload?key=k", "multipart/form-data; boundary=synthetic", "--synthetic--\r\n", "synthetic-token", http.StatusBadRequest},
		{http.MethodPost, "/api/oss/summary/upload?key=k", "multipart/form-data; boundary=synthetic", "--synthetic\r\nContent-Disposition: form-data; name=\"file\"\r\n\r\ntruncated", "synthetic-token", http.StatusBadRequest},
	} {
		w := summaryStorageRequest(h, tc.method, tc.url, tc.body, map[string]string{
			"Content-Type":           tc.contentType,
			domain.AccessTokenHeader: tc.token,
		})
		if w.Code != tc.want {
			t.Fatalf("%+v: status %d, want %d", tc, w.Code, tc.want)
		}
	}

	sendForm := func(url, data string) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "comment.mark")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(part, data); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		return summaryStorageRequest(h, http.MethodPost, url, body.String(), map[string]string{
			"Content-Type":           writer.FormDataContentType(),
			domain.AccessTokenHeader: "synthetic-token",
		})
	}

	whole := sendForm("/api/oss/summary/upload?key=mark-obj", "stale-whole")
	if whole.Code != http.StatusOK || !strings.Contains(whole.Body.String(), `"innerName":"mark-obj"`) || !strings.Contains(whole.Body.String(), domain.MD5Hex([]byte("stale-whole"))) {
		t.Fatalf("whole form upload = (%d, %s)", whole.Code, whole.Body)
	}
	if dl := summaryStorageRequest(h, http.MethodGet, "/api/oss/summary/mark-obj", "", nil); dl.Body.String() != "stale-whole" {
		t.Fatalf("whole GET = %q", dl.Body.String())
	}

	for _, chunk := range []struct{ num, data string }{{"1", "chunk-one:"}, {"2", "chunk-two"}} {
		cw := sendForm("/api/oss/summary/part?key=mark-obj&partNumber="+chunk.num, chunk.data)
		if cw.Code != http.StatusOK || !strings.Contains(cw.Body.String(), domain.MD5Hex([]byte(chunk.data))) {
			t.Fatalf("chunk %s = (%d, %s)", chunk.num, cw.Code, cw.Body)
		}
	}
	if dl := summaryStorageRequest(h, http.MethodGet, "/api/oss/summary/mark-obj", "", nil); dl.Body.String() != "chunk-one:chunk-two" {
		t.Fatalf("chunked GET = %q", dl.Body.String())
	}
}
