//go:build integration

// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package sncloud_test

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/clouddouble"

	sncloud "github.com/altessa-s/sncloud-go"
)

func TestDigestsBothCloudFlavors(t *testing.T) {
	for _, flavor := range []clouddouble.Flavor{clouddouble.Official, clouddouble.Private} {
		t.Run(string(flavor), func(t *testing.T) {
			cloud := clouddouble.New(flavor)
			defer cloud.Close()

			c, _ := loginTestClient(t, cloud)
			ctx := t.Context()
			view := c.Digests()

			groupUID := sncloud.NewDigestIdentifier()
			desc := "Collected notes"
			groupID, err := view.CreateGroup(ctx, "Reading", &desc, groupUID)
			if err != nil || groupID == 0 {
				t.Fatalf("CreateGroup = (%v, %v)", groupID, err)
			}
			if _, err := view.CreateGroup(ctx, "Duplicate", nil, groupUID); !errors.Is(err, sncloud.ErrAlreadyExists) {
				t.Fatalf("duplicate CreateGroup = %v, want ErrAlreadyExists", err)
			}
			if err := view.UpdateGroup(ctx, groupID, "Deep Reading", &desc); err != nil {
				t.Fatalf("UpdateGroup: %v", err)
			}
			groups, err := view.Groups(ctx)
			if err != nil || len(groups) != 1 || groups[0].Name != "Deep Reading" {
				t.Fatalf("Groups = (%+v, %v)", groups, err)
			}

			tagID, err := view.CreateTag(ctx, "key-idea")
			if err != nil || tagID == 0 {
				t.Fatalf("CreateTag = (%v, %v)", tagID, err)
			}
			if err := view.RenameTag(ctx, tagID, "core-idea"); err != nil {
				t.Fatalf("RenameTag: %v", err)
			}
			tags, err := view.Tags(ctx)
			if err != nil || len(tags) != 1 || tags[0].Name != "core-idea" {
				t.Fatalf("Tags = (%+v, %v)", tags, err)
			}

			passage := "Simplicity is prerequisite for reliability."
			author := "Synthetic Author"
			srcType := sncloud.TypedDigestSourceType
			tagSlice := []string{"core-idea"}
			digestID, err := view.Create(ctx, sncloud.DigestDraft{
				Content:         &passage,
				GroupIdentifier: &groupUID,
				SourceType:      &srcType,
				Tags:            &tagSlice,
				Author:          &author,
			})
			if err != nil || digestID == 0 {
				t.Fatalf("Create = (%v, %v)", digestID, err)
			}

			// Without handwriting attached, DownloadHandwriting returns ("", nil).
			initialList, err := view.ByID(ctx, []sncloud.DigestID{digestID})
			if err != nil || len(initialList) != 1 || initialList[0].ID != digestID {
				t.Fatalf("ByID = (%+v, %v)", initialList, err)
			}
			if emptyPath, derr := view.DownloadHandwriting(ctx, initialList[0], nil); derr != nil || emptyPath != "" {
				t.Fatalf("DownloadHandwriting without mark = (%q, %v)", emptyPath, derr)
			}

			// End-to-end handwriting upload and download across both single-part
			// and >8 MiB multipart/chunked transfers in both cloud flavours.
			for _, size := range []int{64, 8*1024*1024 + 19} {
				payload := bytes.Repeat([]byte("m"), size)
				srcFile := filepath.Join(t.TempDir(), fmt.Sprintf("comment-%d.mark", size))
				if err := os.WriteFile(srcFile, payload, 0o600); err != nil {
					t.Fatal(err)
				}
				var upDone, upTotal int64
				hw, err := view.UploadHandwriting(ctx, srcFile, "", func(done, total int64) {
					upDone, upTotal = done, total
				})
				if err != nil {
					t.Fatalf("UploadHandwriting(%d): %v", size, err)
				}
				wantMD5 := md5.Sum(payload)
				if upDone != int64(size) || upTotal != int64(size) || hw.InnerName == "" || hw.MD5 != hex.EncodeToString(wantMD5[:]) {
					t.Fatalf("UploadHandwriting(%d) = %+v (progress %d/%d)", size, hw, upDone, upTotal)
				}

				comment := fmt.Sprintf("Annotated (%d bytes)", size)
				if err := view.Update(ctx, digestID, sncloud.DigestDraft{
					Comment:     &comment,
					Handwriting: &hw,
				}); err != nil {
					t.Fatalf("Update with handwriting(%d): %v", size, err)
				}

				inGroup, err := view.List(ctx, groupUID)
				if err != nil || len(inGroup) != 1 || inGroup[0].ID != digestID {
					t.Fatalf("List(%d) = (%+v, %v)", size, inGroup, err)
				}
				fps, err := view.Fingerprints(ctx, groupUID)
				if err != nil || len(fps) != 1 || fps[0].HandwritingMD5 == nil || *fps[0].HandwritingMD5 != hw.MD5 {
					t.Fatalf("Fingerprints(%d) = (%+v, %v)", size, fps, err)
				}

				var downDone int64
				downloaded, err := view.DownloadHandwriting(ctx, inGroup[0], func(done, _ int64) {
					downDone = done
				})
				if err != nil || downloaded == "" {
					t.Fatalf("DownloadHandwriting(%d) = (%q, %v)", size, downloaded, err)
				}
				gotBytes, err := os.ReadFile(downloaded)
				_ = os.Remove(downloaded)
				if err != nil || !bytes.Equal(gotBytes, payload) || downDone != int64(size) {
					t.Fatalf("DownloadHandwriting(%d) round-trip mismatch: len=%d downDone=%d err=%v", size, len(gotBytes), downDone, err)
				}
			}

			if err := view.Delete(ctx, digestID); err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if err := view.DeleteTag(ctx, tagID); err != nil {
				t.Fatalf("DeleteTag: %v", err)
			}
			if err := view.DeleteGroup(ctx, groupID); err != nil {
				t.Fatalf("DeleteGroup: %v", err)
			}
		})
	}
}
