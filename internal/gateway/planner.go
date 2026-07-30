package gateway

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/saterdoe/oberth/pkg/llm"
)

// Step represents a single execution step.
type Step struct {
	ID         string               `json:"id"`
	ProviderID string               `json:"provider_id"`
	Model      string               `json:"model"`
	DependsOn  []string             `json:"depends_on,omitempty"`
	Fallbacks  []Step               `json:"fallbacks,omitempty"`
	Reviewer   *StepReviewer        `json:"reviewer,omitempty"`
	Tools      []llm.ToolDefinition `json:"tools,omitempty"`
}

// StepReviewer defines a reviewer step configuration.
type StepReviewer struct {
	ProviderID    string `json:"provider_id"`
	Model         string `json:"model"`
	MaxIterations int    `json:"max_iterations"`
}

// Plan takes a RouteResult and produces an ordered list of steps to execute.
// If the result has an ExecutionGraph, it parses it into steps.
// If not, it creates a single step from the simple provider+model.
func Plan(result *RouteResult, taskDescription string) ([]Step, error) {
	if result.ExecutionGraph != nil {
		return planFromGraph(result.ExecutionGraph)
	}

	providerID := result.Provider.ID.String()
	model := result.Rule.Model

	return []Step{
		{
			ID:         "default",
			ProviderID: providerID,
			Model:      model,
		},
	}, nil
}

func planFromGraph(graph map[string]any) ([]Step, error) {
	stepsRaw, ok := graph["steps"]
	if !ok {
		return nil, errors.New("execution_graph missing 'steps' key")
	}

	stepsJSON, err := json.Marshal(stepsRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal steps: %w", err)
	}

	var steps []Step
	if err := json.Unmarshal(stepsJSON, &steps); err != nil {
		return nil, fmt.Errorf("failed to unmarshal steps: %w", err)
	}

	if len(steps) == 0 {
		return nil, errors.New("execution_graph has no steps")
	}

	if err := validateDependencies(steps); err != nil {
		return nil, err
	}

	sorted, err := topologicalSort(steps)
	if err != nil {
		return nil, err
	}

	return sorted, nil
}

func validateDependencies(steps []Step) error {
	stepIDs := make(map[string]bool, len(steps))
	for _, s := range steps {
		if s.ID == "" {
			return errors.New("step has empty id")
		}
		if stepIDs[s.ID] {
			return fmt.Errorf("duplicate step id %q", s.ID)
		}
		stepIDs[s.ID] = true
	}

	for _, s := range steps {
		for _, dep := range s.DependsOn {
			if !stepIDs[dep] {
				return fmt.Errorf("step %q depends on unknown step %q", s.ID, dep)
			}
		}
	}

	return nil
}

func topologicalSort(steps []Step) ([]Step, error) {
	stepByID := make(map[string]Step, len(steps))
	inDegree := make(map[string]int, len(steps))
	dependents := make(map[string][]string)

	for _, s := range steps {
		stepByID[s.ID] = s
		inDegree[s.ID] = len(s.DependsOn)
		for _, dep := range s.DependsOn {
			dependents[dep] = append(dependents[dep], s.ID)
		}
	}

	var queue []string
	for _, s := range steps {
		if inDegree[s.ID] == 0 {
			queue = append(queue, s.ID)
		}
	}

	var sorted []Step
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		sorted = append(sorted, stepByID[id])
		for _, dep := range dependents[id] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(sorted) != len(steps) {
		return nil, errors.New("circular dependency detected in execution graph")
	}

	return sorted, nil
}
