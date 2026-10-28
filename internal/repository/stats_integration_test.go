//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"taskservice/internal/domain"
	"taskservice/internal/platform/apperr"
	"taskservice/internal/platform/database"
	"taskservice/internal/repository"
	"taskservice/internal/service"
)

func TestTeamStats(t *testing.T) {
	ctx := context.Background()

	ctr, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("tasks"),
		tcmysql.WithUsername("tasks"),
		tcmysql.WithPassword("tasks"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })

	base, err := ctr.ConnectionString(ctx,
		"parseTime=true", "loc=UTC", "time_zone=%27%2B00%3A00%27", "charset=utf8mb4", "collation=utf8mb4_unicode_ci")
	require.NoError(t, err)
	require.NoError(t, database.Migrate(base+"&multiStatements=true"))

	db, err := database.Open(ctx, base, database.Pool{MaxOpenConns: 5, MaxIdleConns: 5, ConnMaxLifetime: time.Minute})
	require.NoError(t, err)
	users := repository.NewUserRepository(db)
	teams := repository.NewTeamRepository(db)
	tasks := repository.NewTaskRepository(db)
	comments := repository.NewCommentRepository(db)
	stats := repository.NewStatsRepository(db)

	now := time.Now().UTC().Truncate(time.Second)

	newUser := func(name string) int64 {
		u := domain.User{Email: name + "@example.com", PasswordHash: "x", Name: name}
		require.NoError(t, users.Create(ctx, &u))
		return u.ID
	}
	owner, admin := newUser("owner"), newUser("admin")
	u3, u4, u5, u6 := newUser("u3"), newUser("u4"), newUser("u5"), newUser("u6")
	outsider, otherOwner := newUser("outsider"), newUser("other-owner")

	newTeam := func(name string, ownerID int64, members map[int64]domain.Role) int64 {
		tm := domain.Team{Name: name, CreatedBy: ownerID}
		require.NoError(t, teams.Create(ctx, &tm))
		require.NoError(t, teams.AddMember(ctx, &domain.Member{TeamID: tm.ID, UserID: ownerID, Role: domain.RoleOwner}))
		for id, role := range members {
			require.NoError(t, teams.AddMember(ctx, &domain.Member{TeamID: tm.ID, UserID: id, Role: role}))
		}
		return tm.ID
	}
	teamA := newTeam("A", owner, map[int64]domain.Role{
		admin: domain.RoleAdmin, u3: domain.RoleMember, u4: domain.RoleMember,
		u5: domain.RoleMember, u6: domain.RoleMember,
	})
	teamB := newTeam("B", otherOwner, map[int64]domain.Role{u3: domain.RoleMember})

	newTask := func(teamID int64, status domain.TaskStatus, assignee *int64, createdAgo, closedAgo time.Duration) int64 {
		tk := domain.Task{TeamID: teamID, Title: "task", Status: status, CreatedBy: owner, AssigneeID: assignee}
		require.NoError(t, tasks.Create(ctx, &tk))
		var closedAt *time.Time
		if status == domain.StatusDone {
			c := now.Add(-closedAgo)
			closedAt = &c
		}
		require.NoError(t, db.Exec(ctx, `UPDATE tasks SET created_at = ?, closed_at = ? WHERE id = ?`,
			now.Add(-createdAgo), closedAt, tk.ID))
		return tk.ID
	}
	addComments := func(taskID int64, n int) {
		for range n {
			require.NoError(t, comments.Create(ctx, &domain.Comment{TaskID: taskID, UserID: owner, Content: "c"}))
		}
	}
	day := 24 * time.Hour

	t1 := newTask(teamA, domain.StatusDone, &u3, 10*time.Hour, 2*time.Hour)
	newTask(teamA, domain.StatusDone, &u3, 5*day, 1*day)
	newTask(teamA, domain.StatusDone, &u4, 3*time.Hour, 1*time.Hour)
	newTask(teamA, domain.StatusDone, &u5, 60*day, 40*day)
	newTask(teamA, domain.StatusDone, &u6, 2*day, 1*day)
	t6 := newTask(teamA, domain.StatusInProgress, &u4, time.Hour, 0)
	newTask(teamA, domain.StatusBacklog, nil, time.Hour, 0)
	newTask(teamA, domain.StatusNeedInfo, &u5, time.Hour, 0)
	addComments(t1, 3)
	addComments(t6, 1)

	for range 5 {
		id := newTask(teamB, domain.StatusDone, &u3, 2*time.Hour, time.Hour)
		addComments(id, 2)
	}

	st, err := stats.TeamStats(ctx, teamA, now)
	require.NoError(t, err)

	require.Equal(t, []domain.StatusCount{
		{Status: domain.StatusBacklog, Count: 1},
		{Status: domain.StatusInProgress, Count: 1},
		{Status: domain.StatusNeedInfo, Count: 1},
		{Status: domain.StatusDone, Count: 5},
	}, st.TasksByStatus)

	require.Equal(t, []domain.TopAssignee{
		{UserID: u3, Name: "u3", ClosedTasks: 2},
		{UserID: u4, Name: "u4", ClosedTasks: 1},
		{UserID: u6, Name: "u6", ClosedTasks: 1},
	}, st.TopAssignees30d)

	require.NotNil(t, st.AvgCloseMinutes)
	require.InDelta(t, 7320, *st.AvgCloseMinutes, 0.01)
	require.InDelta(t, 122, *st.AvgCloseHours, 0.01)

	require.EqualValues(t, 4, st.TotalComments)

	big := newTeam("big", owner, nil)
	for range 20 {
		addComments(newTask(big, domain.StatusBacklog, nil, time.Hour, 0), 1)
	}
	for _, u := range []int64{u6, u5, u4, u3} {
		newTask(big, domain.StatusDone, &u, 2*time.Hour, time.Hour)
	}
	st, err = stats.TeamStats(ctx, big, now)
	require.NoError(t, err)
	require.EqualValues(t, 20, st.TotalComments)
	require.Equal(t, []domain.TopAssignee{
		{UserID: u3, Name: "u3", ClosedTasks: 1},
		{UserID: u4, Name: "u4", ClosedTasks: 1},
		{UserID: u5, Name: "u5", ClosedTasks: 1},
	}, st.TopAssignees30d)
	require.InDelta(t, 60, *st.AvgCloseMinutes, 0.01)
	require.InDelta(t, 1, *st.AvgCloseHours, 0.01)

	empty := newTeam("empty", owner, nil)
	st, err = stats.TeamStats(ctx, empty, now)
	require.NoError(t, err)
	require.Len(t, st.TasksByStatus, 4)
	for _, c := range st.TasksByStatus {
		require.Zero(t, c.Count)
	}
	require.Empty(t, st.TopAssignees30d)
	require.Nil(t, st.AvgCloseMinutes)
	require.Nil(t, st.AvgCloseHours)
	require.Zero(t, st.TotalComments)

	svc := service.NewStatsService(stats, service.NewMembership(teams))
	_, err = svc.TeamStats(ctx, owner, teamA)
	require.NoError(t, err)
	_, err = svc.TeamStats(ctx, admin, teamA)
	require.NoError(t, err)
	_, err = svc.TeamStats(ctx, u3, teamA)
	requireKind(t, err, apperr.KindForbidden)
	_, err = svc.TeamStats(ctx, outsider, teamA)
	requireKind(t, err, apperr.KindNotFound)
	_, err = svc.TeamStats(ctx, otherOwner, teamA)
	requireKind(t, err, apperr.KindNotFound)
}

func requireKind(t *testing.T, err error, kind apperr.Kind) {
	t.Helper()
	e, ok := apperr.As(err)
	require.True(t, ok, "expected apperr.Error, got %v", err)
	require.Equal(t, kind, e.Kind)
}
