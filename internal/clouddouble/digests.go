// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

const privateInternalOrigin = "http://127.0.0.1:1"

func (c *Cloud) serveDigests(w http.ResponseWriter, r *http.Request, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	origin, ok := c.requireAppAuth(w, r)
	if !ok {
		return
	}

	if c.serveDigestGroups(w, r, path, origin) ||
		c.serveDigestItems(w, r, path, origin) ||
		c.serveDigestTags(w, r, path, origin) ||
		c.serveDigestHandwriting(w, r, path) {
		return
	}
	http.NotFound(w, r)
}

func (c *Cloud) serveDigestGroups(w http.ResponseWriter, r *http.Request, path, origin string) bool {
	switch {
	case path == domain.PathDigestGroups && r.Method == http.MethodPost:
		if _, ok := decodePageQuery(w, r); !ok {
			return true
		}
		rows := make([]map[string]any, 0, len(c.state.groupOrder))
		for _, id := range c.state.groupOrder {
			rows = append(rows, maps.Clone(c.state.groups[id]))
		}
		Reply(w, map[string]any{keyTotalPages: 1, keySummaryDOList: rows})
		return true
	case path == domain.PathAddDigestGroup && r.Method == http.MethodPost:
		c.handleAddDigestGroup(w, r, origin)
		return true
	case path == domain.PathUpdateDigestGroup && r.Method == http.MethodPut:
		c.handleUpdateDigestGroup(w, r, origin)
		return true
	case path == domain.PathDeleteDigestGroup && r.Method == http.MethodDelete:
		c.handleDeleteDigestGroup(w, r, origin)
		return true
	default:
		return false
	}
}

func (c *Cloud) handleAddDigestGroup(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	uid, _ := body["uniqueIdentifier"].(string)
	for _, g := range c.state.groups {
		if g[keyIsDeleted] != flagDeleted && g["uniqueIdentifier"] == uid {
			Refuse(w, "E0338")
			return
		}
	}
	id := c.nextIDLocked()
	row := maps.Clone(body)
	row["id"] = id
	row["isSummaryGroup"] = flagDeleted
	row[keyIsDeleted] = flagUndeleted
	c.state.groups[id] = row
	c.state.groupOrder = append(c.state.groupOrder, id)
	c.enqueueChangeLocked(origin, eventDigest, map[string]any{
		keyMessageType:     msgAddDigest,
		keyDataType:        "1",
		"id":               id,
		"uniqueIdentifier": uid,
	})
	Reply(w, map[string]any{"id": id})
}

func (c *Cloud) handleUpdateDigestGroup(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	id, ok := parseJSONInt64(body["id"])
	if !ok || c.state.groups[id] == nil || c.state.groups[id][keyIsDeleted] == flagDeleted {
		Refuse(w, "E0339")
		return
	}
	grp := c.state.groups[id]
	grp["name"] = body["name"]
	if desc, hasDesc := body["description"]; hasDesc {
		grp["description"] = desc
	}
	grp["md5Hash"] = body["md5Hash"]
	grp["lastModifiedTime"] = body["lastModifiedTime"]
	c.enqueueChangeLocked(origin, eventDigest, map[string]any{keyMessageType: msgUpdDigest, keyDataType: "1", "id": id})
	Reply(w, nil)
}

func (c *Cloud) handleDeleteDigestGroup(w http.ResponseWriter, r *http.Request, origin string) {
	c.deleteSummaryRow(w, r, origin, c.state.groups, "1", "E0339")
}

func (c *Cloud) serveDigestItems(w http.ResponseWriter, r *http.Request, path, origin string) bool {
	switch {
	case path == domain.PathDigests && r.Method == http.MethodPost:
		c.handleQueryDigests(w, r)
		return true
	case path == domain.PathDigestsByID && r.Method == http.MethodPost:
		c.handleQueryDigestsByID(w, r)
		return true
	case path == domain.PathDigestHashes && r.Method == http.MethodPost:
		c.handleQueryDigestHashes(w, r)
		return true
	case path == domain.PathAddDigest && r.Method == http.MethodPost:
		c.handleAddDigest(w, r, origin)
		return true
	case path == domain.PathUpdateDigest && r.Method == http.MethodPut:
		c.handleUpdateDigest(w, r, origin)
		return true
	case path == domain.PathDeleteDigest && r.Method == http.MethodDelete:
		c.handleDeleteDigest(w, r, origin)
		return true
	default:
		return false
	}
}

func (c *Cloud) handleQueryDigests(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePageQuery(w, r)
	if !ok {
		return
	}
	parent, _ := body["parentUniqueIdentifier"].(string)
	rows := make([]map[string]any, 0, len(c.state.digestOrd))
	for _, id := range c.state.digestOrd {
		d := c.state.digests[id]
		if parent != "" && d["parentUniqueIdentifier"] != parent {
			continue
		}
		rows = append(rows, maps.Clone(d))
	}
	Reply(w, map[string]any{keyTotalPages: 1, keySummaryDOList: rows})
}

func (c *Cloud) handleQueryDigestsByID(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	rawIDs, _ := body["ids"].([]any)
	rows := make([]map[string]any, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, valid := parseJSONInt64(raw)
		if !valid {
			continue
		}
		if d := c.state.digests[id]; d != nil && d[keyIsDeleted] != flagDeleted {
			rows = append(rows, maps.Clone(d))
		}
	}
	Reply(w, map[string]any{keySummaryDOList: rows})
}

func (c *Cloud) handleQueryDigestHashes(w http.ResponseWriter, r *http.Request) {
	body, ok := decodePageQuery(w, r)
	if !ok {
		return
	}
	parent, _ := body["parentUniqueIdentifier"].(string)
	rows := make([]map[string]any, 0, len(c.state.digestOrd))
	for _, id := range c.state.digestOrd {
		d := c.state.digests[id]
		if d[keyIsDeleted] == flagDeleted || (parent != "" && d["parentUniqueIdentifier"] != parent) {
			continue
		}
		var metaMap map[string]any
		if metaStr, hasMeta := d["metadata"].(string); hasMeta && metaStr != "" {
			_ = json.Unmarshal([]byte(metaStr), &metaMap)
		}
		rows = append(rows, map[string]any{
			"id":                   id,
			"md5Hash":              d["md5Hash"],
			"handwriteMd5":         d["handwriteMD5"],
			"commentHandwriteName": d["commentHandwriteName"],
			"lastModifiedTime":     d["lastModifiedTime"],
			"metadataMap":          metaMap,
		})
	}
	Reply(w, map[string]any{keyTotalPages: 1, "summaryInfoVOList": rows})
}

func (c *Cloud) handleAddDigest(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	id := c.nextIDLocked()
	row := maps.Clone(body)
	row["id"] = id
	row["isSummaryGroup"] = flagUndeleted
	row[keyIsDeleted] = flagUndeleted
	c.state.digests[id] = row
	c.state.digestOrd = append(c.state.digestOrd, id)
	c.enqueueChangeLocked(origin, eventDigest, map[string]any{
		keyMessageType:           msgAddDigest,
		keyDataType:              "2",
		"id":                     id,
		"parentUniqueIdentifier": body["parentUniqueIdentifier"],
	})
	Reply(w, map[string]any{"id": id})
}

func (c *Cloud) handleUpdateDigest(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	id, ok := parseJSONInt64(body["id"])
	if !ok || c.state.digests[id] == nil || c.state.digests[id][keyIsDeleted] == flagDeleted {
		Refuse(w, "E0340")
		return
	}
	maps.Copy(c.state.digests[id], body)
	c.enqueueChangeLocked(origin, eventDigest, map[string]any{keyMessageType: msgUpdDigest, keyDataType: "2", "id": id})
	Reply(w, nil)
}

func (c *Cloud) handleDeleteDigest(w http.ResponseWriter, r *http.Request, origin string) {
	c.deleteSummaryRow(w, r, origin, c.state.digests, "2", "E0340")
}

func (c *Cloud) deleteSummaryRow(
	w http.ResponseWriter,
	r *http.Request,
	origin string,
	table map[int64]map[string]any,
	dataType, notFoundCode string,
) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	id, ok := parseJSONInt64(body["id"])
	if !ok || table[id] == nil || table[id][keyIsDeleted] == flagDeleted {
		Refuse(w, notFoundCode)
		return
	}
	table[id][keyIsDeleted] = flagDeleted
	c.enqueueChangeLocked(origin, eventDigest, map[string]any{keyMessageType: msgDelDigest, keyDataType: dataType, "id": id})
	Reply(w, nil)
}

func (c *Cloud) serveDigestTags(w http.ResponseWriter, r *http.Request, path, origin string) bool {
	switch {
	case path == domain.PathDigestTags && r.Method == http.MethodGet:
		rows := make([]map[string]any, 0, len(c.state.tagOrder))
		for _, id := range c.state.tagOrder {
			if t := c.state.tags[id]; t != nil {
				rows = append(rows, maps.Clone(t))
			}
		}
		Reply(w, map[string]any{"summaryTagDOList": rows})
		return true
	case path == domain.PathAddDigestTag && r.Method == http.MethodPost:
		c.handleAddDigestTag(w, r, origin)
		return true
	case path == domain.PathUpdateDigestTag && r.Method == http.MethodPut:
		c.handleUpdateDigestTag(w, r, origin)
		return true
	case path == domain.PathDeleteDigestTag && r.Method == http.MethodDelete:
		c.handleDeleteDigestTag(w, r, origin)
		return true
	default:
		return false
	}
}

func (c *Cloud) handleAddDigestTag(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	name, _ := body["name"].(string)
	for _, t := range c.state.tags {
		if t["name"] == name {
			Refuse(w, "E0335")
			return
		}
	}
	id := c.nextIDLocked()
	c.state.tags[id] = map[string]any{"id": id, "name": name}
	c.state.tagOrder = append(c.state.tagOrder, id)
	c.enqueueChangeLocked(origin, eventDigest, map[string]any{keyMessageType: msgAddDigest, keyDataType: "3", "id": id})
	Reply(w, map[string]any{"id": id})
}

func (c *Cloud) handleUpdateDigestTag(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	id, ok := parseJSONInt64(body["id"])
	if !ok || c.state.tags[id] == nil {
		Refuse(w, "E0336")
		return
	}
	name, _ := body["name"].(string)
	for otherID, t := range c.state.tags {
		if otherID != id && t["name"] == name {
			Refuse(w, "E0335")
			return
		}
	}
	c.state.tags[id]["name"] = name
	c.enqueueChangeLocked(origin, eventDigest, map[string]any{keyMessageType: msgUpdDigest, keyDataType: "3", "id": id})
	Reply(w, nil)
}

func (c *Cloud) handleDeleteDigestTag(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	id, ok := parseJSONInt64(body["id"])
	if !ok || c.state.tags[id] == nil {
		Refuse(w, "E0336")
		return
	}
	delete(c.state.tags, id)
	c.enqueueChangeLocked(origin, eventDigest, map[string]any{keyMessageType: msgDelDigest, keyDataType: "3", "id": id})
	Reply(w, nil)
}

func (c *Cloud) serveDigestHandwriting(w http.ResponseWriter, r *http.Request, path string) bool {
	switch {
	case path == domain.PathApplyDigestHandwriting && r.Method == http.MethodPost:
		body, ok := decodeJSONObject(w, r)
		if !ok {
			return true
		}
		fileName, _ := body["fileName"].(string)
		inner := fmt.Sprintf("stored-%d-%s", c.nextIDLocked(), fileName)
		if c.flavor == Official {
			Reply(w, map[string]any{
				"s3Url":      c.app.URL + "/api/oss/summary/" + inner,
				keyS3Auth:    syntheticSummaryS3Auth,
				keyXAmzDate:  syntheticSummaryAmzDate,
				keyInnerName: inner,
				keyAPIServer: "1",
				"fileServer": "3",
			})
		} else {
			// Emit an unroutable internal origin so private-cloud /api/oss/ URL
			// rewriting to AppHost is exercised end-to-end.
			Reply(w, map[string]any{
				"fullUploadUrl": privateInternalOrigin + "/api/oss/summary/upload?key=" + inner,
				"partUploadUrl": privateInternalOrigin + "/api/oss/summary/part?key=" + inner,
				keyInnerName:    inner,
				keyAPIServer:    "private_cloud",
			})
		}
		return true
	case path == domain.PathDownloadDigestHandwriting && r.Method == http.MethodPost:
		body, ok := decodeJSONObject(w, r)
		if !ok {
			return true
		}
		id, ok := parseJSONInt64(body["id"])
		if !ok || c.state.digests[id] == nil {
			Refuse(w, "E0340")
			return true
		}
		inner, _ := c.state.digests[id]["handwriteInnerName"].(string)
		if inner == "" {
			Refuse(w, "E0340")
			return true
		}
		originURL := c.app.URL
		if c.flavor == Private {
			originURL = privateInternalOrigin
		}
		Reply(w, map[string]any{"url": originURL + "/api/oss/summary/" + inner})
		return true
	default:
		return false
	}
}

func (c *Cloud) serveSummaryStorage(w http.ResponseWriter, r *http.Request, key string) {
	if r.Method == http.MethodGet {
		c.serveSummaryDownload(w, r, key)
		return
	}
	if key == "upload" || key == "part" {
		c.serveSummaryFormUpload(w, r, key == "part")
		return
	}
	c.serveSummaryS3Upload(w, r, key)
}

func (c *Cloud) serveSummaryDownload(w http.ResponseWriter, r *http.Request, key string) {
	if r.Header.Get(domain.AccessTokenHeader) != "" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	c.mu.Lock()
	raw, ok := c.state.hwObjects[key]
	c.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(raw)
}

func (c *Cloud) serveSummaryFormUpload(w http.ResponseWriter, r *http.Request, chunked bool) {
	if r.Method != http.MethodPost || r.Header.Get(domain.AccessTokenHeader) == "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	objKey := r.URL.Query().Get("key")
	reader, err := r.MultipartReader()
	if err != nil || objKey == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	part, err := reader.NextPart()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(io.LimitReader(part, storageLimit))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	c.mu.Lock()
	if chunked {
		if r.URL.Query().Get("partNumber") == "1" {
			c.state.hwObjects[objKey] = nil
		}
		c.state.hwObjects[objKey] = append(c.state.hwObjects[objKey], data...)
		c.mu.Unlock()
		Reply(w, map[string]any{"chunkMd5": domain.MD5Hex(data)})
		return
	}
	c.state.hwObjects[objKey] = data
	c.mu.Unlock()
	Reply(w, map[string]any{keyInnerName: objKey, keyMD5: domain.MD5Hex(data)})
}

func (c *Cloud) serveSummaryS3Upload(w http.ResponseWriter, r *http.Request, key string) {
	if r.Header.Get("Authorization") == "" || r.Header.Get(domain.AccessTokenHeader) != "" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	q := r.URL.Query()
	uploadID := q.Get("uploadId")
	switch r.Method {
	case http.MethodDelete:
		c.mu.Lock()
		delete(c.state.hwUploads, uploadID)
		c.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		c.handleSummaryS3Post(w, q, key, uploadID)
	case http.MethodPut:
		data, err := io.ReadAll(io.LimitReader(r.Body, storageLimit))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		c.mu.Lock()
		if uploadID != "" {
			partNum, _ := strconv.Atoi(q.Get("partNumber"))
			if c.state.hwUploads[uploadID] == nil {
				c.state.hwUploads[uploadID] = make(map[int][]byte)
			}
			c.state.hwUploads[uploadID][partNum] = data
		} else {
			c.state.hwObjects[key] = data
		}
		c.mu.Unlock()
		w.Header().Set("ETag", "synthetic-etag")
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (c *Cloud) handleSummaryS3Post(w http.ResponseWriter, q map[string][]string, key, uploadID string) {
	if _, initiate := q["uploads"]; initiate {
		c.mu.Lock()
		c.state.hwUploads[key] = make(map[int][]byte)
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/xml")
		_ = xml.NewEncoder(w).Encode(struct {
			XMLName xml.Name `xml:"InitiateMultipartUploadResult"`
			ID      string   `xml:"UploadId"`
		}{ID: key})
		return
	}
	c.mu.Lock()
	parts := c.state.hwUploads[uploadID]
	var combined []byte
	for n := 1; n <= len(parts); n++ {
		combined = append(combined, parts[n]...)
	}
	c.state.hwObjects[key] = combined
	delete(c.state.hwUploads, uploadID)
	c.mu.Unlock()
	_, _ = io.WriteString(w, "<CompleteMultipartUploadResult/>")
}

func decodePageQuery(w http.ResponseWriter, r *http.Request) (map[string]any, bool) {
	body, ok := decodeJSONObject(w, r)
	if !ok || body["page"] == nil || body["size"] == nil {
		w.WriteHeader(http.StatusBadRequest)
		return nil, false
	}
	return body, true
}

func parseJSONInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case json.Number:
		i, err := strconv.ParseInt(n.String(), 10, 64)
		return i, err == nil
	case string:
		i, err := strconv.ParseInt(n, 10, 64)
		return i, err == nil
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}
