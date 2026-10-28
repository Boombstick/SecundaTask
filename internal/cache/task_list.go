package cache

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker/v2"

	"taskservice/internal/domain"
)

type Options struct {
	TTL                     time.Duration
	InvalidateTimeout       time.Duration
	InvalidateRetryInterval time.Duration
	BreakerFailures         int
	BreakerOpenTimeout      time.Duration
	BreakerHalfOpenRequests int
}

var invalidateScript = redis.NewScript(`
local keys = redis.call('SMEMBERS', KEYS[1])
for i = 1, #keys, 500 do
    redis.call('DEL', unpack(keys, i, math.min(i + 499, #keys)))
end
redis.call('DEL', KEYS[1])
return #keys
`)

type TaskList struct {
	rdb  *redis.Client
	opts Options
	cb   *gobreaker.CircuitBreaker[any]
	log  *slog.Logger

	mu      sync.Mutex
	pending map[int64]struct{}
}

func NewTaskList(rdb *redis.Client, opts Options, log *slog.Logger) *TaskList {
	cb := gobreaker.NewCircuitBreaker[any](gobreaker.Settings{
		Name:         "redis",
		MaxRequests:  uint32(opts.BreakerHalfOpenRequests),
		Timeout:      opts.BreakerOpenTimeout,
		ReadyToTrip:  func(c gobreaker.Counts) bool { return c.ConsecutiveFailures >= uint32(opts.BreakerFailures) },
		IsSuccessful: func(err error) bool { return err == nil || errors.Is(err, context.Canceled) },
		OnStateChange: func(name string, from, to gobreaker.State) {
			log.Warn("circuit breaker state changed", "breaker", name, "from", from.String(), "to", to.String())
		},
	})
	return &TaskList{rdb: rdb, opts: opts, cb: cb, log: log, pending: map[int64]struct{}{}}
}

func indexKey(teamID int64) string {
	return fmt.Sprintf("tasks:team:%d:keys", teamID)
}

func listKey(teamID int64, f domain.TaskFilter) string {
	return fmt.Sprintf("tasks:team:%d:list:%s", teamID, filterHash(f))
}

func filterHash(f domain.TaskFilter) string {
	opt := func(p *int64) string {
		if p == nil {
			return "-"
		}
		return strconv.FormatInt(*p, 10)
	}
	status := "-"
	if f.Status != nil {
		status = string(*f.Status)
	}
	s := fmt.Sprintf("status=%s|assignee=%s|limit=%d|offset=%d|after=%s",
		status, opt(f.AssigneeID), f.Limit, f.Offset, opt(f.AfterID))
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (c *TaskList) Get(ctx context.Context, teamID int64, f domain.TaskFilter) (domain.TaskPage, bool) {
	if c.isPending(teamID) {
		return domain.TaskPage{}, false
	}
	res, err := c.cb.Execute(func() (any, error) {
		b, err := c.rdb.Get(ctx, listKey(teamID, f)).Bytes()
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return b, err
	})
	if err != nil {
		c.logErr(ctx, "cache get", err)
		return domain.TaskPage{}, false
	}
	b, _ := res.([]byte)
	if b == nil {
		return domain.TaskPage{}, false
	}
	page, err := decodePage(b)
	if err != nil {
		c.log.WarnContext(ctx, "cache page is unreadable", "team_id", teamID, "error", err)
		return domain.TaskPage{}, false
	}
	return page, true
}

func (c *TaskList) Set(ctx context.Context, teamID int64, f domain.TaskFilter, page domain.TaskPage) {
	if c.isPending(teamID) {
		return
	}
	value, err := encodePage(page)
	if err != nil {
		c.log.WarnContext(ctx, "cache page cannot be encoded", "team_id", teamID, "error", err)
		return
	}
	key, idx := listKey(teamID, f), indexKey(teamID)
	_, err = c.cb.Execute(func() (any, error) {
		_, err := c.rdb.TxPipelined(ctx, func(p redis.Pipeliner) error {
			p.Set(ctx, key, value, c.opts.TTL)
			p.SAdd(ctx, idx, key)
			p.Expire(ctx, idx, c.opts.TTL)
			return nil
		})
		return nil, err
	})
	if err != nil {
		c.logErr(ctx, "cache set", err)
	}
}

func (c *TaskList) InvalidateTeam(ctx context.Context, teamID int64) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.opts.InvalidateTimeout)
	defer cancel()
	if err := c.invalidate(ctx, teamID); err != nil {
		c.markPending(teamID)
		c.logErr(ctx, "cache invalidate failed, will retry", err, "team_id", teamID)
	}
}

func (c *TaskList) Run(ctx context.Context) {
	t := time.NewTicker(c.opts.InvalidateRetryInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			for _, teamID := range c.pendingTeams() {
				attemptCtx, cancel := context.WithTimeout(ctx, c.opts.InvalidateTimeout)
				err := c.invalidate(attemptCtx, teamID)
				cancel()
				if err == nil {
					c.log.Info("deferred cache invalidation succeeded", "team_id", teamID)
				}
			}
		}
	}
}

func (c *TaskList) invalidate(ctx context.Context, teamID int64) error {
	_, err := c.cb.Execute(func() (any, error) {
		return nil, invalidateScript.Run(ctx, c.rdb, []string{indexKey(teamID)}).Err()
	})
	if err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.pending, teamID)
	c.mu.Unlock()
	return nil
}

func (c *TaskList) markPending(teamID int64) {
	c.mu.Lock()
	c.pending[teamID] = struct{}{}
	c.mu.Unlock()
}

func (c *TaskList) isPending(teamID int64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.pending[teamID]
	return ok
}

func (c *TaskList) pendingTeams() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := make([]int64, 0, len(c.pending))
	for id := range c.pending {
		ids = append(ids, id)
	}
	return ids
}

func (c *TaskList) logErr(ctx context.Context, op string, err error, attrs ...any) {
	switch {
	case errors.Is(err, gobreaker.ErrOpenState), errors.Is(err, gobreaker.ErrTooManyRequests):
		return
	case errors.Is(err, context.Canceled):
		return
	}
	c.log.WarnContext(ctx, op, append(attrs, "error", err)...)
}
