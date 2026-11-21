// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func responseSnowflake(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(w.Body.String()))
	dec.UseNumber()
	var env struct {
		ID json.Number `json:"id"`
	}
	if err := dec.Decode(&env); err != nil || env.ID == "" {
		t.Fatalf("snowflake id in %s: %v", w.Body.String(), err)
	}
	return env.ID.String()
}

func TestDigestGroupsItemsAndTags(t *testing.T) {
	for _, flavor := range []Flavor{Official, Private} {
		t.Run(string(flavor), func(t *testing.T) {
			_, h := serviceFixture(t, flavor)

			// Group lifecycle: create, duplicate refusal, update with/without description, tombstone delete, UID reuse.
			grpRec, grpOut := exchange(t, h, domain.PathAddDigestGroup, `{"uniqueIdentifier":"grp-1","name":"Reading","description":"Notes","md5Hash":"gh1","lastModifiedTime":100}`)
			requireSuccess(t, grpOut)
			grpID := responseSnowflake(t, grpRec)

			_, dupGrp := exchange(t, h, domain.PathAddDigestGroup, `{"uniqueIdentifier":"grp-1","name":"Duplicate"}`)
			requireCode(t, dupGrp, "E0338")

			_, updGrp := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigestGroup,
				fmt.Sprintf(`{"id":%s,"name":"Deep Reading","description":"Updated","md5Hash":"gh2","lastModifiedTime":200}`, grpID))
			requireSuccess(t, updGrp)
			_, updGrpStrID := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigestGroup,
				fmt.Sprintf(`{"id":%q,"name":"Final Reading","md5Hash":"gh3","lastModifiedTime":300}`, grpID))
			requireSuccess(t, updGrpStrID)

			for _, badBody := range []string{`{"id":-1}`, `{"id":"bad"}`, `{"id":true}`} {
				_, out := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigestGroup, badBody)
				requireCode(t, out, "E0339")
			}

			_, grpPage := exchange(t, h, domain.PathDigestGroups, `{"page":1,"size":100}`)
			requireSuccess(t, grpPage)
			if grpPage[keyTotalPages] != float64(1) {
				t.Fatal(grpPage)
			}
			grpRows := grpPage[keySummaryDOList].([]any)
			if len(grpRows) != 1 || grpRows[0].(map[string]any)["name"] != "Final Reading" || grpRows[0].(map[string]any)["description"] != "Updated" {
				t.Fatalf("unexpected group row: %v", grpRows)
			}

			_, delGrp := exchangeMethod(t, h, http.MethodDelete, domain.PathDeleteDigestGroup, fmt.Sprintf(`{"id":%s}`, grpID))
			requireSuccess(t, delGrp)
			_, delGrpAgain := exchangeMethod(t, h, http.MethodDelete, domain.PathDeleteDigestGroup, fmt.Sprintf(`{"id":%s}`, grpID))
			requireCode(t, delGrpAgain, "E0339")
			_, updDeletedGrp := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigestGroup, fmt.Sprintf(`{"id":%s,"name":"Ghost"}`, grpID))
			requireCode(t, updDeletedGrp, "E0339")

			// A deleted group remains visible as a tombstone in paged queries, while freeing its uniqueIdentifier.
			_, reusedGrp := exchange(t, h, domain.PathAddDigestGroup, `{"uniqueIdentifier":"grp-1","name":"Reborn"}`)
			requireSuccess(t, reusedGrp)
			_, grpPageAfter := exchange(t, h, domain.PathDigestGroups, `{"page":1,"size":100}`)
			requireSuccess(t, grpPageAfter)
			afterRows := grpPageAfter[keySummaryDOList].([]any)
			if len(afterRows) != 2 || afterRows[0].(map[string]any)[keyIsDeleted] != flagDeleted || afterRows[1].(map[string]any)[keyIsDeleted] != flagUndeleted {
				t.Fatalf("expected tombstone + active group, got %v", afterRows)
			}

			// Digest items, by-ID lookup, fingerprint hashes, and soft-delete filtering.
			d1Rec, d1Out := exchange(t, h, domain.PathAddDigest,
				`{"parentUniqueIdentifier":"grp-1","content":"First","md5Hash":"dh1","handwriteMD5":"hm1","commentHandwriteName":"c1.mark","metadata":"{\"author\":\"Synthetic\"}","lastModifiedTime":500}`)
			requireSuccess(t, d1Out)
			d1ID := responseSnowflake(t, d1Rec)

			d2Rec, d2Out := exchange(t, h, domain.PathAddDigest,
				`{"parentUniqueIdentifier":"grp-2","content":"Second","md5Hash":"dh2","lastModifiedTime":600}`)
			requireSuccess(t, d2Out)
			d2ID := responseSnowflake(t, d2Rec)

			_, allDigests := exchange(t, h, domain.PathDigests, `{"page":1,"size":100}`)
			requireSuccess(t, allDigests)
			if len(allDigests[keySummaryDOList].([]any)) != 2 {
				t.Fatal(allDigests)
			}
			_, filteredDigests := exchange(t, h, domain.PathDigests, `{"page":1,"size":100,"parentUniqueIdentifier":"grp-1"}`)
			requireSuccess(t, filteredDigests)
			if rows := filteredDigests[keySummaryDOList].([]any); len(rows) != 1 || rows[0].(map[string]any)["content"] != "First" {
				t.Fatal(filteredDigests)
			}

			_, byID := exchange(t, h, domain.PathDigestsByID, fmt.Sprintf(`{"ids":[%s,%q,999,"bad",false]}`, d1ID, d2ID))
			requireSuccess(t, byID)
			if len(byID[keySummaryDOList].([]any)) != 2 {
				t.Fatal(byID)
			}

			_, hashes := exchange(t, h, domain.PathDigestHashes, `{"page":1,"size":100,"parentUniqueIdentifier":"grp-1"}`)
			requireSuccess(t, hashes)
			hashRows := hashes["summaryInfoVOList"].([]any)
			if len(hashRows) != 1 {
				t.Fatal(hashes)
			}
			firstHash := hashRows[0].(map[string]any)
			if firstHash["md5Hash"] != "dh1" || firstHash["handwriteMd5"] != "hm1" || firstHash["metadataMap"].(map[string]any)["author"] != "Synthetic" {
				t.Fatalf("unexpected fingerprint row: %v", firstHash)
			}

			_, updMissingDigest := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigest, `{"id":-1,"content":"Nope"}`)
			requireCode(t, updMissingDigest, "E0340")
			_, updDigest := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigest, fmt.Sprintf(`{"id":%s,"content":"First Edited","md5Hash":"dh1b"}`, d1ID))
			requireSuccess(t, updDigest)

			_, delDigest := exchangeMethod(t, h, http.MethodDelete, domain.PathDeleteDigest, fmt.Sprintf(`{"id":%s}`, d1ID))
			requireSuccess(t, delDigest)
			_, delDigestAgain := exchangeMethod(t, h, http.MethodDelete, domain.PathDeleteDigest, fmt.Sprintf(`{"id":%s}`, d1ID))
			requireCode(t, delDigestAgain, "E0340")
			_, updDeletedDigest := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigest, fmt.Sprintf(`{"id":%s,"content":"Ghost"}`, d1ID))
			requireCode(t, updDeletedDigest, "E0340")

			// Paged digest query retains tombstones, while ByID and Fingerprints omit deleted rows.
			_, afterList := exchange(t, h, domain.PathDigests, `{"page":1,"size":100,"parentUniqueIdentifier":"grp-1"}`)
			if rows := afterList[keySummaryDOList].([]any); len(rows) != 1 || rows[0].(map[string]any)[keyIsDeleted] != flagDeleted {
				t.Fatalf("expected tombstone in paged digests: %v", afterList)
			}
			_, afterByID := exchange(t, h, domain.PathDigestsByID, fmt.Sprintf(`{"ids":[%s,%s]}`, d1ID, d2ID))
			if len(afterByID[keySummaryDOList].([]any)) != 1 {
				t.Fatalf("ByID returned deleted digest: %v", afterByID)
			}
			_, afterHashes := exchange(t, h, domain.PathDigestHashes, `{"page":1,"size":100}`)
			if len(afterHashes["summaryInfoVOList"].([]any)) != 1 {
				t.Fatalf("Fingerprints returned deleted digest: %v", afterHashes)
			}

			// Tags: uniqueness refusal, self-rename, delete, and listing after deletion.
			t1Rec, t1Out := exchange(t, h, domain.PathAddDigestTag, `{"name":"focus"}`)
			requireSuccess(t, t1Out)
			t1ID := responseSnowflake(t, t1Rec)
			t2Rec, t2Out := exchange(t, h, domain.PathAddDigestTag, `{"name":"archive"}`)
			requireSuccess(t, t2Out)
			t2ID := responseSnowflake(t, t2Rec)

			_, dupTag := exchange(t, h, domain.PathAddDigestTag, `{"name":"focus"}`)
			requireCode(t, dupTag, "E0335")
			_, renameConflict := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigestTag, fmt.Sprintf(`{"id":%s,"name":"archive"}`, t1ID))
			requireCode(t, renameConflict, "E0335")
			_, renameSelf := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigestTag, fmt.Sprintf(`{"id":%s,"name":"focus"}`, t1ID))
			requireSuccess(t, renameSelf)
			_, renameOK := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigestTag, fmt.Sprintf(`{"id":%s,"name":"priority"}`, t1ID))
			requireSuccess(t, renameOK)
			_, renameMissing := exchangeMethod(t, h, http.MethodPut, domain.PathUpdateDigestTag, `{"id":-1,"name":"x"}`)
			requireCode(t, renameMissing, "E0336")

			_, delTag := exchangeMethod(t, h, http.MethodDelete, domain.PathDeleteDigestTag, fmt.Sprintf(`{"id":%s}`, t2ID))
			requireSuccess(t, delTag)
			_, delTagAgain := exchangeMethod(t, h, http.MethodDelete, domain.PathDeleteDigestTag, fmt.Sprintf(`{"id":%s}`, t2ID))
			requireCode(t, delTagAgain, "E0336")

			_, tagsOut := exchangeMethod(t, h, http.MethodGet, domain.PathDigestTags, "")
			requireSuccess(t, tagsOut)
			tagRows := tagsOut["summaryTagDOList"].([]any)
			if len(tagRows) != 1 || tagRows[0].(map[string]any)["name"] != "priority" {
				t.Fatalf("unexpected tags after delete: %v", tagRows)
			}
		})
	}
}

func TestDigestRequestValidation(t *testing.T) {
	_, h := serviceFixture(t, Private)

	r := httptest.NewRequest(http.MethodPost, "/api/"+domain.PathDigests, strings.NewReader(`{"page":1,"size":10}`))
	r.Header.Set("version", domain.ProtocolVersion)
	r.Header.Set("equipmentNo", "synthetic-device")
	w := httptest.NewRecorder()
	h(w, r)
	if !strings.Contains(w.Body.String(), "E0712") {
		t.Fatalf("missing token body = %s", w.Body)
	}

	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{http.MethodPost, domain.PathDigestGroups, `{"page":1}`, http.StatusBadRequest},
		{http.MethodPost, domain.PathDigestGroups, "bad", http.StatusBadRequest},
		{http.MethodPost, domain.PathAddDigestGroup, "bad", http.StatusBadRequest},
		{http.MethodPut, domain.PathUpdateDigestGroup, "bad", http.StatusBadRequest},
		{http.MethodDelete, domain.PathDeleteDigestGroup, "bad", http.StatusBadRequest},
		{http.MethodPost, domain.PathDigests, `{"size":10}`, http.StatusBadRequest},
		{http.MethodPost, domain.PathDigestsByID, "bad", http.StatusBadRequest},
		{http.MethodPost, domain.PathDigestHashes, `{}`, http.StatusBadRequest},
		{http.MethodPost, domain.PathAddDigest, "bad", http.StatusBadRequest},
		{http.MethodPut, domain.PathUpdateDigest, "bad", http.StatusBadRequest},
		{http.MethodDelete, domain.PathDeleteDigest, "bad", http.StatusBadRequest},
		{http.MethodPost, domain.PathAddDigestTag, "bad", http.StatusBadRequest},
		{http.MethodPut, domain.PathUpdateDigestTag, "bad", http.StatusBadRequest},
		{http.MethodDelete, domain.PathDeleteDigestTag, "bad", http.StatusBadRequest},
		{http.MethodPost, domain.PathApplyDigestHandwriting, "bad", http.StatusBadRequest},
		{http.MethodPost, domain.PathDownloadDigestHandwriting, "bad", http.StatusBadRequest},
		{http.MethodPost, "file/unknown/summary", `{}`, http.StatusNotFound},
		{http.MethodGet, domain.PathAddDigest, "", http.StatusNotFound},
	} {
		rec, _ := exchangeMethod(t, h, tc.method, tc.path, tc.body)
		if rec.Code != tc.status {
			t.Fatalf("%s %s: status %d, want %d", tc.method, tc.path, rec.Code, tc.status)
		}
	}
}
