package watcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mt4110/rec-watch/internal/config"
	"github.com/mt4110/rec-watch/internal/convert"
	"github.com/mt4110/rec-watch/internal/history"
)

func TestWatcherConvertsVideoKeepsSourceAndWritesHistory(t *testing.T) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not installed")
	}
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe is not installed")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))

	watchDir := filepath.Join(home, "watch")
	destDir := filepath.Join(home, "out")
	if err := os.MkdirAll(watchDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}

	fixture := filepath.Join(t.TempDir(), "fixture.mov")
	makeFixture := exec.Command(ffmpegPath,
		"-y",
		"-f", "lavfi",
		"-i", "testsrc=duration=1:size=160x90:rate=5",
		"-pix_fmt", "yuv420p",
		fixture,
	)
	if out, err := makeFixture.CombinedOutput(); err != nil {
		t.Fatalf("create fixture: %v\n%s", err, string(out))
	}

	cfg := config.NewDefault()
	cfg.WatchDirs = []string{watchDir}
	cfg.DestDir = destDir
	cfg.BatchStamp = false
	cfg.Notify = false
	cfg.SourcePolicy = "keep"
	cfg.StableTimeout = 5 * time.Second
	cfg.StableInterval = 20 * time.Millisecond
	cfg.StableSamples = 2
	cfg.FFmpegBin = ffmpegPath
	cfg.FPS = 0
	cfg.Mute = true

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := New(cfg, convert.New(cfg))
	done := make(chan struct{})
	go func() {
		defer close(done)
		w.RunContext(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	inputPath := filepath.Join(watchDir, "clip.mov")
	bytes, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inputPath, bytes, 0644); err != nil {
		t.Fatal(err)
	}

	outputPath := waitForMP4(t, destDir, 10*time.Second)
	probe := exec.Command(ffprobePath, "-v", "error", "-show_entries", "format=format_name", "-of", "default=noprint_wrappers=1:nokey=1", outputPath)
	if out, err := probe.CombinedOutput(); err != nil {
		t.Fatalf("ffprobe failed: %v\n%s", err, string(out))
	} else if !strings.Contains(string(out), "mp4") {
		t.Fatalf("ffprobe format = %q, want mp4", string(out))
	}
	if _, err := os.Stat(inputPath); err != nil {
		t.Fatalf("source should be kept: %v", err)
	}

	historyPath, err := history.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	waitForHistory(t, historyPath, 5*time.Second)
	historyBytes, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(historyBytes)), "\n")
	if len(lines) != 1 {
		t.Fatalf("history lines = %d, want 1\n%s", len(lines), string(historyBytes))
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not stop after context cancellation")
	}
}

func waitForMP4(t *testing.T, dir string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		matches, err := filepath.Glob(filepath.Join(dir, "*.mp4"))
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range matches {
			base := filepath.Base(match)
			if !strings.HasPrefix(base, ".") && !strings.Contains(base, ".tmp.") {
				return match
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for mp4 in %s", dir)
	return ""
}

func waitForHistory(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for history at %s", path)
}
