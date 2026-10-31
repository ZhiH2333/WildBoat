package security

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()

	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if hash == "" || hash == "correct-horse-battery" {
		t.Fatal("hash must not be empty or plaintext")
	}

	ok, err := VerifyPassword("correct-horse-battery", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Fatal("expected password to verify")
	}

	ok, err = VerifyPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword wrong: %v", err)
	}
	if ok {
		t.Fatal("expected wrong password to fail")
	}
}

func TestVerifyPasswordRejectsInvalidEncoding(t *testing.T) {
	t.Parallel()

	_, err := VerifyPassword("x", "not-a-hash")
	if err == nil {
		t.Fatal("expected error for invalid hash")
	}
}
