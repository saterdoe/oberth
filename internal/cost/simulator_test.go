package cost

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSimulate_OneScenario_ProjectedCost(t *testing.T) {
	ctx := context.Background()
	input := SimulationInput{
		CurrentMonthlySpend: 100,
		Scenarios: []ScenarioInput{
			{Name: "all-gpt4o-mini", PctGPT4o: 1.0},
		},
	}

	result, err := Simulate(ctx, input, nil)
	require.NoError(t, err)
	require.Len(t, result.Scenarios, 1)
	assert.Equal(t, 100.0, result.CurrentCost)
	assert.Equal(t, 100.0, result.Scenarios[0].ProjectedCost)
	assert.InDelta(t, 0, result.Scenarios[0].MonthlySavings, 0.01)
	assert.InDelta(t, 0, result.Scenarios[0].SavingsPct, 0.01)
}

func TestSimulate_LocalOnly_CostZero(t *testing.T) {
	ctx := context.Background()
	input := SimulationInput{
		CurrentMonthlySpend: 500,
		Scenarios: []ScenarioInput{
			{Name: "all-local", PctLocal: 1.0},
		},
	}

	result, err := Simulate(ctx, input, nil)
	require.NoError(t, err)
	require.Len(t, result.Scenarios, 1)
	assert.Equal(t, 0.0, result.Scenarios[0].ProjectedCost)
	assert.Equal(t, 500.0, result.Scenarios[0].MonthlySavings)
	assert.Equal(t, 1.0, result.Scenarios[0].SavingsPct)
}

func TestSimulate_NonLocal_CostPositive(t *testing.T) {
	ctx := context.Background()
	input := SimulationInput{
		CurrentMonthlySpend: 200,
		Scenarios: []ScenarioInput{
			{Name: "all-gpt4o", PctGPT4: 1.0},
		},
	}

	result, err := Simulate(ctx, input, nil)
	require.NoError(t, err)
	require.Len(t, result.Scenarios, 1)
	assert.Greater(t, result.Scenarios[0].ProjectedCost, 0.0)
}

func TestSimulate_IdentifiesCheapestScenario(t *testing.T) {
	ctx := context.Background()
	input := SimulationInput{
		CurrentMonthlySpend: 300,
		Scenarios: []ScenarioInput{
			{Name: "local", PctLocal: 1.0},
			{Name: "gpt4o", PctGPT4: 1.0},
			{Name: "gpt4o-mini", PctGPT4o: 1.0},
		},
	}

	result, err := Simulate(ctx, input, nil)
	require.NoError(t, err)
	require.Len(t, result.Scenarios, 3)
	assert.Equal(t, "local", result.BestScenario)
	assert.Equal(t, 0.0, result.Scenarios[0].ProjectedCost)
	assert.Greater(t, result.Scenarios[1].ProjectedCost, result.Scenarios[0].ProjectedCost)
	assert.Greater(t, result.Scenarios[2].ProjectedCost, result.Scenarios[0].ProjectedCost)
}

func TestSimulate_WithCustomPrices(t *testing.T) {
	ctx := context.Background()
	prices := map[string]Price{
		"cheap":  {Input: 0.10, Output: 0.30},
		"pricey": {Input: 5.00, Output: 20.00},
	}

	input := SimulationInput{
		CurrentMonthlySpend: 100,
		Scenarios: []ScenarioInput{
			{Name: "cheap-only", PctGPT4o: 1.0},
			{Name: "pricey-only", PctGPT4: 1.0},
		},
	}

	result, err := Simulate(ctx, input, prices)
	require.NoError(t, err)
	require.Len(t, result.Scenarios, 2)
	// Both should have positive projected cost since neither model maps to a price with cost > 0
	// The model keys don't match "cheap"/"pricey", so blended will be 0.
	assert.Equal(t, 0.0, result.Scenarios[0].ProjectedCost)
	assert.Equal(t, 0.0, result.Scenarios[1].ProjectedCost)
	assert.Equal(t, "cheap-only", result.BestScenario)
}

func TestSimulate_MixedDistribution(t *testing.T) {
	ctx := context.Background()
	input := SimulationInput{
		CurrentMonthlySpend: 1000,
		Scenarios: []ScenarioInput{
			{Name: "mixed", PctLocal: 0.3, PctGPT4o: 0.5, PctGPT4: 0.2},
		},
	}

	result, err := Simulate(ctx, input, nil)
	require.NoError(t, err)
	require.Len(t, result.Scenarios, 1)

	// blended = 0.3*0 + 0.5*0.375 + 0.2*6.25 = 0 + 0.1875 + 1.25 = 1.4375
	// volume = 1000 / 0.375 = 2666.67
	// projected = 2666.67 * 1.4375 = 3833.33
	assert.InDelta(t, 3833.33, result.Scenarios[0].ProjectedCost, 0.5)
	assert.InDelta(t, -2833.33, result.Scenarios[0].MonthlySavings, 0.5)
}
