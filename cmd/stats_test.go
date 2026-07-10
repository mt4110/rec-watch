package cmd

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectStatsAggregatesAndDeduplicatesSources(t *testing.T) {
	dir := t.TempDir()
	historyPath := filepath.Join(dir, "history.jsonl")
	logPath := filepath.Join(dir, "rec-watch.log")

	historyLine := `{"type":"conversion_result","input":"a.mov","output":"a.mp4","duration_sec":2,"original_size":100,"converted_size":40,"size_diff":60,"timestamp":"2026-07-10T00:00:00Z"}`
	logLine := `2026/07/10 09:00:00 stats.go:1: {"type":"conversion_result","input":"a.mov","output":"a.mp4","duration_sec":2,"original_size":100,"converted_size":40,"size_diff":60,"timestamp":"2026-07-10T00:00:00Z"}`
	legacyOnly := `2026/07/10 09:01:00 stats.go:2: {"type":"conversion_result","input":"b.mov","output":"b.mp4","duration_sec":3,"original_size":200,"converted_size":100,"size_diff":100,"timestamp":"2026-07-10T00:01:00Z"}`

	if err := os.WriteFile(historyPath, []byte(historyLine+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte(logLine+"\n"+legacyOnly+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	count, diff, duration, usedPaths, err := collectStats([]string{historyPath, logPath})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
	if diff != 160 {
		t.Fatalf("diff = %d, want 160", diff)
	}
	if duration != 5 {
		t.Fatalf("duration = %v, want 5", duration)
	}
	if len(usedPaths) != 2 {
		t.Fatalf("usedPaths = %v, want 2 paths", usedPaths)
	}
}

func TestCollectStatsFromReaderReturnsScannerError(t *testing.T) {
	seen := map[string]struct{}{}
	line := `{"type":"conversion_result","input":"a.mov","output":"a.mp4","duration_sec":2,"original_size":100,"converted_size":40,"size_diff":60,"timestamp":"2026-07-10T00:00:00Z"}` + "\n"

	count, diff, duration, err := collectStatsFromReader(&errorReader{
		data: []byte(line),
		err:  io.ErrUnexpectedEOF,
	}, seen)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("err = %v, want %v", err, io.ErrUnexpectedEOF)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if diff != 60 {
		t.Fatalf("diff = %d, want 60", diff)
	}
	if duration != 2 {
		t.Fatalf("duration = %v, want 2", duration)
	}
}

type errorReader struct {
	data []byte
	err  error
	read bool
}

func (r *errorReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, r.err
	}
	r.read = true
	n := copy(p, r.data)
	return n, nil
}
