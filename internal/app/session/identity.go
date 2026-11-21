// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"crypto/rand"
	"fmt"

	"github.com/altessa-s/sncloud-go/internal/domain"
	"github.com/altessa-s/sncloud-go/internal/port"
)

// RFC 9562 UUID version 4 and variant bits preserve the remaining random bits.
const (
	uuidVersionMask = 0x0f
	uuidVersion4    = 0x40
	uuidVariantMask = 0x3f
	uuidVariantRFC  = 0x80
)

func (s *Session) identity() (domain.DeviceIdentity, error) {
	id, err := s.secrets.Secret(domain.ClientID)
	if err != nil {
		return domain.DeviceIdentity{}, err
	}
	if id == "" {
		id, err = newUUID()
		if err != nil {
			return domain.DeviceIdentity{}, err
		}
		if err := s.writeResult(s.secrets.Keep(id, domain.ClientID)); err != nil {
			return domain.DeviceIdentity{}, err
		}
	}
	return domain.NewDeviceIdentity("", id), nil
}

// Identity returns the persistent installation identity.
func (s *Session) Identity() (domain.DeviceIdentity, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.identity()
}

// AppHeaders constructs app headers with the current call's token.
func (s *Session) AppHeaders(token string, withChannel bool) (port.Header, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appHeaders(token, withChannel)
}
func (s *Session) appHeaders(token string, withChannel bool) (port.Header, error) {
	id, err := s.identity()
	if err != nil {
		return nil, err
	}
	h := port.Header{
		"User-Agent":  {domain.AppUserAgent},
		"version":     {domain.ProtocolVersion},
		"equipmentNo": {domain.EquipmentHeader(id)},
	}
	if withChannel && token != "" {
		h.Set("channel", domain.Channel(id, token, s.started))
	}
	return h, nil
}

// SocketCredentials shares the same identity and process marker as app headers.
func (s *Session) SocketCredentials(expected *domain.CredentialPin) (token, kind, random string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, err = s.accessToken(expected)
	if err != nil {
		return
	}
	id, e := s.identity()
	err = e
	return token, domain.SocketType(id), s.started, err
}

// newUUID uses random version 4 identity, independent of account or device data.
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&uuidVersionMask | uuidVersion4
	b[8] = b[8]&uuidVariantMask | uuidVariantRFC
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
