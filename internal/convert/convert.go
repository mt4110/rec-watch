package convert

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/mt4110/rec-watch/internal/config"
	"github.com/mt4110/rec-watch/internal/split"
)

// SendNotification sends a desktop notification
func SendNotification(title, message, filePath string) {
	if _, err := exec.LookPath("terminal-notifier"); err == nil {
		args := []string{"-title", title, "-message", message, "-sound", "default"}
		if filePath != "" {
			u := url.URL{Scheme: "file", Path: filePath}
			args = append(args, "-open", u.String())
		}
		exec.Command("terminal-notifier", args...).Run()
		return
	}
	// Fallback
	script := fmt.Sprintf(`tell application "System Events" to display notification "%s" with title "%s" sound name "default"`, message, title)
	exec.Command("osascript", "-e", script).Run()
}

type Converter struct {
	Cfg *config.Config
}

type ConvertResult struct {
	InputPath     string        `json:"input_path"`
	OutputPath    string        `json:"output_path"`
	Duration      time.Duration `json:"duration"`
	DurationSec   float64       `json:"duration_sec"`
	OriginalSize  int64         `json:"original_size"`
	ConvertedSize int64         `json:"converted_size"`
	SizeDiff      int64         `json:"size_diff"`
	StartedAt     time.Time     `json:"started_at"`
	FinishedAt    time.Time     `json:"finished_at"`
}

func New(cfg *config.Config) *Converter {
	return &Converter{Cfg: cfg}
}

func (c *Converter) ProcessFiles(files []string) {
	// 出力ディレクトリを作成
	baseOut, _ := filepath.Abs(c.Cfg.DestDir)
	batchDir := baseOut
	if c.Cfg.BatchStamp {
		batchDir = filepath.Join(baseOut, nowStamp())
	}
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		log.Fatalf("出力ディレクトリの作成に失敗: %v", err)
	}

	log.Printf("変換対象: %d件", len(files))
	log.Printf("出力先: %s", batchDir)
	workerCount := c.Cfg.Concurrent
	if workerCount < 1 {
		workerCount = 1
	}
	log.Printf("並列実行数: %d", workerCount)

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, workerCount)

	for _, inPath := range files {
		wg.Add(1)
		semaphore <- struct{}{} // 実行枠を確保

		go func(inPath string) {
			defer func() {
				<-semaphore // 実行枠を解放
				wg.Done()
			}()
			if _, err := c.Convert(inPath, batchDir); err != nil {
				log.Printf("❌ 変換失敗: %s -> %v", inPath, err)
			}
		}(inPath)
	}

	wg.Wait()
	log.Println("✅ すべて完了")
}

func (c *Converter) Convert(inPath string, outDir string) (ConvertResult, error) {
	if c.Cfg.ParallelSplit {
		return c.ConvertSplit(inPath, outDir)
	}

	return c.ConvertOne(inPath, outDir)
}

func (c *Converter) ConvertOne(inPath string, outDir string) (ConvertResult, error) {
	outPath, err := outputPathForInput(inPath, outDir)
	if err != nil {
		return ConvertResult{}, err
	}

	log.Printf("▶ 変換: %s -> %s", inPath, outPath)
	startTime := time.Now()

	if err := c.convertFile(inPath, outPath); err != nil {
		return ConvertResult{}, err
	}

	finishedAt := time.Now()
	result := makeConvertResult(inPath, outPath, startTime, finishedAt)
	if !c.Cfg.DryRun && result.ConvertedSize <= 0 {
		return ConvertResult{}, fmt.Errorf("converted output is empty: %s", outPath)
	}
	return result, nil
}

func outputPathForInput(inPath, outDir string) (string, error) {
	info, err := os.Stat(inPath)
	if err != nil {
		return "", err
	}
	timeStamp := info.ModTime().Format("2006-01-02_15-04-05")
	return filepath.Join(outDir, fmt.Sprintf("%s.mp4", timeStamp)), nil
}

func makeConvertResult(inPath, outPath string, startedAt, finishedAt time.Time) ConvertResult {
	var originalSize int64
	if info, err := os.Stat(inPath); err == nil {
		originalSize = info.Size()
	}
	var convertedSize int64
	if info, err := os.Stat(outPath); err == nil {
		convertedSize = info.Size()
	}
	duration := finishedAt.Sub(startedAt)
	return ConvertResult{
		InputPath:     inPath,
		OutputPath:    outPath,
		Duration:      duration,
		DurationSec:   duration.Seconds(),
		OriginalSize:  originalSize,
		ConvertedSize: convertedSize,
		SizeDiff:      originalSize - convertedSize,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
	}
}

func nowStamp() string {
	return time.Now().Format("20060102")
}

func (c *Converter) ConvertSplit(inPath string, outDir string) (ConvertResult, error) {
	log.Printf("🚀 並列分割モードで処理開始: %s", filepath.Base(inPath))

	info, err := os.Stat(inPath)
	if err != nil {
		return ConvertResult{}, err
	}
	startTime := time.Now()
	timeStamp := info.ModTime().Format("2006-01-02_15-04-05")
	finalOutPath := filepath.Join(outDir, fmt.Sprintf("%s.mp4", timeStamp))

	if c.Cfg.DryRun {
		log.Printf("[DryRun] Would split %s into chunks...", inPath)
		finishedAt := time.Now()
		return makeConvertResult(inPath, finalOutPath, startTime, finishedAt), nil
	}

	tmpDir, err := os.MkdirTemp("", "rec-watch-split-*")
	if err != nil {
		return ConvertResult{}, err
	}
	defer os.RemoveAll(tmpDir)

	s := split.New(c.Cfg.FFmpegBin)
	chunks, err := s.Split(inPath, tmpDir, 300)
	if err != nil {
		return ConvertResult{}, err
	}
	if len(chunks) == 0 {
		return ConvertResult{}, fmt.Errorf("split produced no chunks")
	}

	type chunkResult struct {
		index int
		path  string
		err   error
	}

	results := make([]chunkResult, len(chunks))
	var wg sync.WaitGroup

	chunkWorkers := c.Cfg.Concurrent
	if chunkWorkers < 1 {
		chunkWorkers = 4
	}
	sem := make(chan struct{}, chunkWorkers)

	log.Printf("⚡️ %d個のチャンクを %d並列で変換中...", len(chunks), cap(sem))

	for i, chunk := range chunks {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, chunkPath string) {
			defer func() {
				<-sem
				wg.Done()
			}()

			chunkOutDir := filepath.Join(tmpDir, "converted")
			if err := os.MkdirAll(chunkOutDir, 0755); err != nil {
				results[i] = chunkResult{index: i, err: err}
				return
			}

			outFile := filepath.Join(chunkOutDir, filepath.Base(chunkPath))
			err := c.convertFile(chunkPath, outFile)
			results[i] = chunkResult{index: i, path: outFile, err: err}
			if err != nil {
				log.Printf("⚠️ チャンク変換失敗: %s: %v", chunkPath, err)
			}
		}(i, chunk)
	}
	wg.Wait()

	var convertedChunks []string
	for _, res := range results {
		if res.err != nil {
			return ConvertResult{}, fmt.Errorf("chunk %d failed: %v", res.index, res.err)
		}
		convertedChunks = append(convertedChunks, res.path)
	}

	listFile, err := writeConcatList(tmpDir, convertedChunks)
	if err != nil {
		return ConvertResult{}, err
	}

	log.Println("🔗 チャンクを結合中...")

	mergeArgs := []string{
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-c", "copy",
		finalOutPath,
	}

	cmd := exec.Command(c.ffmpegPath(), mergeArgs...)

	if out, err := cmd.CombinedOutput(); err != nil {
		return ConvertResult{}, fmt.Errorf("merge failed: %v\n%s", err, string(out))
	}

	finishedAt := time.Now()
	result := makeConvertResult(inPath, finalOutPath, startTime, finishedAt)
	if result.ConvertedSize <= 0 {
		return ConvertResult{}, fmt.Errorf("converted output is empty: %s", finalOutPath)
	}
	return result, nil
}

func writeConcatList(tmpDir string, chunks []string) (string, error) {
	listFile := filepath.Join(tmpDir, "concat.txt")
	f, err := os.Create(listFile)
	if err != nil {
		return "", err
	}
	defer f.Close()

	for _, chunk := range chunks {
		abs, err := filepath.Abs(chunk)
		if err != nil {
			return "", err
		}
		if _, err := f.WriteString(fmt.Sprintf("file '%s'\n", abs)); err != nil {
			return "", err
		}
	}

	return listFile, nil
}

func (c *Converter) convertFile(inPath, outPath string) error {
	ffmpegPath := c.ffmpegPath()
	ffmpegArgs := c.ffmpegArgs(inPath, outPath)

	if c.Cfg.DryRun {
		log.Printf("[DryRun] Command: %s %v", ffmpegPath, ffmpegArgs)
		return nil
	}

	cmd := exec.Command(ffmpegPath, ffmpegArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg実行エラー: %v\n%s", err, string(out))
	}
	return nil
}

func (c *Converter) ffmpegPath() string {
	if c.Cfg.FFmpegBin != "" {
		return c.Cfg.FFmpegBin
	}
	return "ffmpeg"
}

func (c *Converter) ffmpegArgs(inPath, outPath string) []string {
	vf := "scale=1920:1080:force_original_aspect_ratio=decrease"
	if !c.Cfg.NoPad {
		vf += ",pad=1920:1080:(ow-iw)/2:(oh-ih)/2"
	}

	args := []string{
		"-i", inPath,
	}

	if c.Cfg.GPU {
		args = append(args, "-c:v", "h264_videotoolbox")
		q := 70
		if c.Cfg.CRF > 0 {
			q = 100 - (c.Cfg.CRF * 2)
			if q < 1 {
				q = 1
			}
		}
		args = append(args, "-q:v", fmt.Sprintf("%d", q))
	} else {
		args = append(args, "-vcodec", "libx264")
		args = append(args, "-preset", c.Cfg.Preset)
		args = append(args, "-crf", fmt.Sprintf("%d", c.Cfg.CRF))
	}

	args = append(args,
		"-vf", vf,
		"-movflags", "+faststart",
	)

	if c.Cfg.FPS > 0 {
		args = append(args, "-r", fmt.Sprintf("%d", c.Cfg.FPS))
	}

	if c.Cfg.Mute {
		args = append(args, "-an")
	} else {
		args = append(args, "-acodec", "aac", "-b:a", "128k", "-ac", "2")
	}

	return append(args, outPath)
}
