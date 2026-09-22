package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/mt4110/rec-watch/internal/config"
	"github.com/mt4110/rec-watch/internal/convert"
	"github.com/mt4110/rec-watch/internal/logger"
	"github.com/mt4110/rec-watch/internal/preflight"
	"github.com/mt4110/rec-watch/internal/tui"
	"github.com/mt4110/rec-watch/internal/watcher"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "TUIモードで監視・変換を行います (Interactive)",
	Run: func(cmd *cobra.Command, args []string) {
		// Mute stdout logging to prevent TUI corruption
		logger.MuteStdout()

		// Load config (already done in PersistentPreRun but okay to access global cfg or reload)
		// PersistentPreRun sets global 'cfg'. We can use that if we are in the same package.
		// Since cmd package is same, we can access 'cfg'. But let's be safe.
		if cfg == nil {
			var err error
			cfg, err = config.Load()
			if err != nil {
				cfg = config.NewDefault()
			}
		}

		// Ensure we have at least one watch dir
		if len(cfg.WatchDirs) == 0 {
			cfg.WatchDirs = []string{"."}
		}
		requireRuntime(preflight.Options{CheckOutputDir: true, WatchDirs: cfg.WatchDirs})

		// Dependencies
		cvt := convert.New(cfg)
		eventChan := make(chan interface{}, 100)

		w := watcher.New(cfg, cvt)
		w.EventChan = eventChan

		// Run Watcher in BG
		go func() {
			w.Run()
		}()

		// Initialize TUI Model
		m := tui.NewModel(cfg, eventChan)

		// Start Bubble Tea Program
		p := tea.NewProgram(m, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Alas, there's been an error: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
