package validate

import (
	"strings"
	"testing"

	"taskservice/internal/platform/apperr"
)

func TestShortTextReportsAllProblems(t *testing.T) {
	err := ShortText("title", strings.Repeat("я", 257)+"\n")
	e, ok := apperr.As(err)
	if !ok || len(e.Details) != 2 {
		t.Errorf("title: want length and single-line problems, got %v", err)
	}
}

func TestPage(t *testing.T) {
	var v apperr.ValidationErrors
	limit, offset := Page(&v, PageQuery{})
	if !v.Empty() || limit != DefaultPageLimit || offset != 0 {
		t.Errorf("defaults: limit=%d offset=%d err=%v", limit, offset, v.Err())
	}
	v = apperr.ValidationErrors{}
	Page(&v, PageQuery{Limit: "0", Offset: "-1"})
	if !v.HasField("limit") || !v.HasField("offset") {
		t.Errorf("want both limit and offset problems, got %v", v.Err())
	}
}

func TestOptionalID(t *testing.T) {
	var v apperr.ValidationErrors
	if id := OptionalID(&v, "id", ""); id != nil || !v.Empty() {
		t.Error("empty id must be nil without problems")
	}
	if id := OptionalID(&v, "id", "7"); id == nil || *id != 7 {
		t.Error("valid id must parse")
	}
	OptionalID(&v, "id", "-1")
	if !v.HasField("id") {
		t.Error("negative id must be rejected")
	}
}

func TestTextLimits(t *testing.T) {
	if err := ShortText("title", strings.Repeat("я", 256)); err != nil {
		t.Errorf("256 chars title: %v", err)
	}
	if err := ShortText("title", strings.Repeat("я", 257)); err == nil {
		t.Error("257 chars title must be rejected")
	}
	if err := ShortText("title", ""); err == nil {
		t.Error("empty title must be rejected")
	}
	if err := ShortText("title", "line\nbreak"); err == nil {
		t.Error("single-line field must reject line breaks")
	}
	if err := LongText("description", strings.Repeat("я", 512), 0); err != nil {
		t.Errorf("512 chars description: %v", err)
	}
	if err := LongText("description", strings.Repeat("я", 513), 0); err == nil {
		t.Error("513 chars description must be rejected")
	}
	if err := LongText("description", "", 0); err != nil {
		t.Errorf("empty description must be allowed: %v", err)
	}
	if err := LongText("content", "", 1); err == nil {
		t.Error("empty comment must be rejected")
	}
	if err := LongText("description", "multi\nline\ttext", 0); err != nil {
		t.Errorf("multi-line text: %v", err)
	}
	if err := LongText("description", "bell\x07", 0); err == nil {
		t.Error("control characters must be rejected")
	}
}
