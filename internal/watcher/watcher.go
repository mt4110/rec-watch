package watcher

import (
	"context"
	"fmt"
	"log"

	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/mt4110/rec-watch/internal/config"
	"github.com/mt4110/rec-watch/internal/convert"
	"github.com/mt4110/rec-watch/internal/fileguard"
	"github.com/mt4110/rec-watch/internal/history"
	"github.com/mt4110/rec-watch/internal/postprocess"
	"github.com/mt4110/rec-watch/internal/prompt"
)

type Watcher struct {
	Cfg       *config.Config
	Converter *convert.Converter
	EventChan chan<- interface{} // Optional: Send events for TUI
}

func New(cfg *config.Config, cvt *convert.Converter) *Watcher {
	return &Watcher{
		Cfg:       cfg,
		Converter: cvt,
	}
}

func (w *Watcher) Run() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	if len(w.Cfg.WatchDirs) == 0 {
		log.Fatal("監視対象のディレクトリが設定されていません")
	}

	done := make(chan bool)

	// 重複処理防止用のマップ
	var processingMu sync.Mutex
	processing := make(map[string]bool)

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				w.handleEvent(event, &processingMu, processing)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("監視エラー:", err)
			}
		}
	}()

	for _, dir := range w.Cfg.WatchDirs {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			log.Printf("⚠️ ディレクトリパスの解決に失敗 (スキップ): %s -> %v", dir, err)
			continue
		}
		if err = watcher.Add(absDir); err != nil {
			log.Printf("⚠️ 監視エラー (スキップ): %s -> %v", dir, err)
		} else {
			log.Printf("監視を開始しました: %s", absDir)
		}
	}

	<-done
}

func (w *Watcher) handleEvent(event fsnotify.Event, processingMu *sync.Mutex, processing map[string]bool) {
	if event.Op&fsnotify.Create != fsnotify.Create && event.Op&fsnotify.Rename != fsnotify.Rename {
		return
	}

	fName := filepath.Base(event.Name)
	if strings.HasPrefix(fName, ".") {
		return
	}

	if !w.isTargetVideo(fName) {
		return
	}

	if !w.shouldProcess(fName) {
		return
	}

	processingMu.Lock()
	if processing[event.Name] {
		processingMu.Unlock()
		log.Printf("すでに処理中です: %s", event.Name)
		return
	}
	processing[event.Name] = true
	processingMu.Unlock()

	log.Printf("新規ファイルを検知: %s", event.Name)

	if w.EventChan != nil {
		// Emit Found Event
		// Use anonymous struct or map to avoid dep?
		// Or define Types in watcher pkg
		w.EventChan <- FileFoundEvent{Path: event.Name, Name: fName}
	}

	go w.processFile(event.Name, fName, processingMu, processing)
}

// Events
type FileFoundEvent struct {
	Path string
	Name string
}
type StartConvertEvent struct {
	Path string
}
type SuccessEvent struct {
	Path    string
	OutPath string
}
type FailureEvent struct {
	Path string
	Err  error
}

func (w *Watcher) isTargetVideo(fName string) bool {
	ext := strings.ToLower(filepath.Ext(fName))
	for _, v := range []string{".mov", ".mp4", ".m4v", ".avi", ".mkv"} {
		if ext == v {
			return true
		}
	}
	return false
}

func (w *Watcher) shouldProcess(fName string) bool {
	lowerName := strings.ToLower(fName)
	// Exclude
	if len(w.Cfg.IgnoreKeywords) > 0 {
		for _, k := range w.Cfg.IgnoreKeywords {
			if strings.Contains(lowerName, strings.ToLower(k)) {
				log.Printf("無視キーワードに一致したためスキップ: %s", fName)
				return false
			}
		}
	}

	// Include
	if len(w.Cfg.Keywords) > 0 {
		included := false
		for _, k := range w.Cfg.Keywords {
			if strings.Contains(lowerName, strings.ToLower(k)) {
				included = true
				break
			}
		}
		if !included {
			log.Printf("キーワードに一致しないためスキップ: %s", fName)
			return false
		}
	}
	return true
}

func (w *Watcher) processFile(path, name string, processingMu *sync.Mutex, processing map[string]bool) {
	defer func() {
		processingMu.Lock()
		delete(processing, path)
		processingMu.Unlock()
	}()

	baseOut, _ := filepath.Abs(w.Cfg.DestDir)
	batchDir := baseOut
	if w.Cfg.BatchStamp {
		batchDir = filepath.Join(baseOut, nowStamp())
	}
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		log.Printf("出力ディレクトリ作成失敗: %v", err)
		return
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		log.Printf("パスの解決に失敗: %v", err)
		return
	}

	log.Printf("変換開始: %s", absPath)
	if _, err := fileguard.WaitUntilStable(context.Background(), absPath, fileguard.StabilityOptions{
		Timeout:       w.Cfg.StableTimeout,
		Interval:      w.Cfg.StableInterval,
		StableSamples: w.Cfg.StableSamples,
	}); err != nil {
		log.Printf("ファイルの安定化待ちに失敗: %v", err)
		if w.EventChan != nil {
			w.EventChan <- FailureEvent{Path: absPath, Err: err}
		}
		if w.Cfg.Notify {
			convert.SendNotification("変換失敗", fmt.Sprintf("%s の準備に失敗しました。", name), "")
		}
		return
	}

	if w.EventChan != nil {
		w.EventChan <- StartConvertEvent{Path: absPath}
	}

	if result, err := w.Converter.Convert(absPath, batchDir); err != nil {
		log.Printf("❌ 変換失敗: %v", err)
		if w.EventChan != nil {
			w.EventChan <- FailureEvent{Path: absPath, Err: err}
		}
		if w.Cfg.Notify {
			convert.SendNotification("変換失敗", fmt.Sprintf("%s の変換に失敗しました。", name), "")
		}
	} else {
		if !w.Cfg.DryRun {
			if err := history.WriteConversionResult(result); err != nil {
				log.Printf("履歴の書き込みに失敗: %v", err)
			}
			decision, err := postprocess.HandleSource(context.Background(), postprocess.SourcePolicy(w.Cfg.SourcePolicy), result, prompt.NewSourcePrompter())
			if err != nil {
				log.Printf("変換元ファイルの処理に失敗: %v", err)
			} else {
				log.Printf("変換元ファイルの処理: %s", decision)
			}
		}
		log.Printf("✅ 変換完了: %s", path)
		if w.EventChan != nil {
			w.EventChan <- SuccessEvent{Path: path, OutPath: result.OutputPath}
		}
		if w.Cfg.Notify {
			convert.SendNotification("変換完了", fmt.Sprintf("%s を変換しました。", name), result.OutputPath)
		}
	}
}

func nowStamp() string {
	return time.Now().Format("20060102")
}
