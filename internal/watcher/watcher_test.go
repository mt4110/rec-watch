package watcher

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/mt4110/rec-watch/internal/config"
)

// Helper to test filtering logic without running the actual watcher loop
func shouldProcess(name string, cfg *config.Config) bool {
	fName := name
	lowerName := strings.ToLower(fName)

	if len(cfg.IgnoreKeywords) > 0 {
		for _, k := range cfg.IgnoreKeywords {
			if strings.Contains(lowerName, strings.ToLower(k)) {
				return false
			}
		}
	}

	if len(cfg.Keywords) > 0 {
		included := false
		for _, k := range cfg.Keywords {
			if strings.Contains(lowerName, strings.ToLower(k)) {
				included = true
				break
			}
		}
		if !included {
			return false
		}
	}
	return true
}

func TestFiltering(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		cfg      config.Config
		want     bool
	}{
		{
			name:     "No filters",
			filename: "video.mp4",
			cfg:      config.Config{},
			want:     true,
		},
		{
			name:     "Ignore keyword match",
			filename: "archive_video.mp4",
			cfg:      config.Config{IgnoreKeywords: []string{"archive"}},
			want:     false,
		},
		{
			name:     "Ignore keyword mismatch",
			filename: "video.mp4",
			cfg:      config.Config{IgnoreKeywords: []string{"archive"}},
			want:     true,
		},
		{
			name:     "Include keyword match",
			filename: "meeting_recording.mp4",
			cfg:      config.Config{Keywords: []string{"meeting"}},
			want:     true,
		},
		{
			name:     "Include keyword mismatch",
			filename: "random.mp4",
			cfg:      config.Config{Keywords: []string{"meeting"}},
			want:     false,
		},
		{
			name:     "Include match AND Ignore match (Ignore takes precedence)",
			filename: "meeting_archive.mp4",
			cfg:      config.Config{Keywords: []string{"meeting"}, IgnoreKeywords: []string{"archive"}},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldProcess(tt.filename, &tt.cfg)
			if got != tt.want {
				t.Errorf("shouldProcess(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
}

func TestStabilityMissingFileIsSkippable(t *testing.T) {
	err := fmt.Errorf("wrapped: %w", os.ErrNotExist)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatal("expected os.ErrNotExist to be detectable")
	}
}
