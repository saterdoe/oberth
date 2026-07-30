package gateway

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/saterdoe/oberth/internal/db/repos"
)

func TestPlan_SimpleRoute(t *testing.T) {
	rule := &repos.RoutingRule{
		ID:    uuid.New(),
		Model: "gpt-4",
	}
	provider := &repos.Provider{
		ID:   uuid.New(),
		Name: "test-provider",
	}
	result := &RouteResult{
		Rule:     rule,
		Provider: provider,
	}

	steps, err := Plan(result, "write tests")
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, "default", steps[0].ID)
	assert.Equal(t, provider.ID.String(), steps[0].ProviderID)
	assert.Equal(t, "gpt-4", steps[0].Model)
}

func TestPlan_WithExecutionGraph(t *testing.T) {
	rule := &repos.RoutingRule{
		ID:    uuid.New(),
		Model: "gpt-4",
	}
	provider := &repos.Provider{
		ID:   uuid.New(),
		Name: "test-provider",
	}
	result := &RouteResult{
		Rule:     rule,
		Provider: provider,
		ExecutionGraph: map[string]any{
			"steps": []any{
				map[string]any{
					"id":          "analyze",
					"provider_id": provider.ID.String(),
					"model":       "gpt-4",
				},
				map[string]any{
					"id":          "generate",
					"provider_id": provider.ID.String(),
					"model":       "claude-3",
				},
				map[string]any{
					"id":          "review",
					"provider_id": provider.ID.String(),
					"model":       "gpt-4",
				},
			},
		},
	}

	steps, err := Plan(result, "write tests")
	require.NoError(t, err)
	require.Len(t, steps, 3)
	assert.Equal(t, "analyze", steps[0].ID)
	assert.Equal(t, "generate", steps[1].ID)
	assert.Equal(t, "review", steps[2].ID)
}

func TestPlan_WithDependencies(t *testing.T) {
	rule := &repos.RoutingRule{ID: uuid.New(), Model: "gpt-4"}
	provider := &repos.Provider{ID: uuid.New(), Name: "test-provider"}
	pid := provider.ID.String()

	result := &RouteResult{
		Rule:     rule,
		Provider: provider,
		ExecutionGraph: map[string]any{
			"steps": []any{
				map[string]any{
					"id":          "generate",
					"provider_id": pid,
					"model":       "claude-3",
					"depends_on":  []any{"analyze"},
				},
				map[string]any{
					"id":          "review",
					"provider_id": pid,
					"model":       "gpt-4",
					"depends_on":  []any{"generate"},
				},
				map[string]any{
					"id":          "analyze",
					"provider_id": pid,
					"model":       "gpt-4",
				},
			},
		},
	}

	steps, err := Plan(result, "write tests")
	require.NoError(t, err)
	require.Len(t, steps, 3)

	stepIDs := make([]string, len(steps))
	for i, s := range steps {
		stepIDs[i] = s.ID
	}

	assert.Equal(t, "analyze", stepIDs[0], "analyze should be first (no deps)")
	assert.Equal(t, "generate", stepIDs[1], "generate should be second (depends on analyze)")
	assert.Equal(t, "review", stepIDs[2], "review should be third (depends on generate)")
}

func TestPlan_CircularDependency(t *testing.T) {
	rule := &repos.RoutingRule{ID: uuid.New(), Model: "gpt-4"}
	provider := &repos.Provider{ID: uuid.New(), Name: "test-provider"}
	pid := provider.ID.String()

	result := &RouteResult{
		Rule:     rule,
		Provider: provider,
		ExecutionGraph: map[string]any{
			"steps": []any{
				map[string]any{
					"id":          "step-a",
					"provider_id": pid,
					"model":       "gpt-4",
					"depends_on":  []any{"step-c"},
				},
				map[string]any{
					"id":          "step-b",
					"provider_id": pid,
					"model":       "gpt-4",
					"depends_on":  []any{"step-a"},
				},
				map[string]any{
					"id":          "step-c",
					"provider_id": pid,
					"model":       "gpt-4",
					"depends_on":  []any{"step-b"},
				},
			},
		},
	}

	_, err := Plan(result, "write tests")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestPlan_MissingDependency(t *testing.T) {
	rule := &repos.RoutingRule{ID: uuid.New(), Model: "gpt-4"}
	provider := &repos.Provider{ID: uuid.New(), Name: "test-provider"}
	pid := provider.ID.String()

	result := &RouteResult{
		Rule:     rule,
		Provider: provider,
		ExecutionGraph: map[string]any{
			"steps": []any{
				map[string]any{
					"id":          "step-a",
					"provider_id": pid,
					"model":       "gpt-4",
					"depends_on":  []any{"nonexistent"},
				},
			},
		},
	}

	_, err := Plan(result, "write tests")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "depends on unknown step")
}

func TestPlan_EmptySteps(t *testing.T) {
	rule := &repos.RoutingRule{ID: uuid.New(), Model: "gpt-4"}
	provider := &repos.Provider{ID: uuid.New(), Name: "test-provider"}

	result := &RouteResult{
		Rule:     rule,
		Provider: provider,
		ExecutionGraph: map[string]any{
			"steps": []any{},
		},
	}

	_, err := Plan(result, "write tests")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no steps")
}

func TestPlan_MissingStepsKey(t *testing.T) {
	rule := &repos.RoutingRule{ID: uuid.New(), Model: "gpt-4"}
	provider := &repos.Provider{ID: uuid.New(), Name: "test-provider"}

	result := &RouteResult{
		Rule:     rule,
		Provider: provider,
		ExecutionGraph: map[string]any{
			"type": "simple",
		},
	}

	_, err := Plan(result, "write tests")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'steps' key")
}
