// Copyright 2025 Nhat-Nguyen Nguyen
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

package serde

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gofrs/uuid/v5"
)

type ErrorResponse interface {
	Unwrap() error
	Error() string
}

func ParseJsonBody[T any](body io.ReadCloser, valuePtr *T) error {
	defer body.Close()
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	return dec.Decode(valuePtr)
}

func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func ToResponseUUIDv7(idStr string) (*[16]byte, error) {
	uid, err := uuid.FromString(idStr)
	if err != nil {
		return nil, err
	}

	return (*[16]byte)(uid.Bytes()), nil
}
