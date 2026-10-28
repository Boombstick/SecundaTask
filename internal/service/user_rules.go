package service

import (
	"fmt"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"taskservice/internal/platform/apperr"
	"taskservice/internal/platform/validate"
)

const (
	MinPasswordLen   = 8
	MaxPasswordBytes = 72
)

func ValidatePersonName(name string) error {
	var v apperr.ValidationErrors
	validate.Len(&v, "name", name, 2, validate.MaxShortText)
	if name == "" {
		return v.Err()
	}
	if first, _ := utf8.DecodeRuneInString(name); !unicode.IsLetter(first) {
		v.Add("name", "name must start with a letter")
	}
	for _, r := range name {
		if !(unicode.IsLetter(r) || unicode.Is(unicode.Mn, r) || r == ' ' || r == '-' || r == '\'' || r == '.') {
			v.Add("name", "name may contain only letters, spaces, hyphens, apostrophes and dots")
			break
		}
	}
	if strings.Contains(name, "  ") {
		v.Add("name", "name must not contain consecutive spaces")
	}
	return v.Err()
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func ValidateEmail(email string) error {
	var v apperr.ValidationErrors
	if email == "" {
		v.Add("email", "email is required")
		return v.Err()
	}
	if utf8.RuneCountInString(email) > validate.MaxShortText {
		v.Add("email", fmt.Sprintf("email must be at most %d characters long", validate.MaxShortText))
	}
	if !emailFormatOK(email) {
		v.Add("email", "email is invalid")
	}
	return v.Err()
}

func emailFormatOK(email string) bool {
	a, err := mail.ParseAddress(email)
	if err != nil || a.Address != email || strings.ContainsAny(email, " \t") {
		return false
	}
	local, domainPart, ok := strings.Cut(email, "@")
	if !ok || len(local) > 64 {
		return false
	}

	dot := strings.LastIndexByte(domainPart, '.')
	if dot <= 0 || len(domainPart)-dot-1 < 2 {
		return false
	}
	for _, r := range domainPart[dot+1:] {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

func ValidatePassword(p string) error {
	var v apperr.ValidationErrors
	if utf8.RuneCountInString(p) < MinPasswordLen {
		v.Add("password", fmt.Sprintf("password must be at least %d characters long", MinPasswordLen))
	}
	if len(p) > MaxPasswordBytes {
		v.Add("password", fmt.Sprintf("password must be at most %d bytes long", MaxPasswordBytes))
	}
	var lower, upper, digit, space bool
	for _, r := range p {
		switch {
		case unicode.IsSpace(r) || unicode.IsControl(r):
			space = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsDigit(r):
			digit = true
		}
	}
	if space {
		v.Add("password", "password must not contain spaces or control characters")
	}
	if !lower {
		v.Add("password", "password must contain a lowercase letter")
	}
	if !upper {
		v.Add("password", "password must contain an uppercase letter")
	}
	if !digit {
		v.Add("password", "password must contain a digit")
	}
	return v.Err()
}
