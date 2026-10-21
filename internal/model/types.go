package model

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("email already exists")
var ErrForbidden = errors.New("forbidden")

type User struct {
	ID                              int
	Username, Email, Password, Role string
	IsBlocked                       bool
}
type Task struct {
	ID, UserID                           int
	Title, Description, Status, Priority string
	DueDate                              *time.Time
	CreatedAt                            time.Time
}
type Comment struct {
	ID, TaskID, UserID int
	Text, Username     string
	CreatedAt          time.Time
}
type Store struct{ Pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{Pool: pool} }
