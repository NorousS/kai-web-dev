package repository

import (
	"github.com/NorousS/kai-web-dev/backend/internal/model"
)

type DashboardStats struct {
	Projects     int64 `json:"projects"`
	Tasks        int64 `json:"tasks"`
	InReview     int64 `json:"in_review"`
	Done         int64 `json:"done"`
	AssignedToMe int64 `json:"assigned_to_me"`
}

func (s *Store) Dashboard(u model.User) (DashboardStats, error) {
	var res DashboardStats
	ids := "SELECT id FROM projects"
	args := []any{}
	if u.Role != "admin" {
		ids = "SELECT project_id FROM memberships WHERE user_id = ?"
		args = append(args, u.ID)
	}
	queries := []struct {
		sql  string
		dest *int64
	}{
		{"SELECT COUNT(*) FROM projects WHERE id IN (" + ids + ")", &res.Projects},
		{"SELECT COUNT(*) FROM tasks WHERE project_id IN (" + ids + ")", &res.Tasks},
		{"SELECT COUNT(*) FROM tasks WHERE project_id IN (" + ids + ") AND status = 'in_review'", &res.InReview},
		{"SELECT COUNT(*) FROM tasks WHERE project_id IN (" + ids + ") AND status = 'done'", &res.Done},
	}
	for _, q := range queries {
		if e := s.db.Raw(q.sql, args...).Scan(q.dest).Error; e != nil {
			return res, e
		}
	}
	e := s.db.Model(&model.Task{}).Where("assignee_id = ? AND status != ?", u.ID, "done").Count(&res.AssignedToMe).Error
	return res, e
}