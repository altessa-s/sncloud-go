// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package transfers

import (
	"context"
	"encoding/xml"
	"net/url"
	"strconv"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/app/retry"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// signedRequest uses storage credentials only; account tokens never reach S3.
func signedRequest(target, method string, s Signature) port.Request {
	return port.Request{Method: method, URL: target, Header: port.Header{
		"Authorization": {s.Authorization}, "x-amz-date": {s.Date}, "x-amz-content-sha256": {"UNSIGNED-PAYLOAD"},
	}}
}

// s3 signs the exact query and body before sending them through the shared transport.
// changes selects the conservative policy for operations that cannot be replayed safely.
func (t *Transfers) s3(
	ctx context.Context, target string, r SigningRequest, signer Signer, file string, section *port.FileSection, p port.Progress, changes bool,
) (port.Response, error) {
	u, err := url.Parse(target)
	if err != nil {
		return port.Response{}, malformed("invalid S3 URL")
	}
	q := make(url.Values)
	for k, v := range r.Params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	// Initiation uses a bare subresource on the wire; signing still uses an empty value.
	if value, ok := r.Params["uploads"]; ok && value == "" {
		u.RawQuery = strings.Replace(u.RawQuery, "uploads=", "uploads", 1)
	}
	var out port.Response
	err = retry.Do(ctx, t.policy, t.clock, changes, func() error {
		// Re-sign after backoff: credentials may expire and the initiating login may end.
		sig, signErr := signer(ctx, r)
		if signErr != nil {
			return signErr
		}
		req := signedRequest(u.String(), r.Method, sig)
		req.File = file
		req.Section = section
		req.Progress = p
		if r.Body != nil {
			req.Body = []byte(*r.Body)
			req.Header.Set("Content-Type", "application/json")
		}
		var e error
		if file != "" {
			out, e = t.transport.Upload(ctx, req)
		} else {
			out, e = t.transport.RoundTrip(ctx, req)
		}
		if e != nil {
			return e
		}
		return accepted(out, target, t.clock.Now())
	})
	return out, err
}

// multipart owns the remote upload until completion and aborts it on failure.
// Initiation and completion are changes: retrying an ambiguous failure could
// create another upload or complete one whose result was already committed.
func (t *Transfers) multipart(ctx context.Context, o Original, target, key string, signer Signer, p port.Progress) error {
	started, err := t.s3(ctx, target, SigningRequest{Method: methodPost, Key: key, Params: map[string]string{"uploads": ""}}, signer, "", nil, nil, true)
	if err != nil {
		return err
	}
	var initiation struct {
		XMLName xml.Name `xml:"InitiateMultipartUploadResult"`
		ID      string   `xml:"UploadId"`
	}
	if decodeStorageXML(started.Body, &initiation) != nil || initiation.ID == "" {
		return malformed("S3 named no upload")
	}
	id := initiation.ID
	err = t.parts(ctx, o, target, key, id, signer, p)
	if err != nil {
		// Unfinished parts remain billable. Cancellation must not prevent abort,
		// but a separate deadline keeps cleanup from blocking the caller forever.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		defer cancel()
		// Abort is best effort: its failure must not hide the original upload error.
		_, _ = t.s3(cleanup, target, SigningRequest{Method: "DELETE", Key: key, Params: map[string]string{uploadIDParam: id}}, signer, "", nil, nil, true)
	}
	return err
}

// parts sends numbered slices in order and completes with the returned ETags.
// A repeated part PUT replaces that number, so it uses the read retry policy.
func (t *Transfers) parts(ctx context.Context, o Original, target, key, id string, signer Signer, p port.Progress) error {
	var completion strings.Builder
	completion.WriteString("<CompleteMultipartUpload>")
	for offset, number := int64(0), 1; offset < o.Size; offset, number = offset+min(ChunkSize, o.Size-offset), number+1 {
		length := min(ChunkSize, o.Size-offset)
		section := &port.FileSection{Offset: offset, Length: length}
		resp, err := t.s3(ctx, target, SigningRequest{Method: "PUT", Key: key,
			Params: map[string]string{uploadIDParam: id, "partNumber": strconv.Itoa(number)}}, signer, o.File, section,
			func(done, _ int64) { report(p, offset+min(done, length), o.Size) }, false)
		if err != nil {
			return err
		}
		etag := resp.Header.Get("ETag")
		if etag == "" {
			return malformed("S3 answered part without ETag")
		}
		completion.WriteString("<Part><PartNumber>" + strconv.Itoa(number) + "</PartNumber><ETag>")
		// ETags are server text and can contain XML metacharacters. Escape them
		// before signing so the signed body and the transmitted body stay identical.
		if err := xml.EscapeText(&completion, []byte(etag)); err != nil {
			return err
		}
		completion.WriteString("</ETag></Part>")
	}
	completion.WriteString("</CompleteMultipartUpload>")
	body := completion.String()
	request := SigningRequest{Method: methodPost, Key: key, Params: map[string]string{uploadIDParam: id}, Body: &body}
	resp, err := t.s3(ctx, target, request, signer, "", nil, nil, true)
	if err != nil {
		return err
	}
	return storageCompletion(resp.Body)
}
