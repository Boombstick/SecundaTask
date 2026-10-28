package repository

import (
	"context"
	"time"

	"taskservice/internal/domain"
	"taskservice/internal/platform/database"
)

type teamModel struct {
	ID        int64 `gorm:"primaryKey"`
	Name      string
	CreatedBy int64
	CreatedAt time.Time
}

func (teamModel) TableName() string { return "teams" }

type memberModel struct {
	TeamID    int64 `gorm:"primaryKey;autoIncrement:false"`
	UserID    int64 `gorm:"primaryKey;autoIncrement:false"`
	Role      string
	CreatedAt time.Time
}

func (memberModel) TableName() string { return "team_members" }

type TeamRepository struct {
	db *database.DB
}

func NewTeamRepository(db *database.DB) *TeamRepository { return &TeamRepository{db: db} }

func (r *TeamRepository) Create(ctx context.Context, t *domain.Team) error {
	m := teamModel{Name: t.Name, CreatedBy: t.CreatedBy}
	if err := r.db.Conn(ctx).Create(&m).Error; err != nil {
		return err
	}
	t.ID, t.CreatedAt = m.ID, m.CreatedAt
	return nil
}

func (r *TeamRepository) ListForUser(ctx context.Context, userID int64) ([]domain.Team, error) {
	teams := []domain.Team{}
	err := r.db.Conn(ctx).Raw(`
		SELECT t.id, t.name, t.created_by, t.created_at, m.role
		FROM teams t
		JOIN team_members m ON m.team_id = t.id
		WHERE m.user_id = ?
		ORDER BY t.id`, userID).Scan(&teams).Error
	return teams, err
}

func (r *TeamRepository) Delete(ctx context.Context, id int64) error {
	return r.db.Conn(ctx).Delete(&teamModel{}, id).Error
}

func (r *TeamRepository) AddMember(ctx context.Context, m *domain.Member) error {
	row := memberModel{TeamID: m.TeamID, UserID: m.UserID, Role: string(m.Role)}
	if err := r.db.Conn(ctx).Create(&row).Error; err != nil {
		switch {
		case database.IsDuplicate(err):
			return domain.ErrAlreadyMember
		case database.IsMissingReference(err):
			return domain.ErrUserNotFound
		}
		return err
	}
	m.CreatedAt = row.CreatedAt
	return nil
}

func (r *TeamRepository) GetMember(ctx context.Context, teamID, userID int64) (domain.Member, error) {
	var m domain.Member
	err := r.db.ScanOne(ctx, &m, domain.ErrMemberNotFound, `
		SELECT m.team_id, m.user_id, m.role, u.email, u.name, m.created_at
		FROM team_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.team_id = ? AND m.user_id = ?`, teamID, userID)
	return m, err
}

func (r *TeamRepository) LockMember(ctx context.Context, teamID, userID int64) (bool, error) {
	var n int64
	err := r.db.Conn(ctx).Raw(
		`SELECT COUNT(*) FROM team_members WHERE team_id = ? AND user_id = ? FOR SHARE`, teamID, userID,
	).Scan(&n).Error
	return n > 0, err
}

func (r *TeamRepository) ListMembers(ctx context.Context, teamID int64) ([]domain.Member, error) {
	members := []domain.Member{}
	err := r.db.Conn(ctx).Raw(`
		SELECT m.team_id, m.user_id, m.role, u.email, u.name, m.created_at
		FROM team_members m
		JOIN users u ON u.id = m.user_id
		WHERE m.team_id = ?
		ORDER BY FIELD(m.role, 'owner', 'admin', 'member'), u.name`, teamID).Scan(&members).Error
	return members, err
}

func (r *TeamRepository) UpdateMemberRole(ctx context.Context, teamID, userID int64, role domain.Role) error {
	return r.db.Conn(ctx).Model(&memberModel{}).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Update("role", string(role)).Error
}

func (r *TeamRepository) DeleteMember(ctx context.Context, teamID, userID int64) error {
	return r.db.Conn(ctx).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Delete(&memberModel{}).Error
}
