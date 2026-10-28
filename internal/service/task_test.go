package service

import (
	"strings"
	"testing"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
)

func TestParseListInputReportsAllProblems(t *testing.T) {
	_, err := parseListInput(ListTasksInput{
		TeamID: "abc", Status: "nope", AssigneeID: "-1", Limit: "1000", Offset: "x", Cursor: "garbage",
	})
	e, ok := apperr.As(err)
	if !ok {
		t.Fatalf("want apperr.Error, got %v", err)
	}
	fields := map[string]bool{}
	for _, d := range e.Details {
		fields[d.Field] = true
	}
	for _, f := range []string{"team_id", "status", "assignee_id", "limit", "offset", "cursor"} {
		if !fields[f] {
			t.Errorf("missing problem for %s in %v", f, fields)
		}
	}

	f, err := parseListInput(ListTasksInput{TeamID: "7", Status: "done", Limit: "5"})
	if err != nil {
		t.Fatalf("valid input: %v", err)
	}
	if f.TeamID != 7 || *f.Status != domain.StatusDone || f.Limit != 5 || f.Offset != 0 {
		t.Errorf("unexpected filter %+v", f)
	}
}

func TestCursorRoundTrip(t *testing.T) {
	id, err := decodeCursor(encodeCursor(42))
	if err != nil || id != 42 {
		t.Fatalf("round trip: id=%d err=%v", id, err)
	}
	for _, bad := range []string{"garbage", encodeCursor(0), strings.Repeat("A", 10)} {
		if _, err := decodeCursor(bad); err == nil {
			t.Errorf("cursor %q must be rejected", bad)
		}
	}
}
