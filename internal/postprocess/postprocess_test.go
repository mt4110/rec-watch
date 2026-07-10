package postprocess

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mt4110/rec-watch/internal/convert"
)

type fakePrompter struct {
	calls    int
	decision Decision
}

func (p *fakePrompter) AskSourceDecision(req SourceDecisionRequest) (Decision, error) {
	p.calls++
	return p.decision, nil
}

func TestCanMoveSourceRequiresSmallerExistingOutput(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.mov")
	outputPath := filepath.Join(dir, "output.mp4")
	if err := os.WriteFile(inputPath, []byte("0123456789"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outputPath, []byte("12345"), 0644); err != nil {
		t.Fatal(err)
	}

	result := convert.ConvertResult{
		InputPath:     inputPath,
		OutputPath:    outputPath,
		OriginalSize:  10,
		ConvertedSize: 5,
		SizeDiff:      5,
	}
	if !CanMoveSource(result) {
		t.Fatal("expected source to be movable")
	}

	result.ConvertedSize = 10
	result.SizeDiff = 0
	if CanMoveSource(result) {
		t.Fatal("expected equal size output to be kept")
	}

	result.ConvertedSize = 5
	result.SizeDiff = 5
	result.OutputPath = filepath.Join(dir, "missing.mp4")
	if CanMoveSource(result) {
		t.Fatal("expected missing output to be kept")
	}
}

func TestHandleSourceKeepPolicyDoesNotPrompt(t *testing.T) {
	prompter := &fakePrompter{decision: DecisionTrash}
	decision, err := HandleSource(context.Background(), SourcePolicyKeep, convert.ConvertResult{}, prompter)
	if err != nil {
		t.Fatal(err)
	}
	if decision != DecisionKeep {
		t.Fatalf("decision = %q, want %q", decision, DecisionKeep)
	}
	if prompter.calls != 0 {
		t.Fatalf("prompt calls = %d, want 0", prompter.calls)
	}
}

func TestHandleSourceAskDoesNotPromptWhenUnsafe(t *testing.T) {
	prompter := &fakePrompter{decision: DecisionTrash}
	decision, err := HandleSource(context.Background(), SourcePolicyAsk, convert.ConvertResult{
		InputPath:     "input.mov",
		OutputPath:    "output.mp4",
		OriginalSize:  100,
		ConvertedSize: 0,
	}, prompter)
	if err != nil {
		t.Fatal(err)
	}
	if decision != DecisionKeep {
		t.Fatalf("decision = %q, want %q", decision, DecisionKeep)
	}
	if prompter.calls != 0 {
		t.Fatalf("prompt calls = %d, want 0", prompter.calls)
	}
}

func TestValidateSourcePolicy(t *testing.T) {
	for _, policy := range []SourcePolicy{SourcePolicyKeep, SourcePolicyTrash, SourcePolicyAsk} {
		if err := ValidateSourcePolicy(policy); err != nil {
			t.Fatalf("ValidateSourcePolicy(%q) failed: %v", policy, err)
		}
	}
	if err := ValidateSourcePolicy("delete"); err == nil {
		t.Fatal("expected invalid policy error")
	}
}
