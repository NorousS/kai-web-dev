// Package model contains the domain entities and GORM persistence metadata (MVC Model).
package model

import "time"

type User struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Name         string    `json:"name" gorm:"size:100;not null"`
	Email        string    `json:"email" gorm:"size:255;uniqueIndex;not null"`
	PasswordHash string    `json:"-" gorm:"not null"`
	Role         string    `json:"role" gorm:"size:12;not null;default:user"`
	CreatedAt    time.Time `json:"created_at"`
}

type Session struct {
	ID        uint       `json:"-" gorm:"primaryKey"`
	UserID    uint       `json:"-" gorm:"index;not null"`
	TokenHash string     `json:"-" gorm:"uniqueIndex;size:64;not null"`
	ExpiresAt time.Time  `json:"-" gorm:"index;not null"`
	RevokedAt *time.Time `json:"-"`
	CreatedAt time.Time  `json:"-"`
}

type Project struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:120;not null"`
	Description string    `json:"description" gorm:"type:text"`
	CreatedBy   uint      `json:"created_by" gorm:"not null"`
	OwnerID     uint      `json:"owner_id" gorm:"index;not null"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Membership struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	ProjectID uint      `json:"project_id" gorm:"uniqueIndex:membership_unique;not null"`
	UserID    uint      `json:"user_id" gorm:"uniqueIndex:membership_unique;not null"`
	Role      string    `json:"role" gorm:"size:12;not null"`
	CreatedAt time.Time `json:"created_at"`
}

type Task struct {
	ID              uint       `json:"id" gorm:"primaryKey"`
	ProjectID       uint       `json:"project_id" gorm:"index;not null"`
	Title           string     `json:"title" gorm:"size:160;not null"`
	Description     string     `json:"description" gorm:"type:text"`
	Status          string     `json:"status" gorm:"size:20;not null;default:todo"`
	Priority        string     `json:"priority" gorm:"size:12;not null;default:medium"`
	UrgencyOverride string     `json:"urgency_override" gorm:"size:12;not null;default:auto"`
	Tags            string     `json:"tags" gorm:"size:400"`
	Epic            string     `json:"epic" gorm:"size:100"`
	CreatedBy       uint       `json:"created_by" gorm:"not null"`
	AssigneeID      *uint      `json:"assignee_id" gorm:"index"`
	DueAt           *time.Time `json:"due_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Review struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	TaskID      uint       `json:"task_id" gorm:"index;not null"`
	SubmittedBy uint       `json:"submitted_by" gorm:"not null"`
	Description string     `json:"description" gorm:"type:text"`
	EvidenceURL string     `json:"evidence_url" gorm:"size:1000"`
	Status      string     `json:"status" gorm:"size:12;not null"`
	ReviewerID  *uint      `json:"reviewer_id"`
	Comment     string     `json:"comment" gorm:"type:text"`
	CreatedAt   time.Time  `json:"created_at"`
	ReviewedAt  *time.Time `json:"reviewed_at"`
}

func Urgency(t Task, now time.Time) string {
	if t.UrgencyOverride != "" && t.UrgencyOverride != "auto" {
		return t.UrgencyOverride
	}
	if t.DueAt == nil || t.Status == "done" {
		return "green"
	}
	remaining := t.DueAt.Sub(now)
	switch {
	case remaining <= 24*time.Hour:
		return "critical"
	case remaining <= 72*time.Hour:
		return "red"
	case remaining <= 7*24*time.Hour:
		return "yellow"
	default:
		return "green"
	}
}