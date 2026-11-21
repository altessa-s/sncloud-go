// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package digests

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/session"
	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

const (
	testClientID         = "aabbccddeeff00112233445566778899"
	snowflake      int64 = 1246815520031113216
	sourceFileSnow int64 = 819922212654940160
)

func syntheticToken() string {
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"exp":4102444800}`))
	return hdr + "." + claims + ".sig"
}

func newTestDigestClient(e domain.Endpoints, tr *porttest.Transport) (*Client, *session.Session, *porttest.Secrets) {
	clk := &porttest.Clock{Time: time.Unix(1780000000, 0)}
	secrets := &porttest.Secrets{}
	_ = secrets.Keep(syntheticToken(), domain.AccessToken)
	_ = secrets.Keep("synthetic@example.com", domain.Account)
	_ = secrets.Keep(testClientID, domain.ClientID)
	policy := domain.DefaultRetryPolicy()
	ch := channel.New(e, tr, clk, policy)
	sess := session.New(ch, secrets, clk)
	b := transfers.New(tr, clk, policy)
	return New(e, ch, sess, b, clk), sess, secrets
}

func decodeBodyNumberPreserved(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("decode JSON %s: %v", raw, err)
	}
	return m
}

const digestRow = `{"id":1246815520031113216,"fileId":819922212654940160,"name":null,"userId":7,"author":"Synthetic Reader",` +
	`"uniqueIdentifier":"d-1","parentUniqueIdentifier":"g-1","content":"Knowledge is power",` +
	`"sourcePath":"/Document/Bacon.pdf","dataSource":"pdf","sourceType":1,"isSummaryGroup":"N",` +
	`"tags":"quotes, philosophy","md5Hash":"a9e0a281783005770d332e903e28b2d1","metadata":"{\"page\":3}",` +
	`"commentStr":"","commentHandwriteName":"c.mark","handwriteInnerName":"abc.mark","handwriteMD5":"ff",` +
	`"creationTime":1773292688000,"lastModifiedTime":1773292690000,"isDeleted":"N"}`

const groupRow = `{"id":5,"uniqueIdentifier":"g-1","name":"Reading","description":"","md5Hash":"26fccddf2f94b1685b184267996e10f8",` +
	`"isSummaryGroup":"Y","creationTime":1773292688000,"lastModifiedTime":0,"isDeleted":"N"}`

const tabletGroup = `{"id":894400129920925696,"name":"Test","uniqueIdentifier":"91ec9680c77eccd1d9f078a0da1a02b9","isSummaryGroup":"Y",` +
	`"description":"","md5Hash":"79054dfc2cb0161cf39e5ed22691b90b","creationTime":1791049603511,"lastModifiedTime":1791049603511,` +
	`"isDeleted":"N","createTime":1791049608000}`

const tabletDigest = `{"id":894400223386796032,"fileId":null,"uniqueIdentifier":null,"parentUniqueIdentifier":"91ec9680c77eccd1d9f078a0da1a02b9",` +
	`"content":"Just a test","sourcePath":"","sourceType":4,"isSummaryGroup":"N","tags":null,"md5Hash":"cf1c6ac9c2cd95c169bd7bb07194d91d",` +
	`"metadata":"{\"unique_identifier\":\"3df339aff74e797efb1986ab3ebaef1a\"}","commentStr":"","commentHandwriteName":"",` +
	`"creationTime":1791049627864,"lastModifiedTime":1791049627864,"isDeleted":"N","createTime":1791049630000}`

func TestListsCreatesUpdatesAndDeletesDigests(t *testing.T) {
	tr := &porttest.Transport{
		Steps: []porttest.Step{
			porttest.OK(`"totalRecords":3,"totalPages":2,"currentPage":1,"pageSize":100,"summaryDOList":[` + digestRow + `]`),
			porttest.OK(`"totalRecords":3,"totalPages":2,"currentPage":2,"pageSize":100,"summaryDOList":[{"id":2,"md5Hash":"x","isDeleted":"Y"}]`),
			porttest.OK(`"id":1246815520031113216`),
			porttest.OK(""),
			porttest.OK(""),
		},
	}
	client, _, _ := newTestDigestClient(domain.Official(), tr)
	ctx := context.Background()

	list, err := client.List(ctx, "g-1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	c0 := decodeBodyNumberPreserved(t, tr.Calls()[0].Body)
	if tr.Calls()[0].URL != "https://viewer.supernote.com/api/file/query/summary" ||
		c0["page"] != json.Number("1") || c0["size"] != json.Number("100") || c0["parentUniqueIdentifier"] != "g-1" {
		t.Fatalf("unexpected page 1 call: %+v", c0)
	}
	if len(list) != 1 || int64(list[0].ID) != snowflake || int64(*list[0].FileID) != sourceFileSnow || list[0].Comment != nil {
		t.Fatalf("unexpected digest: %+v", list)
	}

	content := "Knowledge is power"
	group := "g-1"
	fid := domain.FileID(sourceFileSnow)
	tags := []string{"quotes", "philosophy"}
	madeID, err := client.Create(ctx, domain.DigestDraft{
		UniqueIdentifier: "d-1",
		Content:          &content,
		GroupIdentifier:  &group,
		FileID:           &fid,
		Tags:             &tags,
	})
	if err != nil || int64(madeID) != snowflake {
		t.Fatalf("Create = (%v, %v)", madeID, err)
	}
	addBody := decodeBodyNumberPreserved(t, tr.Calls()[2].Body)
	if _, has := addBody["uniqueIdentifier"]; has {
		t.Fatal("uniqueIdentifier column must be omitted")
	}
	if addBody["metadata"] != `{"unique_identifier":"d-1"}` ||
		addBody["fileId"] != json.Number("819922212654940160") ||
		addBody["tags"] != "quotes,philosophy" ||
		addBody["sourceType"] != json.Number("4") ||
		addBody["md5Hash"] != "a83f342142b6c54bb8ab9c5b31391ef9" {
		t.Fatalf("unexpected Create body: %+v", addBody)
	}

	ds := "pdf"
	if err := client.Update(ctx, domain.DigestID(snowflake), domain.DigestDraft{DataSource: &ds}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updBody := decodeBodyNumberPreserved(t, tr.Calls()[3].Body)
	if len(updBody) != 3 || updBody["id"] != json.Number("1246815520031113216") || updBody["dataSource"] != "pdf" {
		t.Fatalf("expected only id, dataSource and lastModifiedTime, got %+v", updBody)
	}

	if err := client.Delete(ctx, domain.DigestID(snowflake)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	delBody := decodeBodyNumberPreserved(t, tr.Calls()[4].Body)
	if tr.Calls()[4].Method != "DELETE" || delBody["id"] != json.Number("1246815520031113216") {
		t.Fatalf("unexpected Delete: %+v", delBody)
	}
}

func TestGroupsFingerprintsTagsAndPagingRules(t *testing.T) {
	fingerprintRow := `{"id":9,"userId":7,"md5Hash":"aa","handwriteMd5":"bb","commentHandwriteName":"c.mark","lastModifiedTime":1773292690000,"metadataMap":{"author":"Reader"}}`
	tr := &porttest.Transport{
		Steps: []porttest.Step{
			porttest.OK(`"totalPages":1,"currentPage":1,"summaryDOList":[` + groupRow + `]`),
			porttest.OK(`"id":6`),
			porttest.OK(""),
			porttest.OK(`"totalPages":1,"summaryInfoVOList":[` + fingerprintRow + `]`),
			porttest.OK(`"summaryTagDOList":[{"id":3,"name":"quotes","userId":7,"uniqueIdentifier":null}]`),
			porttest.OK(`"id":11`),
			porttest.OK(""),
			porttest.OK(""),
			porttest.Refused("E0335"),
			porttest.Refused("E0336"),
			porttest.OK(`"summaryDOList":[` + digestRow + `]`),
			porttest.OK(`"totalPages":2,"summaryDOList":[` + digestRow + `]`),
			porttest.OK(`"totalPages":2,"summaryDOList":[` + digestRow + `]`),
			porttest.OK(`"summaryDOList":[` + digestRow + `]`),
			porttest.OK(`"totalPages":1,"summaryDOList":[` + groupRow + `]`),
			porttest.OK(""),
			porttest.Refused("E0338"),
		},
	}
	client, _, _ := newTestDigestClient(domain.Official(), tr)
	ctx := context.Background()

	groups, err := client.Groups(ctx)
	if err != nil || len(groups) != 1 || groups[0].ID != 5 || groups[0].Name != "Reading" {
		t.Fatalf("Groups = (%+v, %v)", groups, err)
	}
	grpID, err := client.CreateGroup(ctx, "Reading", nil, "g-2")
	if err != nil || grpID != 6 {
		t.Fatalf("CreateGroup = (%v, %v)", grpID, err)
	}
	gAddBody := decodeBodyNumberPreserved(t, tr.Calls()[1].Body)
	if gAddBody["md5Hash"] != "5fda197f6aeb44b6d49bb7e5e88917ab" {
		t.Fatalf("group md5Hash = %v", gAddBody["md5Hash"])
	}
	if err := client.DeleteGroup(ctx, grpID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}

	fps, err := client.Fingerprints(ctx, "")
	if err != nil || len(fps) != 1 || fps[0].ID != 9 || *fps[0].HandwritingMD5 != "bb" || *fps[0].Author != "Reader" {
		t.Fatalf("Fingerprints = (%+v, %v)", fps, err)
	}

	tags, err := client.Tags(ctx)
	if err != nil || len(tags) != 1 || tags[0].ID != 3 || tags[0].Name != "quotes" {
		t.Fatalf("Tags = (%+v, %v)", tags, err)
	}
	if tr.Calls()[4].Method != "GET" || len(tr.Calls()[4].Body) != 0 {
		t.Fatal("Tags must be bodyless GET")
	}
	tagID, err := client.CreateTag(ctx, "reading")
	if err != nil || tagID != 11 {
		t.Fatalf("CreateTag = (%v, %v)", tagID, err)
	}
	if err := client.RenameTag(ctx, tagID, "books"); err != nil {
		t.Fatalf("RenameTag: %v", err)
	}
	if err := client.DeleteTag(ctx, tagID); err != nil {
		t.Fatalf("DeleteTag: %v", err)
	}
	if _, err := client.CreateTag(ctx, "quotes"); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}
	var nf *domain.NotFoundError
	if err := client.RenameTag(ctx, 4, "x"); !errors.As(err, &nf) || nf.Code != "E0336" {
		t.Fatalf("expected NotFoundError E0336, got %v", err)
	}

	// Short page without totalPages stops after page 1.
	shortPage, err := client.List(ctx, "")
	if err != nil || len(shortPage) != 1 || int64(shortPage[0].ID) != snowflake {
		t.Fatalf("shortPage = (%+v, %v)", shortPage, err)
	}
	// Duplicate across two pages is deduplicated.
	dedup, err := client.List(ctx, "")
	if err != nil || len(dedup) != 1 || int64(dedup[0].ID) != snowflake {
		t.Fatalf("dedup = (%+v, %v)", dedup, err)
	}

	// Empty ByID returns immediately without transport call.
	beforeEmpty := len(tr.Calls())
	emptyByID, err := client.ByID(ctx, nil)
	if err != nil || len(emptyByID) != 0 || len(tr.Calls()) != beforeEmpty {
		t.Fatalf("empty ByID = (%+v, %v)", emptyByID, err)
	}

	byID, err := client.ByID(ctx, []domain.DigestID{domain.DigestID(snowflake)})
	if err != nil || len(byID) != 1 || int64(byID[0].ID) != snowflake {
		t.Fatalf("ByID = (%+v, %v)", byID, err)
	}

	if err := client.UpdateGroup(ctx, 5, "Reading", nil); err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	updGrp := decodeBodyNumberPreserved(t, tr.Calls()[len(tr.Calls())-1].Body)
	if updGrp["md5Hash"] != "f95a59c592ee87c6bbc223c937caffd2" {
		t.Fatalf("updated group md5Hash = %v", updGrp["md5Hash"])
	}
	if _, err := client.CreateGroup(ctx, "Reading", nil, ""); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists E0338, got %v", err)
	}

	// Comma in tag fails locally before touching transport.
	beforeComma := len(tr.Calls())
	commaTags := []string{"a,b"}
	if _, err := client.Create(ctx, domain.DigestDraft{Tags: &commaTags}); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal("expected comma error on Create")
	}
	if err := client.Update(ctx, 1, domain.DigestDraft{Tags: &commaTags}); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal("expected comma error on Update")
	}
	if len(tr.Calls()) != beforeComma {
		t.Fatal("comma tag reached transport")
	}
}

func TestTabletRefingerprintingBookDigestAuthorAndHandwritingAttachment(t *testing.T) {
	// Book digest fingerprint fixture.
	bookMeta := `{"source_size":4429568,"unique_identifier":"ca87f7ee7920898de1f9c15b8f36fb5b"}`
	bookPassage := "A passage"
	bookGroup := ""
	bookType := 1
	bookPath := "INBOX/book.epub"
	bookComment := "Hello"
	bookHW := "550592c2999349ec7a89006ef5bc157c.mark"
	if got := domain.ComputeDigestFingerprint(domain.DigestFingerprintFields{
		Content:         &bookPassage,
		Group:           &bookGroup,
		SourceType:      &bookType,
		SourcePath:      &bookPath,
		Metadata:        &bookMeta,
		Comment:         &bookComment,
		HandwritingName: &bookHW,
	}); got != "5c309a5d70d280285c3120d038aac7bc" {
		t.Fatalf("book digest sum = %q", got)
	}

	tr := &porttest.Transport{
		Steps: []porttest.Step{
			porttest.OK(`"summaryDOList":[` + tabletDigest + `]`),
			porttest.OK(""),
			porttest.OK(`"id":7`),
			porttest.OK(`"summaryDOList":[` + tabletDigest + `]`),
			porttest.OK(""),
			porttest.OK(`"summaryDOList":[` + tabletDigest + `]`),
			porttest.OK(""),
			porttest.OK(`"summaryDOList":[]`),
			porttest.OK(`"totalPages":1,"summaryDOList":[]`),
			porttest.OK(""),
			porttest.Refused("E0712"),
		},
	}
	client, _, _ := newTestDigestClient(domain.Official(), tr)
	ctx := context.Background()
	const tabID = domain.DigestID(894400223386796032)

	newContent := "Knowledge is power"
	if err := client.Update(ctx, tabID, domain.DigestDraft{Content: &newContent}); err != nil {
		t.Fatalf("refingerprint Update: %v", err)
	}
	if decodeBodyNumberPreserved(t, tr.Calls()[1].Body)["md5Hash"] != "46da161db8902c8e836cae21b3c560e8" {
		t.Fatalf("refingerprint md5Hash = %v", decodeBodyNumberPreserved(t, tr.Calls()[1].Body)["md5Hash"])
	}

	shortText := "x"
	writer := "Author One"
	if _, err := client.Create(ctx, domain.DigestDraft{UniqueIdentifier: "d-2", Content: &shortText, Author: &writer}); err != nil {
		t.Fatalf("Create with author: %v", err)
	}
	cBody := decodeBodyNumberPreserved(t, tr.Calls()[2].Body)
	if _, has := cBody["author"]; has {
		t.Fatal("author column must not be sent")
	}
	if cBody["metadata"] != `{"author":"Author One","unique_identifier":"d-2"}` {
		t.Fatalf("metadata = %v", cBody["metadata"])
	}

	if err := client.Update(ctx, tabID, domain.DigestDraft{Author: &writer}); err != nil {
		t.Fatalf("Update author: %v", err)
	}
	uBody := decodeBodyNumberPreserved(t, tr.Calls()[4].Body)
	wantAuthorMeta := `{"author":"Author One","unique_identifier":"3df339aff74e797efb1986ab3ebaef1a"}`
	if uBody["metadata"] != wantAuthorMeta {
		t.Fatalf("updated metadata = %v", uBody["metadata"])
	}

	hw := domain.DigestHandwriting{Name: "abc.mark", InnerName: "2d40f4bb-MAC.mark", MD5: "6a9cb52b577a7354e41c42757c777289"}
	if err := client.Update(ctx, tabID, domain.DigestDraft{Handwriting: &hw}); err != nil {
		t.Fatalf("Update handwriting: %v", err)
	}
	hwBody := decodeBodyNumberPreserved(t, tr.Calls()[6].Body)
	if hwBody["commentHandwriteName"] != "abc.mark" || hwBody["handwriteInnerName"] != "2d40f4bb-MAC.mark" ||
		hwBody["md5Hash"] != "cd9a863951383592b9effb5fb42662a9" {
		t.Fatalf("handwriting body = %+v", hwBody)
	}

	// Missing digest on hashed update -> E0340; missing group on UpdateGroup -> E0339; missing ID on add -> malformed.
	var nf *domain.NotFoundError
	if err := client.Update(ctx, 999, domain.DigestDraft{Content: &newContent}); !errors.As(err, &nf) || nf.Code != "E0340" {
		t.Fatalf("expected E0340, got %v", err)
	}
	if err := client.UpdateGroup(ctx, 999, "Missing", nil); !errors.As(err, &nf) || nf.Code != "E0339" {
		t.Fatalf("expected E0339, got %v", err)
	}
	if _, err := client.CreateTag(ctx, "no-id"); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse on missing id, got %v", err)
	}
	if _, err := client.Tags(ctx); !errors.Is(err, domain.ErrSessionExpired) {
		t.Fatalf("expected ErrSessionExpired, got %v", err)
	}
}

func TestRefusesOverlappingDigestWritesOnSameSession(t *testing.T) {
	arrived := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	tr := &porttest.Transport{
		Steps: []porttest.Step{
			{Run: func(_ context.Context, _ port.Request) (port.Response, error) {
				once.Do(func() { close(arrived) })
				<-release
				return porttest.OK("").Response, nil
			}},
			porttest.OK(""),
		},
	}
	clk := &porttest.Clock{Time: time.Unix(1780000000, 0)}
	secrets := &porttest.Secrets{}
	_ = secrets.Keep(syntheticToken(), domain.AccessToken)
	_ = secrets.Keep(testClientID, domain.ClientID)
	policy := domain.DefaultRetryPolicy()
	ch := channel.New(domain.Official(), tr, clk, policy)
	sess := session.New(ch, secrets, clk)
	b := transfers.New(tr, clk, policy)

	first := New(domain.Official(), ch, sess, b, clk)
	second := New(domain.Official(), ch, sess, b, clk)

	errCh := make(chan error, 1)
	ds := "pdf"
	go func() {
		errCh <- first.Update(context.Background(), 9, domain.DigestDraft{DataSource: &ds})
	}()
	<-arrived

	var rej *domain.RejectedError
	err := second.Delete(context.Background(), 9)
	if !errors.As(err, &rej) || rej.Code != domain.DigestBusyCode {
		t.Fatalf("expected SNCloud.busy RejectedError, got %v", err)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatalf("first Update: %v", err)
	}
	if err := second.Delete(context.Background(), 9); err != nil {
		t.Fatalf("Delete after release: %v", err)
	}
}

func TestHandwritingUploadDownloadAndPrivateOSSRewriting(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	srcFile := filepath.Join(tmpDir, "comment.mark")
	const strokes = "handwritten strokes"
	const strokesMD5 = "6a9cb52b577a7354e41c42757c777289"
	if err := os.WriteFile(srcFile, []byte(strokes), 0600); err != nil {
		t.Fatal(err)
	}

	ep := domain.PrivateCloud("http://nas.local:8080", false)
	tr := &porttest.Transport{
		Steps: []porttest.Step{
			// 1. Official-style S3 PUT slot (s3Url + s3Authorization + server-chosen innerName).
			porttest.OK(`"s3Url":"https://uk-ratta-cloud.s3.eu-west-2.amazonaws.com/2d40f4bb-MAC.mark","s3Authorization":"AWS4-HMAC-SHA256 sig","xamzDate":"20261004T043924Z","innerName":"2d40f4bb-MAC.mark","fileServer":"3","apiServer":1,"url":null`),
			{Response: port.Response{Status: http.StatusOK}},
			// 2. Private-cloud multipart form upload (fullUploadUrl rewritten onto configured AppHost, stored name returned by form answer).
			porttest.OK(`"fullUploadUrl":"http://127.0.0.1:19000/api/oss/upload","partUploadUrl":"http://127.0.0.1:19000/api/oss/part","xAmzDate":"20261004T043924Z"`),
			porttest.OK(`"innerName":"fallback.mark","md5":"` + strokesMD5 + `"`),
			// 3. Private /api/oss/ download rewritten onto configured AppHost and verified against MD5.
			porttest.OK(`"url":"http://127.0.0.1:19000/api/oss/download?s=1"`),
			{Response: port.Response{Status: http.StatusOK, Body: []byte(strokes)}},
			// 4. External S3 download left untouched, failing on checksum mismatch.
			porttest.OK(`"url":"https://uk-ratta-cloud.s3.eu-west-2.amazonaws.com/abc.mark?X-Amz-Signature=s"`),
			{Response: port.Response{Status: http.StatusOK, Body: []byte("corrupted")}},
			// 5. Empty and invalid download links.
			porttest.OK(`"url":""`),
			porttest.OK(`"url":"://bad"`),
		},
	}
	client, _, _ := newTestDigestClient(ep, tr)

	// Digest without handwriting skips network I/O completely.
	if path, err := client.DownloadHandwriting(ctx, domain.Digest{ID: 1}, nil); err != nil || path != "" {
		t.Fatalf("plain digest without handwriting = (%q, %v)", path, err)
	}
	if len(tr.Calls()) != 0 {
		t.Fatal("DownloadHandwriting without innerName touched transport")
	}

	var uploadSawProgress bool
	hw, err := client.UploadHandwriting(ctx, srcFile, "abc.mark", func(done, total int64) {
		if done == total && total == int64(len(strokes)) {
			uploadSawProgress = true
		}
	})
	if err != nil {
		t.Fatalf("UploadHandwriting: %v", err)
	}
	if !uploadSawProgress || hw != (domain.DigestHandwriting{Name: "abc.mark", InnerName: "2d40f4bb-MAC.mark", MD5: strokesMD5}) {
		t.Fatalf("unexpected DigestHandwriting: %+v (progress=%v)", hw, uploadSawProgress)
	}
	calls := tr.Calls()
	applyBody := decodeBodyNumberPreserved(t, calls[0].Body)
	if applyBody["fileName"] != "abc.mark" || calls[0].Header.Get("channel") == "" {
		t.Fatalf("unexpected apply call: %+v", applyBody)
	}
	if calls[1].Method != "PUT" ||
		calls[1].URL != "https://uk-ratta-cloud.s3.eu-west-2.amazonaws.com/2d40f4bb-MAC.mark" ||
		calls[1].Header.Get("Authorization") != "AWS4-HMAC-SHA256 sig" ||
		calls[1].Header.Get(domain.AccessTokenHeader) != "" {
		t.Fatalf("unexpected S3 PUT call: %+v", calls[1])
	}

	// Default generated <32 hex>.mark name and fallback to form stored name when slot omits innerName.
	hwAuto, err := client.UploadHandwriting(ctx, srcFile, "", nil)
	if err != nil || len(hwAuto.Name) != 37 || hwAuto.InnerName != "fallback.mark" {
		t.Fatalf("UploadHandwriting auto = (%+v, %v)", hwAuto, err)
	}
	calls = tr.Calls()
	if calls[3].URL != "http://nas.local:8080/api/oss/upload" || calls[3].Header.Get(domain.AccessTokenHeader) == "" {
		t.Fatalf("expected rewritten private form upload with token, got %s %+v", calls[3].URL, calls[3].Header)
	}

	// Private /api/oss/ link is rewritten onto configured AppHost and returns a caller-owned temp file.
	innerName := "abc.mark"
	wantMD5 := strokesMD5
	var downloadSawProgress bool
	gotPath, err := client.DownloadHandwriting(ctx, domain.Digest{
		ID:                   domain.DigestID(snowflake),
		HandwritingInnerName: &innerName,
		HandwritingMD5:       &wantMD5,
	}, func(done, total int64) {
		if done == total && total == int64(len(strokes)) {
			downloadSawProgress = true
		}
	})
	if err != nil || gotPath == "" || !downloadSawProgress {
		t.Fatalf("DownloadHandwriting = (%q, %v), progress=%v", gotPath, err, downloadSawProgress)
	}
	downloaded, readErr := os.ReadFile(gotPath)
	_ = os.Remove(gotPath)
	if readErr != nil || string(downloaded) != strokes {
		t.Fatalf("downloaded bytes = (%q, %v)", downloaded, readErr)
	}
	calls = tr.Calls()
	if calls[5].URL != "http://nas.local:8080/api/oss/download?s=1" || calls[5].Header.Get(domain.AccessTokenHeader) != "" {
		t.Fatalf("expected rewritten /api/oss/ URL without token, got %q %+v", calls[5].URL, calls[5].Header)
	}

	// Checksum mismatch when bytes disagree with digest's HandwritingMD5, and external S3 URL stays untouched.
	if _, err := client.DownloadHandwriting(ctx, domain.Digest{
		ID:                   domain.DigestID(snowflake),
		HandwritingInnerName: &innerName,
		HandwritingMD5:       &wantMD5,
	}, nil); !errors.Is(err, domain.ErrChecksumMismatch) {
		t.Fatalf("expected ErrChecksumMismatch, got %v", err)
	}
	calls = tr.Calls()
	if calls[7].URL != "https://uk-ratta-cloud.s3.eu-west-2.amazonaws.com/abc.mark?X-Amz-Signature=s" {
		t.Fatalf("external S3 URL must not be rewritten, got %q", calls[7].URL)
	}

	// Missing or invalid download links fail as ErrMalformedResponse.
	if _, err := client.DownloadHandwriting(ctx, domain.Digest{ID: 1, HandwritingInnerName: &innerName}, nil); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal("expected ErrMalformedResponse on empty link")
	}
	if _, err := client.DownloadHandwriting(ctx, domain.Digest{ID: 1, HandwritingInnerName: &innerName}, nil); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatal("expected ErrMalformedResponse on invalid link")
	}
	if _, err := client.UploadHandwriting(ctx, filepath.Join(tmpDir, "missing.mark"), "x.mark", nil); err == nil {
		t.Fatal("expected error when local handwriting file does not exist")
	}

	// Page ceiling stops a runaway digest server.
	runawayTr := &porttest.Transport{
		Steps: []porttest.Step{
			porttest.OK(`"totalPages":99,"summaryDOList":[` + digestRow + `]`),
			porttest.OK(`"totalPages":99,"summaryDOList":[` + digestRow + `]`),
		},
	}
	limited, _, _ := newTestDigestClient(domain.Official(), runawayTr)
	limited.pageMax = 2
	if _, err := limited.List(ctx, ""); !errors.Is(err, domain.ErrMalformedResponse) {
		t.Fatalf("expected ErrMalformedResponse past pageMax, got %v", err)
	}
}

func TestTagCommasRejectedLocally(t *testing.T) {
	tr := &porttest.Transport{}
	client, _, _ := newTestDigestClient(domain.Official(), tr)
	_, createErr := client.CreateTag(t.Context(), "one,two")
	renameErr := client.RenameTag(t.Context(), 3, "one,two")
	tags := []string{"one,two"}
	want := domain.ValidateDigestDraft(domain.DigestDraft{Tags: &tags})
	for _, err := range []error{createErr, renameErr} {
		if !errors.Is(err, domain.ErrMalformedResponse) || err.Error() != want.Error() {
			t.Fatalf("comma error = %v, want %v", err, want)
		}
	}
	if len(tr.Calls()) != 0 {
		t.Fatal("comma tag reached network")
	}
}
