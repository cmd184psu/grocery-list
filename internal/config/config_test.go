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
