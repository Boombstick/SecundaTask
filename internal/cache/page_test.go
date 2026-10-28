package cache

import (
	"reflect"
	"testing"
	"time"

	"taskservice/internal/domain"
)

func TestPageRoundTrip(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	assignee, offset, total, cursor := int64(5), 0, int64(2), "aWQ6MQ"
	in := domain.TaskPage{
		Items: []domain.Task{
			{ID: 2, TeamID: 1, Title: "B", Status: domain.StatusDone, CreatedBy: 1, AssigneeID: &assignee,
				CreatedAt: now, UpdatedAt: now, ClosedAt: &now, Version: 3},
			{ID: 1, TeamID: 1, Title: "A", Description: "d", Status: domain.StatusBacklog, CreatedBy: 1,
				CreatedAt: now, UpdatedAt: now, Version: 1},
		},
		Limit: 2, Offset: &offset, Total: &total, NextCursor: &cursor,
	}
	raw, err := encodePage(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := decodePage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip mismatch:\n in  %+v\n out %+v", in, out)
	}
}
