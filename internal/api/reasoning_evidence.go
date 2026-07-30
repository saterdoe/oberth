package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/saterdoe/oberth/internal/agentruntime"
	"github.com/saterdoe/oberth/internal/reasoning"
	workspacepkg "github.com/saterdoe/oberth/internal/workspace"
)

func attachAutomaticEvidence(turn int, action agentruntime.Action, observation *agentruntime.Observation) {
	if observation == nil || observation.Status != "ok" || observation.Evidence != nil {
		return
	}
	if action.Tool != "read" && action.Tool != "search" && action.Tool != "command" {
		return
	}
	encoded, err := json.Marshal(observation.Data)
	if err != nil {
		return
	}
	evidence := &agentruntime.Evidence{
		ID:   fmt.Sprintf("ev-turn-%03d", turn),
		Hash: fmt.Sprintf("sha256:%x", sha256.Sum256(encoded)),
	}
	switch action.Tool {
	case "read":
		var data struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		if json.Unmarshal(encoded, &data) != nil || strings.TrimSpace(data.Path) == "" {
			return
		}
		evidence.Source = "file:" + data.Path
		evidence.Subject = "file:" + data.Path
		evidence.SubjectHash = fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(data.Content)))
		evidence.Detail = "workspace file read"
	case "search":
		var args struct {
			Query string `json:"query"`
		}
		if json.Unmarshal(action.Arguments, &args) != nil || strings.TrimSpace(args.Query) == "" {
			return
		}
		evidence.Source = "search:" + strings.TrimSpace(args.Query)
		evidence.Detail = "workspace search result"
	case "command":
		var data struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(encoded, &data) != nil || strings.TrimSpace(data.Command) == "" {
			return
		}
		evidence.Source = "command:" + data.Command
		evidence.Subject = "diff"
		evidence.Detail = "verification command result"
	}
	observation.Evidence = evidence
}

func refreshReasoningEvidence(root, diffHash string, current *reasoning.CaseV1) {
	if current == nil {
		return
	}
	reasoning.BindDiffEvidence(current, diffHash)
	files, err := workspacepkg.New(root)
	if err != nil {
		return
	}
	for i := range current.Evidence {
		item := &current.Evidence[i]
		if !strings.HasPrefix(item.Subject, "file:") || item.SubjectHash == "" {
			continue
		}
		relative := strings.TrimPrefix(item.Subject, "file:")
		content, _, err := files.Read(context.Background(), relative)
		if err != nil {
			item.Stale = true
			continue
		}
		currentHash := fmt.Sprintf("sha256:%x", sha256.Sum256(content))
		item.Stale = currentHash != item.SubjectHash
	}
	current.Assessment = reasoning.Assess(current)
}
