package main

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm/clause"
)

type taskView struct {
	Task
	Urgency      string  `json:"urgency"`
	AssigneeName string  `json:"assignee_name"`
	CreatorName  string  `json:"creator_name"`
	LatestReview *Review `json:"latest_review"`
}

func (a *App) taskOutput(t Task) taskView {
	v := taskView{Task: t, Urgency: urgency(t)}
	var creator User
	if a.db.Select("name").First(&creator, t.CreatedBy).Error == nil {
		v.CreatorName = creator.Name
	}
	if t.AssigneeID != nil {
		var assigned User
		if a.db.Select("name").First(&assigned, *t.AssigneeID).Error == nil {
			v.AssigneeName = assigned.Name
		}
	}
	var rev Review
	if a.db.Where("task_id = ?", t.ID).Order("id desc").First(&rev).Error == nil {
		v.LatestReview = &rev
	}
	return v
}
func (a *App) listTasks(c *gin.Context) {
	p, _, ok := mustProject(a, c)
	if !ok {
		return
	}
	q := a.db.Where("project_id = ?", p.ID)
	if status := c.Query("status"); status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	if priority := c.Query("priority"); priority != "" && priority != "all" {
		q = q.Where("priority = ?", priority)
	}
	if s := strings.TrimSpace(c.Query("q")); s != "" {
		q = q.Where("LOWER(title) LIKE ?", "%"+strings.ToLower(s)+"%")
	}
	if s := c.Query("assignee"); s != "" {
		if n, e := strconv.ParseUint(s, 10, 64); e == nil {
			q = q.Where("assignee_id = ?", n)
		}
	}
	var tasks []Task
	if err := q.Order("created_at DESC").Limit(300).Find(&tasks).Error; err != nil {
		fail(c, 500, "Unable to load tasks")
		return
	}
	out := make([]taskView, 0, len(tasks))
	for _, t := range tasks {
		out = append(out, a.taskOutput(t))
	}
	c.JSON(200, gin.H{"tasks": out})
}

type taskInput struct {
	Title           string     `json:"title" binding:"required,min=2,max=160"`
	Description     string     `json:"description" binding:"max=10000"`
	Priority        string     `json:"priority" binding:"required,oneof=low medium high critical"`
	UrgencyOverride string     `json:"urgency_override" binding:"required,oneof=auto green yellow red critical"`
	Epic            string     `json:"epic" binding:"max=100"`
	Tags            string     `json:"tags" binding:"max=400"`
	AssigneeID      *uint      `json:"assignee_id"`
	DueAt           *time.Time `json:"due_at"`
}

func (a *App) validAssignee(p Project, id *uint) bool {
	if id == nil {
		return true
	}
	var n int64
	a.db.Model(&Membership{}).Where("project_id = ? AND user_id = ?", p.ID, *id).Count(&n)
	return n > 0
}
func (a *App) createTask(c *gin.Context) {
	p, m, ok := mustProject(a, c)
	if !ok {
		return
	}
	if !canManage(m) {
		fail(c, 403, "Only managers can create tasks")
		return
	}
	var in taskInput
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "Invalid task: title, priority and urgency override are required")
		return
	}
	if !a.validAssignee(p, in.AssigneeID) {
		fail(c, 400, "Assignee must be a project member")
		return
	}
	t := Task{ProjectID: p.ID, Title: strings.TrimSpace(in.Title), Description: in.Description, Status: "todo", Priority: in.Priority, UrgencyOverride: in.UrgencyOverride, Epic: in.Epic, Tags: in.Tags, CreatedBy: current(c).ID, AssigneeID: in.AssigneeID, DueAt: in.DueAt}
	if a.db.Create(&t).Error != nil {
		fail(c, 500, "Unable to create task")
		return
	}
	a.statsCache.invalidate()
	c.JSON(201, gin.H{"task": a.taskOutput(t)})
}
func (a *App) updateTask(c *gin.Context) {
	p, m, ok := mustProject(a, c)
	if !ok {
		return
	}
	t, ok := a.mustTask(c, p)
	if !ok {
		return
	}
	if !canManage(m) { // assignee may start work, but cannot edit task fields
		if t.AssigneeID == nil || *t.AssigneeID != current(c).ID {
			fail(c, 403, "You cannot update this task")
			return
		}
		var in struct {
			Status string `json:"status" binding:"required,oneof=in_progress"`
		}
		if c.ShouldBindJSON(&in) != nil || t.Status != "todo" {
			fail(c, 400, "Assignee can only start a TODO task")
			return
		}
		a.db.Model(&t).Update("status", "in_progress")
		a.statsCache.invalidate()
		c.JSON(200, gin.H{"task": a.taskOutput(t)})
		return
	}
	if t.Status == "done" {
		fail(c, 409, "Completed tasks cannot be changed")
		return
	}
	if t.Status == "in_review" {
		fail(c, 409, "Review a submitted task before editing")
		return
	}
	var in taskInput
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "Invalid task fields")
		return
	}
	if !a.validAssignee(p, in.AssigneeID) {
		fail(c, 400, "Assignee must be a project member")
		return
	}
	err := a.db.Model(&t).Updates(map[string]any{"title": strings.TrimSpace(in.Title), "description": in.Description, "priority": in.Priority, "urgency_override": in.UrgencyOverride, "epic": in.Epic, "tags": in.Tags, "assignee_id": in.AssigneeID, "due_at": in.DueAt}).Error
	if err != nil {
		fail(c, 500, "Unable to save task")
		return
	}
	a.statsCache.invalidate()
	c.JSON(200, gin.H{"task": a.taskOutput(t)})
}
func (a *App) submitTask(c *gin.Context) {
	p, _, ok := mustProject(a, c)
	if !ok {
		return
	}
	t, ok := a.mustTask(c, p)
	if !ok {
		return
	}
	if t.AssigneeID == nil || *t.AssigneeID != current(c).ID {
		fail(c, 403, "Only assigned worker can submit a review")
		return
	}
	var in struct {
		Description string `json:"description" binding:"max=10000"`
		EvidenceURL string `json:"evidence_url" binding:"omitempty,url,max=1000"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "Invalid review fields")
		return
	}
	in.Description = strings.TrimSpace(in.Description)
	in.EvidenceURL = strings.TrimSpace(in.EvidenceURL)
	if in.Description == "" && in.EvidenceURL == "" {
		fail(c, 400, "Provide work description or evidence URL")
		return
	}
	if in.EvidenceURL != "" {
		u, err := url.Parse(in.EvidenceURL)
		if err != nil || !(u.Scheme == "https" || u.Scheme == "http") || u.Host == "" {
			fail(c, 400, "Evidence URL must be http(s)")
			return
		}
	}
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var locked Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ?", t.ID, p.ID).First(&locked).Error; err != nil {
			return err
		}
		if locked.Status != "todo" && locked.Status != "in_progress" {
			return errConflict
		}
		if err := tx.Model(&locked).Update("status", "in_review").Error; err != nil {
			return err
		}
		return tx.Create(&Review{TaskID: t.ID, SubmittedBy: current(c).ID, Description: in.Description, EvidenceURL: in.EvidenceURL, Status: "pending"}).Error
	})
	if err == errConflict {
		fail(c, 409, "Task is already in review or completed")
		return
	}
	if err != nil {
		fail(c, 500, "Unable to submit task")
		return
	}
	a.statsCache.invalidate()
	c.JSON(200, gin.H{"ok": true})
}
func (a *App) reviewTask(c *gin.Context) {
	p, m, ok := mustProject(a, c)
	if !ok {
		return
	}
	if !canManage(m) {
		fail(c, 403, "Manager or owner only")
		return
	}
	t, ok := a.mustTask(c, p)
	if !ok {
		return
	}
	var in struct {
		Decision string `json:"decision" binding:"required,oneof=approved rejected"`
		Comment  string `json:"comment" binding:"max=3000"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "Decision must be approved or rejected")
		return
	}
	reviewer := current(c).ID
	now := time.Now()
	err := a.db.Transaction(func(tx *gorm.DB) error {
		var locked Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ?", t.ID, p.ID).First(&locked).Error; err != nil {
			return err
		}
		if locked.Status != "in_review" {
			return errConflict
		}
		var rev Review
		if tx.Where("task_id = ? AND status = ?", t.ID, "pending").Order("id DESC").First(&rev).Error != nil {
			return errConflict
		}
		if rev.SubmittedBy == reviewer {
			return errSelfReview
		}
		if err := tx.Model(&rev).Updates(map[string]any{"status": in.Decision, "comment": strings.TrimSpace(in.Comment), "reviewer_id": reviewer, "reviewed_at": now}).Error; err != nil {
			return err
		}
		changes := map[string]any{"status": "in_progress", "completed_at": nil}
		if in.Decision == "approved" {
			changes["status"] = "done"
			changes["completed_at"] = now
		}
		return tx.Model(&locked).Updates(changes).Error
	})
	if err == errConflict {
		fail(c, 409, "No pending review")
		return
	}
	if err == errSelfReview {
		fail(c, 403, "You cannot approve your own work")
		return
	}
	if err != nil {
		fail(c, 500, "Review failed")
		return
	}
	a.statsCache.invalidate()
	c.JSON(200, gin.H{"ok": true})
}

var errConflict = errors.New("conflict")
var errSelfReview = errors.New("self review")