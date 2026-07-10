package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDefault(t *testing.T) {
	cfg := NewDefault()
	if cfg.Concurrent < 1 {
		t.Errorf("expected concurrent >= 1, got %d", cfg.Concurrent)
	}
	if cfg.Notify != true {
		t.Errorf("expected notify default true, got %v", cfg.Notify)
	}
	if cfg.SourcePolicy != "trash" {
		t.Errorf("expected source policy trash, got %s", cfg.SourcePolicy)
	}
	if cfg.StableSamples != 3 {
		t.Errorf("expected stable samples 3, got %d", cfg.StableSamples)
	}
}

func TestLoad_NoFile(t *testing.T) {
	// Temporarily change HOME to a temp dir
	tempDir, _ := os.MkdirTemp("", "recwatch_test")
	defer os.RemoveAll(tempDir)

	t.Setenv("HOME", tempDir) // For macOS/Linux
	// On Windows usually UserProfile, but since user is mac, HOME/UserHomeDir works.

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	// Should be default
	if cfg.Preset != "faster" {
		t.Errorf("expected default preset 'faster', got %s", cfg.Preset)
	}
}

func TestLoad_WithFile(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "recwatch_test")
	defer os.RemoveAll(tempDir)
	t.Setenv("HOME", tempDir)

	configDir := filepath.Join(tempDir, ".config", "rec-watch")
	os.MkdirAll(configDir, 0755)

	yamlContent := `
preset: veryfast
crf: 18
watchDir: /tmp/watch
keywords:
  - test
`
	os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(yamlContent), 0644)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Preset != "veryfast" {
		t.Errorf("expected preset 'veryfast', got %s", cfg.Preset)
	}
	if cfg.CRF != 18 {
		t.Errorf("expected crf 18, got %d", cfg.CRF)
	}
	if len(cfg.Keywords) != 1 || cfg.Keywords[0] != "test" {
		t.Errorf("expected keywords [test], got %v", cfg.Keywords)
	}
}
