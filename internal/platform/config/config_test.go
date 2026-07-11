package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/padma")
	t.Setenv("FIBER_RPC_ENDPOINT", "http://127.0.0.1:8227")
	// Leave the optional vars unset to exercise defaults.
	t.Setenv("INGEST_SOURCE", "")
	t.Setenv("WEBHOOK_MAX_ATTEMPTS", "")
	t.Setenv("WEBHOOK_WORKERS", "")
	t.Setenv("HTTP_ADDR", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IngestSource != "fake" {
		t.Errorf("IngestSource default = %q, want fake", cfg.IngestSource)
	}
	if cfg.WebhookMaxAttempts != 8 {
		t.Errorf("WebhookMaxAttempts default = %d, want 8", cfg.WebhookMaxAttempts)
	}
	if cfg.WebhookWorkers != 4 {
		t.Errorf("WebhookWorkers default = %d, want 4", cfg.WebhookWorkers)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr default = %q, want :8080", cfg.HTTPAddr)
	}
}

func TestLoadParsesOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/x")
	t.Setenv("FIBER_RPC_ENDPOINT", "http://node:8227")
	t.Setenv("FIBER_BISCUIT_TOKEN", "tok")
	t.Setenv("INGEST_SOURCE", "polling")
	t.Setenv("WEBHOOK_MAX_ATTEMPTS", "12")
	t.Setenv("WEBHOOK_WORKERS", "9")
	t.Setenv("HTTP_ADDR", ":9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.FiberBiscuitToken != "tok" || cfg.IngestSource != "polling" ||
		cfg.WebhookMaxAttempts != 12 || cfg.WebhookWorkers != 9 || cfg.HTTPAddr != ":9090" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DATABASE_URL is unset, got nil")
	}
}

func TestLoadRejectsBadInt(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/x")
	t.Setenv("WEBHOOK_WORKERS", "notanumber")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-numeric WEBHOOK_WORKERS, got nil")
	}
}

func TestLoadRejectsBadIngestSource(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://db/x")
	t.Setenv("INGEST_SOURCE", "carrierpigeon")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid INGEST_SOURCE, got nil")
	}
}
