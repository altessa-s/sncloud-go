// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package httpapi

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/altessa-s/sncloud-go/internal/port"
)

// Maximum redacted body bytes printed in a wire log.
const wireBodyLimit = 4096

// WireLevel controls optional sanitized wire logging.
type WireLevel int

// Wire log levels.
const (
	// WireOff disables wire diagnostics.
	WireOff WireLevel = iota
	WireHeaders
	WireBodies
)

// ParseWireLevel accepts the wire-log environment spellings.
func ParseWireLevel(s string) WireLevel {
	switch strings.ToLower(s) {
	case "headers", "1", "true", "yes", "on":
		return WireHeaders
	case "bodies", "body", "full", "2":
		return WireBodies
	}
	return WireOff
}
func secretField(s string) bool {
	s = strings.ToLower(s)
	for _, part := range []string{
		"authorization",
		"token",
		"cookie",
		"signature",
		"secret",
		"xsrf",
		"channel",
		"password",
		"validcode",
		"sign",
	} {
		if strings.Contains(s, part) {
			return true
		}
	}
	return false
}
func safeURL(s string) string {
	u, err := url.Parse(s)
	if err != nil {
		return "<invalid url>"
	}
	count := 0
	for _, values := range u.Query() {
		count += len(values)
	}
	u.User = nil
	u.Opaque = ""
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	if count > 0 {
		return fmt.Sprintf("%s (+%d query items)", u.String(), count)
	}
	return u.String()
}

// credentialBody suppresses entire authentication exchanges, even in body mode.
// These schemas carry passwords, one-use codes and tokens in both directions;
// field-name redaction alone cannot promise to catch a new credential field.
func credentialBody(path string) bool {
	p := strings.ToLower(path)
	return strings.Contains(p, "login") ||
		strings.Contains(p, "validcode") ||
		strings.Contains(p, "random/code") ||
		strings.Contains(p, "/user/query/token")
}
func redact(v any, key string) any {
	if v != nil && secretField(key) {
		return "<redacted>"
	}
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			x[k] = redact(val, k)
		}
	case []any:
		for i, val := range x {
			x[i] = redact(val, "")
		}
	case string:
		if urlValue(x, key) {
			return safeURL(x)
		}
	}
	return v
}

// URL-shaped strings must fail closed even when malformed escapes prevent parsing.
func urlValue(value, key string) bool {
	key = strings.ToLower(key)
	if strings.Contains(key, "url") || strings.Contains(key, "uri") || key == "referer" ||
		strings.Contains(value, "://") || strings.HasPrefix(value, "/") || strings.ContainsAny(value, "?#") {
		return true
	}
	u, err := url.Parse(value)
	return err == nil && (u.RawQuery != "" || u.User != nil || u.Opaque != "" || u.Fragment != "")
}

func bodyText(b []byte, contentType string) string {
	if !strings.Contains(strings.ToLower(contentType), "json") {
		return fmt.Sprintf("%d bytes (not printed)", len(b))
	}
	// Keep numeric lexemes intact while recursively redacting credential fields;
	// decoding into float64 would round snowflake identifiers in diagnostics.
	var v any
	if json.Unmarshal(b, &v, json.WithUnmarshalers(json.UnmarshalFromFunc(func(dec *jsontext.Decoder, n *any) error {
		if dec.PeekKind() != '0' {
			return errors.ErrUnsupported
		}
		raw, err := dec.ReadValue()
		*n = raw.Clone()
		return err
	}))) != nil {
		return fmt.Sprintf("%d bytes of JSON that does not parse (not printed)", len(b))
	}
	out, err := json.Marshal(redact(v, ""))
	if err != nil {
		return "unprintable JSON"
	}
	if len(out) <= wireBodyLimit {
		return string(out)
	}
	cut := out[:wireBodyLimit]
	for !utf8.Valid(cut) {
		cut = cut[:len(cut)-1]
	}
	return fmt.Sprintf("%s… (%d bytes in all)", cut, len(out))
}
func (t *Transport) log(ctx context.Context, arrow string, r *http.Request, status int, h http.Header, b []byte) {
	if t.config.WireLevel == WireOff {
		return
	}
	attrs := []any{"category", "supernote", "method", r.Method, "url", safeURL(r.URL.String())}
	if status != 0 {
		attrs = append(attrs, "status", status)
	}
	t.logger.Log(ctx, slog.LevelDebug, arrow, attrs...)
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		value := strings.Join(h.Values(k), ", ")
		switch {
		case secretField(k):
			value = "<redacted>"
		case strings.EqualFold(k, "Location"), strings.EqualFold(k, "Content-Location"):
			value = safeURL(value)
		case urlValue(value, k):
			value = safeURL(value)
		}
		t.logger.Log(ctx, slog.LevelDebug, "header", "name", k, "value", value)
	}
	if len(b) > 0 {
		value := fmt.Sprintf("%d bytes", len(b))
		if credentialBody(r.URL.Path) {
			value += " (credential exchange — never printed)"
		} else if t.config.WireLevel == WireBodies {
			value = bodyText(b, h.Get("Content-Type"))
		}
		t.logger.Log(ctx, slog.LevelDebug, "body", "value", value)
	}
}

var _ port.Logger = (*slog.Logger)(nil)
