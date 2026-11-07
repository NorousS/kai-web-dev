// Package service owns business rules, permissions and use-case orchestration.
// It never imports Gin or deals with HTTP request/response objects.
package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/NorousS/kai-web-dev/backend/internal/cache"
	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"github.com/NorousS/kai-web-dev/backend/internal/repository"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string        { return e.Message }
func bad(code int, text string) error { return &Error{Status: code, Message: text} }
func dbError(err error, text string) error {
	if err != nil {
		return bad(500, text)
	}
	return nil
}

var ErrConflict = errors.New("task state conflict")
var ErrSelfReview = errors.New("self review")

type Service struct {
	repo   *repository.Store
	secret []byte
	cache  *cache.Dashboard
}

func New(repo *repository.Store, secret string) *Service {
	return &Service{repo: repo, secret: []byte(secret), cache: cache.NewDashboard(10 * time.Second)}
}
func (s *Service) invalidate() { s.cache.Invalidate() }
func hash(raw string) string   { h := sha256.Sum256([]byte(raw)); return hex.EncodeToString(h[:]) }
func randomToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

type tokenClaims struct{ jwt.RegisteredClaims }

func (s *Service) Issue(u model.User) (string, error) {
	now := time.Now()
	expires := now.Add(7 * 24 * time.Hour)
	jti := fmt.Sprintf("%d-%d-%s", u.ID, now.UnixNano(), randomToken())
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, tokenClaims{jwt.RegisteredClaims{Subject: strconv.FormatUint(uint64(u.ID), 10), ID: jti, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires), Issuer: "task-tracker"}})
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", bad(500, "Token signing failed")
	}
	if err = s.repo.CreateSession(&model.Session{UserID: u.ID, TokenHash: hash(jti), ExpiresAt: expires}); err != nil {
		return "", bad(500, "Session error")
	}
	return signed, nil
}
func (s *Service) Authenticate(raw string) (model.User, model.Session, error) {
	var emptyUser model.User
	var emptySession model.Session
	cl := &tokenClaims{}
	tok, err := jwt.ParseWithClaims(raw, cl, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected JWT signing method")
		}
		return s.secret, nil
	}, jwt.WithIssuer("task-tracker"), jwt.WithExpirationRequired())
	if err != nil || !tok.Valid || cl.ID == "" {
		return emptyUser, emptySession, bad(401, "Invalid or expired session")
	}
	id, e := strconv.ParseUint(cl.Subject, 10, 64)
	if e != nil || id == 0 {
		return emptyUser, emptySession, bad(401, "Invalid session")
	}
	sess, e := s.repo.SessionByToken(uint(id), hash(cl.ID), time.Now())
	if e != nil {
		return emptyUser, emptySession, bad(401, "Session expired or revoked")
	}
	u, e := s.repo.UserByID(uint(id))
	if e != nil {
		return emptyUser, emptySession, bad(401, "Account not found")
	}
	return u, sess, nil
}
func (s *Service) Register(name, email, password string) (model.User, error) {
	name = strings.TrimSpace(name)
	email = strings.ToLower(strings.TrimSpace(email))
	if len(name) < 2 || len(name) > 100 || len(password) < 10 || len(password) > 128 {
		return model.User{}, bad(400, "Invalid name or password")
	}
	pass, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return model.User{}, bad(500, "Unable to register")
	}
	u := model.User{Name: name, Email: email, PasswordHash: string(pass), Role: "user"}
	if err = s.repo.CreateUser(&u); err != nil {
		return model.User{}, bad(409, "Email already registered")
	}
	return u, nil
}
func (s *Service) Login(email, password string) (model.User, error) {
	u, err := s.repo.UserByEmail(strings.ToLower(strings.TrimSpace(email)))
	if err != nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return model.User{}, bad(401, "Incorrect email or password")
	}
	return u, nil
}
func (s *Service) Logout(sessionID uint) error {
	return dbError(s.repo.RevokeSession(sessionID, time.Now()), "Unable to log out")
}
func (s *Service) BootstrapAdmin(email, password string) error {
	if email == "" && password == "" {
		return nil
	}
	if email == "" || len(password) < 10 {
		return errors.New("Both admin email and password (10+ characters) are required")
	}
	if _, err := s.repo.UserByEmail(email); err == nil {
		return nil
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.repo.CreateUser(&model.User{Name: "Administrator", Email: email, PasswordHash: string(h), Role: "admin"})
}
func (s *Service) SearchUsers(q string) ([]model.User, error) {
	if len(strings.TrimSpace(q)) < 2 {
		return []model.User{}, nil
	}
	out, e := s.repo.SearchUsers(strings.TrimSpace(q))
	if e != nil {
		return nil, bad(500, "Could not find users")
	}
	return out, nil
}
func (s *Service) AdminUsers(actor model.User) ([]model.User, error) {
	if actor.Role != "admin" {
		return nil, bad(403, "Admin only")
	}
	out, e := s.repo.ListUsers()
	if e != nil {
		return nil, bad(500, "Could not load users")
	}
	return out, nil
}
func (s *Service) AdminRole(actor model.User, userID uint, role string) error {
	if actor.Role != "admin" {
		return bad(403, "Admin only")
	}
	if role != "admin" && role != "user" {
		return bad(400, "Invalid global role")
	}
	if actor.ID == userID && role != "admin" {
		return bad(400, "Cannot demote your own admin account")
	}
	ok, e := s.repo.SetUserRole(userID, role)
	if e != nil {
		return bad(500, "Unable to update user")
	}
	if !ok {
		return bad(404, "User not found")
	}
	return nil
}
func (s *Service) Dashboard(actor model.User) (repository.DashboardStats, error) {
	if cached, ok := s.cache.Get(actor.ID); ok {
		if x, valid := cached.(repository.DashboardStats); valid {
			return x, nil
		}
	}
	x, e := s.repo.Dashboard(actor)
	if e != nil {
		return x, bad(500, "Unable to load dashboard")
	}
	s.cache.Put(actor.ID, x)
	return x, nil
}