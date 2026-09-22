package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/mt4110/rec-watch/internal/launchagent"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "LaunchAgentの状態を表示します",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := launchagent.RunLaunchctl(launchagent.PrintArgs(os.Getuid()))
		if err != nil {
			log.Printf("LaunchAgentは未登録または停止中です: %v\n%s", err, string(out))
			os.Exit(1)
		}
		fmt.Print(string(out))
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "LaunchAgentを起動します",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := launchagent.RunLaunchctl(launchagent.KickstartArgs(os.Getuid()))
		if err != nil {
			log.Fatalf("❌ 起動に失敗しました: %v\n%s", err, string(out))
		}
		log.Println("✅ LaunchAgentを起動しました")
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "LaunchAgentを停止します",
	Run: func(cmd *cobra.Command, args []string) {
		out, err := launchagent.RunLaunchctl(launchagent.BootoutArgs(os.Getuid()))
		if err != nil {
			log.Fatalf("❌ 停止に失敗しました: %v\n%s", err, string(out))
		}
		log.Println("✅ LaunchAgentを停止しました")
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
}
