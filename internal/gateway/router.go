package gateway

import (
	"context"
	"encoding/json"
	"path"
	"sort"

	"github.com/google/uuid"

	"github.com/saterdoe/oberth/internal/db/repos"
)

// RouteResult represents the result of matching a rule.
type RouteResult struct {
	Rule           *repos.RoutingRule
	Provider       *repos.Provider
	ExecutionGraph map[string]any
}

// RouteRequest describes the incoming request to match against routing rules.
type RouteRequest struct {
	RepoPath string
	TaskType string
	UserID   string
}

// Router evaluates routing rules for a given request.
type Router struct {
	ruleRepo     *repos.RoutingRuleRepo
	providerRepo *repos.ProviderRepo
}

// NewRouter creates a new Router.
func NewRouter(ruleRepo *repos.RoutingRuleRepo, providerRepo *repos.ProviderRepo) *Router {
	return &Router{
		ruleRepo:     ruleRepo,
		providerRepo: providerRepo,
	}
}

// Match evaluates all active rules ordered by priority descending.
// Returns the first matching rule and its provider.
// If no rule matches, returns nil.
func (r *Router) Match(ctx context.Context, req RouteRequest) (*RouteResult, error) {
	rules, err := r.ruleRepo.List(ctx)
	if err != nil {
		return nil, err
	}

	activeRules := make([]repos.RoutingRule, 0, len(rules))
	for _, rule := range rules {
		if rule.IsActive {
			activeRules = append(activeRules, rule)
		}
	}

	sort.Slice(activeRules, func(i, j int) bool {
		return activeRules[i].Priority > activeRules[j].Priority
	})

	for _, rule := range activeRules {
		if !matchesRule(rule, req) {
			continue
		}

		provider, err := r.providerRepo.GetByID(ctx, rule.ProviderID)
		if err != nil {
			return nil, err
		}

		var execGraph map[string]any
		if rule.ExecutionGraph != nil {
			if err := json.Unmarshal(rule.ExecutionGraph, &execGraph); err != nil {
				return nil, err
			}
		}

		result := &RouteResult{
			Rule:           &rule,
			Provider:       provider,
			ExecutionGraph: execGraph,
		}
		return result, nil
	}

	return nil, nil
}

func matchesRule(rule repos.RoutingRule, req RouteRequest) bool {
	if rule.MatchRepoPattern != nil {
		matched, err := path.Match(*rule.MatchRepoPattern, req.RepoPath)
		if err != nil || !matched {
			return false
		}
	}

	if rule.MatchTaskType != nil {
		if req.TaskType == "" || *rule.MatchTaskType != req.TaskType {
			return false
		}
	}

	if rule.MatchUserID != nil {
		uid, err := uuid.Parse(req.UserID)
		if err != nil || uid != *rule.MatchUserID {
			return false
		}
	}

	return true
}
