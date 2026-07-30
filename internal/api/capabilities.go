package api

import (
	"net/http"
)

type capabilityItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Category    string `json:"category"`
}

type capabilitiesResponse struct {
	SchemaVersion string           `json:"schema_version"`
	MCPTools      []capabilityItem `json:"mcp_tools"`
	Providers     []capabilityItem `json:"providers"`
	Memory        []capabilityItem `json:"memory"`
	Prompts       []capabilityItem `json:"prompts"`
	Skills        []capabilityItem `json:"skills"`
}

func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	resp := capabilitiesResponse{
		SchemaVersion: "1",
		Memory: []capabilityItem{
			{Name: "Candidate memory", Description: "Proposes versioned, commit-scoped memory for explicit approval; stale evidence expires.", Status: "active", Category: "memory"},
			{Name: "Context manifest", Description: "Traces selected and excluded sources, hashes, reasons, token estimates and compaction.", Status: "active", Category: "memory"},
		},
		Prompts: []capabilityItem{
			{Name: "typed-agent-v1", Description: "Versioned single-agent contract for read, search, patch, reasoning records, command and finish.", Status: "active", Category: "prompt"},
		},
		Skills: []capabilityItem{
			{Name: "Transactional workspace", Description: "Atomic, hash-checked changes confined to an isolated Git worktree.", Status: "active", Category: "skill"},
			{Name: "Durable task runtime", Description: "Lease, heartbeat, cancellation, replay and versioned result bundle.", Status: "active", Category: "skill"},
			{Name: "Policy and cost governance", Description: "Central typed policy checks, scoped approvals, budgets and provider limits.", Status: "active", Category: "skill"},
		},
	}
	if s.mcpServer != nil {
		for _, tool := range s.mcpServer.Tools() {
			resp.MCPTools = append(resp.MCPTools, capabilityItem{
				Name:        tool.Name,
				Description: tool.Description,
				Status:      "active",
				Category:    "mcp",
			})
		}
	}
	if s.providers != nil {
		providers, err := s.providers.List(r.Context())
		if err == nil {
			for _, provider := range providers {
				status := "inactive"
				if provider.IsActive {
					status = "active"
				}
				resp.Providers = append(resp.Providers, capabilityItem{
					Name:        provider.Name,
					Description: provider.ProviderType + " / " + provider.DefaultModel,
					Status:      status,
					Category:    "provider",
				})
			}
		}
	}
	if resp.MCPTools == nil {
		resp.MCPTools = []capabilityItem{}
	}
	if resp.Providers == nil {
		resp.Providers = []capabilityItem{}
	}
	respondJSON(w, http.StatusOK, resp)
}
