package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/saterdoe/oberth/internal/agentruntime"
	"github.com/saterdoe/oberth/internal/permission"
	"github.com/saterdoe/oberth/internal/reasoning"
	workspacepkg "github.com/saterdoe/oberth/internal/workspace"
	"github.com/saterdoe/oberth/pkg/llm"
)

func agentToolDefinitions() []llm.ToolDefinition {
	return []llm.ToolDefinition{
		{Name: "read", Description: "Read one relative workspace file.", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`)},
		{Name: "search", Description: "Search text in the workspace.", InputSchema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"],"additionalProperties":false}`)},
		{Name: "patch", Description: "Replace one exact span or create a new file.", InputSchema: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"operation":{"enum":["replace","create"]},"old_text":{"type":"string"},"new_text":{"type":"string"},"expected_hash":{"type":"string"}},"required":["path","new_text"],"additionalProperties":false}`)},
		{Name: "command", Description: "Run an allowlisted verification command.", InputSchema: json.RawMessage(`{"type":"object","properties":{"program":{"type":"string"},"args":{"type":"array","items":{"type":"string"}}},"required":["program"],"additionalProperties":false}`)},
		{Name: "record_reasoning", Description: "Record one evidence-backed claim, evidence reference or reproducible experiment without exposing private chain-of-thought.", InputSchema: json.RawMessage(`{"type":"object","properties":{"record":{"type":"object","properties":{"id":{"type":"string"},"kind":{"enum":["fact","hypothesis","assumption","unknown","property","decision"]},"statement":{"type":"string"},"status":{"enum":["open","supported","refuted","unresolved","passed","failed","unknown"]},"confidence":{"type":"number","minimum":0,"maximum":1},"evidence_ids":{"type":"array","items":{"type":"string"}},"falsifier":{"type":"string"},"scope":{"type":"string"},"required":{"type":"boolean"},"next_action":{"type":"string"}},"required":["id","kind","statement","status"],"additionalProperties":false},"evidence":{"type":"object","properties":{"id":{"type":"string"},"source":{"type":"string"},"hash":{"type":"string"},"detail":{"type":"string"}},"required":["id","source"],"additionalProperties":false},"experiment":{"type":"object","properties":{"id":{"type":"string"},"question":{"type":"string"},"preconditions":{"type":"array","items":{"type":"string"}},"environment":{"type":"string"},"command":{"type":"string"},"expectation":{"type":"string"},"observation":{"type":"string"},"status":{"enum":["passed","failed","unknown"]},"duration_ms":{"type":"integer","minimum":0},"cost":{"type":"number","minimum":0},"evidence_ids":{"type":"array","items":{"type":"string"}},"claim_ids":{"type":"array","items":{"type":"string"}},"baseline_fingerprint":{"type":"string"},"candidate_fingerprint":{"type":"string"}},"required":["id","question","environment","command","expectation","observation","status","evidence_ids"],"additionalProperties":false}},"oneOf":[{"required":["record"]},{"required":["evidence"]},{"required":["experiment"]}],"additionalProperties":false}`)},
		{Name: "stop_insufficient_evidence", Description: "Stop safely when one recorded unresolved unknown prevents a justified change.", InputSchema: json.RawMessage(`{"type":"object","properties":{"unknown_id":{"type":"string"},"summary":{"type":"string"}},"required":["unknown_id","summary"],"additionalProperties":false}`)},
		{Name: "finish", Description: "Finish after verification with a concise summary.", InputSchema: json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string"}},"required":["summary"],"additionalProperties":false}`)},
	}
}

type approvalResolver func(context.Context, permission.Request) (permission.Decision, bool)

func executeTypedTool(ctx context.Context, workspace workspacepkg.Workspace, runID, taskID, sessionID, taskType, taskRisk string, policy *permission.Engine, approvals approvalResolver, action agentruntime.Action) agentruntime.Observation {
	observation := agentruntime.Observation{SchemaVersion: agentruntime.SchemaVersion, Tool: action.Tool, Status: "ok"}
	fail := func(err error) agentruntime.Observation {
		observation.Status = "failed"
		observation.Error = err.Error()
		return observation
	}
	switch action.Tool {
	case "read":
		var args struct {
			Path string `json:"path"`
		}
		if json.Unmarshal(action.Arguments, &args) != nil || filepath.IsAbs(args.Path) {
			return fail(fmt.Errorf("read requires a relative path"))
		}
		content, err := workspace.Read(ctx, args.Path)
		if err != nil {
			return fail(err)
		}
		observation.Data = map[string]any{"path": args.Path, "content": string(content)}
	case "search":
		var args struct {
			Query string `json:"query"`
		}
		if json.Unmarshal(action.Arguments, &args) != nil || strings.TrimSpace(args.Query) == "" {
			return fail(fmt.Errorf("search requires a query"))
		}
		matches, err := workspace.Search(ctx, workspacepkg.SearchQuery{Text: args.Query})
		if err != nil {
			return fail(err)
		}
		observation.Data = matches
	case "patch":
		root := workspace.Root()
		var args struct {
			Path         string `json:"path"`
			Operation    string `json:"operation,omitempty"`
			OldText      string `json:"old_text"`
			NewText      string `json:"new_text"`
			ExpectedHash string `json:"expected_hash,omitempty"`
		}
		if json.Unmarshal(action.Arguments, &args) != nil || filepath.IsAbs(args.Path) || args.Path == "" {
			return fail(fmt.Errorf("patch requires a relative path and either create operation or old_text"))
		}
		// Tool-capable local models occasionally omit operation=create even when
		// they supply a new path and replacement text. Infer only the safe case:
		// a file that does not already exist. Existing files still require an
		// exact old_text, so this cannot silently overwrite user code.
		if args.Operation == "" && args.OldText == "" {
			if _, err := workspace.Read(ctx, args.Path); err != nil {
				args.Operation = "create"
			} else {
				return fail(fmt.Errorf("patching an existing file requires old_text"))
			}
		}
		if args.Operation != "create" && args.OldText == "" {
			return fail(fmt.Errorf("patch requires a relative path and either create operation or old_text"))
		}
		request := permission.Request{Operation: "file.write", Target: args.Path, RepoPath: root, TaskType: taskType, TaskID: taskID, SessionID: sessionID, RunID: runID, UserID: "local", Risk: taskRisk}
		decision, rule := policy.Evaluate(request)
		if decision == permission.Ask && approvals != nil {
			if resolved, ok := approvals(ctx, request); ok {
				decision = resolved
			}
		}
		if decision != permission.Allow {
			return fail(fmt.Errorf("patch requires approval"))
		}
		set, err := workspace.ApplyPatch(ctx, workspacepkg.Patch{Path: args.Path, Operation: args.Operation, OldText: args.OldText, NewText: args.NewText, ExpectedHash: args.ExpectedHash})
		if err != nil {
			return fail(err)
		}
		policyName := "default"
		if rule != nil {
			policyName = rule.Name
		}
		observation.Data = map[string]any{"path": args.Path, "operation": args.Operation, "change_set_id": set.ID, "policy": policyName, "risk": taskRisk}
	case "command":
		root := workspace.Root()
		var args struct {
			Program string   `json:"program"`
			Args    []string `json:"args"`
		}
		if json.Unmarshal(action.Arguments, &args) != nil {
			return fail(fmt.Errorf("command is not in the verification allowlist"))
		}
		if strings.EqualFold(strings.TrimSpace(args.Program), "git") && len(args.Args) == 1 && strings.TrimSpace(args.Args[0]) == "diff --check" {
			args.Args = []string{"diff", "--check"}
		}
		if !safeAgentCommand(args.Program, args.Args) {
			return fail(fmt.Errorf("command is not in the verification allowlist"))
		}
		target := strings.TrimSpace(strings.Join(append([]string{args.Program}, args.Args...), " "))
		request := permission.Request{Operation: "command.exec", Target: target, RepoPath: root, TaskType: taskType, TaskID: taskID, SessionID: sessionID, RunID: runID, UserID: "local", Risk: "low"}
		decision, rule := policy.Evaluate(request)
		// Commands in safeAgentCommand are a deliberately narrow, read-only
		// verification allowlist. They must not create an approval loop after a
		// successful patch (for example, git diff --check).
		if safeAgentCommand(args.Program, args.Args) {
			decision = permission.Allow
		}
		if decision == permission.Ask && approvals != nil {
			if resolved, ok := approvals(ctx, request); ok {
				decision = resolved
			}
		}
		if decision != permission.Allow {
			return fail(fmt.Errorf("command requires approval"))
		}
		result, err := workspace.Run(ctx, workspacepkg.Command{Program: args.Program, Args: args.Args})
		policyName := "built-in verification allowlist"
		if rule != nil {
			policyName = rule.Name
		}
		observation.Data = map[string]any{"command": target, "cwd": root, "impact": "verification only", "policy": policyName, "result": result}
		if err != nil {
			return fail(err)
		}
	case "record_reasoning":
		reasoningArguments := normalizeLegacyReasoningArguments(action.Arguments)
		args, err := reasoning.ParseActionArguments(reasoningArguments)
		if err != nil {
			return fail(err)
		}
		observation.Data = args
	default:
		return fail(fmt.Errorf("unsupported tool %q", action.Tool))
	}
	return observation
}

func normalizeLegacyReasoningArguments(raw json.RawMessage) json.RawMessage {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return raw
	}
	record, _ := value["record"].(map[string]any)
	if record == nil {
		if statement, ok := value["statement"].(string); ok && strings.TrimSpace(statement) != "" {
			record = map[string]any{"statement": strings.TrimSpace(statement)}
			for _, key := range []string{"id", "kind", "status", "scope", "confidence", "evidence_ids", "falsifier", "required", "next_action"} {
				if item, exists := value[key]; exists {
					record[key] = item
				}
			}
		} else if claim, ok := value["claim"].(string); ok && strings.TrimSpace(claim) != "" {
			record = map[string]any{"statement": strings.TrimSpace(claim), "kind": "assumption"}
			if evidenceID, ok := value["evidence_id"].(string); ok && strings.TrimSpace(evidenceID) != "" {
				record["evidence_ids"] = []string{evidenceID}
				record["status"] = "supported"
			}
		} else {
			return raw
		}
	}
	if _, ok := record["id"].(string); !ok {
		digest := sha256.Sum256([]byte(fmt.Sprint(record["statement"])))
		record["id"] = fmt.Sprintf("model-%x", digest[:6])
	}
	if _, ok := record["kind"].(string); !ok {
		record["kind"] = "assumption"
	}
	if _, ok := record["status"].(string); !ok {
		record["status"] = "open"
	}
	if kind, _ := record["kind"].(string); kind == "unknown" {
		if nextAction, _ := record["next_action"].(string); strings.TrimSpace(nextAction) == "" {
			record["next_action"] = "inspect the missing information before continuing"
		}
	}
	normalized, err := json.Marshal(map[string]any{"record": record})
	if err != nil {
		return raw
	}
	return normalized
}

func safeAgentCommand(program string, args []string) bool {
	if strings.EqualFold(strings.TrimSpace(program), "npm") &&
		len(args) == 2 &&
		strings.EqualFold(strings.TrimSpace(args[0]), "run") &&
		safeNPMBuildScript.MatchString(strings.ToLower(strings.TrimSpace(args[1]))) {
		return true
	}
	command := strings.ToLower(strings.TrimSpace(strings.Join(append([]string{program}, args...), " ")))
	allowed := []string{"git diff --check", "go test", "go vet", "npm test", "npm run test", "npm run typecheck", "npm run build", "cargo test", "python -m pytest", "pytest"}
	for _, prefix := range allowed {
		if command == prefix || strings.HasPrefix(command, prefix+" ") {
			return true
		}
	}
	return false
}

var safeNPMBuildScript = regexp.MustCompile(`^build(?::[a-z0-9][a-z0-9._-]*)+$`)
