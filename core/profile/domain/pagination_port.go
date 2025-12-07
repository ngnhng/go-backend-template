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

package domain

// CursorSigner is the outbound port for signing and verifying cursor tokens.
type CursorSigner interface {
	// Sign returns signed cursor token = base64url(payload) + "." + base64url(algo(payloadB64))
	Sign(payload []byte) (string, error)
	// Verify returns the original payload after validating signature
	Verify(token string) ([]byte, error)
}
