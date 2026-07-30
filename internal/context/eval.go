package context

import (
	"context"
	"fmt"
	"time"
)

type EvalCase struct {
	Name            string         `json:"name"`
	Query           string         `json:"query"`
	TaskType        string         `json:"task_type"`
	ExpectedSources []string       `json:"expected_sources"`
	Options         CompileOptions `json:"options,omitempty"`
}

type EvalThresholds struct {
	MinRecall         float64
	MaxNoiseRatio     float64
	MinSavingsPercent float64
}

type EvalCaseResult struct {
	Name           string  `json:"name"`
	Recall         float64 `json:"recall"`
	NoiseRatio     float64 `json:"noise_ratio"`
	SavingsPercent float64 `json:"savings_percent"`
	Passed         bool    `json:"passed"`
}

type EvalReport struct {
	Passed                bool             `json:"passed"`
	AverageRecall         float64          `json:"average_recall"`
	AverageNoiseRatio     float64          `json:"average_noise_ratio"`
	AverageSavingsPercent float64          `json:"average_savings_percent"`
	Cases                 []EvalCaseResult `json:"cases"`
}

type Strategy string

const (
	StrategyNoRAG   Strategy = "no_rag"
	StrategyLexical Strategy = "lexical"
	StrategyHybrid  Strategy = "hybrid"
	StrategyWide    Strategy = "wide_context"
)

type StrategyEval struct {
	Strategy         Strategy   `json:"strategy"`
	Report           EvalReport `json:"report"`
	LatencyMs        int64      `json:"latency_ms"`
	QualityScore     float64    `json:"quality_score"`
	EstimatedCostPct float64    `json:"estimated_cost_percent"`
	NoRegression     bool       `json:"no_regression"`
}

// CompareStrategies activates a retrieval strategy only when quality does not
// regress against the wide-context baseline. Token/cost savings are secondary.
func CompareStrategies(ctx context.Context, pipelines map[Strategy]*Pipeline, cases []EvalCase, thresholds EvalThresholds, options map[Strategy]CompileOptions) ([]StrategyEval, error) {
	if len(cases) != 30 {
		return nil, fmt.Errorf("strategy gate requires exactly 30 representative cases")
	}
	order := []Strategy{StrategyNoRAG, StrategyLexical, StrategyHybrid, StrategyWide}
	results := make([]StrategyEval, 0, len(order))
	for _, strategy := range order {
		strategyCases := append([]EvalCase(nil), cases...)
		for index := range strategyCases {
			strategyCases[index].Options = options[strategy]
		}
		started := time.Now()
		pipeline := pipelines[strategy]
		if pipeline == nil {
			return nil, fmt.Errorf("missing pipeline for strategy %s", strategy)
		}
		report, err := RunEvals(ctx, pipeline, strategyCases, thresholds)
		if err != nil {
			return nil, err
		}
		quality := report.AverageRecall * (1 - report.AverageNoiseRatio)
		results = append(results, StrategyEval{
			Strategy: strategy, Report: report, LatencyMs: time.Since(started).Milliseconds(),
			QualityScore: quality, EstimatedCostPct: 100 - report.AverageSavingsPercent,
		})
	}
	baseline := results[len(results)-1].QualityScore
	for index := range results {
		results[index].NoRegression = results[index].QualityScore >= baseline
	}
	return results, nil
}

func RunEvals(ctx context.Context, pipeline *Pipeline, cases []EvalCase, thresholds EvalThresholds) (EvalReport, error) {
	if pipeline == nil || len(cases) == 0 {
		return EvalReport{}, fmt.Errorf("pipeline and at least one eval case are required")
	}
	report := EvalReport{Passed: true, Cases: make([]EvalCaseResult, 0, len(cases))}
	for _, testCase := range cases {
		result, err := pipeline.CompileWithOptions(ctx, testCase.Query, testCase.TaskType, testCase.Options)
		if err != nil {
			return report, err
		}
		selected := map[string]bool{}
		for _, source := range result.Sources {
			selected[source] = true
		}
		expected := map[string]bool{}
		matched := 0
		for _, source := range testCase.ExpectedSources {
			expected[source] = true
			if selected[source] {
				matched++
			}
		}
		recall := 1.0
		if len(expected) > 0 {
			recall = float64(matched) / float64(len(expected))
		}
		noise := 0
		for source := range selected {
			if !expected[source] {
				noise++
			}
		}
		noiseRatio := 0.0
		if len(selected) > 0 {
			noiseRatio = float64(noise) / float64(len(selected))
		}
		passed := recall >= thresholds.MinRecall && noiseRatio <= thresholds.MaxNoiseRatio && result.Metrics.SavingsPercent >= thresholds.MinSavingsPercent
		report.Cases = append(report.Cases, EvalCaseResult{testCase.Name, recall, noiseRatio, result.Metrics.SavingsPercent, passed})
		report.AverageRecall += recall
		report.AverageNoiseRatio += noiseRatio
		report.AverageSavingsPercent += result.Metrics.SavingsPercent
		if !passed {
			report.Passed = false
		}
	}
	count := float64(len(cases))
	report.AverageRecall /= count
	report.AverageNoiseRatio /= count
	report.AverageSavingsPercent /= count
	return report, nil
}
