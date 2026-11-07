package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/mail"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func randomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (a *App) register(c *gin.Context) {
	var in struct {
		Name     string `json:"name" binding:"required,min=2,max=100"`
		Email    string `json:"email" binding:"required,email,max=255"`
		Password string `json:"password" binding:"required,min=10,max=128"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, 400, "Name, valid email and password (10+ characters) are required")
		return
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if _, err := mail.ParseAddress(in.Email); err != nil {
		fail(c, 400, "Invalid email")
		return
	}
	h, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(c, 500, "Unable to register")
		return
	}
	user := User{Name: strings.TrimSpace(in.Name), Email: in.Email, PasswordHash: string(h), Role: "user"}
	if err := a.db.Create(&user).Error; err != nil {
		fail(c, 409, "Email already registered")
		return
	}
	a.issue(c, user)
}
func (a *App) login(c *gin.Context) {
	var in struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		fail(c, 400, "Email and password required")
		return
	}
	var u User
	if err := a.db.Where("email = ?", strings.ToLower(strings.TrimSpace(in.Email))).First(&u).Error; err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)) != nil {
		fail(c, 401, "Incorrect email or password")
		return
	}
	a.issue(c, u)
}
func (a *App) logout(c *gin.Context) {
	s := c.MustGet("session").(Session)
	now := time.Now()
	a.db.Model(&s).Update("revoked_at", now)
	c.SetSameSite(2)
	c.SetCookie("tt_session", "", -1, "/", "", a.cookieSecure, true)
	c.JSON(200, gin.H{"ok": true})
}
func (a *App) me(c *gin.Context) { c.JSON(200, gin.H{"user": current(c)}) }
func (a *App) findUsers(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if len(q) < 2 {
		c.JSON(200, gin.H{"users": []User{}})
		return
	}
	var u []User
	a.db.Where("LOWER(name) LIKE ? OR LOWER(email) LIKE ?", "%"+strings.ToLower(q)+"%", "%"+strings.ToLower(q)+"%").Limit(15).Find(&u)
	c.JSON(200, gin.H{"users": u})
}
func (a *App) adminUsers(c *gin.Context) {
	if current(c).Role != "admin" {
		fail(c, 403, "Admin only")
		return
	}
	var u []User
	a.db.Order("id asc").Limit(200).Find(&u)
	c.JSON(200, gin.H{"users": u})
}
func (a *App) adminRole(c *gin.Context) {
	actor := current(c)
	if actor.Role != "admin" {
		fail(c, 403, "Admin only")
		return
	}
	id, ok := idParam(c, "userID")
	if !ok {
		return
	}
	var in struct {
		Role string `json:"role" binding:"required,oneof=admin user"`
	}
	if c.ShouldBindJSON(&in) != nil {
		fail(c, 400, "Role must be admin or user")
		return
	}
	if id == actor.ID && in.Role != "admin" {
		fail(c, 400, "Cannot demote your own admin account")
		return
	}
	if a.db.Model(&User{}).Where("id = ?", id).Update("role", in.Role).RowsAffected == 0 {
		fail(c, 404, "User not found")
		return
	}
	c.JSON(200, gin.H{"ok": true})
}

var _ = gorm.ErrRecordNotFound