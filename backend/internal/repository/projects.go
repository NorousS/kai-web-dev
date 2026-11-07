package repository

import (
	"strings"

	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"gorm.io/gorm/clause"
)

func (s *Store) Project(id uint) (model.Project, error) {
	var p model.Project
	e := s.db.First(&p, id).Error
	return p, e
}
func (s *Store) ProjectForUpdate(id uint) (model.Project, error) {
	var p model.Project
	e := s.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error
	return p, e
}
func (s *Store) Projects(u model.User, q string) ([]model.Project, error) {
	var out []model.Project
	db := s.db.Model(&model.Project{})
	if u.Role != "admin" {
		db = db.Joins("JOIN memberships ON memberships.project_id = projects.id").Where("memberships.user_id = ?", u.ID)
	}
	if q != "" {
		db = db.Where("LOWER(projects.name) LIKE ?", "%"+strings.ToLower(q)+"%")
	}
	e := db.Order("projects.updated_at DESC").Limit(100).Find(&out).Error
	return out, e
}
func (s *Store) CreateProject(p *model.Project) error { return s.db.Create(p).Error }
func (s *Store) UpdateProject(p *model.Project, name, description string) error {
	e := s.db.Model(p).Updates(map[string]any{"name": name, "description": description}).Error
	if e == nil {
		p.Name = name
		p.Description = description
	}
	return e
}
func (s *Store) DeleteProject(id uint) error {
	var ids []uint
	if e := s.db.Model(&model.Task{}).Where("project_id = ?", id).Pluck("id", &ids).Error; e != nil {
		return e
	}
	if len(ids) > 0 {
		if e := s.db.Where("task_id IN ?", ids).Delete(&model.Review{}).Error; e != nil {
			return e
		}
	}
	if e := s.db.Where("project_id = ?", id).Delete(&model.Task{}).Error; e != nil {
		return e
	}
	if e := s.db.Where("project_id = ?", id).Delete(&model.Membership{}).Error; e != nil {
		return e
	}
	return s.db.Where("id = ?", id).Delete(&model.Project{}).Error
}
func (s *Store) Member(projectID, userID uint) (model.Membership, error) {
	var m model.Membership
	e := s.db.Where("project_id = ? AND user_id = ?", projectID, userID).First(&m).Error
	return m, e
}
func (s *Store) AddMember(m *model.Membership) error { return s.db.Create(m).Error }
func (s *Store) SetMemberRole(projectID, userID uint, role string) error {
	return s.db.Model(&model.Membership{}).Where("project_id = ? AND user_id = ?", projectID, userID).Update("role", role).Error
}
func (s *Store) SetOwner(projectID, ownerID, expectedOwner uint) (bool, error) {
	r := s.db.Model(&model.Project{}).Where("id = ? AND owner_id = ?", projectID, expectedOwner).Update("owner_id", ownerID)
	return r.RowsAffected > 0, r.Error
}
func (s *Store) RemoveMember(projectID, userID uint) (bool, error) {
	r := s.db.Where("project_id = ? AND user_id = ?", projectID, userID).Delete(&model.Membership{})
	return r.RowsAffected > 0, r.Error
}
func (s *Store) UnassignTasks(projectID, userID uint) error {
	return s.db.Model(&model.Task{}).Where("project_id = ? AND assignee_id = ?", projectID, userID).Update("assignee_id", nil).Error
}
func (s *Store) CountMembers(projectID uint) (int64, error) {
	var n int64
	e := s.db.Model(&model.Membership{}).Where("project_id = ?", projectID).Count(&n).Error
	return n, e
}
func (s *Store) CountTasks(projectID uint) (int64, error) {
	var n int64
	e := s.db.Model(&model.Task{}).Where("project_id = ?", projectID).Count(&n).Error
	return n, e
}
func (s *Store) ProjectMembers(projectID uint) ([]MemberRow, error) {
	var out []MemberRow
	e := s.db.Table("memberships").Select("memberships.user_id, users.name, users.email, memberships.role").Joins("JOIN users ON users.id = memberships.user_id").Where("memberships.project_id = ?", projectID).Order("memberships.id").Scan(&out).Error
	return out, e
}

type MemberRow struct {
	UserID uint   `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
}