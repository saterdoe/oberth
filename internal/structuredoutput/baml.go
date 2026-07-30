//go:build !windows

package structuredoutput

import (
	"context"
	"fmt"
	"time"

	b "github.com/saterdoe/oberth/internal/generated/baml_client"
	"github.com/saterdoe/oberth/internal/generated/baml_client/types"
)

type BAMLEngine struct {
	logger Logger
}

func NewBAMLEngine(logger Logger) *BAMLEngine {
	return &BAMLEngine{logger: logger}
}

func (e *BAMLEngine) Name() string { return "baml" }

func (e *BAMLEngine) ClassifyTask(ctx context.Context, input ClassifyTaskInput) (*ClassifyTaskOutput, error) {
	start := time.Now()
	var branch *string
	if input.CurrentBranch != "" {
		branch = &input.CurrentBranch
	}
	result, err := b.ClassifyTask(ctx, input.TaskDescription, branch)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		e.logger.LogTrace("ClassifyTask", input, nil, err, latency, 0, 0)
		return nil, fmt.Errorf("baml ClassifyTask: %w", err)
	}
	output := &ClassifyTaskOutput{
		TaskType:               string(result.Task_type),
		Confidence:             result.Confidence,
		RiskLevel:              string(result.Risk_level),
		RequiresGitChanges:     result.Requires_git_changes,
		RequiresHumanApproval:  result.Requires_human_approval,
		SuggestedExecutionMode: string(result.Suggested_execution_mode),
	}
	e.logger.LogTrace("ClassifyTask", input, output, nil, latency, 0, 0)
	return output, nil
}

func (e *BAMLEngine) SelectContext(ctx context.Context, input SelectContextInput) (*SelectContextOutput, error) {
	start := time.Now()
	result, err := b.SelectContext(ctx, input.Task, input.TaskDescription, input.CandidateNotes)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		e.logger.LogTrace("SelectContext", input, nil, err, latency, 0, 0)
		return nil, fmt.Errorf("baml SelectContext: %w", err)
	}
	output := &SelectContextOutput{
		SelectedSources: result.Selected_sources,
		RejectedSources: result.Rejected_sources,
		Reason:          result.Reason,
		Confidence:      result.Confidence,
		EstimatedTokens: int(result.Estimated_tokens),
	}
	e.logger.LogTrace("SelectContext", input, output, nil, latency, 0, 0)
	return output, nil
}

func (e *BAMLEngine) ReviewPatch(ctx context.Context, input ReviewPatchInput) (*ReviewPatchOutput, error) {
	start := time.Now()
	var testOutput *string
	if input.TestOutput != "" {
		testOutput = &input.TestOutput
	}
	result, err := b.ReviewPatch(ctx, input.Task, input.TaskDescription, input.Patch, testOutput)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		e.logger.LogTrace("ReviewPatch", input, nil, err, latency, 0, 0)
		return nil, fmt.Errorf("baml ReviewPatch: %w", err)
	}
	comments := make([]FileComment, len(result.Comments_by_file))
	for i, fc := range result.Comments_by_file {
		comments[i] = FileComment{
			FilePath: fc.File_path,
			Line:     int(fc.Line),
			Comment:  fc.Comment,
			Severity: string(fc.Severity),
		}
	}
	output := &ReviewPatchOutput{
		Approved:              result.Approved,
		Severity:              string(result.Severity),
		BlockingIssues:        result.Blocking_issues,
		CommentsByFile:        comments,
		RequiredChanges:       result.Required_changes,
		ShouldRetryGeneration: result.Should_retry_generation,
	}
	e.logger.LogTrace("ReviewPatch", input, output, nil, latency, 0, 0)
	return output, nil
}

func (e *BAMLEngine) DetectADRCandidate(ctx context.Context, input DetectADRCandidateInput) (*DetectADRCandidateOutput, error) {
	start := time.Now()
	conversation := input.SessionSummary
	if input.GeneratedChanges != "" {
		conversation += "\n\nGenerated changes:\n" + input.GeneratedChanges
	}
	if len(input.UserDecisions) > 0 {
		conversation += "\n\nUser decisions:\n"
		for _, d := range input.UserDecisions {
			conversation += "- " + d + "\n"
		}
	}
	result, err := b.DetectADRCandidate(ctx, conversation)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		e.logger.LogTrace("DetectADRCandidate", input, nil, err, latency, 0, 0)
		return nil, fmt.Errorf("baml DetectADRCandidate: %w", err)
	}
	output := &DetectADRCandidateOutput{
		ShouldCreateADR: result.Status == types.ADRStatusCandidate || result.Status == types.ADRStatusConfirmed,
		Title:           result.Title,
		Decision:        result.Decision,
		Context:         result.Context,
		Confidence:      result.Confidence,
	}
	e.logger.LogTrace("DetectADRCandidate", input, output, nil, latency, 0, 0)
	return output, nil
}

func (e *BAMLEngine) ScoreRisk(ctx context.Context, input ScoreRiskInput) (*ScoreRiskOutput, error) {
	start := time.Now()
	var diffPreview *string
	if input.RepoClassification != "" {
		diffPreview = &input.RepoClassification
	}
	result, err := b.ScoreRisk(ctx, input.TaskDescription, input.FilesToModify, diffPreview)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		e.logger.LogTrace("ScoreRisk", input, nil, err, latency, 0, 0)
		return nil, fmt.Errorf("baml ScoreRisk: %w", err)
	}
	output := &ScoreRiskOutput{
		RiskLevel:        string(result.Overall_risk),
		Reasons:          result.Factors,
		RequiresApproval: string(result.Overall_risk) == "High" || string(result.Overall_risk) == "Critical",
		RequiredChecks:   result.Factors,
	}
	e.logger.LogTrace("ScoreRisk", input, output, nil, latency, 0, 0)
	return output, nil
}
