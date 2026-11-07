package controller

import (
	"github.com/gin-gonic/gin"
)

func (h *Controller) ListProjects(c *gin.Context) {
	items, e := h.S.Projects(Current(c), c.Query("q"))
	if e != nil {
		Fail(c, e)
		return
	}
	c.JSON(200, gin.H{"projects": items})
}
func (h *Controller) CreateProject(c *gin.Context) {
	var in struct {
		Name        string `json:"name" binding:"required,min=2,max=120"`
		Description string `json:"description" binding:"max=2000"`
	}
	if c.ShouldBindJSON(&in) != nil {
		failInput(c, "Project name must be 2-120 characters")
		return
	}
	out, e := h.S.CreateProject(Current(c), in.Name, in.Description)
	if e != nil {
		Fail(c, e)
		return
	}
	c.JSON(201, gin.H{"project": out})
}
func (h *Controller) GetProject(c *gin.Context) {
	id, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	out, members, e := h.S.ProjectDetails(Current(c), id)
	if e != nil {
		Fail(c, e)
		return
	}
	c.JSON(200, gin.H{"project": out, "members": members})
}
func (h *Controller) UpdateProject(c *gin.Context) {
	id, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	var in struct {
		Name        string `json:"name" binding:"required,min=2,max=120"`
		Description string `json:"description" binding:"max=2000"`
	}
	if c.ShouldBindJSON(&in) != nil {
		failInput(c, "Invalid project fields")
		return
	}
	out, e := h.S.UpdateProject(Current(c), id, in.Name, in.Description)
	if e != nil {
		Fail(c, e)
		return
	}
	c.JSON(200, gin.H{"project": out})
}
func (h *Controller) DeleteProject(c *gin.Context) {
	id, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	if e := h.S.DeleteProject(Current(c), id); e != nil {
		Fail(c, e)
		return
	}
	ok(c)
}
func (h *Controller) Invite(c *gin.Context) {
	id, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	var in struct {
		Email string `json:"email" binding:"required,email"`
	}
	if c.ShouldBindJSON(&in) != nil {
		failInput(c, "Enter a registered user email")
		return
	}
	if e := h.S.Invite(Current(c), id, in.Email); e != nil {
		Fail(c, e)
		return
	}
	c.JSON(201, gin.H{"ok": true})
}
func (h *Controller) ChangeMember(c *gin.Context) {
	id, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	userID, valid := ParamID(c, "userID")
	if !valid {
		return
	}
	var in struct {
		Role string `json:"role" binding:"required,oneof=owner manager member"`
	}
	if c.ShouldBindJSON(&in) != nil {
		failInput(c, "Role must be owner, manager or member")
		return
	}
	if e := h.S.ChangeMember(Current(c), id, userID, in.Role); e != nil {
		Fail(c, e)
		return
	}
	ok(c)
}
func (h *Controller) RemoveMember(c *gin.Context) {
	id, valid := ParamID(c, "projectID")
	if !valid {
		return
	}
	userID, valid := ParamID(c, "userID")
	if !valid {
		return
	}
	if e := h.S.RemoveMember(Current(c), id, userID); e != nil {
		Fail(c, e)
		return
	}
	ok(c)
}