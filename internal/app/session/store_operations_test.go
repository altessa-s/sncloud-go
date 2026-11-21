// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/devicefiles"
	"github.com/altessa-s/sncloud-go/internal/app/digests"
	"github.com/altessa-s/sncloud-go/internal/app/feed"
	"github.com/altessa-s/sncloud-go/internal/app/transfers"
	"github.com/altessa-s/sncloud-go/internal/app/webfiles"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func TestFailedStoreWritesStopPinnedOperations(t *testing.T) {
	for _, mutation := range []string{"password", "code", "logout", "forget"} {
		for _, mutate := range []bool{false, true} {
			for _, operation := range []string{"upload", "multipart", "handwriting", "feed", "lock", "web", "web-multipart", "form", "handwriting-form"} {
				t.Run(mutation+map[bool]string{false: "/unchanged/", true: "/changed/"}[mutate]+operation, func(t *testing.T) {
					testFailedStoreOperation(t, mutation, mutate, operation, "expired")
				})
			}
		}
	}
}

func TestSharedStoreOperationsNeverFollowReplacement(t *testing.T) {
	for _, mutation := range []string{"shared-failed", "shared-success", "direct"} {
		for _, freshness := range []string{"expired", "refused", "usable"} {
			for _, operation := range []string{"upload", "multipart", "handwriting", "feed", "lock", "web", "web-multipart", "form", "handwriting-form"} {
				t.Run(mutation+"/"+freshness+"/"+operation, func(t *testing.T) {
					testFailedStoreOperation(t, mutation, true, operation, freshness)
				})
			}
		}
	}
}

func testFailedStoreOperation(t *testing.T, mutation string, mutate bool, operation, freshness string) {
	t.Helper()
	s, tr, store := setup()
	if strings.HasPrefix(operation, "web") {
		s.channel = channel.New(domain.PrivateCloud("https://synthetic.invalid", false), tr, s.clock, domain.DefaultRetryPolicy())
	}
	old := "x." + base64.RawURLEncoding.EncodeToString([]byte(`{"exp":2400}`)) + ".x"
	_ = store.Keep(old, domain.AccessToken)
	if _, err := s.AccessToken(nil); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("persistence failed")
	s.secrets = &uncertainStore{Secrets: store, key: domain.AccessToken, mutate: mutate, remove: mutation == "logout" || mutation == "forget", failure: failure}
	other, _, _ := setup(challenge(), porttest.OK(`"token":"account-b"`))
	other.secrets = store
	if mutation == "shared-failed" {
		other.secrets = &cleanupFailureStore{uncertainStore: &uncertainStore{Secrets: store, key: domain.AccessToken, mutate: true, failure: failure}, cleanup: failure}
	}
	shared := strings.HasPrefix(mutation, "shared-") || mutation == "direct"
	if shared {
		s.secrets = store
	}
	fired := false
	install := func() {
		fired = true
		var err error
		switch mutation {
		case "shared-failed", "shared-success":
			_, err = other.LogIn(t.Context(), "b@example.invalid", "password", nil)
		case "direct":
			err = store.Keep("account-b", domain.AccessToken)
		case "forget":
			err = s.Forget()
		case "logout":
			err = s.LogOut(t.Context())
		default:
			_, err = s.LogIn(t.Context(), "b@example.invalid", "password", nil)
			if mutation == "code" {
				if err != nil {
					t.Fatal(err)
				}
				if err = s.SendVerificationCode(t.Context()); err != nil {
					t.Fatal(err)
				}
				err = s.Verify(t.Context(), "123456")
			}
		}
		if (mutation == "shared-failed" || !shared) && !errors.Is(err, failure) {
			t.Fatalf("mutation: %v", err)
		}
	}
	change := func() {
		if shared && freshness == "refused" {
			// Keep A captured in a request while Y installs B, then return a server
			// refusal for A. Authorized must invalidate A without discarding B.
			refused := porttest.Refused("E0712")
			probe := &porttest.Transport{Steps: []porttest.Step{{Run: func(context.Context, port.Request) (port.Response, error) {
				install()
				return refused.Response, nil
			}}}}
			ch := channel.New(domain.Official(), probe, s.clock, domain.DefaultRetryPolicy())
			e := transfers.Authorized(t.Context(), ch, s, port.Call{Path: "synthetic/refusal"}, nil, nil)
			if !errors.Is(e, domain.ErrSessionExpired) || len(probe.Calls()) != 1 || probe.Calls()[0].Header.Get(domain.AccessTokenHeader) != old {
				t.Fatalf("A refusal not exercised: %v", e)
			}
		} else {
			install()
		}
		if freshness == "expired" {
			s.clock = &porttest.Clock{Time: time.Unix(2101, 0)}
		}
	}

	held := func(reply porttest.Step) porttest.Step {
		return porttest.Step{Run: func(context.Context, port.Request) (port.Response, error) { change(); return reply.Response, reply.Err }}
	}
	storage := porttest.Step{Response: port.Response{Status: 200, Header: port.Header{"ETag": {"tag"}}}}
	slot := porttest.OK(`"url":"https://storage.invalid/object","s3Authorization":"synthetic","xamzDate":"date"`)
	switch operation {
	case "upload", "web":
		tr.Steps = []porttest.Step{slot, held(storage)}
	case "multipart", "web-multipart":
		signature := porttest.OK(`"s3Authorization":"synthetic","xamzDate":"date"`)
		tr.Steps = []porttest.Step{porttest.OK(`"url":"https://storage.invalid/object"`), signature,
			{Response: port.Response{Status: 200, Body: []byte("<InitiateMultipartUploadResult><UploadId>id</UploadId></InitiateMultipartUploadResult>")}}, signature, held(storage)}
	case "form", "handwriting-form":
		tr.Steps = []porttest.Step{porttest.OK(`"fullUploadUrl":"https://storage.invalid/full","partUploadUrl":"https://storage.invalid/part"`), held(porttest.OK(""))}
	case "handwriting":
		tr.Steps = []porttest.Step{held(slot)}
	case "lock":
		tr.Steps = []porttest.Step{porttest.OK(`"synType":false`)}
	}
	switch mutation {
	case "password":
		tr.Steps = append(tr.Steps, challenge(), porttest.OK(`"token":"account-b"`))
	case "code":
		tr.Steps = append(tr.Steps, challenge(), porttest.Step{Err: domain.ErrVerificationRequired}, porttest.OK(`"validCodeKey":"key"`), porttest.OK(""), porttest.OK(`"token":"account-b"`))
	case "logout":
		tr.Steps = append(tr.Steps, porttest.OK(""))
	}
	policy := domain.DefaultRetryPolicy()
	bytes := transfers.New(tr, s.clock, policy)
	device := devicefiles.New(s.channel, s, bytes, "https://storage.invalid")
	var err error
	switch operation {
	case "web", "web-multipart":
		web := webfiles.New(s.channel, s, bytes, "https://storage.invalid", s.clock)
		size := 1
		if operation == "web-multipart" {
			size = int(transfers.ChunkSize) + 1
		}
		err = web.UploadBytes(t.Context(), make([]byte, size), "synthetic.bin", domain.FileID(0))
	case "upload", "multipart", "form":
		size := 1
		if operation == "multipart" || operation == "form" {
			size = int(transfers.ChunkSize) + 1
		}
		_, err = device.UploadBytes(t.Context(), make([]byte, size), "synthetic.bin")
	case "handwriting", "handwriting-form":
		file := filepath.Join(t.TempDir(), "synthetic.mark")
		size := 1
		if operation == "handwriting-form" {
			size = int(transfers.ChunkSize) + 1
		}
		if err := os.WriteFile(file, make([]byte, size), 0600); err != nil {
			t.Fatal(err)
		}
		client := digests.New(domain.Official(), s.channel, s, bytes, s.clock)
		_, err = client.UploadHandwriting(t.Context(), file, "synthetic.mark", nil)
	case "lock":
		err = device.WithSyncLock(t.Context(), func(context.Context, bool) error { change(); return nil })
	case "feed":
		sock := porttest.NewSocket(`0{"sid":"synthetic","pingInterval":5000,"pingTimeout":25000}`, "40", "41")
		dialer := &porttest.SocketDialer{Steps: []porttest.DialStep{{Socket: sock}, {Err: domain.ErrSessionExpired}}}
		f := feed.New(domain.Official(), s, dialer, &porttest.Clock{}, policy)
		for ch, e := range f.Watch(t.Context()) {
			if e != nil {
				err = e
				break
			}
			if ch.Type == domain.ChangeConnected {
				change()
			}
		}
		if len(dialer.Calls()) != 1 {
			t.Errorf("reconnected: %v", dialer.Calls())
		}
	}
	if !fired {
		t.Fatal("mutation boundary not reached")
	}
	if !errors.Is(err, domain.ErrNotLoggedIn) {
		t.Errorf("pinned operation: %v", err)
	}
	if shared {
		token, e := s.AccessToken(nil)
		if e != nil || token != "account-b" {
			t.Errorf("new operation did not adopt B: %v", e)
		}
	}
	for _, r := range tr.Calls() {
		if strings.Contains(r.Header.Get(domain.AccessTokenHeader), "account-b") || strings.Contains(r.Header.Get("channel"), "account-b") {
			t.Errorf("cross-account request: %s", r.URL)
		}
	}
	if shared {
		tr.Steps = append(tr.Steps, porttest.OK(""))
		if e := transfers.Authorized(t.Context(), s.channel, s, port.Call{Path: "synthetic/new"}, nil, nil); e != nil {
			t.Fatal(e)
		}
		calls := tr.Calls()
		if calls[len(calls)-1].Header.Get(domain.AccessTokenHeader) != "account-b" {
			t.Fatal("new request did not use B")
		}
	}

}
