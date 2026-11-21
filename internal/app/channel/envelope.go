// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package channel

import (
	"bytes"
	"encoding/json/v2"
	"errors"
	"fmt"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Only inspect a short prefix when describing a non-JSON answer.
const responsePrefixLimit = 64

type envelope struct {
	Success *domain.WireFlag `json:"success"`
	Code    string           `json:"errorCode"`
	Message string           `json:"errorMsg"`
}

func decodeEnvelope(data []byte) (envelope, error) {
	var e envelope
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return e, fmt.Errorf("not object")
	}
	err := json.Unmarshal(data, &e)
	return e, err
}

// Open interprets the envelope before decoding payload fields. Decode errors
// describe shape only: login responses can contain tokens even when malformed.
func Open(data []byte, out any) error {
	e, err := decodeEnvelope(data)
	if err != nil {
		return &domain.MalformedResponseError{What: "not the service's JSON: " + Shape(data)}
	}
	success := strings.TrimSpace(e.Code) == ""
	if e.Success != nil {
		success = bool(*e.Success)
	}
	if !success {
		return domain.FromEnvelope(e.Code, e.Message)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return &domain.MalformedResponseError{What: "invalid payload: " + Shape(data)}
		}
	}
	return nil
}
func explained(err error, b []byte) error {
	h, ok := errors.AsType[*domain.HTTPError](err)
	if !ok || h.Status < 400 || h.Status >= 500 {
		return err
	}
	e, decodeErr := decodeEnvelope(b)
	if decodeErr == nil && strings.TrimSpace(e.Code) != "" && (e.Success == nil || !bool(*e.Success)) {
		return domain.FromEnvelope(e.Code, e.Message)
	}
	return err
}

// Shape describes an unreadable body without quoting any of its contents.
func Shape(b []byte) string {
	head := bytes.TrimSpace(b[:min(responsePrefixLimit, len(b))])
	kind := "text"
	if len(head) == 0 {
		kind = "empty"
	} else {
		switch head[0] {
		case '<':
			kind = "XML"
			if bytes.Contains(bytes.ToLower(head), []byte("html")) {
				kind = "HTML"
			}
		case '{', '[':
			kind = "JSON of another shape"
		}
	}
	return fmt.Sprintf("%s, %d bytes", kind, len(b))
}
