package validate

import (
	"fmt"
	"strconv"
	"unicode"
	"unicode/utf8"

	"taskservice/internal/platform/apperr"
)

const (
	DefaultPageLimit = 20
	MaxPageLimit     = 100

	MaxShortText = 256
	MaxLongText  = 512
)

func Len(v *apperr.ValidationErrors, field, s string, min, max int) {
	n := utf8.RuneCountInString(s)
	switch {
	case n == 0 && min > 0:
		v.Add(field, field+" is required")
	case n < min:
		v.Add(field, fmt.Sprintf("%s must be at least %d characters long", field, min))
	case n > max:
		v.Add(field, fmt.Sprintf("%s must be at most %d characters long", field, max))
	}
}

func checkText(v *apperr.ValidationErrors, field, s string, multiline bool) {
	if !utf8.ValidString(s) {
		v.Add(field, field+" contains invalid characters")
		return
	}
	for _, r := range s {
		if multiline && (r == '\n' || r == '\r' || r == '\t') {
			continue
		}
		if unicode.IsControl(r) {
			if multiline {
				v.Add(field, field+" contains control characters")
			} else {
				v.Add(field, field+" must be a single line without control characters")
			}
			return
		}
	}
}

func ShortText(field, s string) error {
	var v apperr.ValidationErrors
	Len(&v, field, s, 1, MaxShortText)
	checkText(&v, field, s, false)
	return v.Err()
}

func LongText(field, s string, min int) error {
	var v apperr.ValidationErrors
	Len(&v, field, s, min, MaxLongText)
	checkText(&v, field, s, true)
	return v.Err()
}

type PageQuery struct {
	Limit  string
	Offset string
}

func Page(v *apperr.ValidationErrors, q PageQuery) (limit, offset int) {
	limit = DefaultPageLimit
	if q.Limit != "" {
		n, err := strconv.Atoi(q.Limit)
		switch {
		case err != nil:
			v.Add("limit", "limit must be an integer")
		case n < 1 || n > MaxPageLimit:
			v.Add("limit", fmt.Sprintf("limit must be between 1 and %d", MaxPageLimit))
		default:
			limit = n
		}
	}
	if q.Offset != "" {
		n, err := strconv.Atoi(q.Offset)
		switch {
		case err != nil:
			v.Add("offset", "offset must be an integer")
		case n < 0:
			v.Add("offset", "offset must not be negative")
		default:
			offset = n
		}
	}
	return limit, offset
}

func OptionalID(v *apperr.ValidationErrors, field, raw string) *int64 {
	if raw == "" {
		return nil
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		v.Add(field, field+" must be a positive integer")
		return nil
	}
	return &n
}
