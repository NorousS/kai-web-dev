package controller

import (
	"strconv"
	"strings"

	"github.com/NorousS/kai-web-dev/backend/internal/repository"
	"github.com/NorousS/kai-web-dev/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *Controller) ListTasks(c *gin.Context) {
	id, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	filter := repository.TaskFilter{Status: c.Query("status"), Priority: c.Query("priority"), Query: strings.TrimSpace(c.Query("q"))}
	if raw := c.Query("assignee"); raw != "" {
		if n, e := strconv.ParseUint(raw, 10, 64); e == nil && n != 0 {
			v := uint(n)
			filter.AssigneeID = &v
		}
	}
	out, e := h.S.Tasks(Current(c), id, filter)
	if e != nil {
		Fail(c, e)
		return
	}
	c.JSON(200, gin.H{"tasks": out})
}
func (h *Controller) CreateTask(c *gin.Context) {
	id, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	var in service.TaskInput
	if c.ShouldBindJSON(&in) != nil {
		failInput(c, "Invalid task: title, priority and urgency override are required")
		return
	}
	out, e := h.S.CreateTask(Current(c), id, in)
	if e != nil {
		Fail(c, e)
		return
	}
	c.JSON(201, gin.H{"task": out})
}
func (h *Controller) UpdateTask(c *gin.Context) {
	projectID, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	taskID, valid := ParamID(c, "taskID")
	if !valid {
		return
	}
	// Worker transitions carry only {"status":"in_progress"}. Manager edits
	// carry the complete, validated TaskInput. These are separate request shapes.
	var raw map[string]any
	if c.ShouldBindJSON(&raw) != nil {
		failInput(c, "Invalid task fields")
		return
	}
	var in service.TaskInput
	workerStatus := ""
	if raw["title"] == nil {
		if len(raw) != 1 || raw["status"] != "in_progress" {
			failInput(c, "Assignee can only start a TODO task")
			return
		}
		workerStatus = "in_progress"
	} else {
		// Use Gin's registered validator for the same constraints as createTask.
		// Request is decoded a second time from the already-parsed map.
		if e := parseTaskInput(raw, &in); e != nil {
			failInput(c, "Invalid task fields")
			return
		}
	}
	out, e := h.S.UpdateTask(Current(c), projectID, taskID, in, workerStatus)
	if e != nil {
		Fail(c, e)
		return
	}
	c.JSON(200, gin.H{"task": out})
}
func (h *Controller) SubmitTask(c *gin.Context) {
	projectID, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	taskID, valid := ParamID(c, "taskID")
	if !valid {
		return
	}
	var in struct {
		Description string `json:"description" binding:"max=10000"`
		EvidenceURL string `json:"evidence_url" binding:"omitempty,url,max=1000"`
	}
	if c.ShouldBindJSON(&in) != nil {
		failInput(c, "Invalid review fields")
		return
	}
	if e := h.S.SubmitTask(Current(c), projectID, taskID, in.Description, in.EvidenceURL); e != nil {
		Fail(c, e)
		return
	}
	ok(c)
}
func (h *Controller) ReviewTask(c *gin.Context) {
	projectID, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	taskID, valid := ParamID(c, "taskID")
	if !valid {
		return
	}
	var in struct {
		Decision string `json:"decision" binding:"required,oneof=approved rejected"`
		Comment  string `json:"comment" binding:"max=3000"`
	}
	if c.ShouldBindJSON(&in) != nil {
		failInput(c, "Decision must be approved or rejected")
		return
	}
	if e := h.S.ReviewTask(Current(c), projectID, taskID, in.Decision, in.Comment); e != nil {
		Fail(c, e)
		return
	}
	ok(c)
}