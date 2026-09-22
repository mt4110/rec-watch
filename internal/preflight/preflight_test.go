package preflight

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mt4110/rec-watch/internal/config"
)

func TestRequireRuntimeFailsWithoutFFmpeg(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cfg := config.NewDefault()

	err := RequireRuntime(cfg, Options{})
	if err == nil || !strings.Contains(err.Error(), "ffmpeg が見つかりません") {
		t.Fatalf("expected missing ffmpeg error, got %v", err)
	}
}

func TestRequireRuntimeChecksOutputDir(t *testing.T) {
	binDir := t.TempDir()
	ffmpeg := filepath.Join(binDir, "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	cfg := config.NewDefault()
	cfg.DestDir = filepath.Join(t.TempDir(), "out")

	if err := RequireRuntime(cfg, Options{CheckOutputDir: true}); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(cfg.DestDir); err != nil || !info.IsDir() {
		t.Fatalf("expected output dir to exist, info=%v err=%v", info, err)
	}
}

func TestRequireRuntimeChecksWatchDirs(t *testing.T) {
	binDir := t.TempDir()
	ffmpeg := filepath.Join(binDir, "ffmpeg")
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	cfg := config.NewDefault()
	err := RequireRuntime(cfg, Options{WatchDirs: []string{filepath.Join(t.TempDir(), "missing")}})
	if err == nil || !strings.Contains(err.Error(), "監視対象ディレクトリを確認できません") {
		t.Fatalf("expected missing watch dir error, got %v", err)
	}
}
