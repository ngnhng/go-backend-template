// Copyright 2025 Nguyen Nhat Nguyen
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package hmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// TODO: URL-safe base64 encoding option since we may pass the output onto URLs

type HMACConfig struct {
	Secret string `env:"HMAC_SECRET,notEmpty"`
}

type HMACSigner struct {
	key []byte
}

var (
	ErrMissingKey   = errors.New("missing hmac key")
	ErrInvalidToken = errors.New("invalid token")
)

func newHMACSigner(key []byte) (*HMACSigner, error) {
	if len(key) == 0 {
		return nil, ErrMissingKey
	}
	return &HMACSigner{key: key}, nil
}

// NewHMACSigner builds a HMAC signer using the provided secret
func NewHMACSigner(secKey []byte) (*HMACSigner, error) {
	if len(secKey) == 0 {
		return nil, ErrMissingKey
	}
	return newHMACSigner(secKey)
}

func (h *HMACSigner) Sign(payload []byte) (string, error) {
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, h.key)
	_, _ = mac.Write([]byte(payloadB64))
	sig := mac.Sum(nil)
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return payloadB64 + "." + sigB64, nil
}

func (h *HMACSigner) Verify(token string) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidToken
	}
	payloadB64, sigB64 := parts[0], parts[1]
	mac := hmac.New(sha256.New, h.key)
	_, _ = mac.Write([]byte(payloadB64))
	want := mac.Sum(nil)

	got, err := base64.RawURLEncoding.DecodeString(sigB64)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if !hmac.Equal(want, got) {
		return nil, ErrInvalidToken
	}
	payload, err := base64.RawURLEncoding.DecodeString(payloadB64)
	if err != nil {
		return nil, ErrInvalidToken
	}
	return payload, nil
}
