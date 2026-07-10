//go:build !darwin

package prompt

import "github.com/mt4110/rec-watch/internal/postprocess"

type SourcePrompter struct{}

func NewSourcePrompter() SourcePrompter {
	return SourcePrompter{}
}

func (SourcePrompter) AskSourceDecision(req postprocess.SourceDecisionRequest) (postprocess.Decision, error) {
	return postprocess.DecisionKeep, nil
}
