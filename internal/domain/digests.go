// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	digestRandomBytes = 16
	metaUniqueIDKey   = "unique_identifier"
	metaAuthorKey     = "author"
)

// Digest endpoint paths under /api.
const (
	// PathDigestGroups queries paged digest groups.
	PathDigestGroups = "file/query/summary/group"
	// PathAddDigestGroup creates a digest group.
	PathAddDigestGroup = "file/add/summary/group"
	// PathUpdateDigestGroup updates a digest group.
	PathUpdateDigestGroup = "file/update/summary/group"
	// PathDeleteDigestGroup deletes a digest group.
	PathDeleteDigestGroup = "file/delete/summary/group"

	// PathDigests queries paged digests.
	PathDigests = "file/query/summary"
	// PathDigestsByID fetches digests by numeric identifier list.
	PathDigestsByID = "file/query/summary/id"
	// PathDigestHashes queries paged digest fingerprints.
	PathDigestHashes = "file/query/summary/hash"
	// PathAddDigest creates a digest.
	PathAddDigest = "file/add/summary"
	// PathUpdateDigest updates a digest.
	PathUpdateDigest = "file/update/summary"
	// PathDeleteDigest deletes a digest.
	PathDeleteDigest = "file/delete/summary"

	// PathDigestTags lists all digest tags.
	PathDigestTags = "file/query/summary/tag"
	// PathAddDigestTag creates a digest tag.
	PathAddDigestTag = "file/add/summary/tag"
	// PathUpdateDigestTag renames a digest tag.
	PathUpdateDigestTag = "file/update/summary/tag"
	// PathDeleteDigestTag deletes a digest tag.
	PathDeleteDigestTag = "file/delete/summary/tag"

	// PathApplyDigestHandwriting requests an upload slot for a handwritten comment.
	PathApplyDigestHandwriting = "file/upload/apply/summary"
	// PathDownloadDigestHandwriting requests a download URL for a handwritten comment.
	PathDownloadDigestHandwriting = "file/download/summary"
	// PathS3Auth signs multipart S3 operations on the app host.
	PathS3Auth = "file/s3/auth"
)

// Protocol constants for digests.
const (
	// TypedDigestSourceType is the sourceType recorded for a typed digest not
	// extracted from a document.
	TypedDigestSourceType = 4

	// DigestBusyCode is reported when two concurrent writes target the same
	// digest on one session.
	DigestBusyCode = "SNCloud.busy"
)

// NewDigestIdentifier generates a 32-character lowercase hexadecimal identifier.
func NewDigestIdentifier() string {
	var b [digestRandomBytes]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read cannot fail.
	return hex.EncodeToString(b[:])
}

// DigestID identifies a digest, digest group or digest tag. Unlike FileID,
// the digest endpoints accept and return snowflakes as JSON numbers.
type DigestID int64

// String renders the exact decimal identifier.
func (v DigestID) String() string { return strconv.FormatInt(int64(v), 10) }

// MarshalJSON encodes the identifier as a JSON integer.
func (v DigestID) MarshalJSON() ([]byte, error) { return json.Marshal(int64(v)) }

// UnmarshalJSON accepts either a JSON integer or string without losing precision.
func (v *DigestID) UnmarshalJSON(b []byte) error {
	var n WireInt64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*v = DigestID(n)
	return nil
}

// DigestGroup is a digest library. Digests reference a group by UniqueIdentifier.
type DigestGroup struct {
	ID               DigestID
	UniqueIdentifier string
	Name             string
	Description      *string
	MD5Hash          *string
	Metadata         *string
	Created          *time.Time
	Modified         *time.Time
}

// UnmarshalJSON decodes a summary group row.
func (g *DigestGroup) UnmarshalJSON(b []byte) error {
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	id, err := requiredDigestID(row)
	if err != nil {
		return err
	}
	var uid, name string
	if s := Text(row["uniqueIdentifier"]); s != nil {
		uid = *s
	}
	if s := Text(row["name"]); s != nil {
		name = *s
	}
	*g = DigestGroup{
		ID:               id,
		UniqueIdentifier: uid,
		Name:             name,
		Description:      Text(row["description"]),
		MD5Hash:          Text(row["md5Hash"]),
		Metadata:         Text(row["metadata"]),
		Created:          WireTime(row["creationTime"]),
		Modified:         WireTime(row["lastModifiedTime"]),
	}
	return nil
}

// Digest is a passage captured from a document or entered directly, along with
// its source metadata, tags, typed comment and handwritten comment reference.
type Digest struct {
	ID                   DigestID
	UniqueIdentifier     *string
	GroupIdentifier      *string
	FileID               *FileID
	Content              *string
	SourcePath           *string
	DataSource           *string
	SourceType           *int
	Tags                 []string
	MD5Hash              *string
	Metadata             *string
	Comment              *string
	HandwritingName      *string
	HandwritingInnerName *string
	HandwritingMD5       *string
	Author               *string
	Created              *time.Time
	Modified             *time.Time
}

// UnmarshalJSON decodes a digest row, resolving unique_identifier and author
// precedence from its JSON metadata string when present.
func (d *Digest) UnmarshalJSON(b []byte) error {
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	id, err := requiredDigestID(row)
	if err != nil {
		return err
	}
	meta := Text(row["metadata"])
	uid := Text(row["uniqueIdentifier"])
	if uid == nil && meta != nil {
		uid = metadataString(*meta, metaUniqueIDKey)
	}
	var fileID *FileID
	if raw, ok := row["fileId"]; ok && len(raw) > 0 {
		var fid FileID
		if json.Unmarshal(raw, &fid) == nil {
			fileID = &fid
		}
	}
	var srcType *int
	if n := Integer(row["sourceType"]); n != nil {
		v := int(*n)
		srcType = &v
	}
	author := Text(row["author"])
	if metaAuthor, present := metadataAuthor(meta); present {
		author = metaAuthor
	}
	*d = Digest{
		ID:                   id,
		UniqueIdentifier:     uid,
		GroupIdentifier:      Text(row["parentUniqueIdentifier"]),
		FileID:               fileID,
		Content:              Text(row["content"]),
		SourcePath:           Text(row["sourcePath"]),
		DataSource:           Text(row["dataSource"]),
		SourceType:           srcType,
		Tags:                 splitDigestTags(Text(row["tags"])),
		MD5Hash:              Text(row["md5Hash"]),
		Metadata:             meta,
		Comment:              Text(row["commentStr"]),
		HandwritingName:      Text(row["commentHandwriteName"]),
		HandwritingInnerName: Text(row["handwriteInnerName"]),
		HandwritingMD5:       Text(row["handwriteMD5"]),
		Author:               author,
		Created:              WireTime(row["creationTime"]),
		Modified:             WireTime(row["lastModifiedTime"]),
	}
	return nil
}

// DigestFingerprint carries the summary fields returned by the hash endpoint
// for lightweight synchronization checks.
type DigestFingerprint struct {
	ID              DigestID
	MD5Hash         *string
	HandwritingMD5  *string
	HandwritingName *string
	Modified        *time.Time
	Author          *string
}

// UnmarshalJSON decodes a summary hash row, accepting both handwriteMd5 and
// handwriteMD5 wire spellings.
func (f *DigestFingerprint) UnmarshalJSON(b []byte) error {
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	id, err := requiredDigestID(row)
	if err != nil {
		return err
	}
	hwMD5 := Text(row["handwriteMd5"])
	if hwMD5 == nil {
		hwMD5 = Text(row["handwriteMD5"])
	}
	var author *string
	if rawMap, ok := row["metadataMap"]; ok && len(rawMap) > 0 {
		var metaMap map[string]jsontext.Value
		if json.Unmarshal(rawMap, &metaMap) == nil {
			if rawAuthor, hasAuthor := metaMap[metaAuthorKey]; hasAuthor && rawAuthor.Kind() != 'n' {
				var s string
				if json.Unmarshal(rawAuthor, &s) == nil {
					author = &s
				}
			}
		}
	}
	*f = DigestFingerprint{
		ID:              id,
		MD5Hash:         Text(row["md5Hash"]),
		HandwritingMD5:  hwMD5,
		HandwritingName: Text(row["commentHandwriteName"]),
		Modified:        WireTime(row["lastModifiedTime"]),
		Author:          author,
	}
	return nil
}

// DigestTag is a named tag that can be attached to digests.
type DigestTag struct {
	ID   DigestID
	Name string
}

// UnmarshalJSON decodes a digest tag row.
func (t *DigestTag) UnmarshalJSON(b []byte) error {
	var row map[string]jsontext.Value
	if err := json.Unmarshal(b, &row); err != nil {
		return err
	}
	id, err := requiredDigestID(row)
	if err != nil {
		return err
	}
	name, err := optionalString(row, "name")
	if err != nil {
		return err
	}
	*t = DigestTag{ID: id, Name: name}
	return nil
}

// DigestDraft describes a new digest or a partial update. Nil fields are left
// unchanged on update; UniqueIdentifier and FileID apply only on creation.
type DigestDraft struct {
	UniqueIdentifier string
	Content          *string
	GroupIdentifier  *string
	FileID           *FileID
	SourcePath       *string
	DataSource       *string
	SourceType       *int
	Tags             *[]string
	Metadata         *string
	Comment          *string
	Author           *string
	Handwriting      *DigestHandwriting
	MD5Hash          *string
}

// DigestHandwriting references an uploaded handwritten comment .mark file.
type DigestHandwriting struct {
	Name      string
	InnerName string
	MD5       string
}

// ValidateDigestDraft rejects tags containing commas before any network call.
// The server stores a digest's tags in a single comma-separated string column,
// so any tag containing a comma would split into two tags when read back.
func ValidateDigestDraft(d DigestDraft) error {
	if d.Tags == nil {
		return nil
	}
	for _, tag := range *d.Tags {
		if err := ValidateDigestTag(tag); err != nil {
			return err
		}
	}
	return nil
}

// ValidateDigestTag rejects names that cannot round-trip through comma-joined tags.
func ValidateDigestTag(name string) error {
	if strings.Contains(name, ",") {
		return &MalformedResponseError{What: fmt.Sprintf("a digest tag may not contain a comma: %q", name)}
	}
	return nil
}

// GroupFingerprint computes the MD5 hash the tablet stores on a digest group:
// md5(uniqueIdentifier + name). The service stores md5Hash blindly without
// computing or checking it, while tablets compare it during sync; tablets give
// groups no description, so description is omitted from the sum.
func GroupFingerprint(uniqueIdentifier, name string) string {
	return MD5Hex([]byte(uniqueIdentifier + name))
}

// DigestFingerprintFields lists the fields summed into a digest's MD5 hash in
// protocol concatenation order.
type DigestFingerprintFields struct {
	Content         *string
	Group           *string
	SourceType      *int
	SourcePath      *string
	Metadata        *string
	Comment         *string
	HandwritingName *string
}

// ComputeDigestFingerprint computes the MD5 hash the tablet stores on a digest:
// md5(content + parentUniqueIdentifier + sourceType + sourcePath + metadata +
// commentStr + commentHandwriteName), treating nil fields as empty and summing
// metadata as the exact JSON text stored on the row. The service never verifies
// md5Hash, so matching the tablet's order keeps sync hashes consistent.
func ComputeDigestFingerprint(f DigestFingerprintFields) string {
	var b strings.Builder
	writeOpt := func(s *string) {
		if s != nil {
			b.WriteString(*s)
		}
	}
	writeOpt(f.Content)
	writeOpt(f.Group)
	if f.SourceType != nil {
		b.WriteString(strconv.Itoa(*f.SourceType))
	}
	writeOpt(f.SourcePath)
	writeOpt(f.Metadata)
	writeOpt(f.Comment)
	writeOpt(f.HandwritingName)
	return MD5Hex([]byte(b.String()))
}

// MergeDigestMetadata parses metadata as a JSON object (defaulting to {}),
// sets unique_identifier if given and absent or null, sets author if given,
// and serializes the resulting object with sorted keys and unescaped slashes.
// Number spellings are retained exactly, including large integers and trailing zeros.
func MergeDigestMetadata(metadata, uniqueIdentifier, author *string) (string, error) {
	obj := make(map[string]jsontext.Value)
	if metadata != nil && *metadata != "" {
		trimmed := strings.TrimSpace(*metadata)
		if !strings.HasPrefix(trimmed, "{") || json.Unmarshal([]byte(trimmed), &obj) != nil {
			return "", &MalformedResponseError{What: "a digest's metadata must be a JSON object"}
		}
	}
	if uniqueIdentifier != nil {
		existing, ok := obj[metaUniqueIDKey]
		if !ok || existing.Kind() == 'n' {
			raw, err := json.Marshal(*uniqueIdentifier)
			if err != nil {
				return "", err
			}
			obj[metaUniqueIDKey] = raw
		}
	}
	if author != nil {
		raw, err := json.Marshal(*author)
		if err != nil {
			return "", err
		}
		obj[metaAuthorKey] = raw
	}
	for k, v := range obj {
		vc := append(jsontext.Value(nil), v...)
		// Sort nested objects without normalizing number spellings or precision.
		if vc.Format(jsontext.ReorderRawObjects(true)) == nil {
			obj[k] = vc
		}
	}
	out, err := json.Marshal(obj, json.Deterministic(true))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func requiredDigestID(row map[string]jsontext.Value) (DigestID, error) {
	raw, ok := row["id"]
	if !ok || len(raw) == 0 || raw.Kind() == 'n' {
		return 0, fmt.Errorf("missing id")
	}
	var id DigestID
	if err := json.Unmarshal(raw, &id); err != nil {
		return 0, err
	}
	return id, nil
}

func metadataString(metadata, key string) *string {
	var obj map[string]jsontext.Value
	if !strings.HasPrefix(strings.TrimSpace(metadata), "{") || json.Unmarshal([]byte(metadata), &obj) != nil {
		return nil
	}
	return Text(obj[key])
}

// metadataAuthor distinguishes an absent author key (present == false, fall
// back to the column) from an explicit empty or null author in metadata
// (present == true, clears any legacy column value).
func metadataAuthor(metadata *string) (author *string, present bool) {
	if metadata == nil {
		return nil, false
	}
	var obj map[string]jsontext.Value
	if !strings.HasPrefix(strings.TrimSpace(*metadata), "{") || json.Unmarshal([]byte(*metadata), &obj) != nil {
		return nil, false
	}
	raw, ok := obj[metaAuthorKey]
	if !ok {
		return nil, false
	}
	return Text(raw), true
}

func splitDigestTags(raw *string) []string {
	if raw == nil || *raw == "" {
		return []string{}
	}
	parts := strings.Split(*raw, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}
