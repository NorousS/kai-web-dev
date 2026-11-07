package service

import "github.com/NorousS/kai-web-dev/backend/internal/model"

// TaskDetails loads one authorized task for the server-rendered edit page.
func (s *Service) TaskDetails(actor model.User, projectID, taskID uint) (TaskView, error) {
	if _, _, err := s.allowedProject(actor, projectID); err != nil {
		return TaskView{}, err
	}
	t, err := s.repo.Task(projectID, taskID)
	if err != nil {
		return TaskView{}, bad(404, "Task not found")
	}
	return s.TaskView(t)
}

// StartAssignedTask moves a task into progress for its assignee, including an assignee who is a manager.
func (s *Service) StartAssignedTask(actor model.User, projectID, taskID uint) error {
	if _, _, err := s.allowedProject(actor, projectID); err != nil {
		return err
	}
	t, err := s.repo.Task(projectID, taskID)
	if err != nil {
		return bad(404, "Task not found")
	}
	if t.AssigneeID == nil || *t.AssigneeID != actor.ID {
		return bad(403, "Only the assignee can start this task")
	}
	if t.Status != "todo" {
		return bad(409, "Only a TODO task can be started")
	}
	if err = s.repo.UpdateTask(&t, map[string]any{"status": "in_progress"}); err != nil {
		return bad(500, "Could not start task")
	}
	s.invalidate()
	return nil
}