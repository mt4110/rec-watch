package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/spf13/cobra"

	"github.com/mt4110/rec-watch/internal/config"
	"github.com/mt4110/rec-watch/internal/convert"
	"github.com/mt4110/rec-watch/internal/history"
	"github.com/mt4110/rec-watch/internal/logger"
	"github.com/mt4110/rec-watch/internal/postprocess"
	"github.com/mt4110/rec-watch/internal/prompt"
	"github.com/mt4110/rec-watch/internal/updater"
	"github.com/mt4110/rec-watch/internal/watcher"
)

var (
	cfgFile string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "rec-watch [filesOrDirs...]",
	Short: "動画ファイルを一括で1080pのMP4に変換・監視します。",
	Long:  `macOSの画面収録などで作成された動画ファイルを、H.264形式のMP4に一括変換するCLIツール。監視モード(RecWatch)で自動化も可能。`,
	Args:  cobra.ArbitraryArgs,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		loadedCfg, err := config.Load()
		if err != nil {
			log.Printf("設定ファイルの読み込みに失敗しました (デフォルト値を使用します): %v", err)
			loadedCfg = config.NewDefault()
		}
		cfg = loadedCfg

		updateConfigFromFlags(cmd, cfg)
		logger.Setup(cfg.LogFile)
		updater.CheckFFmpeg()
	},
	Run: func(cmd *cobra.Command, args []string) {

		cvt := convert.New(cfg)

		if flagWatch {
			targets := args
			if len(targets) > 0 {
				cfg.WatchDirs = targets
			}

			if len(cfg.WatchDirs) == 0 {
				cfg.WatchDirs = []string{"."}
			}

			w := watcher.New(cfg, cvt)
			log.Println("👀 監視モードを開始しました (Ctrl+C で終了)")
			w.Run()
			return
		}

		inputPatterns := args
		if len(inputPatterns) == 0 {
			inputPatterns = []string{"."}
		}

		var files []string
		videoExtensions := "{mov,MOV,m4v,mp4,avi,mkv}"
		home, _ := os.UserHomeDir()

		for _, input := range inputPatterns {
			processedInput := input
			if input == "~" {
				processedInput = home
			} else if strings.HasPrefix(input, "~/") {
				processedInput = filepath.Join(home, input[2:])
			}

			var pattern string
			info, err := os.Stat(processedInput)
			if err == nil && info.IsDir() {
				pattern = filepath.Join(processedInput, "**/*."+videoExtensions)
			} else {
				pattern = processedInput
			}

			fsys := os.DirFS(".")
			globPattern := pattern
			isAbs := filepath.IsAbs(pattern)
			if isAbs {
				fsys = os.DirFS("/")
				globPattern, err = filepath.Rel("/", pattern)
				if err != nil {
					log.Printf("警告: パス '%s' の処理に失敗しました: %v", pattern, err)
					continue
				}
			}

			matches, err := doublestar.Glob(fsys, globPattern)
			if err != nil {
				log.Printf("警告: パターン '%s' の検索に失敗しました: %v", pattern, err)
				continue
			}

			if isAbs {
				for i, match := range matches {
					matches[i] = filepath.Join("/", match)
				}
			}

			files = append(files, matches...)
		}

		uniqueFiles := make(map[string]bool)
		var result []string
		for _, f := range files {
			if !uniqueFiles[f] {
				uniqueFiles[f] = true
				result = append(result, f)
			}
		}
		files = result

		if len(files) == 0 {
			log.Println("変換対象が見つかりません。")
			return
		}

		var filteredFiles []string
		if len(cfg.Keywords) > 0 || len(cfg.IgnoreKeywords) > 0 {
			for _, f := range files {
				name := filepath.Base(f)
				lowerName := strings.ToLower(name)

				// Exclude
				excluded := false
				for _, k := range cfg.IgnoreKeywords {
					if strings.Contains(lowerName, strings.ToLower(k)) {
						excluded = true
						break
					}
				}
				if excluded {
					continue
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
						continue
					}
				}

				filteredFiles = append(filteredFiles, f)
			}
		} else {
			filteredFiles = files
		}

		if len(filteredFiles) == 0 {
			log.Println("フィルタリングの結果、対象ファイルがありません。")
			return
		}

		processFiles(context.Background(), cfg, cvt, filteredFiles)
	},
}

// Temporary variables for flags
var (
	flagDest           string
	flagCRF            int
	flagPreset         string
	flagFPS            int
	flagMute           bool
	flagKeywords       []string
	flagIgnoreKeywords []string
	flagNoPad          bool
	flagStampPerFile   bool
	flagNoTrash        bool
	flagSourcePolicy   string
	flagBatchStamp     bool
	flagFFmpegBin      string
	flagConcurrent     int
	flagWatch          bool
	flagNotify         bool
	flagDryRun         bool
	flagStableTimeout  time.Duration
	flagStableInterval time.Duration
	flagStableSamples  int
	flagProfile        string
	flagParallelSplit  bool
	flagGPU            bool
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Define flags
	rootCmd.Flags().StringVar(&flagDest, "dest", "", "出力先ディレクトリ")
	rootCmd.Flags().IntVar(&flagCRF, "crf", 0, "CRF値 (品質)")
	rootCmd.Flags().StringVar(&flagPreset, "preset", "", "エンコードプリセット")
	rootCmd.Flags().IntVar(&flagFPS, "fps", 0, "フレームレート (0で無効)")
	rootCmd.Flags().BoolVar(&flagMute, "mute", false, "音声をミュートする")
	rootCmd.Flags().StringSliceVar(&flagKeywords, "keywords", []string{}, "ファイル名に含まれるキーワードでフィルタ")
	rootCmd.Flags().StringSliceVar(&flagIgnoreKeywords, "ignore-keywords", []string{}, "ファイル名に含まれるキーワードを除外")
	rootCmd.Flags().BoolVar(&flagNoPad, "no-pad", false, "1080pにリサイズする際に黒帯を追加しない")
	rootCmd.Flags().BoolVar(&flagStampPerFile, "stamp-per-file", false, "個別のファイル名にタイムスタンプを追加する")
	rootCmd.Flags().BoolVar(&flagNoTrash, "no-trash", false, "変換元のファイルをゴミ箱に移動しない")
	rootCmd.Flags().StringVar(&flagSourcePolicy, "source-policy", "trash", "変換元ファイルの扱い (keep, trash, ask)")
	rootCmd.Flags().BoolVar(&flagBatchStamp, "batch-stamp", true, "出力先ディレクトリをタイムスタンプ付きで作成する (default true)")
	rootCmd.Flags().StringVar(&flagFFmpegBin, "ffmpeg-bin", "", "ffmpegのバイナリパスを明示的に指定する")
	rootCmd.Flags().IntVar(&flagConcurrent, "concurrent", 0, "並列実行数")
	rootCmd.Flags().BoolVar(&flagWatch, "watch", false, "指定したディレクトリを監視して自動変換する")
	rootCmd.Flags().BoolVar(&flagNotify, "notify", true, "変換完了時にデスクトップ通知を送る")
	rootCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "実行せずにコマンドを表示する")
	rootCmd.Flags().DurationVar(&flagStableTimeout, "stable-timeout", 120*time.Second, "監視時にファイル安定化を待つ最大時間")
	rootCmd.Flags().DurationVar(&flagStableInterval, "stable-interval", time.Second, "監視時にファイルサイズと更新日時を確認する間隔")
	rootCmd.Flags().IntVar(&flagStableSamples, "stable-samples", 3, "安定判定に必要な連続サンプル数")
	rootCmd.Flags().StringVar(&flagProfile, "profile", "", "使用するプロファイル名")
	rootCmd.Flags().BoolVar(&flagParallelSplit, "parallel-split", false, "動画を分割して並列変換する（大容量ファイル向け・爆速）")
	rootCmd.Flags().BoolVar(&flagGPU, "gpu", false, "GPU(VideoToolbox)を使用して変換する（超爆速・画質/圧縮率はCPUに劣る）")
}

func updateConfigFromFlags(cmd *cobra.Command, c *config.Config) {
	flags := cmd.Flags()

	if flags.Changed("profile") {
		entry, ok := c.Profiles[flagProfile]
		if ok {
			if entry.CRF > 0 {
				c.CRF = entry.CRF
			}
			if entry.Preset != "" {
				c.Preset = entry.Preset
			}
			log.Printf("ℹ️ プロファイル '%s' を適用しました (CRF: %d, Preset: %s)", flagProfile, c.CRF, c.Preset)
		} else {
			log.Printf("⚠️ プロファイル '%s' は見つかりませんでした。デフォルト設定を使用します。", flagProfile)
		}
	}

	if flags.Changed("dest") {
		c.DestDir = flagDest
	}
	if flags.Changed("crf") {
		c.CRF = flagCRF
	}
	if flags.Changed("preset") {
		c.Preset = flagPreset
	}
	if flags.Changed("fps") {
		c.FPS = flagFPS
	}
	if flags.Changed("mute") {
		c.Mute = flagMute
	}
	if flags.Changed("keywords") {
		c.Keywords = flagKeywords
	}
	if flags.Changed("ignore-keywords") {
		c.IgnoreKeywords = flagIgnoreKeywords
	}
	if flags.Changed("no-pad") {
		c.NoPad = flagNoPad
	}
	if flags.Changed("stamp-per-file") {
		c.StampPerFile = flagStampPerFile
	}
	if flags.Changed("no-trash") {
		c.NoTrash = flagNoTrash
		if flagNoTrash {
			c.SourcePolicy = "keep"
		} else if !flags.Changed("source-policy") {
			c.SourcePolicy = "trash"
		}
	}
	if flags.Changed("source-policy") {
		c.SourcePolicy = flagSourcePolicy
		if !flags.Changed("no-trash") {
			c.NoTrash = false
		}
	}
	if flags.Changed("no-trash") && c.NoTrash {
		c.SourcePolicy = "keep"
	}
	if flags.Changed("batch-stamp") {
		c.BatchStamp = flagBatchStamp
	}
	if flags.Changed("ffmpeg-bin") {
		c.FFmpegBin = flagFFmpegBin
	}
	if flags.Changed("concurrent") {
		c.Concurrent = flagConcurrent
	}
	if flags.Changed("notify") {
		c.Notify = flagNotify
	}
	if flags.Changed("dry-run") {
		c.DryRun = flagDryRun
	}
	if flags.Changed("stable-timeout") {
		c.StableTimeout = flagStableTimeout
	}
	if flags.Changed("stable-interval") {
		c.StableInterval = flagStableInterval
	}
	if flags.Changed("stable-samples") {
		c.StableSamples = flagStableSamples
	}
	if flags.Changed("parallel-split") {
		c.ParallelSplit = flagParallelSplit
	}
	if flags.Changed("gpu") {
		c.GPU = flagGPU
	}
	if err := config.ValidateSourcePolicy(c.SourcePolicy); err != nil {
		log.Fatal(err)
	}
}

func processFiles(ctx context.Context, cfg *config.Config, cvt *convert.Converter, files []string) {
	baseOut, _ := filepath.Abs(cfg.DestDir)
	batchDir := baseOut
	if cfg.BatchStamp {
		batchDir = filepath.Join(baseOut, time.Now().Format("20060102"))
	}
	if err := os.MkdirAll(batchDir, 0755); err != nil {
		log.Fatalf("出力ディレクトリの作成に失敗: %v", err)
	}

	log.Printf("変換対象: %d件", len(files))
	log.Printf("出力先: %s", batchDir)
	workerCount := cfg.Concurrent
	if workerCount < 1 {
		workerCount = 1
	}
	log.Printf("並列実行数: %d", workerCount)

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, workerCount)
	sourcePrompter := prompt.NewSourcePrompter()

	for _, inPath := range files {
		wg.Add(1)
		semaphore <- struct{}{}

		go func(inPath string) {
			defer func() {
				<-semaphore
				wg.Done()
			}()

			result, err := cvt.Convert(inPath, batchDir)
			if err != nil {
				log.Printf("❌ 変換失敗: %s -> %v", inPath, err)
				return
			}
			if cfg.DryRun {
				return
			}
			if err := history.WriteConversionResult(result); err != nil {
				log.Printf("履歴の書き込みに失敗: %v", err)
			}
			decision, err := postprocess.HandleSource(ctx, postprocess.SourcePolicy(cfg.SourcePolicy), result, sourcePrompter)
			if err != nil {
				log.Printf("変換元ファイルの処理に失敗: %v", err)
				return
			}
			log.Printf("変換元ファイルの処理: %s", decision)
		}(inPath)
	}

	wg.Wait()
	log.Println("✅ すべて完了")
}
