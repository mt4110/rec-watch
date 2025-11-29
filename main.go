package main

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

var (
	dest         string
	crf          int
	preset       string
	fps          int
	mute         bool
	keywords     []string
	noPad        bool
	stampPerFile bool
	noTrash      bool
	batchStamp   bool
	ffmpegBin    string
	concurrent   int
	watch        bool
	notify       bool
)

var rootCmd = &cobra.Command{
	Use:   "rec-watch [filesOrDirs...]",
	Short: "動画ファイルを一括で1080pのMP4に変換・監視します。",
	Long:  `macOSの画面収録などで作成された動画ファイルを、H.264形式のMP4に一括変換するCLIツール。監視モード(RecWatch)で自動化も可能。`,
	Run: func(cmd *cobra.Command, args []string) {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("ホームディレクトリの取得に失敗しました: %s", err)
		}

		// 監視モードの場合
		if watch {
			targetDir := "."
			if len(args) > 0 {
				targetDir = args[0]
			}
			runWatchMode(targetDir)
			return
		}

		// 1. ファイルを検索
		inputPatterns := args
		if len(inputPatterns) == 0 {
			// inputPatterns = []string{"**/*.{mov,MOV,m4v,mp4,avi,mkv}"}
			inputPatterns = []string{"."} // 引数がない場合はカレントディレクトリを対象とする
		}

		var files []string
		videoExtensions := "{mov,MOV,m4v,mp4,avi,mkv}"
		for _, input := range inputPatterns {
			processedInput := input
			// チルダを展開
			if input == "~" {
				processedInput = home
			} else if strings.HasPrefix(input, "~/") {
				processedInput = filepath.Join(home, input[2:])
			}

			// パターンを決定
			var pattern string
			info, err := os.Stat(processedInput)
			if err == nil && info.IsDir() {
				// 引数がディレクトリなら、その配下の動画ファイルを検索
				pattern = filepath.Join(processedInput, "**/*."+videoExtensions)
			} else {
				// 引数がファイル、またはglobパターンなら、それをそのまま使用
				pattern = processedInput
			}

			// パスが絶対パスかどうかで、Globの起点(fsys)を切り替える
			fsys := os.DirFS(".")
			globPattern := pattern
			isAbs := filepath.IsAbs(pattern)
			if isAbs {
				fsys = os.DirFS("/")
				// ルートからの相対パスに変換 (先頭の'/'を削除)
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

			// Globの結果が相対パスで返ってくる場合、絶対パスに戻す
			if isAbs {
				for i, match := range matches {
					matches[i] = filepath.Join("/", match)
				}
			}

			files = append(files, matches...)
		}
		// 重複するファイルパスを削除
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
			os.Exit(0)
		}

		// 2. キーワードでフィルタリング
		var filteredFiles []string
		if len(keywords) > 0 {
			for _, f := range files {
				lowerF := strings.ToLower(f)
				for _, k := range keywords {
					if strings.Contains(lowerF, strings.ToLower(k)) {
						filteredFiles = append(filteredFiles, f)
						break
					}
				}
			}
		} else {
			filteredFiles = files
		}

		if len(filteredFiles) == 0 {
			log.Println("キーワードに一致するファイルが見つかりません。")
			os.Exit(0)
		}

		// 3. 出力ディレクトリを作成
		baseOut, _ := filepath.Abs(dest)
		batchDir := baseOut
		if batchStamp {
			batchDir = filepath.Join(baseOut, nowStamp())
		}
		if err := os.MkdirAll(batchDir, 0755); err != nil {
			log.Fatalf("出力ディレクトリの作成に失敗: %v", err)
		}

		log.Printf("変換対象: %d件", len(filteredFiles))
		log.Printf("出力先: %s", batchDir)
		log.Printf("並列実行数: %d", concurrent)

		// 4. 並列変換処理
		var wg sync.WaitGroup
		semaphore := make(chan struct{}, concurrent)

		for _, inPath := range filteredFiles {
			wg.Add(1)
			semaphore <- struct{}{} // 実行枠を確保

			go func(inPath string) {
				defer func() {
					<-semaphore // 実行枠を解放
					wg.Done()
				}()
				if _, err := convertOne(inPath, batchDir); err != nil {
					log.Printf("❌ 変換失敗: %s -> %v", inPath, err)
				}
			}(inPath)
		}

		wg.Wait() // すべてのゴルーチンの完了を待つ
		log.Println("✅ すべて完了")
	},
}

// moveToTrash はファイルを各OSのゴミ箱に移動します。
func moveToTrash(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin": // macOS
		// macOSではAppleScriptを使うのが最も確実
		cmd := exec.Command("osascript", "-e", `tell application "Finder" to move POSIX file "`+absPath+`" to trash`)
		return cmd.Run()
	case "linux":
		// freedesktop.orgの仕様に準拠した`gio`コマンドを探す
		if _, err := exec.LookPath("gio"); err == nil {
			cmd := exec.Command("gio", "trash", absPath)
			return cmd.Run()
		}
		// `gio`がない場合のフォールバック（より多くの環境で動作する可能性がある）
		// ここでは単純化のため、gioのみをサポート対象とします。
		return fmt.Errorf("gio command not found")
	case "windows":
		// Windowsでは外部ライブラリを使うのが一般的ですが、
		// ここではPowerShellのコマンドレットを呼び出すことで対応します。
		// この方法はPowerShell 5.0以降が必要です。
		psCmd := fmt.Sprintf("Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile('%s', [Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs, [Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin)", absPath)
		cmd := exec.Command("powershell", "-Command", psCmd)
		return cmd.Run()
	default:
		return fmt.Errorf("%s はサポートされていないOSです", runtime.GOOS)
	}
}

func nowStamp() string {
	return time.Now().Format("20060102")
}

func convertOne(inPath string, outDir string) (string, error) {

	// ファイルの更新日時を取得してファイル名にする
	info, err := os.Stat(inPath)
	var timeStamp string
	if err != nil {
		// 取得できない場合は現在時刻
		timeStamp = time.Now().Format("2006-01-02_15-04-05")
	} else {
		timeStamp = info.ModTime().Format("2006-01-02_15-04-05")
	}

	outPath := filepath.Join(outDir, fmt.Sprintf("%s.mp4", timeStamp))

	vf := "scale=1920:1080:force_original_aspect_ratio=decrease"
	if !noPad {
		vf += ",pad=1920:1080:(ow-iw)/2:(oh-ih)/2"
	}

	ffmpegPath := "ffmpeg"
	if ffmpegBin != "" {
		ffmpegPath = ffmpegBin
	}

	ffmpegArgs := []string{
		"-i", inPath,
		"-vcodec", "libx264",
		"-preset", preset,
		"-crf", fmt.Sprintf("%d", crf),
		"-vf", vf,
		"-movflags", "+faststart",
	}

	// ... existing code from previous response
	if fps > 0 {
		ffmpegArgs = append(ffmpegArgs, "-r", fmt.Sprintf("%d", fps))
	}

	if mute {
		ffmpegArgs = append(ffmpegArgs, "-an")
	} else {
		ffmpegArgs = append(ffmpegArgs, "-acodec", "aac", "-b:a", "128k", "-ac", "2")
	}

	ffmpegArgs = append(ffmpegArgs, outPath)

	log.Printf("▶ 変換: %s -> %s", inPath, outPath)
	cmd := exec.Command(ffmpegPath, ffmpegArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg実行エラー: %v\n%s", err, string(output))
	}

	if !noTrash {
		if err := moveToTrash(inPath); err != nil {
			// Log the error but don't fail the whole process
			log.Printf("🗑 ゴミ箱への移動に失敗: %s -> %v", inPath, err)
		}
	}
	return outPath, nil
}

func runWatchMode(dir string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	absDir, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalf("ディレクトリパスの解決に失敗: %v", err)
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
				// ファイル作成または書き込み完了を検知
				// 注意: 画面収録ソフトによっては、書き込み中に何度もWriteイベントが発生する可能性があるため
				// 本来はデバウンス処理が必要ですが、簡易的にCreateとRename(移動してきた場合)を監視します。
				// また、大きなファイルの場合は書き込み完了を待つ必要があります。
				if event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Rename == fsnotify.Rename {
					fName := filepath.Base(event.Name)
					if strings.HasPrefix(fName, ".") {
						continue // 隠しファイルは無視
					}

					ext := strings.ToLower(filepath.Ext(fName))
					isVideo := false
					for _, v := range []string{".mov", ".mp4", ".m4v", ".avi", ".mkv"} {
						if ext == v {
							isVideo = true
							break
						}
					}
					if !isVideo {
						continue
					}

					log.Printf("新規ファイルを検知: %s", event.Name)

					// ファイル書き込み完了を簡易的に待機 (サイズが変化しなくなるまで待つなど)
					// ここでは単純に少し待つ
					time.Sleep(2 * time.Second)

					// ファイルが存在するか確認 (ゴミ箱に移動された場合などはここで弾く)
					if _, err := os.Stat(event.Name); os.IsNotExist(err) {
						log.Printf("ファイルが見つかりません (削除または移動されました): %s", event.Name)
						continue
					}

					// 処理中チェック
					processingMu.Lock()
					if processing[event.Name] {
						processingMu.Unlock()
						log.Printf("すでに処理中です: %s", event.Name)
						continue
					}
					processing[event.Name] = true
					processingMu.Unlock()

					// 処理完了後にフラグを落とす
					defer func(name string) {
						processingMu.Lock()
						delete(processing, name)
						processingMu.Unlock()
					}(event.Name)

					// 出力先
					baseOut, _ := filepath.Abs(dest)
					batchDir := baseOut
					if batchStamp {
						batchDir = filepath.Join(baseOut, nowStamp())
					}
					if err := os.MkdirAll(batchDir, 0755); err != nil {
						log.Printf("出力ディレクトリ作成失敗: %v", err)
						continue
					}

					// 絶対パスに変換してから渡す
					absPath, err := filepath.Abs(event.Name)
					if err != nil {
						log.Printf("パスの解決に失敗: %v", err)
						continue
					}

					log.Printf("変換開始: %s", absPath)
					if outPath, err := convertOne(absPath, batchDir); err != nil {
						log.Printf("❌ 変換失敗: %v", err)
						if notify {
							sendNotification("変換失敗", fmt.Sprintf("%s の変換に失敗しました。", fName), "")
						}
					} else {
						log.Printf("✅ 変換完了: %s", event.Name)
						if notify {
							sendNotification("変換完了", fmt.Sprintf("%s を変換しました。", fName), outPath)
						}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("監視エラー:", err)
			}
		}
	}()

	err = watcher.Add(absDir)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("監視を開始しました: %s", absDir)
	<-done
}

func sendNotification(title, message, filePath string) {
	// terminal-notifierがインストールされているか確認
	if _, err := exec.LookPath("terminal-notifier"); err == nil {
		args := []string{"-title", title, "-message", message, "-sound", "default"}
		if filePath != "" {
			// file:// URLを構築してエンコードする
			u := url.URL{Scheme: "file", Path: filePath}
			args = append(args, "-open", u.String())
		}
		cmd := exec.Command("terminal-notifier", args...)
		if err := cmd.Run(); err != nil {
			log.Printf("terminal-notifierでの通知送信に失敗: %v", err)
		}
		return
	}

	// フォールバック: macOSのSystem Events経由で通知を送る
	script := fmt.Sprintf(`tell application "System Events" to display notification "%s" with title "%s" sound name "default"`, message, title)
	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		log.Printf("通知の送信に失敗: %v", err)
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// デフォルト値の取得
	cwd, _ := os.Getwd()
	defaultDest := filepath.Join(cwd, "out")
	defaultConcurrent := runtime.NumCPU() - 1
	if defaultConcurrent < 1 {
		defaultConcurrent = 1
	}

	// フラグの定義 (yargsのoptionに相当)
	rootCmd.Flags().StringVar(&dest, "dest", defaultDest, "出力先ディレクトリ")
	rootCmd.Flags().IntVar(&crf, "crf", 22, "CRF値 (品質)")
	rootCmd.Flags().StringVar(&preset, "preset", "faster", "エンコードプリセット")
	rootCmd.Flags().IntVar(&fps, "fps", 30, "フレームレート (0で無効)")
	rootCmd.Flags().BoolVar(&mute, "mute", false, "音声をミュートする")
	rootCmd.Flags().StringSliceVar(&keywords, "keywords", []string{}, "ファイル名に含まれるキーワードでフィルタ")
	rootCmd.Flags().BoolVar(&noPad, "no-pad", false, "1080pにリサイズする際に黒帯を追加しない")
	rootCmd.Flags().BoolVar(&stampPerFile, "stamp-per-file", false, "個別のファイル名にタイムスタンプを追加する")
	rootCmd.Flags().BoolVar(&noTrash, "no-trash", false, "変換元のファイルをゴミ箱に移動しない")
	rootCmd.Flags().BoolVar(&batchStamp, "batch-stamp", true, "出力先ディレクトリをタイムスタンプ付きで作成する")
	rootCmd.Flags().StringVar(&ffmpegBin, "ffmpeg-bin", "", "ffmpegのバイナリパスを明示的に指定する")
	rootCmd.Flags().IntVar(&concurrent, "concurrent", defaultConcurrent, "並列実行数")
	rootCmd.Flags().BoolVar(&watch, "watch", false, "指定したディレクトリを監視して自動変換する")
	rootCmd.Flags().BoolVar(&notify, "notify", true, "変換完了時にデスクトップ通知を送る (watchモード時など)")
}

func main() {
	Execute()
}
