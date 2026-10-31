package room

import "testing"

func TestGenerateRoomCodeFormat(t *testing.T) {
	for i := 0; i < 100; i++ {
		code, err := GenerateRoomCode()
		if err != nil {
			t.Fatalf("GenerateRoomCode: %v", err)
		}
		if !IsValidRoomCodeFormat(code) {
			t.Fatalf("invalid code %q", code)
		}
	}
}

func TestNormalizeRoomCode(t *testing.T) {
	if got := NormalizeRoomCode("  ab2k7m "); got != "AB2K7M" {
		t.Fatalf("got %q", got)
	}
}

func TestIsValidRoomCodeFormatRejectsAmbiguous(t *testing.T) {
	for _, code := range []string{"ABCDE1", "ABCDE0", "ABCDEI", "ABCDEO", "ABCD", "ABCDEFG", "abcdef"} {
		if IsValidRoomCodeFormat(code) {
			t.Fatalf("expected invalid: %q", code)
		}
	}
	if !IsValidRoomCodeFormat("ABCDEF") {
		t.Fatal("ABCDEF should be valid")
	}
}
