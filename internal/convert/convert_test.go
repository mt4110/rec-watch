package convert

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mt4110/rec-watch/internal/config"
)

func TestEscapeConcatPath(t *testing.T) {
	got := escapeConcatPath("a'\\b\nc\rd")
	want := "a\\'\\\\b\\nc\\rd"
	if got != want {
		t.Fatalf("escapeConcatPath() = %q, want %q", got, want)
	}
}

func TestOutputPathForInputUsesSourceNameAndAvoidsExistingOutput(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "Screen Recording.mov")
	if err := os.WriteFile(inPath, []byte("input"), 0644); err != nil {
		t.Fatal(err)
	}
	modTime := time.Date(2026, 9, 22, 10, 11, 12, 0, time.Local)
	if err := os.Chtimes(inPath, modTime, modTime); err != nil {
		t.Fatal(err)
	}

	outPath, err := outputPathForInput(inPath, dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(outPath) != "2026-09-22_10-11-12_Screen-Recording.mp4" {
		t.Fatalf("outPath = %q", outPath)
	}
	if err := os.WriteFile(outPath, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	nextPath, err := outputPathForInput(inPath, dir)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(nextPath) != "2026-09-22_10-11-12_Screen-Recording-001.mp4" {
		t.Fatalf("nextPath = %q", nextPath)
	}
}

func TestFFmpegArgsDisableOverwrite(t *testing.T) {
	c := New(testConfig())
	args := c.ffmpegArgs("in.mov", "out.mp4")
	if len(args) < 1 || args[0] != "-n" {
		t.Fatalf("ffmpeg args should start with -n: %v", args)
	}
}

func TestFinalizeOutputRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	tmpPath := filepath.Join(dir, ".tmp.mp4")
	finalPath := filepath.Join(dir, "final.mp4")
	if err := os.WriteFile(tmpPath, []byte("tmp"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(finalPath, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	err := finalizeOutput(tmpPath, finalPath)
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("expected overwrite refusal, got %v", err)
	}
}

func testConfig() *config.Config {
	return &config.Config{CRF: 22, Preset: "faster", FPS: 30}
}
