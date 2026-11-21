// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clouddouble

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func TestJSONObjectRejectsNullAndTrailingData(t *testing.T) {
	for _, body := range []string{"null", "[]", "{}{}", "{} trailing"} {
		w := httptest.NewRecorder()
		if _, ok := decodeJSONObject(w, httptest.NewRequest("POST", "/", strings.NewReader(body))); ok || w.Code != 400 {
			t.Errorf("accepted %q: %d", body, w.Code)
		}
	}
}

func TestMalformedObjectsCannotMutateServices(t *testing.T) {
	f := fileFixture(t, Private)
	for _, body := range []string{"null", "[]", "{}{}", "{} trailing"} {
		for _, path := range []string{"file/schedule/task", "file/2/files/create_folder_v2", domain.PathRandomCode} {
			r := httptest.NewRequest("POST", "/api/"+path, strings.NewReader(body))
			r.Header.Set(domain.AccessTokenHeader, "synthetic-token")
			r.Header.Set("version", domain.ProtocolVersion)
			r.Header.Set("equipmentNo", "synthetic")
			f.cloud.tokens["synthetic-token"] = true
			w := httptest.NewRecorder()
			switch path {
			case "file/schedule/task":
				f.cloud.serveTasks(w, r, path)
			case domain.PathRandomCode:
				f.cloud.serveApp(w, r, false, path)
			default:
				f.device(w, r)
			}
			if w.Code != http.StatusBadRequest {
				t.Errorf("%s %q: %d", path, body, w.Code)
			}
		}
	}
	if len(f.entries) != 0 || len(f.cloud.state.tasks) != 0 || len(f.cloud.challenge) != 0 {
		t.Fatal("malformed request changed state")
	}
}
