package structuredoutput

import "context"

// Engine defines the interface for structured LLM output functions.
// Each method corresponds to a BAML function or its native JSON fallback.
type Engine interface {
	Name() string

	ClassifyTask(ctx context.Context, input ClassifyTaskInput) (*ClassifyTaskOutput, error)
	SelectContext(ctx context.Context, input SelectContextInput) (*SelectContextOutput, error)
	ReviewPatch(ctx context.Context, input ReviewPatchInput) (*ReviewPatchOutput, error)
	DetectADRCandidate(ctx context.Context, input DetectADRCandidateInput) (*DetectADRCandidateOutput, error)
	ScoreRisk(ctx context.Context, input ScoreRiskInput) (*ScoreRiskOutput, error)
}

// Logger is the minimal interface the engine needs for trace logging.
type Logger interface {
	LogTrace(function string, input, output any, err error, latencyMs int64, tokensIn, tokensOut int)
}
