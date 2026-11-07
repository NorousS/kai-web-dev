package main

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type statsCache struct {
	mu      sync.Mutex
	entries map[uint]cachedStats
	ttl     time.Duration
}
type cachedStats struct {
	payload gin.H
	expiry  time.Time
}

func newStatsCache(ttl time.Duration) *statsCache {
	return &statsCache{entries: make(map[uint]cachedStats), ttl: ttl}
}
func (s *statsCache) get(id uint) (gin.H, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.entries[id]
	return v.payload, ok && time.Now().Before(v.expiry)
}
func (s *statsCache) put(id uint, data gin.H) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[id] = cachedStats{data, time.Now().Add(s.ttl)}
}
func (s *statsCache) invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = make(map[uint]cachedStats)
}

type projectView struct {
	Project
	MyRole       string `json:"my_role"`
	MembersCount int64  `json:"members_count"`
	TasksCount   int64  `json:"tasks_count"`
}

func (a *App) viewProject(p Project, u User) projectView {
	var m Membership
	role := "admin"
	if a.db.Where("project_id = ? AND user_id = ?", p.ID, u.ID).First(&m).Error == nil {
		role = m.Role
	}
	var members, tasks int64
	a.db.Model(&Membership{}).Where("project_id = ?", p.ID).Count(&members)
	a.db.Model(&Task{}).Where("project_id = ?", p.ID).Count(&tasks)
	return projectView{Project: p, MyRole: role, MembersCount: members, TasksCount: tasks}
}
func (a *App) listProjects(c *gin.Context) {
	u := current(c)
	var projects []Project
	q := a.db.Model(&Project{})
	if u.Role != "admin" {
		q = q.Joins("JOIN memberships ON memberships.project_id = projects.id").Where("memberships.user_id = ?", u.ID)
	}
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		q = q.Where("LOWER(projects.name) LIKE ?", "%"+strings.ToLower(s)+"%")
	}
	if err := q.Order("projects.updated_at DESC").Limit(100).Find(&projects).Error; err != nil {
		fail(c, 500, "Could not load projects")
		return
	}
	out := make([]projectView, 0, len(projects))
	for _, p := range projects {
		out = append(out, a.viewProject(p, u))
	}
	c.JSON(200, gin.H{"projects": out})
}
func (a *App) createProject(c *gin.Context) {
	var in struct {
		Name        string `json:"name" binding:"required,min=2,max=120"`
		Description string `json:"description" binding:"max=2000"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "Project name must be 2-120 characters")
		return
	}
	u := current(c)
	p := Project{Name: strings.TrimSpace(in.Name), Description: in.Description, CreatedBy: u.ID, OwnerID: u.ID}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&p).Error; err != nil {
			return err
		}
		return tx.Create(&Membership{ProjectID: p.ID, UserID: u.ID, Role: "owner"}).Error
	})
	if err != nil {
		fail(c, 500, "Could not create project")
		return
	}
	a.statsCache.invalidate()
	c.JSON(http.StatusCreated, gin.H{"project": a.viewProject(p, u)})
}
func (a *App) getProject(c *gin.Context) {
	p, _, ok := mustProject(a, c)
	if !ok {
		return
	}
	type memberView struct {
		UserID uint   `json:"user_id"`
		Name   string `json:"name"`
		Email  string `json:"email"`
		Role   string `json:"role"`
	}
	var members []memberView
	a.db.Table("memberships").Select("memberships.user_id, users.name, users.email, memberships.role").Joins("JOIN users ON users.id = memberships.user_id").Where("memberships.project_id = ?", p.ID).Order("memberships.id").Scan(&members)
	if members == nil {
		members = []memberView{}
	}
	c.JSON(200, gin.H{"project": a.viewProject(p, current(c)), "members": members})
}
func (a *App) updateProject(c *gin.Context) {
	p, m, ok := mustProject(a, c)
	if !ok {
		return
	}
	if !canManage(m) {
		fail(c, 403, "Manager or owner only")
		return
	}
	var in struct {
		Name        string `json:"name" binding:"required,min=2,max=120"`
		Description string `json:"description" binding:"max=2000"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "Invalid project fields")
		return
	}
	a.db.Model(&p).Updates(map[string]any{"name": strings.TrimSpace(in.Name), "description": in.Description})
	a.statsCache.invalidate()
	c.JSON(200, gin.H{"project": a.viewProject(p, current(c))})
}
func (a *App) deleteProject(c *gin.Context) {
	p, _, ok := mustProject(a, c)
	if !ok {
		return
	}
	if p.OwnerID != current(c).ID && p.CreatedBy != current(c).ID {
		fail(c, 403, "Only project creator or current owner may delete it")
		return
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var ids []uint
		if err := tx.Model(&Task{}).Where("project_id = ?", p.ID).Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) > 0 {
			if err := tx.Where("task_id IN ?", ids).Delete(&Review{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Where("project_id = ?", p.ID).Delete(&Task{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", p.ID).Delete(&Membership{}).Error; err != nil {
			return err
		}
		return tx.Delete(&p).Error
	})
	if err != nil {
		fail(c, 500, "Cannot delete project")
		return
	}
	a.statsCache.invalidate()
	c.JSON(200, gin.H{"ok": true})
}
func (a *App) invite(c *gin.Context) {
	p, m, ok := mustProject(a, c)
	if !ok {
		return
	}
	if !canManage(m) {
		fail(c, 403, "Manager or owner only")
		return
	}
	var in struct {
		Email string `json:"email" binding:"required,email"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "Enter a registered user email")
		return
	}
	var u User
	if a.db.Where("email = ?", strings.ToLower(strings.TrimSpace(in.Email))).First(&u).Error != nil {
		fail(c, 404, "User not found: ask them to register first")
		return
	}
	member := Membership{ProjectID: p.ID, UserID: u.ID, Role: "member"}
	if a.db.Create(&member).Error != nil {
		fail(c, 409, "User is already in this project")
		return
	}
	a.statsCache.invalidate()
	c.JSON(201, gin.H{"ok": true})
}
func (a *App) changeMember(c *gin.Context) {
	p, _, ok := mustProject(a, c)
	if !ok {
		return
	}
	if p.OwnerID != current(c).ID {
		fail(c, 403, "Only current owner can change member roles")
		return
	}
	userID, ok := idParam(c, "userID")
	if !ok {
		return
	}
	var in struct {
		Role string `json:"role" binding:"required,oneof=owner manager member"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "Role must be owner, manager or member")
		return
	}
	var target Membership
	if a.db.Where("project_id = ? AND user_id = ?", p.ID, userID).First(&target).Error != nil {
		fail(c, 404, "Member not found")
		return
	}
	if in.Role != "owner" && userID == p.OwnerID {
		fail(c, 400, "Transfer ownership before changing your own role")
		return
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		if in.Role == "owner" {
			if userID == p.OwnerID {
				return nil
			}
			if err := tx.Model(&Project{}).Where("id = ? AND owner_id = ?", p.ID, p.OwnerID).Update("owner_id", userID).Error; err != nil {
				return err
			}
			if err := tx.Model(&Membership{}).Where("project_id = ? AND user_id = ?", p.ID, p.OwnerID).Update("role", "manager").Error; err != nil {
				return err
			}
		}
		return tx.Model(&Membership{}).Where("id = ?", target.ID).Update("role", in.Role).Error
	})
	if err != nil {
		fail(c, 500, "Unable to update role")
		return
	}
	a.statsCache.invalidate()
	c.JSON(200, gin.H{"ok": true})
}
func (a *App) removeMember(c *gin.Context) {
	p, _, ok := mustProject(a, c)
	if !ok {
		return
	}
	id, ok := idParam(c, "userID")
	if !ok {
		return
	}
	if id == p.OwnerID {
		fail(c, 400, "Owner must transfer ownership first")
		return
	}
	if id != current(c).ID && p.OwnerID != current(c).ID {
		fail(c, 403, "Only owner can remove another member")
		return
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		r := tx.Where("project_id = ? AND user_id = ?", p.ID, id).Delete(&Membership{})
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected == 0 {
			return errors.New("member not found")
		}
		return tx.Model(&Task{}).Where("project_id = ? AND assignee_id = ?", p.ID, id).Update("assignee_id", nil).Error
	})
	if err != nil {
		fail(c, 404, "Member not found")
		return
	}
	a.statsCache.invalidate()
	c.JSON(200, gin.H{"ok": true})
}
func (a *App) dashboard(c *gin.Context) {
	u := current(c)
	if value, ok := a.statsCache.get(u.ID); ok {
		c.JSON(200, value)
		return
	}
	projectSQL := "SELECT id FROM projects"
	args := []any{}
	if u.Role != "admin" {
		projectSQL = "SELECT project_id FROM memberships WHERE user_id = ?"
		args = append(args, u.ID)
	}
	var projects, tasks, pending, completed, assigned int64
	a.db.Raw("SELECT COUNT(*) FROM projects WHERE id IN ("+projectSQL+")", args...).Scan(&projects)
	a.db.Raw("SELECT COUNT(*) FROM tasks WHERE project_id IN ("+projectSQL+")", args...).Scan(&tasks)
	a.db.Raw("SELECT COUNT(*) FROM tasks WHERE project_id IN ("+projectSQL+") AND status = 'in_review'", args...).Scan(&pending)
	a.db.Raw("SELECT COUNT(*) FROM tasks WHERE project_id IN ("+projectSQL+") AND status = 'done'", args...).Scan(&completed)
	a.db.Model(&Task{}).Where("assignee_id = ? AND status != ?", u.ID, "done").Count(&assigned)
	payload := gin.H{"projects": projects, "tasks": tasks, "in_review": pending, "done": completed, "assigned_to_me": assigned}
	a.statsCache.put(u.ID, payload)
	c.JSON(200, payload)
}