// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"errors"
	"testing"
)

func TestErrorCodes(t *testing.T) {
	cases := map[Kind][]string{
		KindSessionExpired:          {"E0712", "E0085"},
		KindInvalidCredentials:      {"E0019", "E0708", "E0711"},
		KindAccountLocked:           {"E0709", "E0710", "E0061"},
		KindVerificationRequired:    {"E1760"},
		KindInvalidVerificationCode: {"E1756", "E0101"},
		KindAlreadyExists:           {"E0322", "E0335", "E0337", "E0338"},
		KindNotFound: {
			"E0081",
			"E0308",
			"E0311",
			"E0318",
			"E0320",
			"E0321",
			"E0328",
			"E0329",
			"E0336",
			"E0339",
			"E0340",
		},
		KindSyncLocked: {"E0078", "E0079", "E0301"},
	}
	for kind, codes := range cases {
		for _, code := range codes {
			if got := KindOf(FromEnvelope(" "+code+" ", "why")); got != kind {
				t.Fatalf("%s: %s", code, got)
			}
		}
	}
	err := FromEnvelope("unknown", "sentence")
	var r *RejectedError
	if !errors.Is(err, ErrRejected) || !errors.As(err, &r) || r.Message != "sentence" {
		t.Fatal(err)
	}
}
func TestErrorSurface(t *testing.T) {
	if KindHTTP != "http" {
		t.Fatal("HTTP kind spelling changed")
	}
	for _, e := range []Error{
		ErrNotLoggedIn,
		&AccountLockedError{Code: "E0709"},
		&NotFoundError{Code: "E0081"},
		&RejectedError{Code: "other", Message: "sentence"},
		&MalformedResponseError{What: "shape"},
		&ChecksumMismatchError{Expected: "aa", Actual: "bb"},
		&RateLimitedError{},
		&HTTPError{Status: 503, Path: "/x"},
		&TransportError{Reason: ReasonTimedOut},
		&OversizedResponseError{Bytes: 2, Limit: 1},
	} {
		if e.Error() == "" || KindOf(e) != e.Kind() || !errors.Is(e, sentinel(e.Kind())) {
			t.Fatal(e)
		}
	}
	if KindOf(errors.New("other")) != "" {
		t.Fatal("foreign error classified")
	}
}
