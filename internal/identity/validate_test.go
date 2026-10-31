package identity

import "testing"

func TestNormalizeEmail(t *testing.T) {
	t.Parallel()

	got, err := NormalizeEmail("  Player@Example.COM ")
	if err != nil {
		t.Fatalf("NormalizeEmail: %v", err)
	}
	if got != "player@example.com" {
		t.Fatalf("got %q", got)
	}

	if _, err := NormalizeEmail("not-an-email"); err != ErrInvalidInput {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()
	if err := validatePassword("short"); err != ErrInvalidInput {
		t.Fatalf("short password err = %v", err)
	}
	if err := validatePassword("long-enough-password"); err != nil {
		t.Fatalf("valid password err = %v", err)
	}
}
