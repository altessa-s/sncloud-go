// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package digests

import (
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// pageQuery always emits page and size because the server unboxes both and
// fails with HTTP 500 if either is omitted.
type pageQuery struct {
	Page                   int    `json:"page"`
	Size                   int    `json:"size"`
	ParentUniqueIdentifier string `json:"parentUniqueIdentifier,omitzero"`
}

type pageAnswer[T any] struct {
	TotalPages        *domain.WireInt64 `json:"totalPages"`
	SummaryDOList     []T               `json:"summaryDOList"`
	SummaryInfoVOList []T               `json:"summaryInfoVOList"`
}

type digestListAnswer struct {
	SummaryDOList []domain.Digest `json:"summaryDOList"`
}

type tagListAnswer struct {
	SummaryTagDOList []domain.DigestTag `json:"summaryTagDOList"`
}

type madeAnswer struct {
	ID *domain.DigestID `json:"id"`
}

func (m madeAnswer) identifier() (domain.DigestID, error) {
	if m.ID == nil {
		return 0, &domain.MalformedResponseError{What: "the digest API named no new identifier"}
	}
	return *m.ID, nil
}

type byIDRequest struct {
	ID domain.DigestID `json:"id"`
}

type byIDsRequest struct {
	IDs []domain.DigestID `json:"ids"`
}

type tagNameRequest struct {
	Name string `json:"name"`
}

type tagChangeRequest struct {
	ID   domain.DigestID `json:"id"`
	Name string          `json:"name"`
}

type newGroupRequest struct {
	UniqueIdentifier string  `json:"uniqueIdentifier"`
	Name             string  `json:"name"`
	Description      *string `json:"description,omitzero"`
	MD5Hash          string  `json:"md5Hash"`
	CreationTime     int64   `json:"creationTime"`
	LastModifiedTime int64   `json:"lastModifiedTime"`
}

type groupChangeRequest struct {
	ID               domain.DigestID `json:"id"`
	Name             string          `json:"name"`
	Description      *string         `json:"description,omitzero"`
	MD5Hash          string          `json:"md5Hash"`
	LastModifiedTime int64           `json:"lastModifiedTime"`
}

type handwritingApplyRequest struct {
	FileName    string `json:"fileName"`
	EquipmentNo string `json:"equipmentNo"`
}

type handwritingLinkAnswer struct {
	URL *string `json:"url"`
}

// digestBody omits nil draft fields so partial updates touch only the specified
// columns. Author is stored inside Metadata rather than as a top-level field.
type digestBody struct {
	ID                     *domain.DigestID `json:"id,omitzero"`
	FileID                 *int64           `json:"fileId,omitzero"`
	ParentUniqueIdentifier *string          `json:"parentUniqueIdentifier,omitzero"`
	Content                *string          `json:"content,omitzero"`
	SourcePath             *string          `json:"sourcePath,omitzero"`
	DataSource             *string          `json:"dataSource,omitzero"`
	SourceType             *int             `json:"sourceType,omitzero"`
	Tags                   *string          `json:"tags,omitzero"`
	Metadata               *string          `json:"metadata,omitzero"`
	CommentStr             *string          `json:"commentStr,omitzero"`
	CommentHandwriteName   *string          `json:"commentHandwriteName,omitzero"`
	HandwriteInnerName     *string          `json:"handwriteInnerName,omitzero"`
	HandwriteMD5           *string          `json:"handwriteMD5,omitzero"`
	MD5Hash                *string          `json:"md5Hash,omitzero"`
	CreationTime           *int64           `json:"creationTime,omitzero"`
	LastModifiedTime       int64            `json:"lastModifiedTime"`
}

func newDigestBody(
	draft domain.DigestDraft,
	id *domain.DigestID,
	md5Hash *string,
	creationTime *int64,
	lastModifiedTime int64,
) digestBody {
	var fileID *int64
	if id == nil && draft.FileID != nil {
		raw := int64(*draft.FileID)
		fileID = &raw
	}
	var tags *string
	if draft.Tags != nil {
		joined := strings.Join(*draft.Tags, ",")
		tags = &joined
	}
	var hwName, hwInner, hwMD5 *string
	if draft.Handwriting != nil {
		n, in, m := draft.Handwriting.Name, draft.Handwriting.InnerName, draft.Handwriting.MD5
		hwName, hwInner, hwMD5 = &n, &in, &m
	} else if id == nil {
		// New digests without handwriting explicitly send commentHandwriteName: "".
		empty := ""
		hwName = &empty
	}
	return digestBody{
		ID:                     id,
		FileID:                 fileID,
		ParentUniqueIdentifier: draft.GroupIdentifier,
		Content:                draft.Content,
		SourcePath:             draft.SourcePath,
		DataSource:             draft.DataSource,
		SourceType:             draft.SourceType,
		Tags:                   tags,
		Metadata:               draft.Metadata,
		CommentStr:             draft.Comment,
		CommentHandwriteName:   hwName,
		HandwriteInnerName:     hwInner,
		HandwriteMD5:           hwMD5,
		MD5Hash:                md5Hash,
		CreationTime:           creationTime,
		LastModifiedTime:       lastModifiedTime,
	}
}
