package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmd184psu/grocery-list/internal/config"
)

func TestDefaultConfig_SyncIntervalSecondsIsOne(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.SyncIntervalSeconds != 1 {
		t.Fatalf("SyncIntervalSeconds: got %d, want 1", cfg.SyncIntervalSeconds)
	}
}

func TestLoad_ConfigSyncIntervalSecondsFromJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grocery.json")
	data := []byte(`{"sync_interval_seconds": 9}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SyncIntervalSeconds != 9 {
		t.Fatalf("SyncIntervalSeconds: got %d, want 9", cfg.SyncIntervalSeconds)
	}
}

// ── title ──────────────────────────────────────────────────────────────────

func TestDefaultConfig_TitleIsGroceryList(t *testing.T) {
	cfg := config.DefaultConfig()
	if cfg.Title != config.DefaultTitle {
		t.Fatalf("Title: got %q, want %q", cfg.Title, config.DefaultTitle)
	}
}

func TestLoad_TitleFromJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grocery.json")
	data := []byte(`{"title": "Weekend Shop"}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Title != "Weekend Shop" {
		t.Fatalf("Title: got %q, want %q", cfg.Title, "Weekend Shop")
	}
}

func TestLoad_MissingTitleFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "grocery.json")
	// JSON with no title key at all.
	data := []byte(`{"port": 9090}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Title != config.DefaultTitle {
		t.Fatalf("Title: got %q, want %q", cfg.Title, config.DefaultTitle)
	}
}
