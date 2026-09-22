package cmd

import (
	"log"
	"os"

	"github.com/mt4110/rec-watch/internal/launchagent"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "初期セットアップの設定を削除します",
	Long:  `LaunchAgent(plist)の停止と削除を行います。`,
	Run: func(cmd *cobra.Command, args []string) {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatalf("ホームディレクトリの取得に失敗: %v", err)
		}

		paths := launchagent.DefaultPaths(home)

		log.Printf("LaunchAgentを停止しています: %s", launchagent.Label)
		if output, err := launchagent.RunLaunchctl(launchagent.BootoutArgs(os.Getuid())); err != nil {
			log.Printf("⚠️ 停止に失敗しました (すでに停止済みの可能性があります): %v\n%s", err, string(output))
		} else {
			log.Println("✅ 停止成功")
		}

		if _, err := os.Stat(paths.PlistPath); err == nil {
			if err := os.Remove(paths.PlistPath); err != nil {
				log.Fatalf("❌ plistファイルの削除に失敗: %v", err)
			}
			log.Println("✅ plistファイルを削除しました")
		} else {
			log.Println("⚠️ plistファイルが見つかりません")
		}

		log.Println("アンインストール完了 (ログファイルと出力ディレクトリ、rec-watchバイナリ自体は残っています)")
	},
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}
