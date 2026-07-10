package history

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mt4110/rec-watch/internal/convert"
)

func TestWriteConversionResultTo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	startedAt := time.Now()
	finishedAt := startedAt.Add(2 * time.Second)

	err := WriteConversionResultTo(path, convert.ConvertResult{
		InputPath:     "input.mov",
		OutputPath:    "output.mp4",
		DurationSec:   2,
		OriginalSize:  100,
		ConvertedSize: 40,
		SizeDiff:      60,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
	})
	if err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		t.Fatal("expected one history line")
	}
	var entry Entry
	if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Type != "conversion_result" {
		t.Fatalf("Type = %q, want conversion_result", entry.Type)
	}
	if entry.SizeDiff != 60 {
		t.Fatalf("SizeDiff = %d, want 60", entry.SizeDiff)
	}
}
