// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sncloud "github.com/altessa-s/sncloud-go"
)

func TestFacadeDigests(t *testing.T) {
	if id := sncloud.NewDigestIdentifier(); len(id) != 32 {
		t.Fatalf("NewDigestIdentifier len = %d", len(id))
	}

	markPayload := []byte("synthetic-mark")
	var uploaded []byte

	c := newAuthedFacadeClient(t, facadeTransport(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/file/query/summary/group"):
			return jsonResponse(`{"success":true,"totalPages":1,"summaryDOList":[{"id":10,"uniqueIdentifier":"g-uid","name":"Notes","isDeleted":"N"}]}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/add/summary/group"):
			return jsonResponse(`{"success":true,"id":11}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/update/summary/group"):
			return jsonResponse(`{"success":true}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/delete/summary/group"):
			return jsonResponse(`{"success":true}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/query/summary"):
			return jsonResponse(`{"success":true,"totalPages":1,"summaryDOList":[{"id":20,"uniqueIdentifier":"d-uid","parentUniqueIdentifier":"g-uid","content":"Passage","handwriteInnerName":"inner.mark","isDeleted":"N"}]}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/query/summary/id"):
			return jsonResponse(`{"success":true,"summaryDOList":[{"id":20,"uniqueIdentifier":"d-uid","parentUniqueIdentifier":"g-uid","content":"Passage","handwriteInnerName":"inner.mark","isDeleted":"N"}]}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/query/summary/hash"):
			return jsonResponse(`{"success":true,"totalPages":1,"summaryInfoVOList":[{"id":20,"md5Hash":"abc"}]}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/add/summary"):
			return jsonResponse(`{"success":true,"id":21}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/update/summary"):
			return jsonResponse(`{"success":true}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/delete/summary"):
			return jsonResponse(`{"success":true}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/query/summary/tag"):
			return jsonResponse(`{"success":true,"summaryTagDOList":[{"id":30,"name":"review"}]}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/add/summary/tag"):
			return jsonResponse(`{"success":true,"id":31}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/update/summary/tag"):
			return jsonResponse(`{"success":true}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/delete/summary/tag"):
			return jsonResponse(`{"success":true}`), nil
		case strings.HasSuffix(r.URL.Path, "/file/upload/apply/summary"):
			return jsonResponse(`{"success":true,"innerName":"inner.mark","s3Url":"https://s3.example.test/put","s3Authorization":"AWS4-HMAC-SHA256 synthetic","xamzDate":"20261004T043924Z"}`), nil
		case r.URL.Host == "s3.example.test" && r.URL.Path == "/put" && r.Method == http.MethodPut:
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			uploaded = raw
			h := make(http.Header)
			h.Set("ETag", "synthetic-etag")
			return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(strings.NewReader(""))}, nil
		case strings.HasSuffix(r.URL.Path, "/file/download/summary"):
			return jsonResponse(`{"success":true,"url":"https://s3.example.test/get"}`), nil
		case r.URL.Host == "s3.example.test" && r.URL.Path == "/get" && r.Method == http.MethodGet:
			return &http.Response{
				StatusCode:    http.StatusOK,
				ContentLength: int64(len(uploaded)),
				Header:        make(http.Header),
				Body:          io.NopCloser(bytes.NewReader(uploaded)),
			}, nil
		default:
			return jsonResponse(`{"success":false,"errorCode":"E9999"}`), nil
		}
	}))

	ctx := context.Background()
	view := c.Digests()
	groups, err := view.Groups(ctx)
	if err != nil || len(groups) != 1 || groups[0].ID != 10 {
		t.Fatalf("Groups = (%v, %v)", groups, err)
	}
	gid, err := view.CreateGroup(ctx, "Books", nil, "")
	if err != nil || gid != 11 {
		t.Fatalf("CreateGroup = (%v, %v)", gid, err)
	}
	if err := view.UpdateGroup(ctx, 10, "Updated", nil); err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if err := view.DeleteGroup(ctx, 10); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}

	items, err := view.List(ctx, "g-uid")
	if err != nil || len(items) != 1 || items[0].ID != 20 {
		t.Fatalf("List = (%v, %v)", items, err)
	}
	byID, err := view.ByID(ctx, []sncloud.DigestID{20})
	if err != nil || len(byID) != 1 || byID[0].ID != 20 {
		t.Fatalf("ByID = (%v, %v)", byID, err)
	}
	fps, err := view.Fingerprints(ctx, "")
	if err != nil || len(fps) != 1 || fps[0].ID != 20 {
		t.Fatalf("Fingerprints = (%v, %v)", fps, err)
	}
	text := "Passage"
	src := sncloud.TypedDigestSourceType
	did, err := view.Create(ctx, sncloud.DigestDraft{Content: &text, SourceType: &src})
	if err != nil || did != 21 {
		t.Fatalf("Create = (%v, %v)", did, err)
	}
	if err := view.Update(ctx, 20, sncloud.DigestDraft{Comment: &text}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := view.Delete(ctx, 20); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	tags, err := view.Tags(ctx)
	if err != nil || len(tags) != 1 || tags[0].ID != 30 {
		t.Fatalf("Tags = (%v, %v)", tags, err)
	}
	tid, err := view.CreateTag(ctx, "focus")
	if err != nil || tid != 31 {
		t.Fatalf("CreateTag = (%v, %v)", tid, err)
	}
	if err := view.RenameTag(ctx, 30, "later"); err != nil {
		t.Fatalf("RenameTag: %v", err)
	}
	if err := view.DeleteTag(ctx, 30); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}

	markFile := filepath.Join(t.TempDir(), "comment.mark")
	if err := os.WriteFile(markFile, markPayload, 0o600); err != nil {
		t.Fatal(err)
	}
	var upDone int64
	hw, err := view.UploadHandwriting(ctx, markFile, "", func(done, _ int64) { upDone = done })
	if err != nil || hw.InnerName != "inner.mark" || upDone != int64(len(markPayload)) {
		t.Fatalf("UploadHandwriting = (%+v, %v, progress=%d)", hw, err, upDone)
	}
	items[0].HandwritingMD5 = &hw.MD5
	downloadedPath, err := view.DownloadHandwriting(ctx, items[0], nil)
	if err != nil || downloadedPath == "" {
		t.Fatalf("DownloadHandwriting = (%q, %v)", downloadedPath, err)
	}
	gotMark, err := os.ReadFile(downloadedPath)
	_ = os.Remove(downloadedPath)
	if err != nil || !bytes.Equal(gotMark, markPayload) {
		t.Fatalf("downloaded handwriting = (%q, %v)", gotMark, err)
	}
}
