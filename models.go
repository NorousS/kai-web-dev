package main

import (
	"gorm.io/gorm"
	"time"
)

type User struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Name         string    `json:"name" gorm:"size:100;not null"`
	Email        string    `json:"email" gorm:"size:255;uniqueIndex;not null"`
	PasswordHash string    `json:"-" gorm:"not null"`
	Role         string    `json:"role" gorm:"size:12;not null;default:user"`
	CreatedAt    time.Time `json:"created_at"`
}
type Session struct {
	ID        uint      `gorm:"primaryKey"`
	UserID    uint      `gorm:"index;not null"`
	TokenHash string    `gorm:"uniqueIndex;size:64;not null"`
	ExpiresAt time.Time `gorm:"index;not null"`
	RevokedAt *time.Time
	CreatedAt time.Time
}
type Project struct {
	ID          uint      `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"size:120;not null"`
	Description string    `json:"description" gorm:"type:text"`
	CreatedBy   uint      `json:"created_by" gorm:"not null"`
	OwnerID     uint      `json:"owner_id" gorm:"not null;index"`
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

func migrate(db *gorm.DB) error {
	return db.AutoMigrate(&User{}, &Session{}, &Project{}, &Membership{}, &Task{}, &Review{})
}

func urgency(t Task) string {
	if t.UrgencyOverride != "" && t.UrgencyOverride != "auto" {
		return t.UrgencyOverride
	}
	if t.DueAt == nil || t.Status == "done" {
		return "green"
	}
	remaining := time.Until(*t.DueAt)
	if remaining <= 24*time.Hour {
		return "critical"
	}
	if remaining <= 72*time.Hour {
		return "red"
	}
	if remaining <= 7*24*time.Hour {
		return "yellow"
	}
	return "green"
}