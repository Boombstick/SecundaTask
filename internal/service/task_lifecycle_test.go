package service

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"testing"
	"time"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
)

const (
	lcTeam     int64 = 10
	lcOwner    int64 = 1
	lcAdmin    int64 = 2
	lcCreator  int64 = 3
	lcAssignee int64 = 4
	lcOutsider int64 = 9
)

type memTaskStore struct {
	tasks  map[int64]domain.Task
	nextID int64
	lists  int
}

func (s *memTaskStore) Create(_ context.Context, t *domain.Task) error {
	s.nextID++
	now := time.Now().UTC()
	t.ID, t.Version, t.CreatedAt, t.UpdatedAt = s.nextID, 1, now, now
	s.tasks[t.ID] = *t
	return nil
}

func (s *memTaskStore) Get(_ context.Context, id int64) (domain.Task, error) {
	t, ok := s.tasks[id]
	if !ok {
		return domain.Task{}, domain.ErrTaskNotFound
	}
	return t, nil
}

func (s *memTaskStore) GetCard(ctx context.Context, id int64) (domain.TaskCard, error) {
	t, err := s.Get(ctx, id)
	return domain.TaskCard{Task: t}, err
}

func (s *memTaskStore) Update(_ context.Context, t *domain.Task, expectedVersion int) error {
	cur, ok := s.tasks[t.ID]
	if !ok || cur.Version != expectedVersion {
		return domain.ErrVersionConflict
	}
	t.Version, t.UpdatedAt = expectedVersion+1, time.Now().UTC()
	s.tasks[t.ID] = *t
	return nil
}

func (s *memTaskStore) Delete(_ context.Context, id int64, expectedVersion *int) error {
	cur, ok := s.tasks[id]
	switch {
	case ok && expectedVersion != nil && cur.Version != *expectedVersion:
		return domain.ErrVersionConflict
	case !ok:
		return domain.ErrTaskNotFound
	}
	delete(s.tasks, id)
	return nil
}

func (s *memTaskStore) List(_ context.Context, f domain.TaskFilter) ([]domain.Task, *int64, error) {
	s.lists++
	var out []domain.Task
	for _, t := range s.tasks {
		if t.TeamID == f.TeamID && (f.Status == nil || t.Status == *f.Status) &&
			(f.AssigneeID == nil || domain.SameID(t.AssigneeID, f.AssigneeID)) {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b domain.Task) int { return int(b.ID - a.ID) })
	total := int64(len(out))
	return out, &total, nil
}

func (s *memTaskStore) UnassignUser(context.Context, int64, int64) ([]int64, error) { return nil, nil }

type memHistory struct {
	entries []domain.HistoryEntry
	fail    error
}

func (h *memHistory) Add(ctx context.Context, taskID, changedBy int64, changes domain.Changes) error {
	return h.AddBatch(ctx, []int64{taskID}, changedBy, changes)
}

func (h *memHistory) AddBatch(_ context.Context, taskIDs []int64, changedBy int64, changes domain.Changes) error {
	if h.fail != nil {
		return h.fail
	}
	for _, id := range taskIDs {
		h.entries = append(h.entries, domain.HistoryEntry{TaskID: id, ChangedBy: changedBy, Changes: changes})
	}
	return nil
}

func (h *memHistory) List(context.Context, int64, int, int) ([]domain.HistoryEntry, error) {
	return h.entries, nil
}

type memMembers map[[2]int64]domain.Role

func (m memMembers) GetMember(_ context.Context, teamID, userID int64) (domain.Member, error) {
	role, ok := m[[2]int64{teamID, userID}]
	if !ok {
		return domain.Member{}, domain.ErrMemberNotFound
	}
	return domain.Member{TeamID: teamID, UserID: userID, Role: role}, nil
}

func (m memMembers) LockMember(_ context.Context, teamID, userID int64) (bool, error) {
	_, ok := m[[2]int64{teamID, userID}]
	return ok, nil
}

type memCache struct {
	pages       map[string]domain.TaskPage
	invalidated map[int64]int
}

func memCacheKey(teamID int64, f domain.TaskFilter) string {
	status, assignee := "-", "-"
	if f.Status != nil {
		status = string(*f.Status)
	}
	if f.AssigneeID != nil {
		assignee = fmt.Sprint(*f.AssigneeID)
	}
	return fmt.Sprintf("%d|%s|%s|%d|%d", teamID, status, assignee, f.Limit, f.Offset)
}

func (c *memCache) Get(_ context.Context, teamID int64, f domain.TaskFilter) (domain.TaskPage, bool) {
	p, ok := c.pages[memCacheKey(teamID, f)]
	return p, ok
}

func (c *memCache) Set(_ context.Context, teamID int64, f domain.TaskFilter, p domain.TaskPage) {
	c.pages[memCacheKey(teamID, f)] = p
}

func (c *memCache) InvalidateTeam(_ context.Context, teamID int64) {
	c.invalidated[teamID]++
	prefix := fmt.Sprintf("%d|", teamID)
	for k := range c.pages {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.pages, k)
		}
	}
}

type memTx struct {
	store *memTaskStore
	hist  *memHistory
}

func (tx memTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	tasks, nextID, entries := maps.Clone(tx.store.tasks), tx.store.nextID, len(tx.hist.entries)
	if err := fn(ctx); err != nil {
		tx.store.tasks, tx.store.nextID, tx.hist.entries = tasks, nextID, tx.hist.entries[:entries]
		return err
	}
	return nil
}

type taskEnv struct {
	svc   *TaskService
	store *memTaskStore
	hist  *memHistory
	cache *memCache
}

func newTaskEnv() taskEnv {
	store := &memTaskStore{tasks: map[int64]domain.Task{}}
	hist := &memHistory{}
	cache := &memCache{pages: map[string]domain.TaskPage{}, invalidated: map[int64]int{}}
	members := memMembers{
		{lcTeam, lcOwner}:    domain.RoleOwner,
		{lcTeam, lcAdmin}:    domain.RoleAdmin,
		{lcTeam, lcCreator}:  domain.RoleMember,
		{lcTeam, lcAssignee}: domain.RoleMember,
	}
	svc := NewTaskService(store, hist, NewMembership(members), cache, memTx{store: store, hist: hist})
	return taskEnv{svc: svc, store: store, hist: hist, cache: cache}
}

func (e taskEnv) update(userID, taskID int64, title, description string, status domain.TaskStatus, assignee *int64, version int) (domain.TaskCard, error) {
	return e.svc.Update(context.Background(), userID, taskID, UpdateTaskInput{
		Title: &title, Description: description, Status: &status, AssigneeID: assignee, Version: &version,
	})
}

func requireKind(t *testing.T, err error, want apperr.Kind) {
	t.Helper()
	if got := kindOf(err); got != want {
		t.Fatalf("error %v: kind %d, want %d", err, got, want)
	}
}

func TestTaskLifecycle(t *testing.T) {
	e := newTaskEnv()
	ctx := context.Background()

	card, err := e.svc.Create(ctx, lcCreator, CreateTaskInput{
		TeamID: lcTeam, Title: "  Write tests  ", Description: "cover the cycle", AssigneeID: ptr(lcAssignee),
	})
	if err != nil {
		t.Fatal(err)
	}
	id := card.ID
	if card.Status != domain.StatusBacklog || card.Version != 1 || card.Title != "Write tests" ||
		card.CreatedBy != lcCreator || card.ClosedAt != nil {
		t.Fatalf("created task: %+v", card.Task)
	}
	if len(e.hist.entries) != 1 || len(e.hist.entries[0].Changes) != 4 {
		t.Fatalf("creation must be one history entry with 4 fields: %+v", e.hist.entries)
	}
	if e.cache.invalidated[lcTeam] != 1 {
		t.Fatalf("creation must invalidate the team cache once, got %d", e.cache.invalidated[lcTeam])
	}

	card, err = e.update(lcAssignee, id, "Write tests", "cover the cycle", domain.StatusInProgress, ptr(lcAssignee), 1)
	if err != nil {
		t.Fatalf("assignee changes status: %v", err)
	}
	if card.Version != 2 || card.Status != domain.StatusInProgress {
		t.Fatalf("after status change: %+v", card.Task)
	}
	last := e.hist.entries[len(e.hist.entries)-1]
	if last.ChangedBy != lcAssignee || len(last.Changes) != 1 ||
		last.Changes["status"] != (domain.FieldChange{Old: domain.StatusBacklog, New: domain.StatusInProgress}) {
		t.Fatalf("history must hold only the status change by the assignee: %+v", last)
	}

	_, err = e.update(lcAssignee, id, "Renamed", "cover the cycle", domain.StatusInProgress, ptr(lcAssignee), 2)
	requireKind(t, err, apperr.KindForbidden)
	_, err = e.update(lcAssignee, id, "Write tests", "cover the cycle", domain.StatusInProgress, ptr(lcAdmin), 2)
	requireKind(t, err, apperr.KindForbidden)

	_, err = e.update(lcCreator, id, "Write tests", "cover the cycle", domain.StatusNeedInfo, ptr(lcAssignee), 1)
	requireKind(t, err, apperr.KindConflict)

	card, err = e.update(lcAssignee, id, "Write tests", "cover the cycle", domain.StatusDone, ptr(lcAssignee), 2)
	if err != nil {
		t.Fatal(err)
	}
	if card.ClosedAt == nil || card.Version != 3 {
		t.Fatalf("done must set closed_at: %+v", card.Task)
	}
	if _, ok := e.hist.entries[len(e.hist.entries)-1].Changes["closed_at"]; !ok {
		t.Fatal("closing must be recorded in history")
	}

	card, err = e.update(lcCreator, id, "Write more tests", "", domain.StatusInProgress, ptr(lcAdmin), 3)
	if err != nil {
		t.Fatalf("creator reopens and reassigns: %v", err)
	}
	if card.ClosedAt != nil || card.Version != 4 || !domain.SameID(card.AssigneeID, ptr(lcAdmin)) {
		t.Fatalf("after reopen: %+v", card.Task)
	}

	entries, invalidations := len(e.hist.entries), e.cache.invalidated[lcTeam]
	card, err = e.update(lcCreator, id, "Write more tests", "", domain.StatusInProgress, ptr(lcAdmin), 4)
	if err != nil {
		t.Fatal(err)
	}
	if card.Version != 4 || len(e.hist.entries) != entries || e.cache.invalidated[lcTeam] != invalidations {
		t.Fatal("PUT without changes must not bump the version, write history or invalidate the cache")
	}

	_, err = e.update(lcCreator, id, "Write more tests", "", domain.StatusInProgress, ptr(lcOutsider), 4)
	requireKind(t, err, apperr.KindValidation)

	_, err = e.svc.Card(ctx, lcOutsider, id)
	requireKind(t, err, apperr.KindNotFound)
	_, err = e.update(lcOutsider, id, "x", "", domain.StatusDone, nil, 4)
	requireKind(t, err, apperr.KindNotFound)

	requireKind(t, e.svc.Delete(ctx, lcAssignee, id, nil), apperr.KindForbidden)
	stale := 3
	requireKind(t, e.svc.Delete(ctx, lcCreator, id, &stale), apperr.KindConflict)
	current := 4
	if err := e.svc.Delete(ctx, lcCreator, id, &current); err != nil {
		t.Fatal(err)
	}
	_, err = e.svc.Card(ctx, lcCreator, id)
	requireKind(t, err, apperr.KindNotFound)
}

func TestTaskCreate_AccessAndAssignee(t *testing.T) {
	e := newTaskEnv()
	ctx := context.Background()

	_, err := e.svc.Create(ctx, lcOutsider, CreateTaskInput{TeamID: lcTeam, Title: "t"})
	requireKind(t, err, apperr.KindNotFound)

	_, err = e.svc.Create(ctx, lcCreator, CreateTaskInput{TeamID: lcTeam, Title: "", AssigneeID: ptr(lcOutsider)})
	e2, ok := apperr.As(err)
	if !ok || e2.Kind != apperr.KindValidation {
		t.Fatalf("want validation error, got %v", err)
	}
	fields := map[string]bool{}
	for _, d := range e2.Details {
		fields[d.Field] = true
	}
	if !fields["title"] || !fields["assignee_id"] {
		t.Errorf("all problems must be reported together, got %v", fields)
	}
	if len(e.store.tasks) != 0 || e.cache.invalidated[lcTeam] != 0 {
		t.Error("rejected create must not store anything or touch the cache")
	}
}

func TestTaskWrites_AtomicWithHistory(t *testing.T) {
	e := newTaskEnv()
	ctx := context.Background()
	boom := errors.New("history insert failed")

	e.hist.fail = boom
	if _, err := e.svc.Create(ctx, lcCreator, CreateTaskInput{TeamID: lcTeam, Title: "t"}); !errors.Is(err, boom) {
		t.Fatalf("create: %v", err)
	}
	if len(e.store.tasks) != 0 || e.cache.invalidated[lcTeam] != 0 {
		t.Fatal("task must be rolled back together with its history")
	}

	e.hist.fail = nil
	card, err := e.svc.Create(ctx, lcCreator, CreateTaskInput{TeamID: lcTeam, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	e.hist.fail = boom
	if _, err := e.update(lcCreator, card.ID, "t", "", domain.StatusDone, nil, 1); !errors.Is(err, boom) {
		t.Fatalf("update: %v", err)
	}
	if got := e.store.tasks[card.ID]; got.Version != 1 || got.Status != domain.StatusBacklog || got.ClosedAt != nil {
		t.Fatalf("update must be rolled back together with its history: %+v", got)
	}
}

func TestTaskList_CacheFilledAndInvalidatedByWrites(t *testing.T) {
	e := newTaskEnv()
	ctx := context.Background()
	in := ListTasksInput{TeamID: fmt.Sprint(lcTeam)}

	card, err := e.svc.Create(ctx, lcCreator, CreateTaskInput{TeamID: lcTeam, Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		p, err := e.svc.List(ctx, lcAssignee, in)
		if err != nil || len(p.Items) != 1 {
			t.Fatalf("list: %v %+v", err, p)
		}
	}
	if e.store.lists != 1 {
		t.Fatalf("second list must be served from cache, store hit %d times", e.store.lists)
	}

	if _, err := e.update(lcCreator, card.ID, "t", "", domain.StatusDone, nil, 1); err != nil {
		t.Fatal(err)
	}
	p, err := e.svc.List(ctx, lcAssignee, in)
	if err != nil || e.store.lists != 2 || p.Items[0].Status != domain.StatusDone {
		t.Fatalf("list after update must come from the store with fresh data: lists=%d %+v", e.store.lists, p.Items)
	}

	_, err = e.svc.List(ctx, lcOutsider, in)
	requireKind(t, err, apperr.KindNotFound)
	if e.store.lists != 2 {
		t.Error("non-member must be rejected before reading tasks")
	}
}
