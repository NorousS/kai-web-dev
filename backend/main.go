package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type App struct {
	db           *gorm.DB
	secret       []byte
	cookieSecure bool
	statsCache   *statsCache
}
type claims struct{ jwt.RegisteredClaims }

func env(k, fallback string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return fallback
}
func main() {
	dsn := env("DATABASE_URL", "host=localhost user=tracker password=tracker dbname=tracker port=5432 sslmode=disable")
	secret := env("JWT_SECRET", "")
	if len(secret) < 32 {
		log.Fatal("JWT_SECRET must contain at least 32 characters")
	}
	var db *gorm.DB
	var err error
	for i := 0; i < 30; i++ {
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("waiting for database: %v", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal(err)
	}
	if err = migrate(db); err != nil {
		log.Fatal(err)
	}
	a := &App{db: db, secret: []byte(secret), cookieSecure: env("COOKIE_SECURE", "false") == "true", statsCache: newStatsCache(10 * time.Second)}
	if err := a.bootstrapAdmin(); err != nil {
		log.Fatal(err)
	}
	router := a.routes()
	log.Printf("Task Tracker API ready on :%s", env("PORT", "8080"))
	log.Fatal(router.Run(":" + env("PORT", "8080")))
}
func (a *App) bootstrapAdmin() error {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL")))
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if email == "" && password == "" {
		return nil
	}
	if email == "" || len(password) < 10 {
		return errors.New("BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD (10+ chars) must both be set")
	}
	var count int64
	a.db.Model(&User{}).Where("email = ?", email).Count(&count)
	if count != 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return a.db.Create(&User{Name: "Administrator", Email: email, PasswordHash: string(hash), Role: "admin"}).Error
}
func fail(c *gin.Context, code int, msg string) { c.AbortWithStatusJSON(code, gin.H{"error": msg}) }
func idParam(c *gin.Context, k string) (uint, bool) {
	n, e := strconv.ParseUint(c.Param(k), 10, 64)
	if e != nil || n == 0 {
		fail(c, 400, "Invalid ID")
		return 0, false
	}
	return uint(n), true
}
func (a *App) routes() *gin.Engine {
	r := gin.Default()
	r.Use(gin.Recovery(), originGuard())
	r.GET("/health", func(c *gin.Context) {
		var n int64
		if err := a.db.Raw("SELECT 1").Scan(&n).Error; err != nil {
			fail(c, 503, "Database unavailable")
			return
		}
		c.JSON(200, gin.H{"ok": true})
	})
	r.GET("/openapi.yaml", func(c *gin.Context) { c.File("/app/docs/openapi.yaml") })
	r.GET("/swagger", func(c *gin.Context) { c.Data(200, "text/html; charset=utf-8", []byte(swaggerHTML)) })
	api := r.Group("/api")
	api.POST("/auth/register", a.register)
	api.POST("/auth/login", a.login)
	auth := api.Group("")
	auth.Use(a.authenticate())
	auth.POST("/auth/logout", a.logout)
	auth.GET("/auth/me", a.me)
	auth.GET("/users", a.findUsers)
	auth.GET("/dashboard", a.dashboard)
	auth.GET("/projects", a.listProjects)
	auth.POST("/projects", a.createProject)
	auth.GET("/projects/:projectID", a.getProject)
	auth.PATCH("/projects/:projectID", a.updateProject)
	auth.DELETE("/projects/:projectID", a.deleteProject)
	auth.POST("/projects/:projectID/invitations", a.invite)
	auth.PATCH("/projects/:projectID/members/:userID", a.changeMember)
	auth.DELETE("/projects/:projectID/members/:userID", a.removeMember)
	auth.GET("/projects/:projectID/tasks", a.listTasks)
	auth.POST("/projects/:projectID/tasks", a.createTask)
	auth.PATCH("/projects/:projectID/tasks/:taskID", a.updateTask)
	auth.POST("/projects/:projectID/tasks/:taskID/submit", a.submitTask)
	auth.POST("/projects/:projectID/tasks/:taskID/review", a.reviewTask)
	auth.GET("/admin/users", a.adminUsers)
	auth.PATCH("/admin/users/:userID/role", a.adminRole)
	return r
}
func originGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case "GET", "HEAD", "OPTIONS":
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		if origin != "" {
			valid := "http://" + c.Request.Host
			secure := "https://" + c.Request.Host
			if origin != valid && origin != secure {
				fail(c, 403, "Cross-origin requests are not allowed")
				return
			}
		}
		c.Next()
	}
}
func tokenHash(jti string) string { h := sha256.Sum256([]byte(jti)); return hex.EncodeToString(h[:]) }
func (a *App) issue(c *gin.Context, u User) {
	now := time.Now()
	jti := fmt.Sprintf("%d-%d-%s", u.ID, now.UnixNano(), randomID())
	exp := now.Add(7 * 24 * time.Hour)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims{jwt.RegisteredClaims{Subject: strconv.FormatUint(uint64(u.ID), 10), ID: jti, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp), Issuer: "task-tracker"}})
	signed, err := token.SignedString(a.secret)
	if err != nil {
		fail(c, 500, "Token signing failed")
		return
	}
	if err := a.db.Create(&Session{UserID: u.ID, TokenHash: tokenHash(jti), ExpiresAt: exp}).Error; err != nil {
		fail(c, 500, "Session error")
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("tt_session", signed, int((7 * 24 * time.Hour).Seconds()), "/", "", a.cookieSecure, true)
	c.JSON(200, gin.H{"user": u, "token": signed})
}
func (a *App) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := ""
		auth := c.GetHeader("Authorization")
		if strings.HasPrefix(auth, "Bearer ") {
			raw = strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		}
		if raw == "" {
			raw, _ = c.Cookie("tt_session")
		}
		if raw == "" {
			fail(c, 401, "Sign in required")
			return
		}
		cl := &claims{}
		tok, err := jwt.ParseWithClaims(raw, cl, func(t *jwt.Token) (any, error) {
			if t.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("unexpected signing method")
			}
			return a.secret, nil
		}, jwt.WithIssuer("task-tracker"), jwt.WithExpirationRequired())
		if err != nil || !tok.Valid || cl.ID == "" {
			fail(c, 401, "Invalid or expired session")
			return
		}
		id, err := strconv.ParseUint(cl.Subject, 10, 64)
		if err != nil {
			fail(c, 401, "Invalid session")
			return
		}
		var sess Session
		if err := a.db.Where("token_hash = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?", tokenHash(cl.ID), id, time.Now()).First(&sess).Error; err != nil {
			fail(c, 401, "Session expired or revoked")
			return
		}
		var user User
		if err := a.db.First(&user, id).Error; err != nil {
			fail(c, 401, "Account not found")
			return
		}
		c.Set("user", user)
		c.Set("session", sess)
		c.Next()
	}
}
func current(c *gin.Context) User { return c.MustGet("user").(User) }
func (a *App) member(c *gin.Context, projectID uint) (Membership, bool) {
	u := current(c)
	var m Membership
	if err := a.db.Where("project_id = ? AND user_id = ?", projectID, u.ID).First(&m).Error; err != nil {
		if u.Role == "admin" {
			return Membership{ProjectID: projectID, UserID: u.ID, Role: "admin"}, true
		}
		fail(c, 403, "You are not a member of this project")
		return m, false
	}
	return m, true
}
func canManage(m Membership) bool {
	return m.Role == "owner" || m.Role == "manager" || m.Role == "admin"
}
func mustProject(a *App, c *gin.Context) (Project, Membership, bool) {
	id, ok := idParam(c, "projectID")
	if !ok {
		return Project{}, Membership{}, false
	}
	var p Project
	if a.db.First(&p, id).Error != nil {
		fail(c, 404, "Project not found")
		return p, Membership{}, false
	}
	m, ok := a.member(c, id)
	return p, m, ok
}
func (a *App) mustTask(c *gin.Context, p Project) (Task, bool) {
	id, ok := idParam(c, "taskID")
	if !ok {
		return Task{}, false
	}
	var t Task
	if a.db.Where("id = ? AND project_id = ?", id, p.ID).First(&t).Error != nil {
		fail(c, 404, "Task not found")
		return t, false
	}
	return t, true
}

const swaggerHTML = `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Task Tracker API Docs</title><link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head><body><div id="swagger-ui"></div><script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script><script>SwaggerUIBundle({url:'/openapi.yaml',dom_id:'#swagger-ui',persistAuthorization:true})</script></body></html>`

// Compile-time check that bcrypt remains available in the authentication module.
var _ = bcrypt.DefaultCost
var _ = subtle.ConstantTimeCompare