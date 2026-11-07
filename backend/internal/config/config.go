package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	Port, DatabaseURL, JWTSecret, AdminEmail, AdminPassword string
	CookieSecure                                            bool
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func Load() (Config, error) {
	c := Config{
		Port:          env("PORT", "8080"),
		DatabaseURL:   env("DATABASE_URL", "host=localhost user=tracker password=tracker dbname=tasktracker port=5432 sslmode=disable"),
		JWTSecret:     env("JWT_SECRET", ""),
		CookieSecure:  env("COOKIE_SECURE", "false") == "true",
		AdminEmail:    strings.ToLower(strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL"))),
		AdminPassword: os.Getenv("BOOTSTRAP_ADMIN_PASSWORD"),
	}
	if len(c.JWTSecret) < 32 {
		return c, errors.New("JWT_SECRET must be at least 32 characters")
	}
	if c.AdminEmail != "" || c.AdminPassword != "" {
		if c.AdminEmail == "" || len(c.AdminPassword) < 10 {
			return c, errors.New("BOOTSTRAP_ADMIN_EMAIL and BOOTSTRAP_ADMIN_PASSWORD (10+ chars) must both be configured")
		}
	}
	return c, nil
}