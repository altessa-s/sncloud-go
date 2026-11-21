// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"encoding/json/v2"
	"testing"
)

func TestEndpointCleartextPolicy(t *testing.T) {
	for _, host := range []string{"http://cloud.example.com", "http://192.168.1.2", "http://localhost", "http://127.0.0.2", "http://[::1]"} {
		for _, private := range []bool{false, true} {
			e := Endpoints{AppHost: host, WebHost: host, SocketHost: host, PrivateCloud: private}
			want := private || host == "http://localhost" || host == "http://127.0.0.2" || host == "http://[::1]"
			if err := e.Validate(); (err == nil) != want {
				t.Errorf("%s private=%v: %v", host, private, err)
			}
		}
	}
}
func TestCapacityRequiresBothCounts(t *testing.T) {
	for _, raw := range []string{`{}`, `{"usedCapacity":1}`, `{"totalCapacity":2}`, `{"usedCapacity":null,"totalCapacity":2}`} {
		var c Capacity
		if err := json.Unmarshal([]byte(raw), &c); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	var c Capacity
	if err := json.Unmarshal([]byte(`{"usedCapacity":"9007199254740993","totalCapacity":9223372036854775807}`), &c); err != nil || c.Used != 9007199254740993 || c.Total != 9223372036854775807 {
		t.Fatal(c, err)
	}
}
func TestEntryNullableDefaults(t *testing.T) {
	var e Entry
	if err := json.Unmarshal([]byte(`{"id":1,"fileName":"x","directoryId":null,"isFolder":null}`), &e); err != nil || e.ParentID != 0 || e.IsFolder {
		t.Fatal(e, err)
	}
	for _, raw := range []string{`{"id":1,"fileName":"x","directoryId":true}`, `{"id":1,"fileName":"x","isFolder":{}}`} {
		if err := json.Unmarshal([]byte(raw), &e); err == nil {
			t.Fatal("invalid optional accepted", raw)
		}
	}
}

func TestCapacityFieldsArePlainInt64(t *testing.T) {
	c := Capacity{}
	if _, ok := any(c.Used).(int64); !ok {
		t.Fatal("Used is not int64")
	}
	if _, ok := any(c.Total).(int64); !ok {
		t.Fatal("Total is not int64")
	}
}
