package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mt4110/rec-watch/internal/convert"
)

type Entry struct {
	Type          string    `json:"type"`
	Input         string    `json:"input"`
	Output        string    `json:"output"`
	DurationSec   float64   `json:"duration_sec"`
	OriginalSize  int64     `json:"original_size"`
	ConvertedSize int64     `json:"converted_size"`
	SizeDiff      int64     `json:"size_diff"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at"`
	Timestamp     string    `json:"timestamp"`
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Application Support", "RecWatch", "history.jsonl"), nil
}

func WriteConversionResult(result convert.ConvertResult) error {
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	return WriteConversionResultTo(path, result)
}

func WriteConversionResultTo(path string, result convert.ConvertResult) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	entry := Entry{
		Type:          "conversion_result",
		Input:         result.InputPath,
		Output:        result.OutputPath,
		DurationSec:   result.DurationSec,
		OriginalSize:  result.OriginalSize,
		ConvertedSize: result.ConvertedSize,
		SizeDiff:      result.SizeDiff,
		StartedAt:     result.StartedAt,
		FinishedAt:    result.FinishedAt,
		Timestamp:     result.FinishedAt.Format(time.RFC3339),
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write history: %w", err)
	}
	return nil
}
