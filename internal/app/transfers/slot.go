// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package transfers

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"path"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// UploadSlot is the shared apply response, including the digest handwriting variant.
type UploadSlot struct {
	URL, Authorization, Date, FullURL, PartURL, InnerName, APIServer string
	FileServer                                                       *domain.WireEcho
}

// UnmarshalJSON accepts alternate date and URL spellings and preserves fileServer's scalar type.
func (s *UploadSlot) UnmarshalJSON(b []byte) error {
	var w struct {
		URL     string         `json:"url"`
		S3URL   string         `json:"s3Url"`
		Auth    string         `json:"s3Authorization"`
		Date    string         `json:"xamzDate"`
		AltDate string         `json:"xAmzDate"`
		Full    string         `json:"fullUploadUrl"`
		Part    string         `json:"partUploadUrl"`
		Inner   string         `json:"innerName"`
		API     jsontext.Value `json:"apiServer"`
		Server  jsontext.Value `json:"fileServer"`
	}
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	*s = UploadSlot{URL: w.URL, Authorization: w.Auth, Date: w.Date, FullURL: w.Full, PartURL: w.Part, InnerName: w.Inner}
	if s.URL == "" {
		s.URL = w.S3URL
	}
	if s.Date == "" {
		s.Date = w.AltDate
	}
	if v := domain.Text(w.API); v != nil {
		s.APIServer = *v
	} else if n := domain.Integer(w.API); n != nil {
		s.APIServer = domain.FileID(*n).String()
	}
	var echo domain.WireEcho
	if json.Unmarshal(w.Server, &echo) == nil {
		s.FileServer = &echo
	}
	return nil
}

// SigningRequest describes exactly one S3 request to authorize.
type SigningRequest struct {
	Method, Key string
	Params      map[string]string
	Body        *string
}

// Signature contains credentials for storage alone.
type Signature struct {
	Authorization string `json:"s3Authorization"`
	Date          string `json:"xamzDate"`
}

// Signer signs each multipart step, including cleanup, under the initiating login.
type Signer func(context.Context, SigningRequest) (Signature, error)

// NewSigner creates a channel-backed signer usable by any byte client.
func NewSigner(ch port.Channel, s port.FileSession, server *domain.WireEcho, pin *domain.CredentialPin) Signer {
	return func(ctx context.Context, r SigningRequest) (Signature, error) {
		body := struct {
			Server  *domain.WireEcho  `json:"fileServer,omitempty"`
			Method  string            `json:"method"`
			Params  map[string]string `json:"params"`
			Headers map[string]string `json:"awsHeaders"`
			URI     string            `json:"uri"`
			Data    *string           `json:"data,omitempty"`
		}{server, r.Method, r.Params, map[string]string{}, r.Key, r.Body}
		var out Signature
		err := Authorized(ctx, ch, s, port.Call{Path: "file/s3/auth", Body: body}, pin, &out)
		if err == nil && (out.Authorization == "" || out.Date == "") {
			err = malformed("S3 signature missing authorization or date")
		}
		return out, err
	}
}

// FormServer pins form credentials and the destination origin to configuration.
type FormServer struct {
	Host  string
	Token func() (string, error)
}

// Send uploads a measured source; the returned stored name belongs in upload/finish.
func (t *Transfers) Send(ctx context.Context, o Original, s UploadSlot, signer Signer, server *FormServer, p port.Progress) (string, error) {
	if s.APIServer == "0" {
		return "", &domain.RejectedError{Code: "OSS", Message: "uploads to Aliyun OSS are not supported"}
	}
	if (s.APIServer == "1" || s.APIServer == "") && s.URL != "" {
		u, err := storageURL(s.URL)
		if err != nil {
			return "", malformed("invalid object URL")
		}
		key := path.Base(u.Path)
		if u.Path == "" || strings.HasSuffix(u.Path, "/") || key == "." || key == ".." {
			return "", malformed("object URL has no key")
		}
		if o.Size > ChunkSize && signer != nil {
			err = t.multipart(ctx, o, s.URL, key, signer, p)
			return key, err
		}
		if s.Authorization != "" {
			err = t.put(ctx, o.File, s.URL, Signature{Authorization: s.Authorization, Date: s.Date}, p)
			if s.InnerName != "" {
				key = s.InnerName
			}
			return key, err
		}
	}
	if s.FullURL != "" {
		var err error
		s, err = s.formTarget(server)
		if err != nil {
			return "", err
		}

		name, err := t.form(ctx, o, s, server, p)
		if err != nil {
			return "", err
		}
		if name == "" {
			name = s.InnerName
		}
		if name == "" {
			return "", malformed("upload named no stored file")
		}
		return name, nil
	}
	return "", malformed("upload/apply named nowhere to put bytes")
}

func report(p port.Progress, done, total int64) {
	if p != nil {
		p(done, total)
	}
}

// checksum accepts an omitted server checksum but rejects a declared mismatch.
func checksum(expected, actual string) error {
	actual = strings.ToLower(actual)
	if actual != "" && expected != actual {
		return &domain.ChecksumMismatchError{Expected: expected, Actual: actual}
	}
	return nil
}
