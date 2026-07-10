package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
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

		historyPath, _ := history.DefaultPath()
		logPath := filepath.Join(home, "Library/Logs/rec-watch.log")
		if cfg != nil && cfg.LogFile != "" {
			logPath = cfg.LogFile
		}

		statsPath := historyPath
		if _, err := os.Stat(statsPath); err != nil {
			statsPath = logPath
		}

		f, err := os.Open(statsPath)
		if err != nil {
			log.Fatalf("履歴ファイルを開けませんでした: %v", err)
		}
		defer f.Close()

		var totalDiff int64
		var totalCount int
		var totalDuration float64

		// For verification output mostly
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			// Log lines usually start with date/time "2023/01/01 10:00:00 filename.go:10: {"type":...}"
			// We need to find the start of JSON.
			idx := strings.Index(line, "{")
			if idx == -1 {
				continue
			}
			jsonPart := line[idx:]

			var entry LogEntry
			if err := json.Unmarshal([]byte(jsonPart), &entry); err != nil {
				continue
			}

			if entry.Type == "conversion_result" {
				totalCount++
				totalDiff += entry.SizeDiff
				totalDuration += entry.DurationSec
			}
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
		fmt.Println(separator)
	},
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
