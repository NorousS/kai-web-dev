package router

import (
	"log"
	"net/http"

	"github.com/NorousS/kai-web-dev/backend/internal/controller"
	"github.com/NorousS/kai-web-dev/backend/internal/service"
	"github.com/NorousS/kai-web-dev/backend/internal/view"
	"github.com/gin-gonic/gin"
)

// registerHTMLRoutes connects the server-rendered View to MVC controllers.
// The existing /api routes remain available for Swagger and external clients.
func registerHTMLRoutes(r *gin.Engine, svc *service.Service, secureCookie bool) {
	h, err := controller.NewPages(svc, secureCookie)
	if err != nil {
		log.Panicf("HTML template initialization: %v", err)
	}
	css, err := view.Stylesheet()
	if err != nil {
		log.Panicf("CSS asset: %v", err)
	}
	r.GET("/static/style.css", func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=3600")
		c.Data(http.StatusOK, "text/css; charset=utf-8", css)
	})
	pages := r.Group("")
	pages.Use(h.CSRF())
	pages.GET("/login", h.LoginPage)
	pages.POST("/login", h.Login)
	pages.GET("/register", h.RegisterPage)
	pages.POST("/register", h.Register)
	private := pages.Group("")
	private.Use(h.RequireAuth())
	private.GET("/", h.Dashboard)
	private.POST("/logout", h.Logout)
	private.GET("/about", h.About)
	private.GET("/projects", h.Projects)
	private.POST("/projects", h.CreateProject)
	private.GET("/projects/:projectID", h.Project)
	private.POST("/projects/:projectID/update", h.UpdateProject)
	private.POST("/projects/:projectID/delete", h.DeleteProject)
	private.POST("/projects/:projectID/invite", h.Invite)
	private.POST("/projects/:projectID/members/:userID/role", h.ChangeMember)
	private.POST("/projects/:projectID/members/:userID/remove", h.RemoveMember)
	private.GET("/projects/:projectID/tasks/new", h.TaskForm)
	private.POST("/projects/:projectID/tasks", h.CreateTask)
	private.GET("/projects/:projectID/tasks/:taskID/edit", h.TaskForm)
	private.POST("/projects/:projectID/tasks/:taskID/update", h.UpdateTask)
	private.POST("/projects/:projectID/tasks/:taskID/start", h.StartTask)
	private.POST("/projects/:projectID/tasks/:taskID/submit", h.SubmitTask)
	private.POST("/projects/:projectID/tasks/:taskID/review", h.ReviewTask)
	private.GET("/admin", h.Admin)
	private.POST("/admin/users/:userID/role", h.AdminRole)
}