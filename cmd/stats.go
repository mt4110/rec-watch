package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mt4110/rec-watch/internal/history"
)

type LogEntry struct {
	Type          string  `json:"type"`
	Input         string  `json:"input"`
	Output        string  `json:"output"`
	DurationSec   float64 `json:"duration_sec"`
	OriginalSize  int64   `json:"original_size"`
	ConvertedSize int64   `json:"converted_size"`
	SizeDiff      int64   `json:"size_diff"`
	Timestamp     string  `json:"timestamp"`
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "変換統計を表示します",
	Long:  `過去の変換履歴(ログファイル)を集計し、削減されたファイルサイズや変換時間を表示します。`,
	Run: func(cmd *cobra.Command, args []string) {
		home, _ := os.UserHomeDir()
		logPath := filepath.Join(home, "Library", "Logs", "rec-watch.log")
		if cfg != nil && cfg.LogFile != "" {
			logPath = cfg.LogFile
		}

		sourcePaths := []string{logPath}
		historyPath, err := history.DefaultPath()
		if err == nil {
			sourcePaths = append([]string{historyPath}, sourcePaths...)
		} else {
			log.Fatalf("履歴ファイルの場所を解決できませんでした: %v", err)
		}

		totalCount, totalDiff, totalDuration, usedPaths, err := collectStats(sourcePaths)
		if err != nil {
			log.Fatalf("統計ソースの読み取りに失敗しました (%s): %v", strings.Join(sourcePaths, ", "), err)
		}

		const separator = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
		fmt.Println(separator)
		fmt.Printf("📊 RecWatch 統計レポート\n")
		fmt.Println(separator)
		fmt.Printf("総変換数:       %d 本\n", totalCount)
		fmt.Printf("合計削減サイズ: %s\n", formatBytes(totalDiff))
		fmt.Printf("合計処理時間:   %s\n", formatDuration(totalDuration))
		if totalCount > 0 {
			fmt.Printf("平均削減率:     %.1f MB/本\n", float64(totalDiff)/float64(totalCount)/1024/1024)
		}
		if len(usedPaths) > 0 {
			fmt.Printf("集計ソース:     %s\n", strings.Join(usedPaths, ", "))
		}
		fmt.Println(separator)
	},
}

func collectStats(paths []string) (int, int64, float64, []string, error) {
	seen := map[string]struct{}{}
	var totalCount int
	var totalDiff int64
	var totalDuration float64
	var usedPaths []string
	var lastErr error

	for _, path := range paths {
		if path == "" {
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			lastErr = errors.Join(lastErr, fmt.Errorf("open stats source %s: %w", path, err))
			continue
		}

		usedPaths = append(usedPaths, path)
		count, diff, duration, scanErr := collectStatsFromReader(f, seen)
		totalCount += count
		totalDiff += diff
		totalDuration += duration
		if scanErr != nil {
			lastErr = errors.Join(lastErr, fmt.Errorf("scan stats source %s: %w", path, scanErr))
		}
		if err := f.Close(); err != nil {
			lastErr = errors.Join(lastErr, fmt.Errorf("close stats source %s: %w", path, err))
		}
	}

	if lastErr != nil {
		return totalCount, totalDiff, totalDuration, usedPaths, lastErr
	}

	return totalCount, totalDiff, totalDuration, usedPaths, nil
}

func collectStatsFromReader(r io.Reader, seen map[string]struct{}) (int, int64, float64, error) {
	scanner := bufio.NewScanner(r)

	var totalCount int
	var totalDiff int64
	var totalDuration float64

	for scanner.Scan() {
		entry, ok := parseLogEntry(scanner.Text())
		if !ok || entry.Type != "conversion_result" {
			continue
		}
		key := statsEntryKey(entry)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		totalCount++
		totalDiff += entry.SizeDiff
		totalDuration += entry.DurationSec
	}

	return totalCount, totalDiff, totalDuration, scanner.Err()
}

func parseLogEntry(line string) (LogEntry, bool) {
	idx := strings.Index(line, "{")
	if idx == -1 {
		return LogEntry{}, false
	}

	var entry LogEntry
	if err := json.Unmarshal([]byte(line[idx:]), &entry); err != nil {
		return LogEntry{}, false
	}
	return entry, true
}

func statsEntryKey(entry LogEntry) string {
	return strings.Join([]string{
		entry.Timestamp,
		entry.Input,
		entry.Output,
		fmt.Sprintf("%.6f", entry.DurationSec),
		fmt.Sprintf("%d", entry.SizeDiff),
	}, "|")
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func formatDuration(sec float64) string {
	d := time.Duration(sec * float64(time.Second))
	return d.String()
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
