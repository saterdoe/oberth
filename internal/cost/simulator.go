package cost

import (
	"context"
)

// SimulationInput represents a hypothetical traffic distribution.
type SimulationInput struct {
	CurrentMonthlySpend float64         `json:"current_monthly_spend"`
	Scenarios           []ScenarioInput `json:"scenarios"`
}

// ScenarioInput defines a single what-if distribution across models.
type ScenarioInput struct {
	Name      string  `json:"name"`
	PctLocal  float64 `json:"pct_local"`
	PctGPT4o  float64 `json:"pct_gpt4o"`
	PctGPT4   float64 `json:"pct_gpt4"`
	PctClaude float64 `json:"pct_claude"`
	PctGemini float64 `json:"pct_gemini"`
}

// SimulationResult shows projected costs for each scenario.
type SimulationResult struct {
	CurrentCost  float64          `json:"current_cost"`
	Scenarios    []ScenarioResult `json:"scenarios"`
	BestScenario string           `json:"best_scenario"`
}

// ScenarioResult holds the projected outcome for one scenario.
type ScenarioResult struct {
	Name           string  `json:"name"`
	ProjectedCost  float64 `json:"projected_cost"`
	MonthlySavings float64 `json:"monthly_savings"`
	SavingsPct     float64 `json:"savings_pct"`
}

// Price per 1M tokens for a model.
type Price struct {
	Input  float64 `json:"input"`
	Output float64 `json:"output"`
}

// DefaultPrices are the built-in approximate prices per 1M tokens.
var DefaultPrices = map[string]Price{
	"local":           {Input: 0.0, Output: 0.0},
	"gpt-4o-mini":     {Input: 0.15, Output: 0.60},
	"gpt-4o":          {Input: 2.50, Output: 10.00},
	"claude-sonnet-4": {Input: 3.00, Output: 15.00},
	"gemini-1.5-pro":  {Input: 1.25, Output: 5.00},
}

var modelKeys = []string{"local", "gpt-4o-mini", "gpt-4o", "claude-sonnet-4", "gemini-1.5-pro"}

func pctSlice(s ScenarioInput) []float64 {
	return []float64{s.PctLocal, s.PctGPT4o, s.PctGPT4, s.PctClaude, s.PctGemini}
}

// unitCost returns the cost per 1M total tokens assuming a 50/50 input/output split.
func unitCost(p Price) float64 {
	return (p.Input + p.Output) / 2.0
}

// Simulate projects costs for multiple scenarios.
// It estimates monthly token volume from current_monthly_spend using
// gpt-4o-mini as the reference price, then applies each scenario's
// distribution to compute projected costs.
func Simulate(ctx context.Context, input SimulationInput, prices map[string]Price) (*SimulationResult, error) {
	if prices == nil {
		prices = DefaultPrices
	}

	ref := prices["gpt-4o-mini"]
	refUnit := unitCost(ref)
	if refUnit == 0 {
		for _, p := range prices {
			if u := unitCost(p); u > 0 {
				refUnit = u
				break
			}
		}
	}

	monthlyUnits := input.CurrentMonthlySpend / refUnit

	var scenarios []ScenarioResult
	cheapest := ""
	cheapestCost := -1.0

	for _, s := range input.Scenarios {
		pcts := pctSlice(s)
		var blended float64
		for i, key := range modelKeys {
			p, ok := prices[key]
			if !ok {
				continue
			}
			blended += pcts[i] * unitCost(p)
		}

		proj := monthlyUnits * blended
		savings := input.CurrentMonthlySpend - proj
		var savingsPct float64
		if input.CurrentMonthlySpend > 0 {
			savingsPct = savings / input.CurrentMonthlySpend
		}

		scenarios = append(scenarios, ScenarioResult{
			Name:           s.Name,
			ProjectedCost:  proj,
			MonthlySavings: savings,
			SavingsPct:     savingsPct,
		})

		if cheapestCost < 0 || proj < cheapestCost {
			cheapestCost = proj
			cheapest = s.Name
		}
	}

	return &SimulationResult{
		CurrentCost:  input.CurrentMonthlySpend,
		Scenarios:    scenarios,
		BestScenario: cheapest,
	}, nil
}
