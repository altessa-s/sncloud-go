// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package transfers

import (
	"context"
	"crypto/md5" // #nosec G501 -- The service mandates MD5 for content integrity.
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/altessa-s/sncloud-go/internal/app/channel"
	"github.com/altessa-s/sncloud-go/internal/app/retry"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// put can retry the same object bytes safely: a repeated PUT replaces the same key.
func (t *Transfers) put(ctx context.Context, file, target string, s Signature, p port.Progress) error {
	r := signedRequest(target, "PUT", s)
	r.File = file
	r.Progress = p
	return retry.Do(ctx, t.policy, t.clock, false, func() error {
		out, err := t.transport.Upload(ctx, r)
		if err != nil {
			return err
		}
		return accepted(out, target, t.clock.Now())
	})
}

// form selects whole or numbered private uploads and checks any returned checksum.
func (t *Transfers) form(ctx context.Context, o Original, s UploadSlot, server *FormServer, p port.Progress) (string, error) {
	if o.Size <= ChunkSize || s.PartURL == "" {
		var out struct {
			Inner string `json:"innerName"`
			MD5   string `json:"md5"`
		}
		_, err := t.sendForm(ctx, o, 0, o.Size, s.FullURL, server, p, &out)
		if err != nil {
			return "", err
		}
		return out.Inner, checksum(o.MD5, out.MD5)
	}
	id := fmt.Sprintf("%d_%s", t.clock.Now().UnixMilli(), rand.Text())
	count := 1 + (o.Size-1)/ChunkSize
	for index := range count {
		offset := index * ChunkSize
		length := min(ChunkSize, o.Size-offset)
		u, err := url.Parse(s.PartURL)
		if err != nil {
			return "", malformed("invalid part URL")
		}
		q := u.Query()
		q.Set(uploadIDParam, id)
		q.Set("partNumber", strconv.FormatInt(index+1, 10))
		q.Set("totalChunks", strconv.FormatInt(count, 10))
		u.RawQuery = q.Encode()
		var out struct {
			MD5 string `json:"chunkMd5"`
		}
		sum, err := t.sendForm(ctx, o, offset, length, u.String(), server, p, &out)
		if err != nil {
			return "", err
		}
		if err := checksum(sum, out.MD5); err != nil {
			return "", err
		}
	}
	return "", nil
}

// sendForm uploads a replayable body, counting payload bytes rather than MIME framing.
func (t *Transfers) sendForm(
	ctx context.Context, o Original, offset, length int64, target string, server *FormServer, p port.Progress, out any,
) (string, error) {
	file, contentType, header, sum, err := multipartFile(o, offset, length)
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(file) }() // Best-effort scratch cleanup cannot change the transfer result.
	h := port.Header{"Content-Type": {contentType}}
	r := port.Request{Method: methodPost, URL: target, Header: h, File: file, Progress: func(done, _ int64) {
		report(p, offset+min(max(0, done-header), length), o.Size)
	}}
	err = retry.Do(ctx, t.policy, t.clock, false, func() error {
		if server != nil {
			token, e := server.Token()
			if e != nil {
				return e
			}
			r.Header.Set("x-access-token", token)
		}
		resp, e := t.transport.Upload(ctx, r)
		if e != nil {
			return e
		}
		if e = accepted(resp, target, t.clock.Now()); e != nil {
			return e
		}
		return channel.Open(resp.Body, out)
	})
	return sum, err
}

// multipartFile streams MIME framing and a source section into a private file.
// Sanitizing the filename prevents it from adding headers to the form part.
func multipartFile(o Original, offset, length int64) (file, contentType string, header int64, sum string, err error) {
	boundary := "SNCloud-" + rand.Text()
	name := strings.NewReplacer("\\", "\\\\", "\"", "'", "\r", " ", "\n", " ").Replace(o.Name)
	head := "--" + boundary + "\r\nContent-Disposition: form-data; name=\"file\"; filename=\"" + name + "\"\r\n" +
		"Content-Type: application/octet-stream\r\n\r\n"
	hash := md5.New() // #nosec G401 -- Required by the storage protocol, not used for authentication.
	file, err = scratch(func(f *os.File) error {
		if _, e := f.WriteString(head); e != nil {
			return e
		}
		if e := copySlice(io.MultiWriter(f, hash), o.File, offset, length); e != nil {
			return e
		}
		_, e := f.WriteString("\r\n--" + boundary + "--\r\n")
		return e
	})
	return file, "multipart/form-data; boundary=" + boundary, int64(len(head)), hex.EncodeToString(hash.Sum(nil)), err
}

// formTarget pins credentials to the configured origin before validating the
// destination. Private servers may advertise an unreachable internal origin.
func (s UploadSlot) formTarget(server *FormServer) (UploadSlot, error) {
	if server != nil {
		s.FullURL = moved(s.FullURL, server.Host)
		if s.PartURL != "" {
			s.PartURL = moved(s.PartURL, server.Host)
		}
	}
	if _, err := storageURL(s.FullURL); err != nil {
		return s, err
	}
	if s.PartURL != "" {
		if _, err := storageURL(s.PartURL); err != nil {
			return s, err
		}
	}
	return s, nil
}
