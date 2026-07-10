package postprocess

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mt4110/rec-watch/internal/convert"
)

type SourcePolicy string

const (
	SourcePolicyKeep  SourcePolicy = "keep"
	SourcePolicyTrash SourcePolicy = "trash"
	SourcePolicyAsk   SourcePolicy = "ask"
)

type Decision string

const (
	DecisionKeep  Decision = "keep"
	DecisionTrash Decision = "trash"
)

type SourceDecisionRequest struct {
	InputPath     string
	OutputPath    string
	OriginalSize  int64
	ConvertedSize int64
	SizeDiff      int64
}

type Prompter interface {
	AskSourceDecision(req SourceDecisionRequest) (Decision, error)
}

func HandleSource(ctx context.Context, policy SourcePolicy, result convert.ConvertResult, prompter Prompter) (Decision, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return DecisionKeep, err
	}

	if policy == "" {
		policy = SourcePolicyTrash
	}
	if err := ValidateSourcePolicy(policy); err != nil {
		return DecisionKeep, err
	}
	if policy == SourcePolicyKeep {
		return DecisionKeep, nil
	}
	if !CanMoveSource(result) {
		return DecisionKeep, nil
	}

	decision := DecisionTrash
	if policy == SourcePolicyAsk {
		if prompter == nil {
			return DecisionKeep, nil
		}
		var err error
		decision, err = prompter.AskSourceDecision(SourceDecisionRequest{
			InputPath:     result.InputPath,
			OutputPath:    result.OutputPath,
			OriginalSize:  result.OriginalSize,
			ConvertedSize: result.ConvertedSize,
			SizeDiff:      result.SizeDiff,
		})
		if err != nil {
			return DecisionKeep, err
		}
	}

	if decision != DecisionTrash {
		return DecisionKeep, nil
	}
	if err := moveToTrash(ctx, result.InputPath); err != nil {
		return DecisionKeep, err
	}
	return DecisionTrash, nil
}

func ValidateSourcePolicy(policy SourcePolicy) error {
	switch policy {
	case SourcePolicyKeep, SourcePolicyTrash, SourcePolicyAsk:
		return nil
	default:
		return fmt.Errorf("invalid source policy %q: use keep, trash, or ask", policy)
	}
}

func CanMoveSource(result convert.ConvertResult) bool {
	if result.InputPath == "" || result.OutputPath == "" {
		return false
	}
	if result.OriginalSize <= 0 || result.ConvertedSize <= 0 {
		return false
	}
	if result.ConvertedSize >= result.OriginalSize {
		return false
	}
	inputInfo, err := os.Stat(result.InputPath)
	if err != nil || inputInfo.IsDir() || inputInfo.Size() <= 0 {
		return false
	}
	outputInfo, err := os.Stat(result.OutputPath)
	if err != nil || outputInfo.IsDir() || outputInfo.Size() <= 0 {
		return false
	}
	return true
}

func moveToTrash(ctx context.Context, path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	switch runtime.GOOS {
	case "darwin":
		script := `on run argv
tell application "Finder" to move POSIX file (item 1 of argv) to trash
end run`
		return exec.CommandContext(ctx, "osascript", "-e", script, absPath).Run()
	case "linux":
		if _, err := exec.LookPath("gio"); err == nil {
			return exec.CommandContext(ctx, "gio", "trash", absPath).Run()
		}
		return fmt.Errorf("gio command not found")
	case "windows":
		escapedPath := strings.ReplaceAll(absPath, "'", "''")
		psCmd := fmt.Sprintf("Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile('%s', [Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs, [Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin)", escapedPath)
		return exec.CommandContext(ctx, "powershell", "-Command", psCmd).Run()
	default:
		return fmt.Errorf("%s is not supported for trash", runtime.GOOS)
	}
}
