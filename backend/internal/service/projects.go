package service

import (
	"errors"
	"strings"

	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"github.com/NorousS/kai-web-dev/backend/internal/repository"
)

// Project operations and DTOs.

type ProjectView struct {
	model.Project
	MyRole       string `json:"my_role"`
	MembersCount int64  `json:"members_count"`
	TasksCount   int64  `json:"tasks_count"`
}

func (s *Service) ProjectView(p model.Project, actor model.User) (ProjectView, error) {
	role := "admin"
	if m, e := s.repo.Member(p.ID, actor.ID); e == nil {
		role = m.Role
	}
	members, e := s.repo.CountMembers(p.ID)
	if e != nil {
		return ProjectView{}, bad(500, "Unable to load project")
	}
	tasks, e := s.repo.CountTasks(p.ID)
	if e != nil {
		return ProjectView{}, bad(500, "Unable to load project")
	}
	return ProjectView{Project: p, MyRole: role, MembersCount: members, TasksCount: tasks}, nil
}
func (s *Service) allowedProject(actor model.User, id uint) (model.Project, model.Membership, error) {
	p, e := s.repo.Project(id)
	if e != nil {
		return p, model.Membership{}, bad(404, "Project not found")
	}
	m, e := s.repo.Member(id, actor.ID)
	if e != nil && actor.Role == "admin" {
		return p, model.Membership{ProjectID: id, UserID: actor.ID, Role: "admin"}, nil
	}
	if e != nil {
		return p, m, bad(403, "You are not a member of this project")
	}
	return p, m, nil
}
func canManage(m model.Membership) bool {
	return m.Role == "owner" || m.Role == "manager" || m.Role == "admin"
}
func (s *Service) Projects(actor model.User, q string) ([]ProjectView, error) {
	pp, e := s.repo.Projects(actor, strings.TrimSpace(q))
	if e != nil {
		return nil, bad(500, "Could not load projects")
	}
	out := make([]ProjectView, 0, len(pp))
	for _, p := range pp {
		v, e := s.ProjectView(p, actor)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) CreateProject(actor model.User, name, description string) (ProjectView, error) {
	p := model.Project{Name: strings.TrimSpace(name), Description: description, CreatedBy: actor.ID, OwnerID: actor.ID}
	e := s.repo.Tx(func(tx *repository.Store) error {
		if e := tx.CreateProject(&p); e != nil {
			return e
		}
		return tx.AddMember(&model.Membership{ProjectID: p.ID, UserID: actor.ID, Role: "owner"})
	})
	if e != nil {
		return ProjectView{}, bad(500, "Could not create project")
	}
	s.invalidate()
	return s.ProjectView(p, actor)
}
func (s *Service) ProjectDetails(actor model.User, id uint) (ProjectView, []repository.MemberRow, error) {
	p, _, e := s.allowedProject(actor, id)
	if e != nil {
		return ProjectView{}, nil, e
	}
	v, e := s.ProjectView(p, actor)
	if e != nil {
		return v, nil, e
	}
	members, e := s.repo.ProjectMembers(id)
	if e != nil {
		return v, nil, bad(500, "Could not load members")
	}
	if members == nil {
		members = []repository.MemberRow{}
	}
	return v, members, nil
}
func (s *Service) UpdateProject(actor model.User, id uint, name, description string) (ProjectView, error) {
	p, m, e := s.allowedProject(actor, id)
	if e != nil {
		return ProjectView{}, e
	}
	if !canManage(m) {
		return ProjectView{}, bad(403, "Manager or owner only")
	}
	if e = s.repo.UpdateProject(&p, strings.TrimSpace(name), description); e != nil {
		return ProjectView{}, bad(500, "Could not save project")
	}
	s.invalidate()
	return s.ProjectView(p, actor)
}
func (s *Service) DeleteProject(actor model.User, id uint) error {
	p, _, e := s.allowedProject(actor, id)
	if e != nil {
		return e
	}
	if p.OwnerID != actor.ID && p.CreatedBy != actor.ID {
		return bad(403, "Only project creator or current owner may delete it")
	}
	if e = s.repo.Tx(func(tx *repository.Store) error { return tx.DeleteProject(id) }); e != nil {
		return bad(500, "Cannot delete project")
	}
	s.invalidate()
	return nil
}
func (s *Service) Invite(actor model.User, id uint, email string) error {
	_, m, e := s.allowedProject(actor, id)
	if e != nil {
		return e
	}
	if !canManage(m) {
		return bad(403, "Manager or owner only")
	}
	u, e := s.repo.UserByEmail(strings.ToLower(strings.TrimSpace(email)))
	if e != nil {
		return bad(404, "User not found: ask them to register first")
	}
	if e = s.repo.AddMember(&model.Membership{ProjectID: id, UserID: u.ID, Role: "member"}); e != nil {
		return bad(409, "User is already in this project")
	}
	s.invalidate()
	return nil
}
func (s *Service) ChangeMember(actor model.User, projectID, userID uint, role string) error {
	p, _, e := s.allowedProject(actor, projectID)
	if e != nil {
		return e
	}
	if p.OwnerID != actor.ID {
		return bad(403, "Only current owner can change member roles")
	}
	if role != "owner" && role != "manager" && role != "member" {
		return bad(400, "Invalid project role")
	}
	if _, e = s.repo.Member(projectID, userID); e != nil {
		return bad(404, "Member not found")
	}
	if userID == p.OwnerID && role != "owner" {
		return bad(400, "Transfer ownership before changing your own role")
	}
	e = s.repo.Tx(func(tx *repository.Store) error {
		if role == "owner" && userID != p.OwnerID {
			// Lock the ownership row to avoid concurrent owner transfers.
			locked, err := tx.ProjectForUpdate(projectID)
			if err != nil {
				return err
			}
			if locked.OwnerID != p.OwnerID {
				return ErrConflict
			}
			updated, err := tx.SetOwner(projectID, userID, p.OwnerID)
			if err != nil {
				return err
			}
			if !updated {
				return ErrConflict
			}
			if err = tx.SetMemberRole(projectID, p.OwnerID, "manager"); err != nil {
				return err
			}
		}
		return tx.SetMemberRole(projectID, userID, role)
	})
	if errors.Is(e, ErrConflict) {
		return bad(409, "Ownership was changed concurrently")
	}
	if e != nil {
		return bad(500, "Unable to update role")
	}
	s.invalidate()
	return nil
}
func (s *Service) RemoveMember(actor model.User, projectID, userID uint) error {
	p, _, e := s.allowedProject(actor, projectID)
	if e != nil {
		return e
	}
	if userID == p.OwnerID {
		return bad(400, "Owner must transfer ownership first")
	}
	if userID != actor.ID && p.OwnerID != actor.ID {
		return bad(403, "Only owner can remove another member")
	}
	e = s.repo.Tx(func(tx *repository.Store) error {
		ok, e := tx.RemoveMember(projectID, userID)
		if e != nil {
			return e
		}
		if !ok {
			return ErrConflict
		}
		return tx.UnassignTasks(projectID, userID)
	})
	if errors.Is(e, ErrConflict) {
		return bad(404, "Member not found")
	}
	if e != nil {
		return bad(500, "Unable to remove member")
	}
	s.invalidate()
	return nil
}