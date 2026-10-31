package security

import (
	"bytes"
	"testing"
)

func TestNewOpaqueToken(t *testing.T) {
	t.Parallel()

	raw, hash, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken: %v", err)
	}
	if raw == "" {
		t.Fatal("expected non-empty raw token")
	}
	if len(hash) != 32 {
		t.Fatalf("hash len = %d, want 32", len(hash))
	}
	if !bytes.Equal(HashOpaqueToken(raw), hash) {
		t.Fatal("HashOpaqueToken mismatch")
	}

	raw2, hash2, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("NewOpaqueToken second: %v", err)
	}
	if raw == raw2 {
		t.Fatal("tokens must be unique")
	}
	if bytes.Equal(hash, hash2) {
		t.Fatal("hashes must differ")
	}
}
