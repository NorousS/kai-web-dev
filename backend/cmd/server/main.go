package main

import (
	"log"
	"time"

	"github.com/NorousS/kai-web-dev/backend/internal/config"
	"github.com/NorousS/kai-web-dev/backend/internal/migration"
	"github.com/NorousS/kai-web-dev/backend/internal/repository"
	"github.com/NorousS/kai-web-dev/backend/internal/router"
	"github.com/NorousS/kai-web-dev/backend/internal/service"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	var db *gorm.DB
	for i := 0; i < 30; i++ {
		db, err = gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("waiting for database: %v", err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal(err)
	}
	if err = migration.Apply(db); err != nil {
		log.Fatal("database migration: ", err)
	}
	svc := service.New(repository.New(db), cfg.JWTSecret)
	if err = svc.BootstrapAdmin(cfg.AdminEmail, cfg.AdminPassword); err != nil {
		log.Fatal(err)
	}
	log.Printf("Task Tracker API ready on :%s", cfg.Port)
	log.Fatal(router.New(svc, db, cfg.CookieSecure).Run(":" + cfg.Port))
}