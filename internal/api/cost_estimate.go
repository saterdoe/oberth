package api

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type costEstimateRequest struct {
	TaskType       string `json:"task_type"`
	ModelInput     string `json:"model_input"`
	ModelOutput    string `json:"model_output"`
	ContextSize    int    `json:"context_size"`
	TaskComplexity string `json:"task_complexity"` // simple, medium, complex
}

type costEstimateItem struct {
	Step      string  `json:"step"`
	Model     string  `json:"model"`
	EstTokens int     `json:"est_tokens"`
	Cost      float64 `json:"cost"`
}

type costEstimateResponse struct {
	Items     []costEstimateItem `json:"items"`
	TotalCost float64            `json:"total_cost"`
	Breakdown string             `json:"breakdown"`
}

// Approximate rates per 1K tokens (USD)
var modelRates = map[string]struct {
	input  float64
	output float64
}{
	"gpt-4o-mini":        {0.00015, 0.00060},
	"gpt-4o":             {0.00250, 0.01000},
	"claude-sonnet-4":    {0.00300, 0.01500},
	"claude-haiku-3":     {0.00025, 0.00125},
	"gemini-1.5-pro":     {0.00125, 0.00500},
	"gemini-1.5-flash":   {0.000075, 0.00030},
	"nemotron-3-nano-4b": {0, 0},
	"local":              {0, 0},
}

// Estimate tokens per task type
var taskTokenEstimates = map[string]struct {
	files    int
	analysis int
	codeGen  int
	review   int
}{
	"implementation": {5, 500, 2000, 800},
	"review":         {3, 1500, 0, 500},
	"debug":          {4, 800, 500, 400},
	"architecture":   {2, 1200, 500, 600},
	"docs":           {1, 300, 1000, 200},
	"research":       {0, 1000, 0, 300},
}

func (s *Server) handleCostEstimate(w http.ResponseWriter, r *http.Request) {
	var req costEstimateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "invalid JSON body", nil)
		return
	}

	if req.ModelInput == "" {
		req.ModelInput = "gpt-4o-mini"
	}
	if req.ModelOutput == "" {
		req.ModelOutput = "gpt-4o-mini"
	}
	if req.TaskComplexity == "" {
		req.TaskComplexity = "medium"
	}

	est, ok := taskTokenEstimates[req.TaskType]
	if !ok {
		est = taskTokenEstimates["implementation"]
	}

	complexityMult := 1.0
	switch req.TaskComplexity {
	case "simple":
		complexityMult = 0.5
	case "complex":
		complexityMult = 2.0
	}

	inRate := modelRates[req.ModelInput]
	if inRate.input == 0 && req.ModelInput != "local" {
		inRate = modelRates["gpt-4o-mini"]
	}
	outRate := modelRates[req.ModelOutput]
	if outRate.output == 0 && req.ModelOutput != "local" {
		outRate = modelRates["gpt-4o-mini"]
	}

	items := []costEstimateItem{}

	// Context retrieval
	ctxTokens := int(float64(est.files*200+req.ContextSize) * complexityMult)
	items = append(items, costEstimateItem{
		Step:      "context-retrieval",
		Model:     "local",
		EstTokens: ctxTokens,
		Cost:      0,
	})

	// Analysis
	analysisTokens := int(float64(est.analysis) * complexityMult)
	analysisCost := float64(analysisTokens) / 1000 * inRate.input
	items = append(items, costEstimateItem{
		Step:      "analysis",
		Model:     req.ModelInput,
		EstTokens: analysisTokens,
		Cost:      analysisCost,
	})

	// Code generation (if applicable)
	if est.codeGen > 0 {
		codeTokens := int(float64(est.codeGen) * complexityMult)
		codeCost := float64(codeTokens) / 1000 * outRate.output
		items = append(items, costEstimateItem{
			Step:      "generation",
			Model:     req.ModelOutput,
			EstTokens: codeTokens,
			Cost:      codeCost,
		})
	}

	// Review
	reviewTokens := int(float64(est.review) * complexityMult)
	reviewCost := float64(reviewTokens) / 1000 * outRate.input
	items = append(items, costEstimateItem{
		Step:      "review",
		Model:     req.ModelOutput,
		EstTokens: reviewTokens,
		Cost:      reviewCost,
	})

	total := 0.0
	for _, item := range items {
		total += item.Cost
	}

	breakdown := fmt.Sprintf("~$%.4f (%s: %s → %s)", total, req.TaskType, req.ModelInput, req.ModelOutput)

	respondJSON(w, http.StatusOK, costEstimateResponse{
		Items:     items,
		TotalCost: total,
		Breakdown: breakdown,
	})
}

func estimateCallCost(model string, inputTokens, outputTokens int) (float64, float64) {
	rate, ok := modelRates[model]
	if !ok {
		rate = modelRates["gpt-4o-mini"]
	}
	return float64(inputTokens) / 1000 * rate.input, float64(outputTokens) / 1000 * rate.output
}
