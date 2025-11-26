package domain

// CursorSigner is the outbound port for signing and verifying cursor tokens.
type CursorSigner interface {
	// Sign returns signed cursor token = base64url(payload) + "." + base64url(algo(payloadB64))
	Sign(payload []byte) (string, error)
	// Verify returns the original payload after validating signature
	Verify(token string) ([]byte, error)
}
