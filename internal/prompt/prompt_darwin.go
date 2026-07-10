//go:build darwin

package prompt

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/mt4110/rec-watch/internal/postprocess"
)

type SourcePrompter struct{}

func NewSourcePrompter() SourcePrompter {
	return SourcePrompter{}
}

func (SourcePrompter) AskSourceDecision(req postprocess.SourceDecisionRequest) (postprocess.Decision, error) {
	savedPercent := 0.0
	if req.OriginalSize > 0 {
		savedPercent = float64(req.SizeDiff) / float64(req.OriginalSize) * 100
	}
	message := fmt.Sprintf(
		"変換が完了しました。\n\n元ファイル: %s\n変換後: %s\n削減率: %.1f%%\n\n元ファイルをゴミ箱へ移動しますか？",
		formatBytes(req.OriginalSize),
		formatBytes(req.ConvertedSize),
		savedPercent,
	)
	script := `on run argv
set dialogMessage to item 1 of argv
display dialog dialogMessage buttons {"残す", "ゴミ箱へ移動"} default button "ゴミ箱へ移動" with title "RecWatch"
return button returned of result
end run`

	out, err := exec.Command("osascript", "-e", script, message).Output()
	if err != nil {
		return postprocess.DecisionKeep, err
	}
	if strings.TrimSpace(string(out)) == "ゴミ箱へ移動" {
		return postprocess.DecisionTrash, nil
	}
	return postprocess.DecisionKeep, nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
