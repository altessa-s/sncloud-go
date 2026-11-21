// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"bytes"
	"encoding/xml"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func storageRequest(f *Files, method, url, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, strings.NewReader(body))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	f.storage(w, r)
	return w
}

func TestStorageApplyAndDownload(t *testing.T) {
	for _, flavor := range []Flavor{Official, Private} {
		t.Run(string(flavor), func(t *testing.T) {
			f := fileFixture(t, flavor)
			for _, tc := range []struct {
				handler http.HandlerFunc
				path    string
			}{{f.web, "file/upload/apply"}, {f.device, "file/3/files/upload/apply"}} {
				_, out := exchange(t, tc.handler, tc.path, `{}`)
				requireSuccess(t, out)
				if flavor == Private {
					if out["apiServer"] != "private_cloud" || !strings.Contains(out["fullUploadUrl"].(string), "/oss/upload?key=") {
						t.Fatal(out)
					}
				} else {
					if out["apiServer"] != "1" || out["s3Authorization"] != "synthetic-signature" {
						t.Fatal(out)
					}
					if f.cloud.handlers["false:oss/"+f.next.String()] == nil {
						t.Fatal("object handler missing")
					}
				}
			}
			id := f.Seed(0, "note", "note", false, []byte("abcdef"))
			for _, tc := range []struct {
				query, span, token string
				status             int
				body               string
			}{
				{"?id=" + id.String(), "", "", 200, "abcdef"}, {"?id=" + id.String(), "bytes=1-99", "", 206, "bcdef"}, {"?id=" + id.String(), "bytes=99-100", "", 416, ""}, {"?id=" + id.String(), "bad", "", 416, ""}, {"?id=bad", "", "", 404, ""}, {"?id=-1", "", "", 404, ""}, {"?id=" + id.String(), "", "leak", 403, ""}, {"?png=1", "", "", 200, "\x89PNG\r\n\x1a\nsynthetic"},
			} {
				w := storageRequest(f, "GET", f.downloadURL()+tc.query, "", map[string]string{"Range": tc.span, "x-access-token": tc.token})
				if w.Code != tc.status || w.Body.String() != tc.body {
					t.Fatalf("%+v: %d %q", tc, w.Code, w.Body.String())
				}
				if tc.status == 206 && w.Header().Get("Content-Range") != "bytes 1-5/6" {
					t.Fatal(w.Header())
				}
			}
		})
	}
}

func TestS3StorageLifecycle(t *testing.T) {
	f := fileFixture(t, Official)
	auth := map[string]string{"Authorization": "synthetic-signature"}
	for _, headers := range []map[string]string{nil, {"Authorization": "synthetic-signature", "x-access-token": "leak"}} {
		if w := storageRequest(f, "PUT", "/api/oss/object", "data", headers); w.Code != 401 {
			t.Fatal(w.Code)
		}
	}
	w := storageRequest(f, "PUT", "/api/oss/object", "whole", auth)
	if w.Code != 200 || w.Header().Get("ETag") == "" || string(f.objects["object"]) != "whole" {
		t.Fatal(w, f.objects)
	}
	w = storageRequest(f, "POST", "/api/oss/multi?uploads", "", auth)
	var init struct {
		ID string `xml:"UploadId"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &init); err != nil || init.ID != "multi" {
		t.Fatal(init, err)
	}
	for _, part := range []struct{ number, body string }{{"2", "second"}, {"1", "first"}} {
		w = storageRequest(f, "PUT", "/api/oss/multi?uploadId=multi&partNumber="+part.number, part.body, auth)
		if w.Header().Get("ETag") == "" {
			t.Fatal(w.Header())
		}
	}
	w = storageRequest(f, "POST", "/api/oss/multi?uploadId=multi", `<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>synthetic-etag</ETag></Part><Part><PartNumber>2</PartNumber><ETag>synthetic-etag</ETag></Part></CompleteMultipartUpload>`, auth)
	if w.Body.String() != "<CompleteMultipartUploadResult/>" || string(f.objects["multi"]) != "firstsecond" || f.uploads["multi"] != nil {
		t.Fatal(w.Body, f.objects, f.uploads)
	}
	storageRequest(f, "POST", "/api/oss/abort?uploads", "", auth)
	w = storageRequest(f, "DELETE", "/api/oss/abort?uploadId=abort", "", auth)
	if w.Code != 204 || f.uploads["abort"] != nil {
		t.Fatal(w.Code, f.uploads)
	}
	r := httptest.NewRequest("PUT", "/api/oss/broken", nil)
	r.Body = io.NopCloser(failingReader{})
	r.Header.Set("Authorization", "signature")
	w = httptest.NewRecorder()
	f.storage(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestPrivateStorageForms(t *testing.T) {
	f := fileFixture(t, Private)
	for _, tc := range []struct{ contentType, body, token string }{{"", "", ""}, {"", "", "token"}, {"multipart/form-data; boundary=synthetic", "--synthetic--\r\n", "token"}, {"multipart/form-data; boundary=synthetic", "--synthetic\r\nContent-Disposition: form-data; name=\"file\"\r\n\r\ntruncated", "token"}} {
		w := storageRequest(f, "POST", "/api/oss/upload?key=object", tc.body, map[string]string{"Content-Type": tc.contentType, "x-access-token": tc.token})
		want := 400
		if tc.token == "" {
			want = 401
		}
		if w.Code != want {
			t.Fatal(w.Code, tc)
		}
	}
	send := func(path, data string) *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "synthetic.bin")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(part, data); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		return storageRequest(f, "POST", path, body.String(), map[string]string{"Content-Type": writer.FormDataContentType(), "x-access-token": "synthetic-token"})
	}
	w := send("/api/oss/upload?key=object", "old")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"innerName":"object"`) || string(f.objects["object"]) != "old" {
		t.Fatal(w.Body, f.objects)
	}
	for _, part := range []struct{ number, data string }{{"1", "first"}, {"2", "second"}, {"2", "second"}} {
		w = send("/api/oss/part?key=object&uploadId=synthetic&totalChunks=2&partNumber="+part.number, part.data)
		if !strings.Contains(w.Body.String(), fileHash([]byte(part.data))) {
			t.Fatal(w.Body)
		}
	}
	// A fresh upload cannot expose an incomplete sequence or mix upload IDs.
	for _, query := range []string{
		"partNumber=1&totalChunks=2", "uploadId=bad&partNumber=0&totalChunks=2",
		"uploadId=bad&partNumber=3&totalChunks=2", "uploadId=bad&partNumber=x&totalChunks=2",
		"uploadId=bad&partNumber=1&totalChunks=x", "uploadId=bad&partNumber=1&totalChunks=0",
		"uploadId=synthetic&partNumber=1&totalChunks=3",
	} {
		if got := send("/api/oss/part?key=object&"+query, "corrupt"); got.Code != 400 {
			t.Errorf("accepted %s: %d", query, got.Code)
		}
	}
	if got := send("/api/oss/part?key=other&uploadId=synthetic&partNumber=1&totalChunks=2", "corrupt"); got.Code != 400 {
		t.Error("upload ID reused for different object")
	}
	if got := send("/api/oss/part?key=object&uploadId=new&partNumber=2&totalChunks=2", "replacement"); got.Code != 200 {
		t.Fatal(got.Code)
	}
	if string(f.objects["object"]) != "firstsecond" {
		t.Fatal(f.objects)
	}
}

func TestStorageRejectsMalformedRequests(t *testing.T) {
	for _, tc := range []struct {
		method, url, body string
		status            int
	}{
		{"PUT", "/api/oss/x?uploadId=unknown&partNumber=1", "x", 404},
		{"POST", "/api/oss/x?uploadId=unknown", "", 404},
		{"DELETE", "/api/oss/x?uploadId=unknown", "", 404},
		{"PATCH", "/api/oss/x", "", 405},
		{"PUT", "/api/oss/x?uploadId=x&partNumber=bad", "x", 400},
		{"PUT", "/api/oss/x?uploadId=x&partNumber=0", "x", 400},
		{"POST", "/api/oss/x?uploadId=x", "", 400},
		{"POST", "/api/oss/x?uploadId=x", `<Wrong/>`, 400},
		{"POST", "/api/oss/x?uploadId=x", `<CompleteMultipartUpload/>`, 400},
		{"POST", "/api/oss/x?uploadId=x", `<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>wrong</ETag></Part></CompleteMultipartUpload>`, 400},
		{"POST", "/api/oss/x?uploadId=x", `<CompleteMultipartUpload><Part><PartNumber>2</PartNumber><ETag>synthetic-etag</ETag></Part><Part><PartNumber>1</PartNumber><ETag>synthetic-etag</ETag></Part></CompleteMultipartUpload>`, 400},
	} {
		t.Run(tc.method+tc.url+tc.body, func(t *testing.T) {
			f := fileFixture(t, Official)
			f.uploads["x"] = map[int][]byte{1: []byte("one"), 2: []byte("two")}
			w := storageRequest(f, tc.method, tc.url, tc.body, map[string]string{"Authorization": "synthetic"})
			if w.Code != tc.status {
				t.Fatalf("got %d want %d", w.Code, tc.status)
			}
			if len(f.objects) != 0 {
				t.Fatal("invalid request changed objects")
			}
		})
	}
}
func TestStorageRejectsInvertedRange(t *testing.T) {
	f := fileFixture(t, Official)
	id := f.Seed(0, "x", "x", false, []byte("abcdef"))
	w := storageRequest(f, "GET", f.downloadURL()+"?id="+id.String(), "", map[string]string{"Range": "bytes=4-2"})
	if w.Code != 416 {
		t.Fatal(w.Code)
	}
}
