package structuredoutput

import (
	"context"
	"errors"
)

// BAML's Go runtime does not currently ship a Windows implementation. The
// manager catches this error and uses the native JSON engine instead.
type BAMLEngine struct{ logger Logger }

func NewBAMLEngine(logger Logger) *BAMLEngine { return &BAMLEngine{logger: logger} }
func (e *BAMLEngine) Name() string            { return "baml" }
func bamlUnavailable() error                  { return errors.New("BAML runtime is unavailable on Windows") }
func (e *BAMLEngine) ClassifyTask(context.Context, ClassifyTaskInput) (*ClassifyTaskOutput, error) {
	return nil, bamlUnavailable()
}
func (e *BAMLEngine) SelectContext(context.Context, SelectContextInput) (*SelectContextOutput, error) {
	return nil, bamlUnavailable()
}
func (e *BAMLEngine) ReviewPatch(context.Context, ReviewPatchInput) (*ReviewPatchOutput, error) {
	return nil, bamlUnavailable()
}
func (e *BAMLEngine) DetectADRCandidate(context.Context, DetectADRCandidateInput) (*DetectADRCandidateOutput, error) {
	return nil, bamlUnavailable()
}
func (e *BAMLEngine) ScoreRisk(context.Context, ScoreRiskInput) (*ScoreRiskOutput, error) {
	return nil, bamlUnavailable()
}
