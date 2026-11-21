// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json/v2"
	"errors"
	"testing"
)

func TestDigestIDWireRoundTrip(t *testing.T) {
	const snowflake int64 = 819922212654940160
	id := DigestID(snowflake)
	if id.String() != "819922212654940160" {
		t.Fatalf("String = %q", id.String())
	}
	b, err := json.Marshal(id)
	if err != nil || string(b) != "819922212654940160" {
		t.Fatalf("Marshal DigestID = %s, err=%v", b, err)
	}
	var fromNum, fromStr DigestID
	if err := json.Unmarshal([]byte("819922212654940160"), &fromNum); err != nil || fromNum != id {
		t.Fatalf("fromNum = %v, err=%v", fromNum, err)
	}
	if err := json.Unmarshal([]byte(`"819922212654940160"`), &fromStr); err != nil || fromStr != id {
		t.Fatalf("fromStr = %v, err=%v", fromStr, err)
	}
	var bad DigestID
	if err := json.Unmarshal([]byte(`"nope"`), &bad); err == nil {
		t.Fatal("expected invalid DigestID error")
	}
}

func TestDigestModelsAndFingerprints(t *testing.T) {
	if id := NewDigestIdentifier(); len(id) != 32 {
		t.Fatalf("NewDigestIdentifier len = %d", len(id))
	}

	if got := GroupFingerprint("g-2", "Reading"); got != "5fda197f6aeb44b6d49bb7e5e88917ab" {
		t.Fatalf("GroupFingerprint = %q", got)
	}

	uid := "3df339aff74e797efb1986ab3ebaef1a"
	author := "Synthetic Author"
	meta, err := MergeDigestMetadata(nil, &uid, &author)
	if err != nil {
		t.Fatalf("MergeDigestMetadata: %v", err)
	}
	if meta != `{"author":"Synthetic Author","unique_identifier":"3df339aff74e797efb1986ab3ebaef1a"}` {
		t.Fatalf("unexpected merged metadata: %s", meta)
	}
	content := "Passage"
	group := "g-1"
	srcType := TypedDigestSourceType
	comment := "Note"
	sum := ComputeDigestFingerprint(DigestFingerprintFields{
		Content:    &content,
		Group:      &group,
		SourceType: &srcType,
		Metadata:   &meta,
		Comment:    &comment,
	})
	// Independently computed MD5 of Passage + g-1 + 4 + metadata + Note, in tablet order.
	const wantSum = "fb33a3369795248a2e4621ce019df2de"
	if sum != wantSum {
		t.Fatalf("ComputeDigestFingerprint = %q", sum)
	}

	// Existing unique_identifier is preserved; slashes are not escaped; nested keys sorted.
	existing := `{"path":"a/b","unique_identifier":"keep-me","nested":{"z":1,"a":2}}`
	other := "ignored"
	merged, err := MergeDigestMetadata(&existing, &other, nil)
	if err != nil {
		t.Fatalf("MergeDigestMetadata existing: %v", err)
	}
	if merged != `{"nested":{"a":2,"z":1},"path":"a/b","unique_identifier":"keep-me"}` {
		t.Fatalf("merged = %s", merged)
	}

	for _, badMeta := range []string{`[1,2]`, `"str"`, `{bad`} {
		s := badMeta
		if _, err := MergeDigestMetadata(&s, nil, nil); err == nil {
			t.Fatalf("expected error for non-object metadata %s", badMeta)
		}
	}

	goodTags := []string{"quotes", "books"}
	if err := ValidateDigestDraft(DigestDraft{Tags: &goodTags}); err != nil {
		t.Fatalf("ValidateDigestDraft valid: %v", err)
	}
	badTags := []string{"a,b"}
	if _, ok := errors.AsType[*MalformedResponseError](ValidateDigestDraft(DigestDraft{Tags: &badTags})); !ok {
		t.Fatal("expected MalformedResponseError for comma tag")
	}
}

func TestDigestGroupFingerprintAndTagUnmarshaling(t *testing.T) {
	var grp DigestGroup
	groupJSON := `{"id":5,"uniqueIdentifier":"g-1","name":"Reading","description":"","md5Hash":"26fccddf2f94b1685b184267996e10f8","creationTime":1773292688000,"lastModifiedTime":0}`
	if err := json.Unmarshal([]byte(groupJSON), &grp); err != nil {
		t.Fatalf("DigestGroup: %v", err)
	}
	if grp.ID != 5 || grp.UniqueIdentifier != "g-1" || grp.Description != nil || grp.Modified != nil || grp.Created == nil {
		t.Fatalf("unexpected DigestGroup: %+v", grp)
	}

	// Metadata author overrides legacy column, including clearing with empty string.
	digestJSON := `{
		"id":"819922212654940160","uniqueIdentifier":"","parentUniqueIdentifier":"g-1","fileId":"42",
		"content":"Passage","sourcePath":"/Document/book.pdf","dataSource":"pdf","sourceType":1,
		"tags":" quotes , reading ,, ","md5Hash":"abc","metadata":"{\"author\":\"\",\"unique_identifier\":\"d-1\"}",
		"author":"Legacy Column","commentStr":"Typed","commentHandwriteName":"h.mark",
		"handwriteInnerName":"in.mark","handwriteMD5":"ff","creationTime":1773292688000,"lastModifiedTime":1773292689000
	}`
	var d Digest
	if err := json.Unmarshal([]byte(digestJSON), &d); err != nil {
		t.Fatalf("Digest: %v", err)
	}
	if d.UniqueIdentifier == nil || *d.UniqueIdentifier != "d-1" || d.Author != nil || d.FileID == nil || *d.FileID != 42 {
		t.Fatalf("unexpected Digest: %+v", d)
	}
	if len(d.Tags) != 2 || d.Tags[0] != "quotes" || d.Tags[1] != "reading" {
		t.Fatalf("unexpected Tags: %v", d.Tags)
	}

	// Legacy column author used when metadata has no author key.
	var legacy Digest
	if err := json.Unmarshal([]byte(`{"id":1,"author":"ColumnWriter","metadata":"not-json"}`), &legacy); err != nil {
		t.Fatalf("legacy Digest: %v", err)
	}
	if legacy.Author == nil || *legacy.Author != "ColumnWriter" || len(legacy.Tags) != 0 {
		t.Fatalf("unexpected legacy Digest: %+v", legacy)
	}

	var fp DigestFingerprint
	if err := json.Unmarshal([]byte(`{"id":9,"md5Hash":"aa","handwriteMd5":"bb","commentHandwriteName":"c.mark","lastModifiedTime":1773292690000,"metadataMap":{"author":"Reader"}}`), &fp); err != nil {
		t.Fatalf("DigestFingerprint: %v", err)
	}
	if fp.ID != 9 || fp.HandwritingMD5 == nil || *fp.HandwritingMD5 != "bb" || fp.Author == nil || *fp.Author != "Reader" {
		t.Fatalf("unexpected DigestFingerprint: %+v", fp)
	}
	var fpUpper DigestFingerprint
	if err := json.Unmarshal([]byte(`{"id":10,"handwriteMD5":"cc"}`), &fpUpper); err != nil || fpUpper.HandwritingMD5 == nil || *fpUpper.HandwritingMD5 != "cc" {
		t.Fatalf("upper handwriteMD5: %+v, err=%v", fpUpper, err)
	}

	var tag DigestTag
	if err := json.Unmarshal([]byte(`{"id":3,"name":"quotes"}`), &tag); err != nil || tag.ID != 3 || tag.Name != "quotes" {
		t.Fatalf("DigestTag: %+v, err=%v", tag, err)
	}

	for _, bad := range []string{`[]`, `{}`, `{"id":null}`, `{"id":"bad"}`} {
		var bg DigestGroup
		var bd Digest
		var bfp DigestFingerprint
		var bt DigestTag
		if json.Unmarshal([]byte(bad), &bg) == nil || json.Unmarshal([]byte(bad), &bd) == nil ||
			json.Unmarshal([]byte(bad), &bfp) == nil || json.Unmarshal([]byte(bad), &bt) == nil {
			t.Fatalf("expected error for %s", bad)
		}
	}
	var badTagName DigestTag
	if json.Unmarshal([]byte(`{"id":1,"name":123}`), &badTagName) == nil {
		t.Fatal("expected error for non-string tag name")
	}
}

func TestMetadataPreservesNumbers(t *testing.T) {
	metadata := `{"z":9007199254740993,"nested":{"z":1.2300,"a":[9223372036854775807,-9007199254740993]},"a":18446744073709551615}`
	author := "Reader"
	const want = `{"a":18446744073709551615,"author":"Reader","nested":{"a":[9223372036854775807,-9007199254740993],"z":1.2300},"z":9007199254740993}`
	for range 2 {
		got, err := MergeDigestMetadata(&metadata, nil, &author)
		if err != nil || got != want {
			t.Fatalf("merge = %s, %v; want %s", got, err, want)
		}
		metadata = got
	}
}

func TestFingerprintEveryField(t *testing.T) {
	content, group, path, metadata, comment, handwriting := "content", "group", "/path", `{"page":2}`, "comment", "hand.mark"
	sourceType := 4
	fields := DigestFingerprintFields{&content, &group, &sourceType, &path, &metadata, &comment, &handwriting}
	// Independent MD5 of the literal fields concatenated in tablet order; every field is distinct and nonempty.
	const want = "5f954f20389070a7111d88e20fba955f"
	if got := ComputeDigestFingerprint(fields); got != want {
		t.Fatalf("fingerprint = %s, want %s", got, want)
	}
	if got := ComputeDigestFingerprint(DigestFingerprintFields{}); got != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Fatalf("empty fingerprint = %s", got)
	}
}
