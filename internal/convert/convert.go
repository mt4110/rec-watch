package convert

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	finalOutPath := outPath
	convertOutPath := outPath
	if !c.Cfg.DryRun {
		tmp, err := tempOutputPath(outDir, finalOutPath)
		if err != nil {
			return ConvertResult{}, err
		}
		convertOutPath = tmp
		defer os.Remove(tmp)
	}

	if err := c.convertFile(inPath, convertOutPath); err != nil {
		return ConvertResult{}, err
	}
	if !c.Cfg.DryRun {
		if err := finalizeOutput(convertOutPath, finalOutPath); err != nil {
			return ConvertResult{}, err
		}
	}

	finishedAt := time.Now()
	result := makeConvertResult(inPath, finalOutPath, startTime, finishedAt)
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
	stem := sanitizeFileStem(strings.TrimSuffix(filepath.Base(inPath), filepath.Ext(inPath)))
	if stem == "" {
		stem = "recording"
	}
	return uniqueOutputPath(outDir, fmt.Sprintf("%s_%s", timeStamp, stem), ".mp4")
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
	stem := sanitizeFileStem(strings.TrimSuffix(filepath.Base(inPath), filepath.Ext(inPath)))
	if stem == "" {
		stem = "recording"
	}
	finalOutPath, err := uniqueOutputPath(outDir, fmt.Sprintf("%s_%s", timeStamp, stem), ".mp4")
	if err != nil {
		return ConvertResult{}, err
	}

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

	tmpOutPath, err := tempOutputPath(outDir, finalOutPath)
	if err != nil {
		return ConvertResult{}, err
	}
	defer os.Remove(tmpOutPath)

	mergeArgs := []string{
		"-n",
		"-f", "concat",
		"-safe", "0",
		"-i", listFile,
		"-c", "copy",
		tmpOutPath,
	}

	cmd := exec.Command(c.ffmpegPath(), mergeArgs...)

	if out, err := cmd.CombinedOutput(); err != nil {
		return ConvertResult{}, fmt.Errorf("merge failed: %v\n%s", err, string(out))
	}
	if err := finalizeOutput(tmpOutPath, finalOutPath); err != nil {
		return ConvertResult{}, err
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

	for _, chunk := range chunks {
		abs, err := filepath.Abs(chunk)
		if err != nil {
			return "", errors.Join(err, f.Close())
		}
		if _, err := f.WriteString(fmt.Sprintf("file '%s'\n", escapeConcatPath(abs))); err != nil {
			return "", errors.Join(err, f.Close())
		}
	}

	if err := f.Close(); err != nil {
		return "", err
	}
	return listFile, nil
}

func escapeConcatPath(path string) string {
	escaped := make([]byte, 0, len(path))
	for i := 0; i < len(path); i++ {
		switch c := path[i]; c {
		case '\\', '\'':
			escaped = append(escaped, '\\', c)
		case '\n':
			escaped = append(escaped, '\\', 'n')
		case '\r':
			escaped = append(escaped, '\\', 'r')
		default:
			escaped = append(escaped, c)
		}
	}
	return string(escaped)
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
		"-n",
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

func uniqueOutputPath(outDir, stem, ext string) (string, error) {
	for i := 0; i < 1000; i++ {
		name := stem + ext
		if i > 0 {
			name = fmt.Sprintf("%s-%03d%s", stem, i, ext)
		}
		path := filepath.Join(outDir, name)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if errors.Is(err, os.ErrNotExist) {
			return path, nil
		} else {
			return "", err
		}
	}
	return "", fmt.Errorf("could not allocate unique output path for %s%s", stem, ext)
}

func tempOutputPath(outDir, finalOutPath string) (string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", err
	}
	pattern := "." + strings.TrimSuffix(filepath.Base(finalOutPath), filepath.Ext(finalOutPath)) + "-*.tmp.mp4"
	f, err := os.CreateTemp(outDir, pattern)
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}

func finalizeOutput(tmpPath, finalPath string) error {
	if _, err := os.Stat(finalPath); err == nil {
		return fmt.Errorf("refusing to overwrite existing output: %s", finalPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return fmt.Errorf("rename converted output: %w", err)
	}
	return nil
}

func sanitizeFileStem(stem string) string {
	stem = strings.TrimSpace(stem)
	var b strings.Builder
	lastDash := false
	for _, r := range stem {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-_")
}
