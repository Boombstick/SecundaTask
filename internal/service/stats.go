package service

import (
	"context"
	"time"

	"taskservice/internal/domain"
)

type StatsStore interface {
	TeamStats(ctx context.Context, teamID int64, now time.Time) (domain.TeamStats, error)
}

type StatsService struct {
	stats   StatsStore
	members *Membership
	now     func() time.Time
}

func NewStatsService(stats StatsStore, members *Membership) *StatsService {
	return &StatsService{stats: stats, members: members, now: func() time.Time { return time.Now().UTC() }}
}

func (s *StatsService) TeamStats(ctx context.Context, userID, teamID int64) (domain.TeamStats, error) {
	a, err := s.members.Actor(ctx, teamID, userID)
	if err != nil {
		return domain.TeamStats{}, err
	}
	if err := CanViewStats(a); err != nil {
		return domain.TeamStats{}, err
	}
	return s.stats.TeamStats(ctx, teamID, s.now())
}
