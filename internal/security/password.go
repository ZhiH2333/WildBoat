package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	argonSaltLen = 16
)

// HashPassword returns a PHC-style Argon2id hash of the password.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	return encodeArgon2id(salt, hash), nil
}

// VerifyPassword reports whether password matches an Argon2id PHC hash.
func VerifyPassword(password, encodedHash string) (bool, error) {
	salt, hash, time, memory, threads, keyLen, err := decodeArgon2id(encodedHash)
	if err != nil {
		return false, err
	}
	computed := argon2.IDKey([]byte(password), salt, time, memory, threads, keyLen)
	if subtle.ConstantTimeCompare(hash, computed) == 1 {
		return true, nil
	}
	return false, nil
}

func encodeArgon2id(salt, hash []byte) string {
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argonMemory,
		argonTime,
		argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

func decodeArgon2id(encoded string) (salt, hash []byte, time, memory uint32, threads uint8, keyLen uint32, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, 0, errors.New("invalid argon2id hash encoding")
	}
	var version int
	if _, err = fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return nil, nil, 0, 0, 0, 0, fmt.Errorf("parse argon2 version: %w", err)
	}
	if version != argon2.Version {
		return nil, nil, 0, 0, 0, 0, fmt.Errorf("unsupported argon2 version %d", version)
	}
	if _, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return nil, nil, 0, 0, 0, 0, fmt.Errorf("parse argon2 params: %w", err)
	}
	salt, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return nil, nil, 0, 0, 0, 0, fmt.Errorf("decode salt: %w", err)
	}
	hash, err = base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return nil, nil, 0, 0, 0, 0, fmt.Errorf("decode hash: %w", err)
	}
	return salt, hash, time, memory, threads, uint32(len(hash)), nil
}
