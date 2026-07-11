// Package config loads padma's runtime configuration from environment variables
// (see spec-B.md §7). It keeps zero third-party dependencies: plain os.Getenv
// plus typed parsing and validation.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config is the fully-resolved runtime configuration shared by every mode
// (indexer, api, dispatcher).
type Config struct {
	FiberRPCEndpoint   string
	FiberBiscuitToken  string
	IngestSource       string // pubsub | polling | fake
	DatabaseURL        string
	WebhookMaxAttempts int
	WebhookWorkers     int
	HTTPAddr           string
}

// Load reads and validates configuration from the environment, applying the
// documented defaults. DATABASE_URL is always required; per-mode requirements
// (e.g. FIBER_RPC_ENDPOINT for indexer/api) are validated by each command.
func Load() (Config, error) {
	cfg := Config{
		FiberRPCEndpoint:  os.Getenv("FIBER_RPC_ENDPOINT"),
		FiberBiscuitToken: os.Getenv("FIBER_BISCUIT_TOKEN"),
		IngestSource:      envOr("INGEST_SOURCE", "fake"),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		HTTPAddr:          envOr("HTTP_ADDR", ":8080"),
	}

	var err error
	if cfg.WebhookMaxAttempts, err = envInt("WEBHOOK_MAX_ATTEMPTS", 8); err != nil {
		return Config{}, err
	}
	if cfg.WebhookWorkers, err = envInt("WEBHOOK_WORKERS", 4); err != nil {
		return Config{}, err
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("config: DATABASE_URL is required")
	}
	switch cfg.IngestSource {
	case "pubsub", "polling", "fake":
	default:
		return Config{}, fmt.Errorf("config: invalid INGEST_SOURCE %q (want pubsub|polling|fake)", cfg.IngestSource)
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("config: %s must be an integer: %w", key, err)
	}
	return n, nil
}
