package repository

import (
	"strings"

	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"gorm.io/gorm/clause"
)

type TaskFilter struct {
	Status, Priority, Query string
	AssigneeID              *uint
}

func (s *Store) Tasks(projectID uint, f TaskFilter) ([]model.Task, error) {
	db := s.db.Where("project_id = ?", projectID)
	if f.Status != "" && f.Status != "all" {
		db = db.Where("status = ?", f.Status)
	}
	if f.Priority != "" && f.Priority != "all" {
		db = db.Where("priority = ?", f.Priority)
	}
	if f.Query != "" {
		db = db.Where("LOWER(title) LIKE ?", "%"+strings.ToLower(f.Query)+"%")
	}
	if f.AssigneeID != nil {
		db = db.Where("assignee_id = ?", *f.AssigneeID)
	}
	var out []model.Task
	e := db.Order("created_at DESC").Limit(300).Find(&out).Error
	return out, e
}
func (s *Store) Task(projectID, taskID uint) (model.Task, error) {
	var t model.Task
	e := s.db.Where("id = ? AND project_id = ?", taskID, projectID).First(&t).Error
	return t, e
}
func (s *Store) LockTask(projectID, taskID uint) (model.Task, error) {
	var t model.Task
	e := s.db.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ?", taskID, projectID).First(&t).Error
	return t, e
}
func (s *Store) CreateTask(t *model.Task) error { return s.db.Create(t).Error }
func (s *Store) UpdateTask(t *model.Task, fields map[string]any) error {
	return s.db.Model(t).Updates(fields).Error
}
func (s *Store) LatestReview(taskID uint) (model.Review, error) {
	var r model.Review
	e := s.db.Where("task_id = ?", taskID).Order("id DESC").First(&r).Error
	return r, e
}
func (s *Store) PendingReview(taskID uint) (model.Review, error) {
	var r model.Review
	e := s.db.Where("task_id = ? AND status = ?", taskID, "pending").Order("id DESC").First(&r).Error
	return r, e
}
func (s *Store) CreateReview(r *model.Review) error { return s.db.Create(r).Error }
func (s *Store) UpdateReview(r *model.Review, fields map[string]any) error {
	return s.db.Model(r).Updates(fields).Error
}