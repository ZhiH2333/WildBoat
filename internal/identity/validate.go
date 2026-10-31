package identity

import (
	"net/mail"
	"strings"
	"unicode/utf8"
)

const (
	minPasswordLen    = 8
	maxPasswordLen    = 128
	minDisplayNameLen = 2
	maxDisplayNameLen = 32
	maxAvatarIDLen    = 64
	maxBioLen         = 280
)

// NormalizeEmail lowercases and trims an email for uniqueness.
func NormalizeEmail(email string) (string, error) {
	email = strings.TrimSpace(strings.ToLower(email))
	if email == "" {
		return "", ErrInvalidInput
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email {
		return "", ErrInvalidInput
	}
	if !strings.Contains(email, ".") || !strings.Contains(email, "@") {
		return "", ErrInvalidInput
	}
	return email, nil
}

func validatePassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < minPasswordLen || n > maxPasswordLen {
		return ErrInvalidInput
	}
	return nil
}

func validateDisplayName(name string) error {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	if n < minDisplayNameLen || n > maxDisplayNameLen {
		return ErrInvalidInput
	}
	return nil
}

func validateAvatarID(avatarID string) error {
	avatarID = strings.TrimSpace(avatarID)
	if avatarID == "" || utf8.RuneCountInString(avatarID) > maxAvatarIDLen {
		return ErrInvalidInput
	}
	return nil
}

func validateBio(bio string) error {
	if utf8.RuneCountInString(bio) > maxBioLen {
		return ErrInvalidInput
	}
	return nil
}
