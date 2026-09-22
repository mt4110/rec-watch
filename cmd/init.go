package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/mt4110/rec-watch/internal/launchagent"
	"github.com/mt4110/rec-watch/internal/preflight"
	"github.com/spf13/cobra"
)

var (
	installWatchDir string
	installDestDir  string
	installBinPath  string
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "LaunchAgentを生成して登録します",
	Long:  `録画監視用のLaunchAgent(plist)を生成し、launchctl bootstrap/kickstartで登録します。`,
	Run: func(cmd *cobra.Command, args []string) {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("ホームディレクトリの取得に失敗: %v", err)
		}
		execPath, err := os.Executable()
		if err != nil {
			log.Fatalf("実行ファイルパスの取得に失敗: %v", err)
		}

		paths := launchagent.DefaultPaths(home)
		data, err := launchagent.ResolveInstallOptions(launchagent.InstallOptions{
			HomeDir:    home,
			BinaryPath: installBinPath,
			WatchDir:   installWatchDir,
			DestDir:    installDestDir,
		}, execPath)
		if err != nil {
			log.Fatal(err)
		}
		if err := launchagent.EnsureInstallDirs(data, paths); err != nil {
			log.Fatal(err)
		}
		runtimeCfg := *cfg
		runtimeCfg.DestDir = data.DestDir
		runtimeCfg.SourcePolicy = "keep"
		if err := preflight.RequireRuntime(&runtimeCfg, preflight.Options{
			CheckOutputDir: true,
			WatchDirs:      []string{data.WatchDir},
		}); err != nil {
			log.Fatalf("❌ LaunchAgent登録前の実行前チェックに失敗しました:\n%v", err)
		}
		if err := launchagent.WritePlist(paths.PlistPath, data); err != nil {
			log.Fatal(err)
		}
		log.Printf("✅ plistファイルを作成: %s", paths.PlistPath)
		log.Printf("監視対象: %s", data.WatchDir)
		log.Printf("出力先: %s", data.DestDir)

		uid := os.Getuid()
		if output, err := launchagent.RunLaunchctl(launchagent.BootoutArgs(uid)); err != nil {
			log.Printf("ℹ️ 既存LaunchAgentの停止はスキップしました: %v\n%s", err, string(output))
		}
		if output, err := launchagent.RunLaunchctl(launchagent.BootstrapArgs(uid, paths.PlistPath)); err != nil {
			log.Fatalf("❌ launchctl bootstrap 失敗: %v\n%s", err, string(output))
		}
		if output, err := launchagent.RunLaunchctl(launchagent.KickstartArgs(uid)); err != nil {
			log.Fatalf("❌ launchctl kickstart 失敗: %v\n%s", err, string(output))
		}
		log.Println("✅ LaunchAgentを登録して起動しました")
	},
}

var initCmd = &cobra.Command{
	Use:    "init",
	Short:  "初期セットアップを行います",
	Long:   `installの後方互換エイリアスです。LaunchAgent(plist)の生成・登録を行います。`,
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprintln(os.Stderr, "`rec-watch init` は非推奨です。`rec-watch install` を実行します。")
		installCmd.Run(cmd, args)
	},
}

func init() {
	installCmd.Flags().StringVar(&installWatchDir, "watch", "", "監視対象ディレクトリ")
	installCmd.Flags().StringVar(&installDestDir, "dest", "", "変換後ファイルの出力先ディレクトリ")
	installCmd.Flags().StringVar(&installBinPath, "bin", "", "LaunchAgentに登録するrec-watchバイナリの絶対パス")
	initCmd.Flags().StringVar(&installWatchDir, "watch", "", "監視対象ディレクトリ")
	initCmd.Flags().StringVar(&installDestDir, "dest", "", "変換後ファイルの出力先ディレクトリ")
	initCmd.Flags().StringVar(&installBinPath, "bin", "", "LaunchAgentに登録するrec-watchバイナリの絶対パス")
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(initCmd)
}
