// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package transfers

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"math"
	"mime/multipart"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/session"
	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
	"github.com/altessa-s/sncloud-go/internal/porttest"
)

func transfer(steps ...porttest.Step) (*Transfers, *porttest.Transport) {
	tr := &porttest.Transport{Steps: steps}
	return New(tr, &porttest.Clock{}, domain.DefaultRetryPolicy()), tr
}

func source(t *testing.T, data []byte) Original {
	t.Helper()
	file, e := Scratch(data)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { os.Remove(file) })
	o, e := Measure(file, "test.bin")
	if e != nil {
		t.Fatal(e)
	}
	return o
}

func TestFetchChecksAndCleanup(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	cases := []struct {
		name     string
		response port.Response
		hash     string
		span     *ByteRange
		limit    int64
		want     error
	}{
		{"hash", port.Response{Status: 200, Body: []byte("hello")}, "5D41402ABC4B2A76B9719D911017C592", nil, 5, nil},
		{"mismatch", port.Response{Status: 200, Body: []byte("bad")}, "expected", nil, 5, domain.ErrChecksumMismatch},
		{"storage401", port.Response{Status: 401}, "", nil, 5, domain.ErrHTTP},
		{"tooLarge", port.Response{Status: 200, Body: []byte("hello")}, "", nil, 4, domain.ErrOversizedResponse},
		{"empty", port.Response{Status: 200}, "", nil, 0, nil},
		{"zeroCeiling", port.Response{Status: 200, Body: []byte("x")}, "", nil, 0, domain.ErrOversizedResponse},
		{"range", port.Response{Status: 206, Header: port.Header{"Content-Range": {"bytes 1-3/5"}}, Body: []byte("ell")}, "wrong", &ByteRange{First: 1, Last: 3}, 5, nil},
		{"shortEOF", port.Response{Status: 206, Header: port.Header{"Content-Range": {"bytes 1-4/5"}}, Body: []byte("ello")}, "", &ByteRange{First: 1, Last: 99}, 5, nil},
		{"ignoredRange", port.Response{Status: 200, Body: []byte("ell")}, "", &ByteRange{First: 1, Last: 3}, 5, domain.ErrMalformedResponse},
		{"noRange", port.Response{Status: 206, Body: []byte("ell")}, "", &ByteRange{First: 1, Last: 3}, 5, domain.ErrMalformedResponse},
		{"wrongRange", port.Response{Status: 206, Header: port.Header{"Content-Range": {"bytes 0-2/5"}}, Body: []byte("hel")}, "", &ByteRange{First: 1, Last: 3}, 5, domain.ErrMalformedResponse},
		{"shortBody", port.Response{Status: 206, Header: port.Header{"Content-Range": {"bytes 1-3/5"}}, Body: []byte("el")}, "", &ByteRange{First: 1, Last: 3}, 5, domain.ErrMalformedResponse},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr, record := transfer(porttest.Step{Response: tc.response})
			file, e := tr.Fetch(t.Context(), "https://storage.invalid/object", tc.limit, tc.hash, tc.span, func(int64, int64) {})
			if tc.want != nil {
				if !errors.Is(e, tc.want) {
					t.Fatal(file, e)
				}
			} else {
				if e != nil {
					t.Fatal(e)
				}
				os.Remove(file)
			}
			if record.Calls()[0].Header.Get(domain.AccessTokenHeader) != "" {
				t.Fatal("credential leaked")
			}
			files, _ := os.ReadDir(os.Getenv("TMPDIR"))
			if len(files) != 0 {
				t.Fatal("leaked files", files)
			}
		})
	}
	for _, span := range []*ByteRange{{First: -1, Last: 0}, {First: 3, Last: 2}, {First: 0, Last: math.MaxInt64}} {
		tr, _ := transfer()
		if _, e := tr.Fetch(t.Context(), "https://storage.invalid/x", 5, "", span, nil); !errors.Is(e, domain.ErrMalformedResponse) {
			t.Fatal(e)
		}
	}
	tr, _ := transfer()
	if _, e := tr.Fetch(t.Context(), "x", -1, "", nil, nil); e == nil {
		t.Fatal("negative ceiling")
	}
}

func TestContentRange(t *testing.T) {
	for _, s := range []string{"bytes 0-4/5", "BYTES 0-4/*", " bytes 2-2/3 "} {
		if _, _, _, ok := contentRange(s); !ok {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"", "bytes", "items 0-4/5", "bytes 0-4", "bytes 0/5", "bytes a-4/5", "bytes -1-4/5", "bytes 0-a/5", "bytes 4-3/5", "bytes 0-4/4", "bytes 0-4/x"} {
		if _, _, _, ok := contentRange(s); ok {
			t.Fatal(s)
		}
	}
	if checkSize("/absent", 0) == nil {
		t.Fatal("missing file")
	}
	if served(ByteRange{First: 0, Last: 0}, port.Response{Status: 206, Header: port.Header{"Content-Range": {"bytes 0-0/1"}}, File: "/absent"}) == nil {
		t.Fatal("missing range file")
	}
}

func TestSlotVariants(t *testing.T) {
	for _, raw := range []string{
		`{"s3Url":"https://storage.invalid/a","s3Authorization":"signed","xAmzDate":"date","apiServer":1,"fileServer":"opaque"}`,
		`{"url":"https://storage.invalid/a","s3Authorization":"signed","xamzDate":"date","apiServer":"1","fileServer":2}`,
	} {
		var s UploadSlot
		if e := json.Unmarshal([]byte(raw), &s); e != nil || s.URL != "https://storage.invalid/a" || s.Date != "date" || s.APIServer != "1" || s.FileServer == nil {
			t.Fatal(s, e)
		}
	}
	var s UploadSlot
	if json.Unmarshal([]byte(`{"url":4}`), &s) == nil {
		t.Fatal("wrong URL type accepted")
	}
	if e := json.Unmarshal([]byte(`{"apiServer":{},"fileServer":null}`), &s); e != nil || s.FileServer != nil {
		t.Fatal(s, e)
	}
}

func TestFormAndScratch(t *testing.T) {
	o := source(t, []byte("hello"))
	o.Name = "bad\"\r\nname"
	file, ct, head, _, e := multipartFile(o, 1, 3)
	if e != nil {
		t.Fatal(e)
	}
	defer os.Remove(file)
	data, e := os.ReadFile(file)
	if e != nil {
		t.Fatal(e)
	}
	reader := multipart.NewReader(strings.NewReader(string(data)), strings.TrimPrefix(ct, "multipart/form-data; boundary="))
	part, e := reader.NextPart()
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(part)
	if e != nil || string(b) != "ell" || head <= 0 || strings.ContainsAny(part.FileName(), "\r\n\"") {
		t.Fatal(string(b), e, part.FileName())
	}
	for _, pair := range [][2]int64{{-1, 1}, {0, -1}, {0, 99}} {
		if e := copySlice(io.Discard, o.File, pair[0], pair[1]); e == nil {
			t.Fatal("invalid slice accepted")
		}
	}
	if e := copySlice(io.Discard, "/absent", 0, 1); e == nil {
		t.Fatal("missing source")
	}
	if _, e := Measure("/absent", ""); e == nil {
		t.Fatal("missing source")
	}
	if _, _, _, _, e := multipartFile(Original{File: "/absent"}, 0, 1); e == nil {
		t.Fatal("missing form source")
	}
	if _, e := scratch(func(*os.File) error { return errors.New("write failure") }); e == nil {
		t.Fatal("write failure ignored")
	}
}

func TestSendSingleAndForm(t *testing.T) {
	o := source(t, []byte("hello"))
	tr, record := transfer(porttest.Step{Response: port.Response{Status: 200}})
	name, e := tr.Send(t.Context(), o, UploadSlot{URL: "https://storage.invalid/key", Authorization: "signature", Date: "date", InnerName: "inner"}, nil, nil, nil)
	if e != nil || name != "inner" || record.Calls()[0].Header.Get("x-amz-content-sha256") != "UNSIGNED-PAYLOAD" {
		t.Fatal(name, e)
	}
	tr, record = transfer(porttest.OK(`"innerName":"stored","md5":"5D41402ABC4B2A76B9719D911017C592"`))
	var done int64
	name, e = tr.Send(t.Context(), o, UploadSlot{FullURL: "http://internal.invalid/api/oss/upload?signature=x"}, nil,
		&FormServer{Host: "https://configured.invalid", Token: func() (string, error) { return "synthetic", nil }}, func(n, total int64) {
			done = n
			if total != 5 {
				t.Fatal(total)
			}
		})
	if e != nil || name != "stored" || done != 5 || record.Calls()[0].URL != "https://configured.invalid/api/oss/upload?signature=x" {
		t.Fatal(name, e, done)
	}
	for _, slot := range []UploadSlot{{APIServer: "0"}, {}, {URL: "https://storage.invalid/key"}, {URL: "%zz", Authorization: "sig"}} {
		tr, _ = transfer()
		if _, e = tr.Send(t.Context(), o, slot, nil, nil, nil); e == nil {
			t.Fatal("invalid slot accepted", slot)
		}
	}
	for _, step := range []porttest.Step{porttest.OK(`"md5":"wrong"`), porttest.OK(""), {Err: context.Canceled}, {Response: port.Response{Status: 401}}, {Response: port.Response{Status: 200, Body: []byte("invalid")}}} {
		tr, _ = transfer(step)
		if _, e = tr.Send(t.Context(), o, UploadSlot{FullURL: "https://storage.invalid/form"}, nil, nil, nil); e == nil {
			t.Fatal("form failure ignored")
		}
	}
	tr, _ = transfer(porttest.Step{Err: context.Canceled})
	if _, e = tr.Send(t.Context(), o, UploadSlot{URL: "https://storage.invalid/x", Authorization: "sig"}, nil, nil, nil); e == nil {
		t.Fatal("put failure ignored")
	}
}

func TestPrivateParts(t *testing.T) {
	o := source(t, make([]byte, ChunkSize+1))
	first := source(t, make([]byte, ChunkSize))
	last := source(t, []byte{0})
	tr, record := transfer(porttest.OK(`"chunkMd5":"`+first.MD5+`"`), porttest.OK(`"chunkMd5":"`+last.MD5+`"`))
	slot := UploadSlot{FullURL: "https://storage.invalid/full", PartURL: "https://storage.invalid/part?signed=yes", InnerName: "stored", APIServer: "private_cloud"}
	var done int64
	name, e := tr.Send(t.Context(), o, slot, nil, &FormServer{Host: "https://configured.invalid", Token: func() (string, error) { return "synthetic", nil }}, func(n, _ int64) { done = n })
	if e != nil || name != "stored" || done != o.Size {
		t.Fatal(name, e, done)
	}
	for i, r := range record.Calls() {
		u, _ := url.Parse(r.URL)
		if u.Query().Get("partNumber") != []string{"1", "2"}[i] || u.Query().Get("totalChunks") != "2" || u.Query().Get("signed") != "yes" {
			t.Fatal(r.URL)
		}
	}
	for _, step := range []porttest.Step{porttest.OK(`"chunkMd5":"wrong"`), {Err: context.Canceled}} {
		tr, _ = transfer(step)
		if _, e = tr.Send(t.Context(), o, slot, nil, nil, nil); e == nil {
			t.Fatal("part failure ignored")
		}
	}
	slot.PartURL = "%zz"
	tr, _ = transfer()
	if _, e = tr.Send(t.Context(), o, slot, nil, nil, nil); e == nil {
		t.Fatal("invalid part URL")
	}
}

func TestS3MultipartAndAbort(t *testing.T) {
	o := source(t, make([]byte, ChunkSize+1))
	start := porttest.Step{Response: port.Response{Status: 200, Body: []byte("<InitiateMultipartUploadResult><UploadId>upload&amp;id</UploadId></InitiateMultipartUploadResult>")}}
	part := porttest.Step{Response: port.Response{Status: 200, Header: port.Header{"ETag": {"\"tag&amp;\""}}}}
	finish := porttest.Step{Response: port.Response{Status: 200, Body: []byte("<CompleteMultipartUploadResult/>")}}
	for _, failure := range []string{"", "part", "etag", "complete", "embedded", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			steps := []porttest.Step{start, part, part, finish}
			switch failure {
			case "part":
				steps = []porttest.Step{start, {Err: errors.New("part failure")}, finish}
			case "etag":
				steps = []porttest.Step{start, finish, finish}
			case "complete":
				steps = []porttest.Step{start, part, part, {Response: port.Response{Status: 503}}, finish}
			case "embedded":
				steps = []porttest.Step{start, part, part, {Response: port.Response{Status: 200, Body: []byte("<Error><Code>BadDigest</Code><Message>bad</Message></Error>")}}, finish}
			case "cancel":
				steps = []porttest.Step{start, {Err: context.Canceled}, finish}
			}
			tr, record := transfer(steps...)
			var signed []SigningRequest
			signer := func(ctx context.Context, r SigningRequest) (Signature, error) {
				if ctx.Err() != nil {
					t.Fatal("cleanup context canceled")
				}
				signed = append(signed, r)
				return Signature{Authorization: "synthetic", Date: "date"}, nil
			}
			name, e := tr.Send(t.Context(), o, UploadSlot{URL: "https://storage.invalid/key", APIServer: "1"}, signer, nil, func(int64, int64) {})
			if failure == "" {
				if e != nil || name != "key" || len(signed) != 4 {
					t.Fatal(name, e, signed)
				}
				want := "<CompleteMultipartUpload><Part><PartNumber>1</PartNumber><ETag>&#34;tag&amp;amp;&#34;</ETag></Part><Part><PartNumber>2</PartNumber><ETag>&#34;tag&amp;amp;&#34;</ETag></Part></CompleteMultipartUpload>"
				if *signed[3].Body != want || string(record.Calls()[3].Body) != want {
					t.Fatal(*signed[3].Body)
				}
			} else if e == nil || signed[len(signed)-1].Method != "DELETE" {
				t.Fatal(e, signed)
			}
			for _, r := range record.Calls() {
				if r.File != "" {
					if r.File != o.File || r.Section == nil || int64(len(r.Body)) != r.Section.Length {
						t.Fatalf("part did not use a source section after %q: %+v", failure, r)
					}
					if _, err := os.Stat(o.File); err != nil {
						t.Fatalf("source removed: %v", err)
					}
				}
				if r.Header.Get(domain.AccessTokenHeader) != "" {
					t.Fatal("token on S3")
				}
			}
		})
	}
	for _, step := range []porttest.Step{{Err: context.Canceled}, finish} {
		tr, _ := transfer(step)
		if _, e := tr.Send(t.Context(), o, UploadSlot{URL: "https://storage.invalid/key"}, func(context.Context, SigningRequest) (Signature, error) { return Signature{}, nil }, nil, nil); e == nil {
			t.Fatal("init failure ignored")
		}
	}
	tr, _ := transfer()
	if _, e := tr.s3(t.Context(), "https://storage.invalid/x", SigningRequest{}, func(context.Context, SigningRequest) (Signature, error) { return Signature{}, errors.New("signer") }, "", nil, nil, false); e == nil {
		t.Fatal("sign failure")
	}
	if _, e := tr.s3(t.Context(), "%zz", SigningRequest{}, func(context.Context, SigningRequest) (Signature, error) { return Signature{}, nil }, "", nil, nil, false); e == nil {
		t.Fatal("bad URL")
	}
	if decodeStorageXML([]byte("<UploadId>broken"), new(string)) == nil {
		t.Fatal("broken XML accepted")
	}
}

func TestSignerAndAuthorization(t *testing.T) {
	tr := &porttest.Transport{Steps: []porttest.Step{porttest.OK(`"s3Authorization":"signed","xamzDate":"date"`), porttest.Refused("E0712")}}
	clock := &porttest.Clock{}
	ch := channel.New(domain.PrivateCloud("https://synthetic.invalid", false), tr, clock, domain.DefaultRetryPolicy())
	store := &porttest.Secrets{}
	if e := store.Keep("synthetic", domain.AccessToken); e != nil {
		t.Fatal(e)
	}
	s := session.New(ch, store, clock)
	signer := NewSigner(ch, s, nil, s.Pin())
	sig, e := signer(t.Context(), SigningRequest{Method: "PUT", Params: map[string]string{"partNumber": "1"}, Key: "object"})
	if e != nil || sig.Authorization != "signed" {
		t.Fatal(sig, e)
	}
	if e = Authorized(t.Context(), ch, s, port.Call{}, nil, nil); !errors.Is(e, domain.ErrSessionExpired) {
		t.Fatal(e)
	}
	if e = Authorized(t.Context(), ch, s, port.Call{}, nil, nil); !errors.Is(e, domain.ErrNotLoggedIn) {
		t.Fatal(e)
	}
	if e = Authorized(t.Context(), ch, headerFailure{}, port.Call{}, nil, nil); e == nil {
		t.Fatal("header failure ignored")
	}
}

type headerFailure struct{ port.FileSession }

func (headerFailure) AccessToken(*domain.CredentialPin) (string, error) { return "synthetic", nil }
func (headerFailure) AppHeaders(string, bool) (port.Header, error) {
	return nil, errors.New("identity unavailable")
}

func TestURLRewriting(t *testing.T) {
	for _, tc := range []struct{ input, host, want string }{
		{"http://internal:99/api/oss/download?a=b", "https://configured.invalid", "https://configured.invalid/api/oss/download?a=b"},
		{"https://storage.invalid/object", "https://configured.invalid", "https://storage.invalid/object"},
		{"%zz", "https://configured.invalid", "%zz"},
		{"https://storage.invalid/api/oss/a", "%zz", "https://storage.invalid/api/oss/a"},
	} {
		if got := ServedFrom(tc.input, tc.host); got != tc.want {
			t.Fatal(got, tc.want)
		}
	}
	if moved("%zz", "https://configured.invalid") != "%zz" {
		t.Fatal("bad URL changed")
	}
	if accepted(port.Response{Status: 200}, "%zz", time.Time{}) == nil {
		t.Fatal("bad storage URL")
	}
}

func TestCanceledMultipartAbortsWithLiveContext(t *testing.T) {
	o := source(t, make([]byte, ChunkSize+1))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tr, record := transfer(
		porttest.Step{Response: port.Response{Status: 200, Body: []byte("<InitiateMultipartUploadResult><UploadId>id</UploadId></InitiateMultipartUploadResult>")}},
		porttest.Step{Run: func(context.Context, port.Request) (port.Response, error) {
			cancel()
			return port.Response{}, context.Canceled
		}},
		porttest.Step{Run: func(ctx context.Context, r port.Request) (port.Response, error) {
			if ctx.Err() != nil || r.Method != "DELETE" {
				t.Fatal("abort was canceled", ctx.Err(), r.Method)
			}
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("abort has no deadline")
			}
			return port.Response{Status: 503}, nil
		}},
	)
	signer := func(ctx context.Context, _ SigningRequest) (Signature, error) {
		if ctx.Err() != nil {
			t.Fatal("signer canceled")
		}
		return Signature{}, nil
	}
	_, e := tr.Send(ctx, o, UploadSlot{URL: "https://storage.invalid/key"}, signer, nil, nil)
	if !errors.Is(e, context.Canceled) || len(record.Calls()) != 3 {
		t.Fatal(e, record.Calls())
	}
	part := record.Calls()[1]
	if part.File != o.File || part.Section == nil || part.Section.Offset != 0 || part.Section.Length != ChunkSize {
		t.Fatalf("upload did not use a source section: %+v", part)
	}
	if _, err := os.Stat(o.File); err != nil {
		t.Fatalf("canceled upload removed source: %v", err)
	}
}

func TestMalformedStorageTargetsFailBeforeIO(t *testing.T) {
	for _, target := range []string{"", "/relative", "//storage.invalid/key", "ftp://storage.invalid/key", "https://", "%zz"} {
		tr, record := transfer()
		_, err := tr.Fetch(t.Context(), target, 1, "", nil, nil)
		if !errors.Is(err, domain.ErrMalformedResponse) || len(record.Calls()) != 0 {
			t.Errorf("%q: %v calls=%d", target, err, len(record.Calls()))
		}
	}
	o := source(t, []byte("x"))
	for _, target := range []string{"https://storage.invalid", "https://storage.invalid/", "/key", "ftp://storage.invalid/key", "https://storage.invalid/.."} {
		tr, record := transfer()
		_, err := tr.Send(t.Context(), o, UploadSlot{URL: target, Authorization: "synthetic"}, nil, nil, nil)
		if !errors.Is(err, domain.ErrMalformedResponse) || len(record.Calls()) != 0 {
			t.Errorf("%q: %v calls=%d", target, err, len(record.Calls()))
		}
	}
	for _, target := range []string{"/relative", "ftp://storage.invalid/upload", "%zz"} {
		tr, record := transfer()
		_, err := tr.Send(t.Context(), o, UploadSlot{FullURL: target}, nil, nil, nil)
		if !errors.Is(err, domain.ErrMalformedResponse) || len(record.Calls()) != 0 {
			t.Errorf("form %q: %v calls=%d", target, err, len(record.Calls()))
		}
	}
}
func TestSignerRejectsMissingCredentials(t *testing.T) {
	for _, fields := range []string{"", `"s3Authorization":"sig"`, `"xamzDate":"date"`, `"s3Authorization":"","xamzDate":"date"`} {
		wire := &porttest.Transport{Steps: []porttest.Step{porttest.OK(fields)}}
		clock := &porttest.Clock{}
		ch := channel.New(domain.Official(), wire, clock, domain.DefaultRetryPolicy())
		store := &porttest.Secrets{}
		_ = store.Keep("synthetic", domain.AccessToken)
		sess := session.New(ch, store, clock)
		_, err := NewSigner(ch, sess, nil, sess.Pin())(t.Context(), SigningRequest{Method: "PUT", Key: "key"})
		if !errors.Is(err, domain.ErrMalformedResponse) {
			t.Errorf("%s: %v", fields, err)
		}
	}
}
func TestMultipartNamespaceErrorAndBareUploads(t *testing.T) {
	o := source(t, []byte("x"))
	tr, record := transfer(
		porttest.Step{Response: port.Response{Status: 200, Body: []byte(`<InitiateMultipartUploadResult><UploadId>id</UploadId></InitiateMultipartUploadResult>`)}},
		porttest.Step{Response: port.Response{Status: 200, Header: port.Header{"ETag": {"synthetic"}}}},
		porttest.Step{Response: port.Response{Status: 200, Body: []byte(`<Error xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Message>failed</Message></Error>`)}},
		porttest.Step{Response: port.Response{Status: 204}},
	)
	err := tr.multipart(t.Context(), o, "https://storage.invalid/key", "key", func(context.Context, SigningRequest) (Signature, error) {
		return Signature{Authorization: "sig", Date: "date"}, nil
	}, nil)
	var rejected *domain.RejectedError
	if !errors.As(err, &rejected) || rejected.Code != "S3" || rejected.Message != "failed" {
		t.Errorf("%v", err)
	}
	calls := record.Calls()
	if calls[0].URL != "https://storage.invalid/key?uploads" {
		t.Error(calls[0].URL)
	}
	if len(calls) != 4 || calls[len(calls)-1].Method != "DELETE" {
		t.Error("no abort", calls)
	}
}
