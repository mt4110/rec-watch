package cmd

import (
	"testing"

	"github.com/spf13/cobra"

	"github.com/mt4110/rec-watch/internal/config"
)

func TestUpdateConfigFromFlagsNoTrashForcesKeep(t *testing.T) {
	command := &cobra.Command{}
	command.Flags().Bool("no-trash", false, "")
	command.Flags().String("source-policy", "trash", "")

	if err := command.Flags().Set("no-trash", "true"); err != nil {
		t.Fatal(err)
	}
	if err := command.Flags().Set("source-policy", "ask"); err != nil {
		t.Fatal(err)
	}

	flagNoTrash = true
	flagSourcePolicy = "ask"

	cfg := config.NewDefault()
	updateConfigFromFlags(command, cfg)

	if cfg.SourcePolicy != "keep" {
		t.Fatalf("SourcePolicy = %q, want %q", cfg.SourcePolicy, "keep")
	}
}
