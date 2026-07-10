package fileguard

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWaitUntilStable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recording.mov")
	if err := os.WriteFile(path, []byte("video"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := WaitUntilStable(context.Background(), path, StabilityOptions{
		Timeout:       time.Second,
		Interval:      10 * time.Millisecond,
		StableSamples: 2,
	})
	if err != nil {
		t.Fatalf("WaitUntilStable failed: %v", err)
	}
	if result.Path != path {
		t.Fatalf("Path = %q, want %q", result.Path, path)
	}
	if result.Size != 5 {
		t.Fatalf("Size = %d, want 5", result.Size)
	}
	if result.SampleCount < 2 {
		t.Fatalf("SampleCount = %d, want >= 2", result.SampleCount)
	}
}

func TestWaitUntilStableMissingFile(t *testing.T) {
	_, err := WaitUntilStable(context.Background(), filepath.Join(t.TempDir(), "missing.mov"), StabilityOptions{
		Timeout:       30 * time.Millisecond,
		Interval:      10 * time.Millisecond,
		StableSamples: 2,
	})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestWaitUntilStableContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := WaitUntilStable(ctx, filepath.Join(t.TempDir(), "missing.mov"), StabilityOptions{
		Timeout:       time.Second,
		Interval:      10 * time.Millisecond,
		StableSamples: 2,
	})
	if err == nil {
		t.Fatal("expected context error")
	}
}
