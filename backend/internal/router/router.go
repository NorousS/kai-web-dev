// Package router registers Gin endpoints and assembles middleware + controllers.
package router

import (
	"net/http"

	"github.com/NorousS/kai-web-dev/backend/internal/controller"
	"github.com/NorousS/kai-web-dev/backend/internal/service"
	"github.com/NorousS/kai-web-dev/backend/internal/view"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func New(s *service.Service, db *gorm.DB, secureCookie bool) *gin.Engine {
	h := controller.New(s, secureCookie)
	r := gin.Default()
	r.Use(controller.OriginGuard())
	r.GET("/health", func(c *gin.Context) {
		var one int64
		if db.Raw("SELECT 1").Scan(&one).Error != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Database unavailable"})
			return
		}
		c.JSON(200, gin.H{"ok": true})
	})
	r.GET("/openapi.yaml", func(c *gin.Context) { c.File("/app/docs/openapi.yaml") })
	r.GET("/swagger", func(c *gin.Context) { c.Data(200, "text/html; charset=utf-8", []byte(view.SwaggerHTML)) })
	api := r.Group("/api")
	api.POST("/auth/register", h.Register)
	api.POST("/auth/login", h.Login)
	auth := api.Group("")
	auth.Use(h.RequireAuth())
	auth.POST("/auth/logout", h.Logout)
	auth.GET("/auth/me", h.Me)
	auth.GET("/users", h.FindUsers)
	auth.GET("/dashboard", h.Dashboard)
	auth.GET("/projects", h.ListProjects)
	auth.POST("/projects", h.CreateProject)
	auth.GET("/projects/:projectID", h.GetProject)
	auth.PATCH("/projects/:projectID", h.UpdateProject)
	auth.DELETE("/projects/:projectID", h.DeleteProject)
	auth.POST("/projects/:projectID/invitations", h.Invite)
	auth.PATCH("/projects/:projectID/members/:userID", h.ChangeMember)
	auth.DELETE("/projects/:projectID/members/:userID", h.RemoveMember)
	auth.GET("/projects/:projectID/tasks", h.ListTasks)
	auth.POST("/projects/:projectID/tasks", h.CreateTask)
	auth.PATCH("/projects/:projectID/tasks/:taskID", h.UpdateTask)
	auth.POST("/projects/:projectID/tasks/:taskID/submit", h.SubmitTask)
	auth.POST("/projects/:projectID/tasks/:taskID/review", h.ReviewTask)
	auth.GET("/admin/users", h.AdminUsers)
	auth.PATCH("/admin/users/:userID/role", h.AdminRole)
	return r
}