// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package neterr classifies transport failures without retaining request URLs.
package neterr

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"os"
	"slices"

	"github.com/altessa-s/go-atlas-core/errors/network"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Classify maps a transport failure onto a protocol error.
//
// A canceled or expired context is returned as that context error. A
// deadline that passed while ctx is still alive is not the caller's: it is
// a timer inside the transport, such as net/http's wait for response
// headers, which reports itself as a deadline. That is a timeout, which the
// retry policy repeats. Without a ctx to ask, a deadline stays a deadline.
//
// A body cut short is a lost connection, and so is an HTTP/2 connection
// closed because its ping went unanswered; a failed TLS handshake is a
// failed secure connection. Anything this function does not recognize is
// reduced to a URL-free transport reason; protocol errors keep their identity.
func Classify(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if ctx != nil {
			return transport(domain.ReasonTimedOut)
		}
		return context.DeadlineExceeded
	}
	// Keep the protocol cause, not URL-bearing transport wrappers around it.
	if protocol, ok := errors.AsType[domain.Error](err); ok {
		return protocol
	}
	if dns, ok := errors.AsType[*net.DNSError](err); ok {
		if dns.IsNotFound {
			return transport(domain.ReasonCannotFindHost)
		}
		return transport(domain.ReasonDNSLookupFailed)
	}
	if code, ok := errnoCode(err); ok {
		if mapped := fromErrno(code); mapped != nil {
			return mapped
		}
	}
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return transport(domain.ReasonConnectionLost)
	}
	// A remote TLS alert arrives inside a *net.OpError, so this comes before
	// the lost-connection fallback below.
	if isTLSFailure(err) {
		return transport(domain.ReasonTLS)
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return transport(domain.ReasonTimedOut)
	}
	if _, ok := errors.AsType[*net.OpError](err); ok {
		return transport(domain.ReasonConnectionLost)
	}
	if network.IsRequestTimeoutError(err) {
		return transport(domain.ReasonTimedOut)
	}
	if isHTTP2ConnectionLost(err) {
		return transport(domain.ReasonConnectionLost)
	}
	return transport(domain.ReasonOther)
}

// http2ConnectionLost is how net/http words the failure of every request on
// an HTTP/2 connection it closed because a health ping went unanswered. It
// exports neither a value nor a type for it, so the text is all there is to
// recognize; an integration test of package httpapi holds the wording.
const http2ConnectionLost = "http2: client connection lost"

// isHTTP2ConnectionLost looks for that failure at the root of err. The
// wrappers above it are asked one layer at a time: one that redacts its
// message still unwraps to the cause.
func isHTTP2ConnectionLost(err error) bool {
	for err != nil {
		switch wrapper := err.(type) { //nolint:errorlint // the chain is walked one layer at a time
		case interface{ Unwrap() error }:
			err = wrapper.Unwrap()
		case interface{ Unwrap() []error }:
			return slices.ContainsFunc(wrapper.Unwrap(), isHTTP2ConnectionLost)
		default:
			return err.Error() == http2ConnectionLost
		}
	}
	return false
}

// The Winsock numbers errnoCode reports. Unix constants differ by OS; these
// are the ones the protocol already uses.
const (
	wsaNetUnreach  = 10051
	wsaConnAborted = 10053
	wsaConnReset   = 10054
	wsaTimedOut    = 10060
	wsaConnRefused = 10061
	wsaHostUnreach = 10065
)

func fromErrno(code int) error {
	switch code {
	case wsaConnRefused:
		return transport(domain.ReasonCannotConnectToHost)
	case wsaTimedOut:
		return transport(domain.ReasonTimedOut)
	case wsaHostUnreach, wsaNetUnreach:
		return transport(domain.ReasonNotConnectedToInternet)
	case wsaConnReset, wsaConnAborted:
		return transport(domain.ReasonConnectionLost)
	default:
		return nil
	}
}

// isTLSFailure reports a failed handshake: a certificate that did not
// verify, an alert from the peer, or a peer that does not speak TLS.
func isTLSFailure(err error) bool {
	if _, ok := errors.AsType[*tls.CertificateVerificationError](err); ok {
		return true
	}
	if _, ok := errors.AsType[x509.UnknownAuthorityError](err); ok {
		return true
	}
	if _, ok := errors.AsType[x509.HostnameError](err); ok {
		return true
	}
	if _, ok := errors.AsType[x509.CertificateInvalidError](err); ok {
		return true
	}
	if _, ok := errors.AsType[tls.AlertError](err); ok {
		return true
	}
	_, ok := errors.AsType[tls.RecordHeaderError](err)
	return ok
}

func transport(reason domain.TransportReason) error { return &domain.TransportError{Reason: reason} }
