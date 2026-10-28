package cache

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"

	"taskservice/internal/domain"
)

func newTestCache(t *testing.T) (*TaskList, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	c := NewTaskList(rdb, Options{
		TTL:                     5 * time.Minute,
		InvalidateTimeout:       time.Second,
		InvalidateRetryInterval: 10 * time.Millisecond,
		BreakerFailures:         3,
		BreakerOpenTimeout:      time.Hour,
		BreakerHalfOpenRequests: 1,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return c, mr
}

func page(ids ...int64) domain.TaskPage {
	p := domain.TaskPage{Limit: 20}
	for _, id := range ids {
		p.Items = append(p.Items, domain.Task{ID: id, TeamID: 1, Title: "t", Status: domain.StatusBacklog, Version: 1})
	}
	return p
}

func filter(teamID int64, status domain.TaskStatus) domain.TaskFilter {
	f := domain.TaskFilter{TeamID: teamID, Limit: 20}
	if status != "" {
		f.Status = &status
	}
	return f
}

func TestTaskList_SetGetByTeamAndFilter(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	all, done := filter(1, ""), filter(1, domain.StatusDone)
	if _, hit := c.Get(ctx, 1, all); hit {
		t.Fatal("empty cache must miss")
	}
	c.Set(ctx, 1, all, page(1, 2))
	c.Set(ctx, 1, done, page(2))

	got, hit := c.Get(ctx, 1, all)
	if !hit || len(got.Items) != 2 {
		t.Fatalf("all: hit=%v items=%d", hit, len(got.Items))
	}
	got, hit = c.Get(ctx, 1, done)
	if !hit || len(got.Items) != 1 || got.Items[0].ID != 2 {
		t.Fatalf("done filter must have its own entry: hit=%v %+v", hit, got.Items)
	}
	if _, hit := c.Get(ctx, 2, filter(2, "")); hit {
		t.Fatal("another team must not see team 1 pages")
	}

	if ttl := mr.TTL(listKey(1, all)); ttl <= 0 || ttl > 5*time.Minute {
		t.Errorf("page TTL = %v, want (0, 5m]", ttl)
	}
	if keys, _ := mr.SMembers(indexKey(1)); len(keys) != 2 {
		t.Errorf("team index has %d keys, want 2", len(keys))
	}
}

func TestTaskList_InvalidateTeamDropsOnlyThatTeam(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()

	c.Set(ctx, 1, filter(1, ""), page(1))
	c.Set(ctx, 1, filter(1, domain.StatusDone), page(1))
	c.Set(ctx, 2, filter(2, ""), page(9))

	c.InvalidateTeam(ctx, 1)

	if _, hit := c.Get(ctx, 1, filter(1, "")); hit {
		t.Error("team 1 page survived invalidation")
	}
	if _, hit := c.Get(ctx, 1, filter(1, domain.StatusDone)); hit {
		t.Error("team 1 filtered page survived invalidation")
	}
	if mr.Exists(indexKey(1)) {
		t.Error("team 1 index survived invalidation")
	}
	if _, hit := c.Get(ctx, 2, filter(2, "")); !hit {
		t.Error("team 2 page must not be touched")
	}
}

func TestTaskList_FailedInvalidationBypassesCacheUntilRetried(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()
	f := filter(1, "")
	c.Set(ctx, 1, f, page(1))

	mr.SetError("ERR redis is down")
	c.InvalidateTeam(ctx, 1)
	if !c.isPending(1) {
		t.Fatal("failed invalidation must mark the team pending")
	}
	mr.SetError("")

	if _, hit := c.Get(ctx, 1, f); hit {
		t.Fatal("pending team must not be served from cache")
	}
	c.Set(ctx, 1, filter(1, domain.StatusDone), page(1))
	if mr.Exists(listKey(1, filter(1, domain.StatusDone))) {
		t.Fatal("pending team must not be cached")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go c.Run(runCtx)
	waitFor(t, func() bool { return !c.isPending(1) })

	if mr.Exists(listKey(1, f)) {
		t.Error("background retry must delete the stale page")
	}
	c.Set(ctx, 1, f, page(2))
	if got, hit := c.Get(ctx, 1, f); !hit || got.Items[0].ID != 2 {
		t.Error("cache must work again after the retry")
	}
}

func TestTaskList_BreakerOpensAndDegradesToMiss(t *testing.T) {
	c, mr := newTestCache(t)
	ctx := context.Background()
	mr.SetError("ERR redis is down")

	for range 3 {
		if _, hit := c.Get(ctx, 1, filter(1, "")); hit {
			t.Fatal("unavailable redis must be a miss")
		}
	}
	if st := c.cb.State(); st != gobreaker.StateOpen {
		t.Fatalf("breaker state = %v after 3 failures, want open", st)
	}

	mr.SetError("")
	c.Set(ctx, 1, filter(1, ""), page(1))
	if mr.Exists(listKey(1, filter(1, ""))) {
		t.Error("open breaker must skip writes")
	}
}

func TestTaskList_CanceledRequestDoesNotTripBreaker(t *testing.T) {
	c, _ := newTestCache(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for range 5 {
		c.Get(ctx, 1, filter(1, ""))
	}
	if st := c.cb.State(); st != gobreaker.StateClosed {
		t.Errorf("breaker state = %v after canceled requests, want closed", st)
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
