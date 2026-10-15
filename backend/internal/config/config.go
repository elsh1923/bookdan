// Package config reads runtime settings from the environment and refuses unsafe combinations.
package config

import (
	"errors"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Env         string
	HTTPAddr    string
	DatabaseURL string
	// DemoMode shows the OTP instead of sending an SMS. Must be off in production.
	DemoMode bool
}

// Load reads backend/.env when it exists (development convenience; real environment variables win),
// then the process environment.
func Load() Config {
	_ = godotenv.Load() // a missing .env is normal in production

	return Config{
		Env:         get("APP_ENV", "development"),
		HTTPAddr:    get("HTTP_ADDR", ":8080"),
		DatabaseURL: get("DATABASE_URL", ""),
		DemoMode:    get("DEMO_MODE", "false") == "true",
	}
}

func (c Config) IsProduction() bool { return c.Env == "production" }

// Validate rejects configurations that must never run, so a misconfigured deploy fails at startup.
func (c Config) Validate() error {
	if c.IsProduction() {
		if c.DemoMode {
			return errors.New("DEMO_MODE must be off when APP_ENV=production")
		}
		if c.DatabaseURL == "" {
			return errors.New("DATABASE_URL is required when APP_ENV=production")
		}
	}
	return nil
}

func get(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}
