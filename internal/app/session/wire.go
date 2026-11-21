// Copyright 2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package session

import (
	"github.com/altessa-s/sncloud-go/internal/domain"
)

// Official app wire discriminators; browser logins use a different equipment code.
const appEquipment = 2

// Login methods distinguish phone numbers from email addresses.
const (
	phoneLogin = 1
	emailLogin = 2
)

// The app distinguishes explicit null from an omitted field. These pointers
// deliberately have no omitempty tag, including the unused browser identity.
type challengeRequest struct {
	Account     string `json:"account"`
	CountryCode *int   `json:"countryCode"`
}

type loginRequest struct {
	Account     string `json:"account"`
	Password    string `json:"password"`
	CountryCode *int   `json:"countryCode"`
	// Timestamp echoes both the challenge's value and its JSON scalar type.
	Timestamp domain.WireEcho `json:"timestamp"`
	Equipment int             `json:"equipment"`
	// LoginMethod must be numeric: 1 for phone, 2 for email.
	LoginMethod int     `json:"loginMethod"`
	EquipmentNo *string `json:"equipmentNo"`
	Browser     *string `json:"browser"`
	Language    string  `json:"language"`
	Devices     string  `json:"devices"`
}

type mailCodeRequest struct {
	Email    string `json:"email"`
	Language string `json:"language"`
}

type checkCodeRequest struct {
	Code string `json:"validCode"`
	Key  string `json:"validCodeKey"`
}

type codeLoginRequest struct {
	// CountryCode and Telephone remain explicit nulls for emailed-code login.
	CountryCode *int    `json:"countryCode"`
	Telephone   *string `json:"telephone"`
	Equipment   int     `json:"equipment"`
	Code        string  `json:"validCode"`
	Key         string  `json:"validCodeKey"`
	Devices     string  `json:"devices"`
	Email       string  `json:"email"`
}
