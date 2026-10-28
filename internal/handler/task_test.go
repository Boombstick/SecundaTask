package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
	"taskservice/internal/service"
)

type fakeTaskService struct {
	TaskService
	page     domain.TaskPage
	card     domain.TaskCard
	err      error
	gotInput service.ListTasksInput
}

func (f *fakeTaskService) List(_ context.Context, _ int64, in service.ListTasksInput) (domain.TaskPage, error) {
	f.gotInput = in
	return f.page, f.err
}

func (f *fakeTaskService) Card(_ context.Context, _, _ int64) (domain.TaskCard, error) {
	return f.card, f.err
}

func TestTaskCard_EntitiesInsteadOfIDs(t *testing.T) {
	svc := &fakeTaskService{card: domain.TaskCard{
		Task:     domain.Task{ID: 7, TeamID: 1, CreatedBy: 2, Status: domain.StatusBacklog},
		Team:     domain.Entity{ID: 1, Name: "Backend"},
		Creator:  domain.Entity{ID: 2, Name: "Anna"},
		Assignee: &domain.Entity{ID: 3, Name: "Boris"},
	}}
	r := chi.NewRouter()
	NewTaskHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes(r)

	get := func() map[string]any {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks/7", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d: %s", rec.Code, rec.Body)
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body
	}

	body := get()
	want := map[string]map[string]any{
		"team":       {"id": float64(1), "name": "Backend"},
		"created_by": {"id": float64(2), "name": "Anna"},
		"assignee":   {"id": float64(3), "name": "Boris"},
	}
	for field, exp := range want {
		got, _ := body[field].(map[string]any)
		if got["id"] != exp["id"] || got["name"] != exp["name"] {
			t.Errorf("%s = %v, want %v", field, body[field], exp)
		}
	}
	for _, field := range []string{"team_id", "assignee_id"} {
		if _, ok := body[field]; ok {
			t.Errorf("card must not contain %q", field)
		}
	}

	svc.card.Assignee = nil
	if body := get(); body["assignee"] != nil {
		t.Errorf("assignee = %v, want null", body["assignee"])
	}
}

func serveList(t *testing.T, svc *fakeTaskService, query string) *httptest.ResponseRecorder {
	t.Helper()
	r := chi.NewRouter()
	NewTaskHandler(svc, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes(r)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tasks?"+query, nil))
	return rec
}

func TestTaskList_ShortItems(t *testing.T) {
	assignee := int64(2)
	svc := &fakeTaskService{
		page: domain.TaskPage{
			Items: []domain.Task{{ID: 7, TeamID: 1, Title: "T", Description: "secret", Status: domain.StatusDone, CreatedBy: 1, AssigneeID: &assignee, Version: 3}},
			Limit: 20,
		},
	}

	rec := serveList(t, svc, "team_id=1&status=done&limit=20")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if svc.gotInput.TeamID != "1" || svc.gotInput.Status != "done" || svc.gotInput.Limit != "20" {
		t.Errorf("query not passed through: %+v", svc.gotInput)
	}
	var body struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 1 {
		t.Fatalf("unexpected body: %s", rec.Body)
	}
	for _, field := range []string{"description", "version", "created_at", "updated_at", "closed_at"} {
		if _, ok := body.Items[0][field]; ok {
			t.Errorf("list item must not contain %q", field)
		}
	}
}

func TestTaskList_ServiceErrorMapsToStatus(t *testing.T) {
	svc := &fakeTaskService{err: apperr.ValidationField("team_id", "team_id is required")}
	rec := serveList(t, svc, "")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Details []struct {
				Field string `json:"field"`
			} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "validation_error" || len(body.Error.Details) != 1 || body.Error.Details[0].Field != "team_id" {
		t.Errorf("unexpected error body: %s", rec.Body)
	}
}
