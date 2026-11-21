// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package domain

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// Endpoints separates the app, web and change-feed origins. Public clouds require
// HTTPS except on loopback hosts. Selecting PrivateCloud allows HTTP and disables
// the dial guard for all hosts: the caller vouches for those destinations.
type Endpoints struct {
	AppHost, WebHost, SocketHost string
	WebUsesXSRF                  bool
	// PrivateCloud allows HTTP for private servers without certificates and disables
	// the dial guard, even for public hosts. The caller vouches for all hosts.
	PrivateCloud bool
}

const cleartextScheme = "http"

// AccountPage is the account-management website.
const AccountPage = "https://cloud.supernote.com"

// Official returns the global service's hosts.
func Official() Endpoints {
	return Endpoints{
		"https://viewer.supernote.com",
		"https://cloud.supernote.com",
		"https://app-wss-file.supernote.com",
		true,
		false,
	}
}

// PrivateCloud uses one host and allows HTTP for servers without certificates.
// The caller vouches for all destinations: the dial guard is disabled even for
// public hosts. Without this choice, only loopback hosts may use HTTP.
func PrivateCloud(host string, usesXSRF bool) Endpoints {
	return Endpoints{host, host, host, usesXSRF, true}
}

// EndpointsFromEnvironment applies an explicitly requested debug override.
// New never reads SNCLOUD_HOST implicitly.
func EndpointsFromEnvironment(env map[string]string) Endpoints {
	e := Official()
	u, err := url.Parse(env["SNCLOUD_HOST"])
	if err == nil && u.Hostname() != "" && u.User == nil {
		host := strings.ToLower(u.Hostname())
		scheme := strings.ToLower(u.Scheme)
		if scheme == "https" || scheme == cleartextScheme && (host == "localhost" || host == "127.0.0.1" || host == "::1") {
			e.AppHost = u.String()
			e.WebHost = u.String()
			e.SocketHost = u.String()
		}
	}
	switch strings.ToLower(env["SNCLOUD_XSRF"]) {
	case "off", "0", "false", "no":
		e.WebUsesXSRF = false
	}
	return e
}

// Validate checks caller-supplied endpoint configuration before constructing a client.
// It rejects endpoints that cannot carry HTTP requests.
func (e Endpoints) Validate() error {
	for _, s := range []string{e.AppHost, e.WebHost, e.SocketHost} {
		u, err := url.Parse(s)
		if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != cleartextScheme) || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("sncloud: invalid endpoint")
		}

		if u.Scheme == cleartextScheme && !e.PrivateCloud && !loopback(u.Hostname()) {
			return fmt.Errorf("sncloud: public endpoint requires HTTPS")
		}
	}
	return nil
}

func loopback(host string) bool {
	ip, err := netip.ParseAddr(host)
	return strings.EqualFold(host, "localhost") || err == nil && ip.IsLoopback()
}

// App resolves a protocol path beneath the app host's api directory, for callers
// constructing requests with a custom transport.
func (e Endpoints) App(path string) string {
	return strings.TrimRight(e.AppHost, "/") + "/api/" + strings.TrimLeft(path, "/")
}

// Web resolves a protocol path beneath the web host's api directory, for callers
// constructing requests with a custom transport.
func (e Endpoints) Web(path string) string {
	return strings.TrimRight(e.WebHost, "/") + "/api/" + strings.TrimLeft(path, "/")
}

// Protocol header values and session paths.
const (
	// AccessTokenHeader carries the bearer token on app requests.
	AccessTokenHeader = "x-access-token"
	// XSRFHeader carries the current browser anti-forgery token.
	XSRFHeader = "X-XSRF-TOKEN"
	// XSRFCookie names the cookie paired with the anti-forgery header.
	XSRFCookie = "XSRF-TOKEN"
	// ProtocolVersion is the app protocol version accepted by the service.
	ProtocolVersion = "202407"
	// AppUserAgent identifies app protocol requests.
	AppUserAgent = "Dart/3.8 (dart:io)"
	// WebUserAgent identifies browser protocol requests.
	WebUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"
	// Language selects English service messages.
	Language = "EN"
	// LogCategory names this client in structured diagnostics.
	LogCategory = "supernote"
	// PathRandomCode requests the single-use password salt.
	PathRandomCode = "official/user/query/random/code"
	// PathLogin exchanges a salted digest for a token.
	PathLogin = "official/user/account/login/new"
	// PathLogout ends the remote session.
	PathLogout = "user/logout"
	// PathMailCode requests an emailed verification code.
	PathMailCode = "user/mail/validcode"
	// PathCheckCode checks a code without completing the login.
	PathCheckCode = "user/check/validcode"
	// PathCodeLogin exchanges a checked code for a token.
	PathCodeLogin = "user/sms/login"
)
