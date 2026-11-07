package service

import (
	"errors"
	"net/url"
	"strings"
	"time"

	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"github.com/NorousS/kai-web-dev/backend/internal/repository"
)

// Task operations.

type TaskInput struct {
	Title           string     `json:"title" binding:"required,min=2,max=160"`
	Description     string     `json:"description" binding:"max=10000"`
	Priority        string     `json:"priority" binding:"required,oneof=low medium high critical"`
	UrgencyOverride string     `json:"urgency_override" binding:"required,oneof=auto green yellow red critical"`
	Epic            string     `json:"epic" binding:"max=100"`
	Tags            string     `json:"tags" binding:"max=400"`
	AssigneeID      *uint      `json:"assignee_id"`
	DueAt           *time.Time `json:"due_at"`
}
type TaskView struct {
	model.Task
	Urgency      string        `json:"urgency"`
	AssigneeName string        `json:"assignee_name"`
	CreatorName  string        `json:"creator_name"`
	LatestReview *model.Review `json:"latest_review"`
}

func (s *Service) TaskView(t model.Task) (TaskView, error) {
	v := TaskView{Task: t, Urgency: model.Urgency(t, time.Now())}
	if u, e := s.repo.UserByID(t.CreatedBy); e == nil {
		v.CreatorName = u.Name
	}
	if t.AssigneeID != nil {
		if u, e := s.repo.UserByID(*t.AssigneeID); e == nil {
			v.AssigneeName = u.Name
		}
	}
	if r, e := s.repo.LatestReview(t.ID); e == nil {
		v.LatestReview = &r
	}
	return v, nil
}
func (s *Service) validAssignee(projectID uint, id *uint) bool {
	if id == nil {
		return true
	}
	_, e := s.repo.Member(projectID, *id)
	return e == nil
}
func (s *Service) Tasks(actor model.User, projectID uint, f repository.TaskFilter) ([]TaskView, error) {
	if _, _, e := s.allowedProject(actor, projectID); e != nil {
		return nil, e
	}
	ts, e := s.repo.Tasks(projectID, f)
	if e != nil {
		return nil, bad(500, "Unable to load tasks")
	}
	out := make([]TaskView, 0, len(ts))
	for _, t := range ts {
		v, _ := s.TaskView(t)
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) CreateTask(actor model.User, projectID uint, in TaskInput) (TaskView, error) {
	_, m, e := s.allowedProject(actor, projectID)
	if e != nil {
		return TaskView{}, e
	}
	if !canManage(m) {
		return TaskView{}, bad(403, "Only managers can create tasks")
	}
	if !s.validAssignee(projectID, in.AssigneeID) {
		return TaskView{}, bad(400, "Assignee must be a project member")
	}
	t := model.Task{ProjectID: projectID, Title: strings.TrimSpace(in.Title), Description: in.Description, Status: "todo", Priority: in.Priority, UrgencyOverride: in.UrgencyOverride, Epic: in.Epic, Tags: in.Tags, CreatedBy: actor.ID, AssigneeID: in.AssigneeID, DueAt: in.DueAt}
	if e = s.repo.CreateTask(&t); e != nil {
		return TaskView{}, bad(500, "Unable to create task")
	}
	s.invalidate()
	return s.TaskView(t)
}
func (s *Service) UpdateTask(actor model.User, projectID, taskID uint, in TaskInput, workerStatus string) (TaskView, error) {
	_, m, e := s.allowedProject(actor, projectID)
	if e != nil {
		return TaskView{}, e
	}
	t, e := s.repo.Task(projectID, taskID)
	if e != nil {
		return TaskView{}, bad(404, "Task not found")
	}
	if !canManage(m) {
		if t.AssigneeID == nil || *t.AssigneeID != actor.ID {
			return TaskView{}, bad(403, "You cannot update this task")
		}
		if workerStatus != "in_progress" || t.Status != "todo" {
			return TaskView{}, bad(400, "Assignee can only start a TODO task")
		}
		if e = s.repo.UpdateTask(&t, map[string]any{"status": "in_progress"}); e != nil {
			return TaskView{}, bad(500, "Unable to update task")
		}
	} else {
		if workerStatus != "" {
			return TaskView{}, bad(400, "Manager must submit task fields to edit")
		}
		if len(strings.TrimSpace(in.Title)) < 2 {
			return TaskView{}, bad(400, "Invalid task fields")
		}
		if t.Status == "done" {
			return TaskView{}, bad(409, "Completed tasks cannot be changed")
		}
		if t.Status == "in_review" {
			return TaskView{}, bad(409, "Review a submitted task before editing")
		}
		if !s.validAssignee(projectID, in.AssigneeID) {
			return TaskView{}, bad(400, "Assignee must be a project member")
		}
		fields := map[string]any{"title": strings.TrimSpace(in.Title), "description": in.Description, "priority": in.Priority, "urgency_override": in.UrgencyOverride, "epic": in.Epic, "tags": in.Tags, "assignee_id": in.AssigneeID, "due_at": in.DueAt}
		if e = s.repo.UpdateTask(&t, fields); e != nil {
			return TaskView{}, bad(500, "Unable to save task")
		}
	}
	s.invalidate()
	t, _ = s.repo.Task(projectID, taskID)
	return s.TaskView(t)
}
func (s *Service) SubmitTask(actor model.User, projectID, taskID uint, description, evidence string) error {
	if _, _, e := s.allowedProject(actor, projectID); e != nil {
		return e
	}
	t, e := s.repo.Task(projectID, taskID)
	if e != nil {
		return bad(404, "Task not found")
	}
	if t.AssigneeID == nil || *t.AssigneeID != actor.ID {
		return bad(403, "Only assigned worker can submit a review")
	}
	description = strings.TrimSpace(description)
	evidence = strings.TrimSpace(evidence)
	if description == "" && evidence == "" {
		return bad(400, "Provide work description or evidence URL")
	}
	if evidence != "" {
		u, err := url.Parse(evidence)
		if err != nil || !(u.Scheme == "http" || u.Scheme == "https") || u.Host == "" {
			return bad(400, "Evidence URL must be http(s)")
		}
	}
	e = s.repo.Tx(func(tx *repository.Store) error {
		locked, e := tx.LockTask(projectID, taskID)
		if e != nil {
			return e
		}
		if locked.AssigneeID == nil || *locked.AssigneeID != actor.ID || !(locked.Status == "todo" || locked.Status == "in_progress") {
			return ErrConflict
		}
		if e = tx.UpdateTask(&locked, map[string]any{"status": "in_review"}); e != nil {
			return e
		}
		return tx.CreateReview(&model.Review{TaskID: taskID, SubmittedBy: actor.ID, Description: description, EvidenceURL: evidence, Status: "pending"})
	})
	if errors.Is(e, ErrConflict) {
		return bad(409, "Task is already in review, reassigned or completed")
	}
	if e != nil {
		return bad(500, "Unable to submit task")
	}
	s.invalidate()
	return nil
}
func (s *Service) ReviewTask(actor model.User, projectID, taskID uint, decision, comment string) error {
	_, m, e := s.allowedProject(actor, projectID)
	if e != nil {
		return e
	}
	if !canManage(m) {
		return bad(403, "Manager or owner only")
	}
	if _, e = s.repo.Task(projectID, taskID); e != nil {
		return bad(404, "Task not found")
	}
	now := time.Now()
	e = s.repo.Tx(func(tx *repository.Store) error {
		t, e := tx.LockTask(projectID, taskID)
		if e != nil {
			return e
		}
		if t.Status != "in_review" {
			return ErrConflict
		}
		rev, e := tx.PendingReview(taskID)
		if e != nil {
			return ErrConflict
		}
		if rev.SubmittedBy == actor.ID {
			return ErrSelfReview
		}
		if e = tx.UpdateReview(&rev, map[string]any{"status": decision, "comment": strings.TrimSpace(comment), "reviewer_id": actor.ID, "reviewed_at": now}); e != nil {
			return e
		}
		fields := map[string]any{"status": "in_progress", "completed_at": nil}
		if decision == "approved" {
			fields["status"] = "done"
			fields["completed_at"] = now
		}
		return tx.UpdateTask(&t, fields)
	})
	if errors.Is(e, ErrConflict) {
		return bad(409, "No pending review")
	}
	if errors.Is(e, ErrSelfReview) {
		return bad(403, "You cannot approve your own work")
	}
	if e != nil {
		return bad(500, "Review failed")
	}
	s.invalidate()
	return nil
}