// Package repository is the only layer that talks directly to GORM and SQL.
package repository

import (
	"strings"
	"time"

	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"gorm.io/gorm"
)

type Store struct{ db *gorm.DB }

func New(db *gorm.DB) *Store { return &Store{db: db} }
func (s *Store) Tx(fn func(*Store) error) error {
	return s.db.Transaction(func(tx *gorm.DB) error { return fn(New(tx)) })
}

func (s *Store) CreateUser(u *model.User) error { return s.db.Create(u).Error }
func (s *Store) UserByEmail(email string) (model.User, error) {
	var u model.User
	e := s.db.Where("email = ?", email).First(&u).Error
	return u, e
}
func (s *Store) UserByID(id uint) (model.User, error) {
	var u model.User
	e := s.db.First(&u, id).Error
	return u, e
}
func (s *Store) SearchUsers(q string) ([]model.User, error) {
	var out []model.User
	q = "%" + strings.ToLower(q) + "%"
	e := s.db.Where("LOWER(name) LIKE ? OR LOWER(email) LIKE ?", q, q).Limit(15).Find(&out).Error
	return out, e
}
func (s *Store) ListUsers() ([]model.User, error) {
	var out []model.User
	e := s.db.Order("id asc").Limit(200).Find(&out).Error
	return out, e
}
func (s *Store) SetUserRole(id uint, role string) (bool, error) {
	r := s.db.Model(&model.User{}).Where("id = ?", id).Update("role", role)
	return r.RowsAffected > 0, r.Error
}
func (s *Store) CreateSession(sess *model.Session) error { return s.db.Create(sess).Error }
func (s *Store) SessionByToken(id uint, hash string, now time.Time) (model.Session, error) {
	var x model.Session
	e := s.db.Where("token_hash = ? AND user_id = ? AND revoked_at IS NULL AND expires_at > ?", hash, id, now).First(&x).Error
	return x, e
}
func (s *Store) RevokeSession(id uint, now time.Time) error {
	return s.db.Model(&model.Session{}).Where("id = ?", id).Update("revoked_at", now).Error
}