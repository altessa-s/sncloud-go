// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package neterr

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

func TestClassification(t *testing.T) {
	for _, tc := range []struct {
		err    error
		reason domain.TransportReason
	}{
		{context.DeadlineExceeded, domain.ReasonTimedOut},
		{
			&net.DNSError{Err: "synthetic", Name: "host.invalid", IsNotFound: true},
			domain.ReasonCannotFindHost,
		},
		{syscall.ECONNREFUSED, domain.ReasonCannotConnectToHost},
		{io.ErrUnexpectedEOF, domain.ReasonConnectionLost},
		{errors.New("signed-secret"), domain.ReasonOther},
	} {
		err := Classify(context.Background(), &url.Error{Op: "GET", URL: "https://example/o?secret=hidden", Err: tc.err})
		var e *domain.TransportError
		if !errors.As(err, &e) || e.Reason != tc.reason {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(Classify(ctx, io.EOF), context.Canceled) {
		t.Fatal("cancellation lost")
	}
	if Classify(ctx, nil) != nil {
		t.Fatal("nil changed")
	}
}

func TestTLSAndProtocolErrors(t *testing.T) {
	err := Classify(context.Background(), x509.UnknownAuthorityError{})
	var e *domain.TransportError
	if !errors.As(err, &e) || e.Reason != domain.ReasonTLS {
		t.Fatal(err)
	}
	if Classify(context.Background(), domain.ErrSessionExpired) != domain.ErrSessionExpired {
		t.Fatal("protocol error changed")
	}
	if Classify(context.Background(), context.Canceled) != context.Canceled {
		t.Fatal("cancellation classified")
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "synthetic timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestHouseNetworkCategories(t *testing.T) {
	cases := []struct {
		err    error
		reason domain.TransportReason
	}{
		{&net.DNSError{Err: "temporary"}, domain.ReasonDNSLookupFailed},
		{syscall.ETIMEDOUT, domain.ReasonTimedOut},
		{syscall.ENETUNREACH, domain.ReasonNotConnectedToInternet},
		{syscall.EHOSTUNREACH, domain.ReasonNotConnectedToInternet},
		{syscall.ECONNRESET, domain.ReasonConnectionLost},
		{syscall.ECONNABORTED, domain.ReasonConnectionLost},
		{syscall.EPIPE, domain.ReasonConnectionLost},
		{syscall.EINVAL, domain.ReasonOther},
		{&tls.CertificateVerificationError{Err: errors.New("synthetic")}, domain.ReasonTLS},
		{x509.HostnameError{}, domain.ReasonTLS},
		{x509.CertificateInvalidError{}, domain.ReasonTLS},
		{&net.OpError{Err: tls.AlertError(40)}, domain.ReasonTLS},
		{tls.RecordHeaderError{}, domain.ReasonTLS},
		{os.ErrDeadlineExceeded, domain.ReasonTimedOut},
		{&net.OpError{Err: errors.New("lost")}, domain.ReasonConnectionLost},
		{timeoutError{}, domain.ReasonTimedOut},
		{fmt.Errorf("wrapped: %w", errors.New(http2ConnectionLost)), domain.ReasonConnectionLost},
		{
			errors.Join(errors.New("other"), errors.New(http2ConnectionLost)),
			domain.ReasonConnectionLost,
		},
	}
	for _, tc := range cases {
		var failure *domain.TransportError
		err := Classify(context.Background(), tc.err)
		if !errors.As(err, &failure) || failure.Reason != tc.reason {
			t.Fatalf("%T: got %v, want %s", tc.err, err, tc.reason)
		}
	}
	if Classify(nil, context.DeadlineExceeded) != context.DeadlineExceeded {
		t.Fatal("contextless deadline changed")
	}
	if fromErrno(-1) != nil || isHTTP2ConnectionLost(nil) {
		t.Fatal("unknown cause classified")
	}
}

func TestProtocolErrorDropsCredentialBearingWrappers(t *testing.T) {
	cause := &domain.MalformedResponseError{What: "invalid response"}
	wrapped := fmt.Errorf("request https://user:hidden@host.invalid/x?signature=hidden: %w", &url.Error{Op: "GET", URL: "https://host.invalid/x?signature=hidden", Err: cause})
	got := Classify(t.Context(), wrapped)
	if !errors.Is(got, domain.ErrMalformedResponse) {
		t.Fatal(got)
	}
	for err := got; err != nil; err = errors.Unwrap(err) {
		if strings.Contains(err.Error(), "hidden") {
			t.Fatalf("credential in error chain: %v", err)
		}
	}
}
