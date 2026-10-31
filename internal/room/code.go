package room

import (
	"crypto/rand"
	"fmt"
	"strings"
)

// Room code alphabet: A-Z excluding I/O, digits 2-9.
const roomCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

const roomCodeLength = 6

// GenerateRoomCode returns a cryptographically random 6-character room code.
func GenerateRoomCode() (string, error) {
	buf := make([]byte, roomCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate room code: %w", err)
	}
	out := make([]byte, roomCodeLength)
	for i := 0; i < roomCodeLength; i++ {
		out[i] = roomCodeAlphabet[int(buf[i])%len(roomCodeAlphabet)]
	}
	return string(out), nil
}

// NormalizeRoomCode trims and uppercases a client-supplied code.
func NormalizeRoomCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// IsValidRoomCodeFormat reports whether code matches the frozen alphabet/length.
func IsValidRoomCodeFormat(code string) bool {
	if len(code) != roomCodeLength {
		return false
	}
	for i := 0; i < len(code); i++ {
		c := code[i]
		if !strings.ContainsRune(roomCodeAlphabet, rune(c)) {
			return false
		}
	}
	return true
}
