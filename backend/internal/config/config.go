// Package config reads the service configuration from environment variables.
package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port string

	SpannerProject  string
	SpannerInstance string
	SpannerDatabase string
	// SpannerAutoSetup creates the instance, database and schema on boot.
	// Meant for the local emulator, where nothing exists until we create it.
	SpannerAutoSetup bool

	// Optional integrations: each one is disabled when its settings are empty.
	GeminiAPIKey  string
	GeminiModel   string
	GeminiBaseURL string

	DriveCredentialsFile string
	DriveFolderID        string

	WorkflowURL string
}

func Load() Config {
	return Config{
		Port:             env("PORT", "8080"),
		SpannerProject:   env("SPANNER_PROJECT", "tcit-local"),
		SpannerInstance:  env("SPANNER_INSTANCE", "posts-instance"),
		SpannerDatabase:  env("SPANNER_DATABASE", "posts-db"),
		SpannerAutoSetup: env("SPANNER_AUTO_SETUP", "false") == "true",

		GeminiAPIKey:  os.Getenv("GEMINI_API_KEY"),
		GeminiModel:   env("GEMINI_MODEL", "gemini-flash-lite-latest"),
		GeminiBaseURL: env("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com/v1beta"),

		DriveCredentialsFile: os.Getenv("DRIVE_CREDENTIALS_FILE"),
		DriveFolderID:        os.Getenv("DRIVE_FOLDER_ID"),

		WorkflowURL: os.Getenv("WORKFLOW_URL"),
	}
}

func (c Config) SpannerInstancePath() string {
	return fmt.Sprintf("projects/%s/instances/%s", c.SpannerProject, c.SpannerInstance)
}

func (c Config) SpannerDatabasePath() string {
	return fmt.Sprintf("%s/databases/%s", c.SpannerInstancePath(), c.SpannerDatabase)
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
