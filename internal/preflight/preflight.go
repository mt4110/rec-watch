package preflight

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mt4110/rec-watch/internal/config"
)

type Options struct {
	CheckOutputDir bool
	WatchDirs      []string
}

func RequireRuntime(cfg *config.Config, opt Options) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	if err := requireFFmpeg(cfg.FFmpegBin); err != nil {
		return err
	}
	if opt.CheckOutputDir && !cfg.DryRun {
		if err := requireWritableDir(cfg.DestDir); err != nil {
			return err
		}
	}
	for _, dir := range opt.WatchDirs {
		if err := requireExistingDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func requireFFmpeg(ffmpegBin string) error {
	path := ffmpegBin
	if path == "" {
		resolved, err := exec.LookPath("ffmpeg")
		if err != nil {
			return errors.New("ffmpeg が見つかりません。rec-watch は動画変換に ffmpeg が必要です。\n\nInstall:\n  brew install ffmpeg")
		}
		path = resolved
	} else if !filepath.IsAbs(path) {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return fmt.Errorf("指定された ffmpeg が見つかりません: %s", ffmpegBin)
		}
		path = resolved
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, path, "-version").CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("ffmpeg の起動確認がタイムアウトしました: %s", path)
		}
		return fmt.Errorf("ffmpeg を実行できません: %s\n%s", path, string(out))
	}
	return nil
}

func requireWritableDir(dir string) error {
	if dir == "" {
		return errors.New("出力先ディレクトリが空です")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("出力先ディレクトリの解決に失敗しました: %w", err)
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		return fmt.Errorf("出力先ディレクトリを作成できません: %s: %w", abs, err)
	}
	tmp, err := os.CreateTemp(abs, ".rec-watch-preflight-*")
	if err != nil {
		return fmt.Errorf("出力先ディレクトリに書き込めません: %s: %w", abs, err)
	}
	name := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("出力先ディレクトリの書き込み確認に失敗しました: %w", err)
	}
	if err := os.Remove(name); err != nil {
		return fmt.Errorf("出力先ディレクトリの書き込み確認ファイルを削除できません: %w", err)
	}
	return nil
}

func requireExistingDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("監視対象ディレクトリの解決に失敗しました: %s: %w", dir, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("監視対象ディレクトリを確認できません: %s: %w", abs, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("監視対象はディレクトリではありません: %s", abs)
	}
	return nil
}
