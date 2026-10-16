// Package config loads application configuration from environment variables
// (optionally populated from a local .env file).
package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all settings required to run the application.
type Config struct {
	TelegramBotToken string
	DatabaseURL      string
	HTTPAddr         string
	// FamilyAdminTelegramID is the Telegram ID of the head of the family.
	// 0 means "not configured yet" — every new family member then
	// registers as a regular member until this is set.
	FamilyAdminTelegramID int64
}

// Load reads a .env file if present (real env vars always win) and builds
// a validated Config. It returns an error if a required value is missing
// or malformed.
func Load() (*Config, error) {
	// .env is optional: in production real environment variables are used
	// instead, so a missing file is not an error.
	_ = godotenv.Load()

	cfg := &Config{
		TelegramBotToken: os.Getenv("TELEGRAM_BOT_TOKEN"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		HTTPAddr:         envOrDefault("HTTP_ADDR", ":8080"),
	}

	if cfg.TelegramBotToken == "" {
		return nil, fmt.Errorf("config: TELEGRAM_BOT_TOKEN is required")
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("config: DATABASE_URL is required")
	}

	if raw := os.Getenv("FAMILY_ADMIN_TELEGRAM_ID"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("config: invalid FAMILY_ADMIN_TELEGRAM_ID: %w", err)
		}
		cfg.FamilyAdminTelegramID = id
	} else {
		log.Println("config: FAMILY_ADMIN_TELEGRAM_ID is not set — no family member will get admin rights yet")
	}

	return cfg, nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
