package apperr

import (
	"errors"
	"reflect"
	"testing"
)

var errTeamNotFound = NotFound("team_not_found", "team not found")

func TestValidationErrors_Empty(t *testing.T) {
	var v ValidationErrors
	v.Check(nil)
	if !v.Empty() || v.Err() != nil {
		t.Fatalf("no problems recorded, want nil error, got %v", v.Err())
	}
}

func TestValidationErrors_CollectsEverything(t *testing.T) {
	var v ValidationErrors
	v.Add("title", "title is required")
	ok := v.Check(ValidationField("status", "status is invalid"))
	multi := &ValidationErrors{}
	multi.Add("password", "too short")
	multi.Add("password", "needs a digit")
	v.Check(multi.Err())
	v.Check(nil)

	if ok {
		t.Error("Check must return false for an error")
	}
	e, isErr := As(v.Err())
	if !isErr || e.Kind != KindValidation || e.Code != "validation_error" {
		t.Fatalf("want validation error, got %v", v.Err())
	}
	want := []FieldError{
		{"title", "title is required"},
		{"status", "status is invalid"},
		{"password", "too short"},
		{"password", "needs a digit"},
	}
	if !reflect.DeepEqual(e.Details, want) {
		t.Errorf("details = %v, want %v", e.Details, want)
	}
	if e.Message != "title is required; status is invalid; too short; needs a digit" {
		t.Errorf("message = %q", e.Message)
	}
	if !v.HasField("password") || v.HasField("email") {
		t.Error("HasField is wrong")
	}
}

func TestValidationErrors_NonValidationErrorWins(t *testing.T) {
	var v ValidationErrors
	v.Add("title", "title is required")
	dbErr := errors.New("connection refused")
	v.Check(dbErr)
	v.Check(errTeamNotFound)
	if !errors.Is(v.Err(), dbErr) {
		t.Fatalf("want the infrastructure error, got %v", v.Err())
	}
}
