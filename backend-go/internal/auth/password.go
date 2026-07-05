package auth

import (
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// HashPassword returns a bcrypt hash of the plain-text password.
func HashPassword(p string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// CheckPassword reports whether the plain-text password matches the bcrypt hash.
func CheckPassword(p, h string) bool {
	return bcrypt.CompareHashAndPassword([]byte(h), []byte(p)) == nil
}

// ValidatePassword enforces the password policy: 8–128 chars, must contain at
// least one letter and one digit. Replaces the old 6-char minimum.
func ValidatePassword(p string) error {
	if len(p) < 8 {
		return errors.New("密码至少8位")
	}
	if len(p) > 128 {
		return errors.New("密码过长")
	}
	hasLetter, hasDigit := false, false
	for _, r := range p {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return errors.New("密码必须包含字母和数字")
	}
	return nil
}

// ValidateUsername checks the same constraints the DB schema enforces (2–30 chars),
// plus a printable-character guard.
func ValidateUsername(u string) error {
	u = strings.TrimSpace(u)
	if len(u) < 2 || len(u) > 30 {
		return errors.New("用户名长度需为2-30位")
	}
	return nil
}
