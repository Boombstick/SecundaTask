package apperr

import (
	"errors"
	"fmt"
	"strings"
)

type Kind int

const (
	KindValidation Kind = iota + 1
	KindUnauthorized
	KindForbidden
	KindNotFound
	KindConflict
)

type Error struct {
	Kind    Kind
	Code    string
	Message string
	Details []FieldError
}

type FieldError struct {
	Field   string
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

func newErr(kind Kind, code, msg string) *Error {
	return &Error{Kind: kind, Code: code, Message: msg}
}

func Validation(msg string) *Error { return newErr(KindValidation, "validation_error", msg) }

func ValidationField(field, msg string) *Error {
	e := Validation(msg)
	e.Details = []FieldError{{Field: field, Message: msg}}
	return e
}

func Unauthorized(msg string) *Error   { return newErr(KindUnauthorized, "unauthorized", msg) }
func Forbidden(msg string) *Error      { return newErr(KindForbidden, "forbidden", msg) }
func NotFound(code, msg string) *Error { return newErr(KindNotFound, code, msg) }
func Conflict(code, msg string) *Error { return newErr(KindConflict, code, msg) }

func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

func IsKind(err error, kind Kind) bool {
	e, ok := As(err)
	return ok && e.Kind == kind
}

type ValidationErrors struct {
	items []FieldError
	other error
}

func (v *ValidationErrors) Add(field, msg string) {
	v.items = append(v.items, FieldError{Field: field, Message: msg})
}

func (v *ValidationErrors) Check(err error) bool {
	if err == nil {
		return true
	}
	e, ok := As(err)
	if !ok || e.Kind != KindValidation {
		if v.other == nil {
			v.other = err
		}
		return false
	}
	if len(e.Details) == 0 {
		v.Add("", e.Message)
	} else {
		v.items = append(v.items, e.Details...)
	}
	return false
}

func (v *ValidationErrors) HasField(field string) bool {
	for _, it := range v.items {
		if it.Field == field {
			return true
		}
	}
	return false
}

func (v *ValidationErrors) Empty() bool { return len(v.items) == 0 && v.other == nil }

func (v *ValidationErrors) Err() error {
	if v.other != nil {
		return v.other
	}
	if len(v.items) == 0 {
		return nil
	}
	msgs := make([]string, len(v.items))
	for i, it := range v.items {
		msgs[i] = it.Message
	}
	e := Validation(strings.Join(msgs, "; "))
	e.Details = append([]FieldError(nil), v.items...)
	return e
}
