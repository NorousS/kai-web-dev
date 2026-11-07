// Package controller implements the MVC Controller: parse HTTP input, call services, return JSON.
// SQL queries and business decisions are intentionally not implemented here.
package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"github.com/NorousS/kai-web-dev/backend/internal/service"
	"github.com/gin-gonic/gin"
)

type Controller struct {
	S            *service.Service
	CookieSecure bool
}

func New(s *service.Service, secure bool) *Controller { return &Controller{S: s, CookieSecure: secure} }
func Fail(c *gin.Context, err error) {
	var fault *service.Error
	if errors.As(err, &fault) {
		c.AbortWithStatusJSON(fault.Status, gin.H{"error": fault.Message})
		return
	}
	c.AbortWithStatusJSON(500, gin.H{"error": "Internal server error"})
}
func failInput(c *gin.Context, text string) { c.AbortWithStatusJSON(400, gin.H{"error": text}) }
func ParamID(c *gin.Context, k string) (uint, bool) {
	id, e := strconv.ParseUint(c.Param(k), 10, 64)
	if e != nil || id == 0 {
		failInput(c, "Invalid ID")
		return 0, false
	}
	return uint(id), true
}
func Current(c *gin.Context) model.User { return c.MustGet("user").(model.User) }
func (h *Controller) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := ""
		if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
			raw = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
		if raw == "" {
			raw, _ = c.Cookie("tt_session")
		}
		if raw == "" {
			c.AbortWithStatusJSON(401, gin.H{"error": "Sign in required"})
			return
		}
		u, s, err := h.S.Authenticate(raw)
		if err != nil {
			Fail(c, err)
			return
		}
		c.Set("user", u)
		c.Set("session", s)
		c.Next()
	}
}
func OriginGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case "GET", "HEAD", "OPTIONS":
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		if origin != "" && origin != "http://"+c.Request.Host && origin != "https://"+c.Request.Host {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Cross-origin requests are not allowed"})
			return
		}
		c.Next()
	}
}
func ok(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) }