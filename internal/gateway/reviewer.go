package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/saterdoe/oberth/pkg/llm"
)

// ReviewResult represents the outcome of a review.
type ReviewResult string

const (
	ReviewApproved         ReviewResult = "approved"
	ReviewChangesRequested ReviewResult = "changes_requested"
	ReviewRejected         ReviewResult = "rejected"
)

// ReviewResponse contains the reviewer's evaluation.
type ReviewResponse struct {
	Result   ReviewResult `json:"result"`
	Feedback string       `json:"feedback,omitempty"`
	Summary  string       `json:"summary,omitempty"`
}

// Reviewer executes the critic/reviewer pattern:
//  1. Generator produces output (using the step's provider)
//  2. Reviewer reviews the output (using the reviewer's provider)
//  3. If approved → return output
//  4. If changes_requested → generator iterates with feedback
//  5. If rejected → return error
type Reviewer struct {
	executor *StepExecutor
}

// NewReviewer creates a new Reviewer.
func NewReviewer(executor *StepExecutor) *Reviewer {
	return &Reviewer{executor: executor}
}

// ExecuteWithReview runs a step with a reviewer.
// generatorMessages: the messages sent to the generator
// step: the step config (has Reviewer field with provider/model/max_iterations)
// generatorStep: a Step with the generator's provider/model (without reviewer)
//
// Returns the final approved output, or error if rejected or max iterations exceeded.
func (r *Reviewer) ExecuteWithReview(
	ctx context.Context,
	generatorMessages []llm.Message,
	step Step,
	generatorStep Step,
) (*llm.ChatResponse, error) {
	if step.Reviewer == nil {
		return r.executor.ExecuteStep(ctx, generatorStep, generatorMessages)
	}

	messages := generatorMessages
	maxIter := step.Reviewer.MaxIterations

	for iteration := 0; ; iteration++ {
		resp, err := r.executor.ExecuteStep(ctx, generatorStep, messages)
		if err != nil {
			return nil, fmt.Errorf("generator execution failed: %w", err)
		}

		reviewResp, err := r.callReviewer(ctx, step, resp.Content)
		if err != nil {
			return nil, fmt.Errorf("reviewer execution failed: %w", err)
		}

		switch reviewResp.Result {
		case ReviewApproved:
			return resp, nil
		case ReviewRejected:
			return nil, fmt.Errorf("review rejected: %s", reviewResp.Summary)
		case ReviewChangesRequested:
			if iteration >= maxIter {
				return nil, fmt.Errorf("max iterations (%d) exceeded: %s", maxIter, reviewResp.Feedback)
			}
			messages = append(generatorMessages, llm.Message{
				Role:    "user",
				Content: fmt.Sprintf("Feedback: %s\nPlease revise the output based on the feedback.", reviewResp.Feedback),
			})
		}
	}
}

func (r *Reviewer) callReviewer(ctx context.Context, step Step, output string) (*ReviewResponse, error) {
	reviewPrompt := fmt.Sprintf(
		"Review the following output. Respond with one of: approved, changes_requested (with specific feedback), or rejected. Give a brief summary.\n\nOutput:\n%s",
		output,
	)

	reviewStep := Step{
		ID:         step.ID + "-reviewer",
		ProviderID: step.Reviewer.ProviderID,
		Model:      step.Reviewer.Model,
	}

	resp, err := r.executor.ExecuteStep(ctx, reviewStep, []llm.Message{
		{Role: "user", Content: reviewPrompt},
	})
	if err != nil {
		return nil, err
	}

	slog.Info("reviewer token usage",
		"provider", step.Reviewer.ProviderID,
		"model", step.Reviewer.Model,
		"input_tokens", resp.InputTokens,
		"output_tokens", resp.OutputTokens,
	)

	return parseReviewResponse(resp.Content)
}

func parseReviewResponse(content string) (*ReviewResponse, error) {
	result := ReviewChangesRequested
	var feedback, summary string

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Result:"):
			val := strings.TrimSpace(strings.TrimPrefix(line, "Result:"))
			switch strings.ToLower(val) {
			case "approved":
				result = ReviewApproved
			case "changes_requested":
				result = ReviewChangesRequested
			case "rejected":
				result = ReviewRejected
			default:
				slog.Warn("unrecognized review result, defaulting to changes_requested",
					"result", val,
				)
				result = ReviewChangesRequested
			}
		case strings.HasPrefix(line, "Feedback:"):
			feedback = strings.TrimSpace(strings.TrimPrefix(line, "Feedback:"))
		case strings.HasPrefix(line, "Summary:"):
			summary = strings.TrimSpace(strings.TrimPrefix(line, "Summary:"))
		}
	}

	return &ReviewResponse{
		Result:   result,
		Feedback: feedback,
		Summary:  summary,
	}, nil
}
