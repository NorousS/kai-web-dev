// Package migration implements versioned Code First schema migrations.
package migration

import (
	"time"

	"github.com/NorousS/kai-web-dev/backend/internal/model"
	"gorm.io/gorm"
)

type schemaMigration struct {
	Version   uint `gorm:"primaryKey"`
	AppliedAt time.Time
}

func (schemaMigration) TableName() string { return "schema_migrations" }

// Apply is safe for an existing database originally managed through AutoMigrate.
// New schema changes should be added as new numbered migration functions.
func Apply(db *gorm.DB) error {
	if err := db.AutoMigrate(&schemaMigration{}); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&schemaMigration{}).Where("version = ?", 1).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.AutoMigrate(&model.User{}, &model.Session{}, &model.Project{}, &model.Membership{}, &model.Task{}, &model.Review{}); err != nil {
			return err
		}
		return tx.Create(&schemaMigration{Version: 1, AppliedAt: time.Now().UTC()}).Error
	})
}