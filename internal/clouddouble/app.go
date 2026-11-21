// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

const (
	defaultSnowflakeBase  int64 = 900000000000000000
	defaultInboxCreatedMS int64 = 1773000000000

	keyTaskListID    = "taskListId"
	keyTaskID        = "taskId"
	keyCreateTime    = "createTime"
	keySummaryDOList = "summaryDOList"
	keyTotalPages    = "totalPages"
	keyDataType      = "dataType"
	keyInnerName     = "innerName"
	keyS3Auth        = "s3Authorization"
	keyXAmzDate      = "xamzDate"
	keyAPIServer     = "apiServer"
	keyMD5           = "md5"
	keyCode          = "code"
	keyMessageType   = "messageType"
	keyIsDeleted     = "isDeleted"

	syntheticSummaryS3Auth  = "AWS4-HMAC-SHA256 synthetic"
	syntheticSummaryAmzDate = "20261004T043924Z"

	flagDeleted   = "Y"
	flagUndeleted = "N"

	eventFiles   = "ServerMessage"
	eventTodo    = "to-do"
	eventDigest  = "digest"
	msgQuery     = "QUERY"
	msgAddDigest = "ADD_DIGEST"
	msgUpdDigest = "UPDATE_DIGEST"
	msgDelDigest = "DELETE_DIGEST"
)

type appState struct {
	nextSnow   int64
	taskLists  map[string]map[string]any
	listOrder  []string
	tasks      map[string]map[string]any
	taskOrder  []string
	groups     map[int64]map[string]any
	groupOrder []int64
	digests    map[int64]map[string]any
	digestOrd  []int64
	tags       map[int64]map[string]any
	tagOrder   []int64
	hwObjects  map[string][]byte
	hwUploads  map[string]map[int][]byte
	subs       map[*feedSub]struct{}
}

func (c *Cloud) initServices() {
	s := &appState{
		nextSnow:  defaultSnowflakeBase,
		taskLists: make(map[string]map[string]any),
		tasks:     make(map[string]map[string]any),
		groups:    make(map[int64]map[string]any),
		digests:   make(map[int64]map[string]any),
		tags:      make(map[int64]map[string]any),
		hwObjects: make(map[string][]byte),
		hwUploads: make(map[string]map[int][]byte),
		subs:      make(map[*feedSub]struct{}),
	}
	s.taskLists["1"] = map[string]any{
		keyTaskListID:  "1",
		"title":        "Inbox",
		"lastModified": int64(0),
		keyCreateTime:  defaultInboxCreatedMS,
		keyIsDeleted:   flagUndeleted,
	}
	s.listOrder = append(s.listOrder, "1")
	c.state = s
}

func (c *Cloud) closeServices() {
	c.mu.Lock()
	subs := make([]*feedSub, 0, len(c.state.subs))
	for sub := range c.state.subs {
		subs = append(subs, sub)
	}
	c.mu.Unlock()
	for _, sub := range subs {
		sub.close()
	}
}

func (c *Cloud) serveServices(w http.ResponseWriter, r *http.Request, path string) bool {
	if r.URL.Path == "/socket.io/" {
		c.serveSocketIO(w, r)
		return true
	}
	if after, ok := strings.CutPrefix(path, "oss/summary/"); ok {
		c.serveSummaryStorage(w, r, after)
		return true
	}
	if path == domain.PathS3Auth && r.Method == http.MethodPost {
		c.mu.Lock()
		_, ok := c.requireAppAuth(w, r)
		c.mu.Unlock()
		if ok {
			Reply(w, map[string]any{keyS3Auth: syntheticSummaryS3Auth, keyXAmzDate: syntheticSummaryAmzDate})
		}
		return true
	}
	if strings.HasPrefix(path, "file/schedule/") {
		c.serveTasks(w, r, path)
		return true
	}
	if strings.HasPrefix(path, "file/") && strings.Contains(path, "summary") {
		c.serveDigests(w, r, path)
		return true
	}
	return false
}

// requireAppAuth validates app-host headers and bearer token under c.mu.
// It returns the caller's socket device type parsed from the channel header.
func (c *Cloud) requireAppAuth(w http.ResponseWriter, r *http.Request) (originType string, ok bool) {
	if r.Header.Get("version") != domain.ProtocolVersion || r.Header.Get("equipmentNo") == "" {
		w.WriteHeader(http.StatusBadRequest)
		return "", false
	}
	token := r.Header.Get(domain.AccessTokenHeader)
	if !c.tokens[token] {
		Refuse(w, "E0712")
		return "", false
	}
	ch := r.Header.Get("channel")
	if rest, hasPrefix := strings.CutPrefix(ch, token+"_"); hasPrefix {
		if idx := strings.LastIndexByte(rest, '_'); idx > 0 {
			originType = rest[:idx]
		}
	}
	return originType, true
}

func decodeJSONObject(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	return decodeObject[map[string]any](w, r)
}

// decodeObject rejects null and multiple documents before any handler mutates state.
func decodeObject[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	var body *T
	if dec.Decode(&body) != nil || body == nil {
		w.WriteHeader(http.StatusBadRequest)
		var zero T
		return zero, false
	}
	if _, err := dec.Token(); err != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		var zero T
		return zero, false
	}
	return *body, true
}

func (c *Cloud) nextIDLocked() int64 {
	c.state.nextSnow++
	return c.state.nextSnow
}
