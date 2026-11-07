package controller

import (
	"net/http"
	"strings"

	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"github.com/gin-gonic/gin"
)

func (h *Controller) issue(c *gin.Context, u model.User) {
	token, e := h.S.Issue(u)
	if e != nil {
		Fail(c, e)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("tt_session", token, int((7 * 24 * 60 * 60)), "/", "", h.CookieSecure, true)
	c.JSON(200, gin.H{"user": u, "token": token})
}
func (h *Controller) Register(c *gin.Context) {
	var in struct {
		Name     string `json:"name" binding:"required,min=2,max=100"`
		Email    string `json:"email" binding:"required,email,max=255"`
		Password string `json:"password" binding:"required,min=10,max=128"`
	}
	if c.ShouldBindJSON(&in) != nil {
		failInput(c, "Name, valid email and password (10+ characters) are required")
		return
	}
	u, e := h.S.Register(in.Name, in.Email, in.Password)
	if e != nil {
		Fail(c, e)
		return
	}
	h.issue(c, u)
}
func (h *Controller) Login(c *gin.Context) {
	var in struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if c.ShouldBindJSON(&in) != nil {
		failInput(c, "Email and password required")
		return
	}
	u, e := h.S.Login(in.Email, in.Password)
	if e != nil {
		Fail(c, e)
		return
	}
	h.issue(c, u)
}
func (h *Controller) Logout(c *gin.Context) {
	sess := c.MustGet("session").(model.Session)
	if e := h.S.Logout(sess.ID); e != nil {
		Fail(c, e)
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("tt_session", "", -1, "/", "", h.CookieSecure, true)
	ok(c)
}
func (h *Controller) Me(c *gin.Context) { c.JSON(200, gin.H{"user": Current(c)}) }
func (h *Controller) FindUsers(c *gin.Context) {
	u, e := h.S.SearchUsers(strings.TrimSpace(c.Query("q")))
	if e != nil {
		Fail(c, e)
		return
	}
	c.JSON(200, gin.H{"users": u})
}
func (h *Controller) AdminUsers(c *gin.Context) {
	u, e := h.S.AdminUsers(Current(c))
	if e != nil {
		Fail(c, e)
		return
	}
	c.JSON(200, gin.H{"users": u})
}
func (h *Controller) AdminRole(c *gin.Context) {
	id, valid := ParamID(c, "userID")
	if !valid {
		return
	}
	var in struct {
		Role string `json:"role" binding:"required,oneof=admin user"`
	}
	if c.ShouldBindJSON(&in) != nil {
		failInput(c, "Role must be admin or user")
		return
	}
	if e := h.S.AdminRole(Current(c), id, in.Role); e != nil {
		Fail(c, e)
		return
	}
	ok(c)
}
func (h *Controller) Dashboard(c *gin.Context) {
	out, e := h.S.Dashboard(Current(c))
	if e != nil {
		Fail(c, e)
		return
	}
	c.JSON(200, out)
}