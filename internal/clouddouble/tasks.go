// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"maps"
	"net/http"
	"strconv"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func (c *Cloud) serveTasks(w http.ResponseWriter, r *http.Request, path string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	origin, ok := c.requireAppAuth(w, r)
	if !ok {
		return
	}

	switch {
	case path == domain.PathTaskLists && r.Method == http.MethodPost:
		c.handleTaskLists(w, r)
	case path == domain.PathTaskList && r.Method == http.MethodPost:
		c.handleCreateTaskList(w, r, origin)
	case path == domain.PathTaskList && r.Method == http.MethodPut:
		c.handleRenameTaskList(w, r, origin)
	case strings.HasPrefix(path, domain.PathTaskList+"/"):
		c.handleTaskListByID(w, r, origin, strings.TrimPrefix(path, domain.PathTaskList+"/"))
	case path == domain.PathTasks && r.Method == http.MethodPost:
		c.handleTasksAll(w, r)
	case path == domain.PathTask && r.Method == http.MethodPost:
		c.handleCreateTask(w, r, origin)
	case path == domain.PathTask && r.Method == http.MethodPut:
		c.handleUpdateTask(w, r, origin)
	case strings.HasPrefix(path, domain.PathTask+"/") && r.Method == http.MethodDelete:
		c.handleDeleteTask(w, origin, strings.TrimPrefix(path, domain.PathTask+"/"))
	default:
		http.NotFound(w, r)
	}
}

func (c *Cloud) handleTaskLists(w http.ResponseWriter, r *http.Request) {
	c.replyTaskCollection(w, r, c.state.listOrder, c.state.taskLists, "pageToken", "scheduleTaskGroup")
}

func (c *Cloud) handleCreateTaskList(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	title, _ := body["title"].(string)
	id, _ := body[keyTaskListID].(string)
	if id == "" {
		id = strconv.FormatInt(c.nextIDLocked(), 16)
	}
	for c.state.taskLists[id] != nil {
		id += "0"
	}
	c.state.taskLists[id] = map[string]any{
		keyTaskListID:  id,
		"title":        title,
		"lastModified": body["lastModified"],
		keyCreateTime:  body[keyCreateTime],
		keyIsDeleted:   flagUndeleted,
	}
	c.state.listOrder = append(c.state.listOrder, id)
	c.enqueueChangeLocked(origin, eventTodo, map[string]any{keyMessageType: msgQuery, keyTaskListID: id})
	Reply(w, map[string]any{keyTaskListID: id})
}

func (c *Cloud) handleRenameTaskList(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	id, _ := body[keyTaskListID].(string)
	if existing := c.state.taskLists[id]; existing != nil {
		existing["title"] = body["title"]
		existing["lastModified"] = body["lastModified"]
		c.enqueueChangeLocked(origin, eventTodo, map[string]any{keyMessageType: msgQuery, keyTaskListID: id})
	}
	Reply(w, nil)
}

func (c *Cloud) handleTaskListByID(w http.ResponseWriter, r *http.Request, origin, id string) {
	existing := c.state.taskLists[id]
	switch r.Method {
	case http.MethodGet:
		if existing == nil || existing[keyIsDeleted] == flagDeleted {
			Refuse(w, "E0328")
			return
		}
		Reply(w, maps.Clone(existing))
	case http.MethodDelete:
		if existing == nil || existing[keyIsDeleted] == flagDeleted {
			Refuse(w, "E0328")
			return
		}
		existing[keyIsDeleted] = flagDeleted
		for _, t := range c.state.tasks {
			if t[keyTaskListID] == id {
				t[keyIsDeleted] = flagDeleted
			}
		}
		c.enqueueChangeLocked(origin, eventTodo, map[string]any{keyMessageType: msgQuery, keyTaskListID: id})
		Reply(w, nil)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (c *Cloud) handleTasksAll(w http.ResponseWriter, r *http.Request) {
	c.replyTaskCollection(w, r, c.state.taskOrder, c.state.tasks, "nextPageToken", "scheduleTask")
}

func (c *Cloud) replyTaskCollection(
	w http.ResponseWriter,
	r *http.Request,
	order []string,
	table map[string]map[string]any,
	tokenKey, listKey string,
) {
	if _, ok := decodeJSONObject(w, r); !ok {
		return
	}
	rows := make([]map[string]any, 0, len(order))
	for _, id := range order {
		rows = append(rows, maps.Clone(table[id]))
	}
	Reply(w, map[string]any{tokenKey: nil, listKey: rows})
}

func (c *Cloud) handleCreateTask(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	if listID, hasList := body[keyTaskListID].(string); hasList && listID != "" {
		grp := c.state.taskLists[listID]
		if grp == nil || grp[keyIsDeleted] == flagDeleted {
			Refuse(w, "E0328")
			return
		}
	}
	id, _ := body[keyTaskID].(string)
	if id == "" {
		id = strconv.FormatInt(c.nextIDLocked(), 16)
	}
	for c.state.tasks[id] != nil {
		id += "0"
	}
	row := maps.Clone(body)
	row[keyTaskID] = id
	row[keyIsDeleted] = flagUndeleted
	c.state.tasks[id] = row
	c.state.taskOrder = append(c.state.taskOrder, id)
	c.enqueueChangeLocked(origin, eventTodo, map[string]any{
		keyMessageType: msgQuery,
		keyTaskListID:  body[keyTaskListID],
		keyTaskID:      id,
	})
	Reply(w, map[string]any{keyTaskID: id})
}

func (c *Cloud) handleUpdateTask(w http.ResponseWriter, r *http.Request, origin string) {
	body, ok := decodeJSONObject(w, r)
	if !ok {
		return
	}
	id, _ := body[keyTaskID].(string)
	existing := c.state.tasks[id]
	if existing == nil || existing[keyIsDeleted] == flagDeleted {
		Refuse(w, "E0329")
		return
	}
	row := maps.Clone(body)
	row[keyIsDeleted] = flagUndeleted
	c.state.tasks[id] = row
	c.enqueueChangeLocked(origin, eventTodo, map[string]any{
		keyMessageType: msgQuery,
		keyTaskListID:  body[keyTaskListID],
		keyTaskID:      id,
	})
	Reply(w, map[string]any{keyTaskID: nil})
}

func (c *Cloud) handleDeleteTask(w http.ResponseWriter, origin, id string) {
	existing := c.state.tasks[id]
	if existing == nil || existing[keyIsDeleted] == flagDeleted {
		Refuse(w, "E0329")
		return
	}
	existing[keyIsDeleted] = flagDeleted
	c.enqueueChangeLocked(origin, eventTodo, map[string]any{
		keyMessageType: msgQuery,
		keyTaskListID:  existing[keyTaskListID],
		keyTaskID:      id,
	})
	Reply(w, nil)
}
