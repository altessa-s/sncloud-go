// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/base64"
	"encoding/json/v2"
	"testing"
)

func TestFeedSignatureAndChanges(t *testing.T) {
	// Unsigned, expired JWT for user 1. The base64 MAC contains '+', '/' and '='.
	encode := base64.RawURLEncoding.EncodeToString
	token := encode([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + encode([]byte(`{"userId":1,"exp":1}`)) + "."
	input := token + "_ANDROID_1002"
	const want = "sGp919U8eJMJo3HBbtxhgqjmXlDgM7LZ0bz5DNp9w"
	if got := FeedSignature(input); got != want {
		t.Fatalf("FeedSignature = %q, want %q", got, want)
	}
	if ChangeFileAdded.String() != "DOWNLOADFILE" || DigestItemDigest.String() != "2" {
		t.Fatal("unexpected String()")
	}

	fileJSON := `{
		"messageType":"MOVEFOLDER","fileType":"FOLDE","id":"819922212654940160","newId":9,
		"originalName":"Old","newName":"New","md5":"ABCDEF","size":"1024",
		"directoryId":"0","goDirectoryId":"5","originalPath":"/Old","newPath":"/Dest/New",
		"equipmentNo":"MACOS123","timestamp":1773292688000
	}`
	var fc FileChange
	if err := json.Unmarshal([]byte(fileJSON), &fc); err != nil {
		t.Fatalf("FileChange: %v", err)
	}
	if fc.Kind != ChangeFolderMoved || !fc.IsFolder || fc.ID == nil || *fc.ID != 819922212654940160 || fc.MD5 == nil || *fc.MD5 != "abcdef" {
		t.Fatalf("unexpected FileChange: %+v", fc)
	}

	var tc TaskChange
	if err := json.Unmarshal([]byte(`{"messageType":"QUERY","taskListId":"1","taskId":"t-1","equipmentNo":"MACOS123","timestamp":1773292688000}`), &tc); err != nil {
		t.Fatalf("TaskChange: %v", err)
	}
	if tc.Kind != ChangeTasksChanged || tc.ListID == nil || *tc.ListID != "1" || tc.TaskID == nil || *tc.TaskID != "t-1" {
		t.Fatalf("unexpected TaskChange: %+v", tc)
	}

	var dcStr, dcNum DigestChange
	if err := json.Unmarshal([]byte(`{"messageType":"ADD_DIGEST","dataType":"2","id":77,"parentUniqueIdentifier":"g-1","uniqueIdentifier":"u-1","equipmentNo":"ANDROID1","timestamp":1773292688000}`), &dcStr); err != nil {
		t.Fatalf("DigestChange str: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"messageType":"UPDATE_DIGEST","dataType":3,"id":"88"}`), &dcNum); err != nil {
		t.Fatalf("DigestChange num: %v", err)
	}
	if dcStr.Item != DigestItemDigest || dcStr.ID == nil || *dcStr.ID != 77 || dcNum.Item != DigestItemTag || dcNum.ID == nil || *dcNum.ID != 88 {
		t.Fatalf("unexpected DigestChanges: %+v / %+v", dcStr, dcNum)
	}

	var badFC FileChange
	var badTC TaskChange
	var badDC DigestChange
	if json.Unmarshal([]byte(`[]`), &badFC) == nil || json.Unmarshal([]byte(`[]`), &badTC) == nil || json.Unmarshal([]byte(`[]`), &badDC) == nil {
		t.Fatal("expected slice decode errors")
	}
}
