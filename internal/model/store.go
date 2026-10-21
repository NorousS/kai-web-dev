package model

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func databaseError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		return ErrConflict
	}
	return err
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func (s *Store) CreateUser(ctx context.Context, username, email, passwordHash, role string) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx, `INSERT INTO users (username,email,password,role) VALUES ($1,$2,$3,$4)
		RETURNING id,username,email,role,is_blocked`, username, normalizeEmail(email), passwordHash, role).
		Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.IsBlocked)
	return u, databaseError(err)
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx, `SELECT id,username,email,password,role,is_blocked FROM users WHERE lower(email)=$1`, normalizeEmail(email)).
		Scan(&u.ID, &u.Username, &u.Email, &u.Password, &u.Role, &u.IsBlocked)
	return u, databaseError(err)
}

func (s *Store) UserByID(ctx context.Context, id int) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx, `SELECT id,username,email,role,is_blocked FROM users WHERE id=$1`, id).
		Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.IsBlocked)
	return u, databaseError(err)
}

func affected(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return databaseError(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) UpdateProfile(ctx context.Context, id int, username, email string) error {
	return affected(s.Pool.Exec(ctx, `UPDATE users SET username=$2,email=$3 WHERE id=$1`, id, username, normalizeEmail(email)))
}

func (s *Store) CreateSession(ctx context.Context, userID int, hash string, expires time.Time) error {
	// Serialize session creation with blocking so a concurrent login cannot leave a live session.
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var blocked bool
	if err = tx.QueryRow(ctx, `SELECT is_blocked FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&blocked); err != nil {
		return databaseError(err)
	}
	if blocked {
		return ErrForbidden
	}
	if _, err = tx.Exec(ctx, `INSERT INTO sessions (token_hash,user_id,expires_at) VALUES ($1,$2,$3)`, hash, userID, expires); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) SessionUser(ctx context.Context, hash string) (User, error) {
	var u User
	err := s.Pool.QueryRow(ctx, `SELECT u.id,u.username,u.email,u.role,u.is_blocked FROM users u
		JOIN sessions s ON s.user_id=u.id WHERE s.token_hash=$1 AND s.expires_at>now() AND NOT u.is_blocked`, hash).
		Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.IsBlocked)
	return u, databaseError(err)
}

func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, hash)
	return err
}

func (s *Store) CreateTask(ctx context.Context, userID int, t Task) (int, error) {
	if t.Status == "" {
		t.Status = "new"
	}
	if t.Priority == "" {
		t.Priority = "medium"
	}
	var id int
	err := s.Pool.QueryRow(ctx, `INSERT INTO tasks (user_id,title,description,status,priority,due_date)
		VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`, userID, t.Title, t.Description, t.Status, t.Priority, t.DueDate).Scan(&id)
	return id, err
}

const taskColumns = `id,user_id,title,description,status,priority,due_date,created_at`

func scanTask(row pgx.Row) (Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.UserID, &t.Title, &t.Description, &t.Status, &t.Priority, &t.DueDate, &t.CreatedAt)
	return t, databaseError(err)
}

func (s *Store) Tasks(ctx context.Context, userID int, all bool, q, status string) ([]Task, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+taskColumns+` FROM tasks
		WHERE ($2 OR user_id=$1) AND ($3='' OR strpos(lower(title),lower($3))>0)
		AND ($4='' OR status=$4) ORDER BY created_at DESC,id DESC`, userID, all, q, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (s *Store) TaskByID(ctx context.Context, id, userID int, all bool) (Task, error) {
	return scanTask(s.Pool.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE id=$1 AND ($3 OR user_id=$2)`, id, userID, all))
}

func (s *Store) UpdateTask(ctx context.Context, id, userID int, t Task) error {
	return affected(s.Pool.Exec(ctx, `UPDATE tasks SET title=$3,description=$4,status=$5,priority=$6,due_date=$7
		WHERE id=$1 AND user_id=$2`, id, userID, t.Title, t.Description, t.Status, t.Priority, t.DueDate))
}

func (s *Store) DeleteTask(ctx context.Context, id, userID int, all bool) error {
	return affected(s.Pool.Exec(ctx, `DELETE FROM tasks WHERE id=$1 AND ($3 OR user_id=$2)`, id, userID, all))
}

func (s *Store) Comments(ctx context.Context, taskID int) ([]Comment, error) {
	rows, err := s.Pool.Query(ctx, `SELECT c.id,c.task_id,c.user_id,c.text,u.username,c.created_at
		FROM comments c JOIN users u ON u.id=c.user_id WHERE c.task_id=$1 ORDER BY c.created_at,c.id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Comment{}
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.TaskID, &c.UserID, &c.Text, &c.Username, &c.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

func (s *Store) AddComment(ctx context.Context, taskID, userID int, text string) error {
	return affected(s.Pool.Exec(ctx, `INSERT INTO comments (task_id,user_id,text)
		SELECT id,$2,$3 FROM tasks WHERE id=$1 AND user_id=$2`, taskID, userID, text))
}

func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,username,email,role,is_blocked FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Email, &u.Role, &u.IsBlocked); err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

func (s *Store) SetBlocked(ctx context.Context, id int, blocked bool) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var role string
	if err = tx.QueryRow(ctx, `SELECT role FROM users WHERE id=$1 FOR UPDATE`, id).Scan(&role); err != nil {
		return databaseError(err)
	}
	if role == "admin" {
		return ErrForbidden
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET is_blocked=$2 WHERE id=$1`, id, blocked); err != nil {
		return err
	}
	if blocked {
		if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
