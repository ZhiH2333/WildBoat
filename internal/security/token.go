package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

const opaqueTokenBytes = 32

// NewOpaqueToken returns a URL-safe opaque token and its SHA-256 hash.
func NewOpaqueToken() (raw string, hash []byte, err error) {
	buf := make([]byte, opaqueTokenBytes)
	if _, err = rand.Read(buf); err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(buf)
	sum := sha256.Sum256([]byte(raw))
	return raw, sum[:], nil
}

// HashOpaqueToken returns the SHA-256 digest of a raw opaque token.
func HashOpaqueToken(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}
