package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const SchemaVersion = "1"

var (
	ErrTurnLimit            = errors.New("agent turn limit reached")
	ErrInvalidAction        = errors.New("invalid typed agent action")
	ErrModelFormatExhausted = errors.New("model response remained unusable after format recovery")
	ErrBudgetExceeded       = errors.New("agent budget exceeded")
)

type Action struct {
	SchemaVersion string          `json:"schema_version"`
	Tool          string          `json:"tool"`
	Arguments     json.RawMessage `json:"arguments"`
	Summary       string          `json:"summary,omitempty"`
}

type Observation struct {
	SchemaVersion string    `json:"schema_version"`
	Tool          string    `json:"tool"`
	Status        string    `json:"status"`
	Data          any       `json:"data,omitempty"`
	Error         string    `json:"error,omitempty"`
	Evidence      *Evidence `json:"evidence,omitempty"`
}

type Evidence struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Hash        string `json:"hash"`
	Subject     string `json:"subject,omitempty"`
	SubjectHash string `json:"subject_hash,omitempty"`
	Detail      string `json:"detail,omitempty"`
}

type Message struct {
	Role    string
	Content string
}

type ModelResponse struct {
	Content      string
	Model        string
	InputTokens  int
	OutputTokens int
}

type Model func(context.Context, []Message) (ModelResponse, error)
type ExecuteTool func(context.Context, Action) Observation
type OnTurn func(turn int, action Action, observation *Observation)

type Config struct {
	MaxTurns            int
	MaxInputTokens      int
	MaxOutputTokens     int
	MaxToolCalls        int
	MaxFormatRetries    int
	MaxProtocolRetries  int
	MaxObservationBytes int
	MaxDuration         time.Duration
	Model               Model
	Execute             ExecuteTool
	OnTurn              OnTurn
}

type Result struct {
	Summary       string
	Termination   string
	UnknownID     string
	Model         string
	Turns         int
	InputTokens   int
	OutputTokens  int
	Actions       []Action
	Observations  []Observation
	JSONFallbacks int
}

func Run(ctx context.Context, systemPrompt, intention string, cfg Config) (Result, error) {
	if cfg.Model == nil || cfg.Execute == nil {
		return Result{}, errors.New("model and tool executor are required")
	}
	if cfg.MaxTurns <= 0 {
		cfg.MaxTurns = 12
	}
	if cfg.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.MaxDuration)
		defer cancel()
	}
	messages := []Message{{Role: "system", Content: systemPrompt}, {Role: "user", Content: intention}}
	result := Result{}
	formatRetries := 0
	protocolRetries := 0
	unresolvedUnknowns := map[string]struct{}{}
	for turn := 1; turn <= cfg.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if cfg.MaxInputTokens > 0 {
			reservation := 0
			for _, message := range messages {
				reservation += len([]rune(message.Content))/4 + 4
			}
			if result.InputTokens+reservation > cfg.MaxInputTokens {
				return result, ErrBudgetExceeded
			}
		}
		response, err := cfg.Model(ctx, messages)
		if err != nil {
			return result, err
		}
		result.Turns = turn
		result.Model = response.Model
		result.InputTokens += response.InputTokens
		result.OutputTokens += response.OutputTokens
		if (cfg.MaxInputTokens > 0 && result.InputTokens > cfg.MaxInputTokens) ||
			(cfg.MaxOutputTokens > 0 && result.OutputTokens > cfg.MaxOutputTokens) {
			return result, ErrBudgetExceeded
		}
		action, fallback, err := parseAction(response.Content)
		if err != nil {
			if errors.Is(err, ErrInvalidAction) && formatRetries < cfg.MaxFormatRetries && turn < cfg.MaxTurns {
				formatRetries++
				result.JSONFallbacks++
				messages = append(messages,
					Message{Role: "assistant", Content: response.Content},
					Message{Role: "user", Content: "Your previous response was not one valid typed JSON action. Return exactly one JSON object with schema_version, tool, and arguments. Do not use markdown, comments, or multiple objects. Parser error: " + err.Error()},
				)
				continue
			}
			if errors.Is(err, ErrInvalidAction) {
				return result, fmt.Errorf("%w: %v", ErrModelFormatExhausted, err)
			}
			return result, err
		}
		if fallback {
			result.JSONFallbacks++
		}
		if action.Tool == "finish" {
			// A model must not be allowed to turn a failed mutation or verification
			// into a successful-looking summary. Give it one protocol correction per
			// failed observation so it can repair the action or verify the work.
			failed := 0
			for _, observation := range result.Observations {
				if observation.Status == "failed" {
					failed++
				}
			}
			if failed > 0 && protocolRetries < cfg.MaxProtocolRetries && turn < cfg.MaxTurns {
				protocolRetries++
				messages = append(messages,
					Message{Role: "assistant", Content: response.Content},
					Message{Role: "user", Content: fmt.Sprintf("Protocol correction: %d tool action(s) failed. Do not claim success or finish yet. Inspect the failed observation, repair or explicitly verify the requested work, then run an allowed verification command before finish.", failed)},
				)
				continue
			}
			result.Actions = append(result.Actions, action)
			result.Summary = strings.TrimSpace(action.Summary)
			if cfg.OnTurn != nil {
				cfg.OnTurn(turn, action, nil)
			}
			return result, nil
		}
		if action.Tool == "stop_insufficient_evidence" {
			var terminal struct {
				UnknownID string `json:"unknown_id"`
				Summary   string `json:"summary"`
			}
			if json.Unmarshal(action.Arguments, &terminal) != nil ||
				strings.TrimSpace(terminal.UnknownID) == "" ||
				strings.TrimSpace(terminal.Summary) == "" {
				return result, ErrInvalidAction
			}
			terminal.UnknownID = strings.TrimSpace(terminal.UnknownID)
			if _, ok := unresolvedUnknowns[terminal.UnknownID]; !ok {
				if protocolRetries < cfg.MaxProtocolRetries && turn < cfg.MaxTurns {
					protocolRetries++
					messages = append(messages,
						Message{Role: "assistant", Content: response.Content},
						Message{Role: "user", Content: fmt.Sprintf(
							"Protocol correction: unknown_id %q was not recorded by a successful record_reasoning action as kind unknown with status unresolved. Continue working on the requested task, or first record that matching unresolved unknown before stopping.",
							terminal.UnknownID,
						)},
					)
					continue
				}
				return result, ErrInvalidAction
			}
			result.Actions = append(result.Actions, action)
			result.Summary = strings.TrimSpace(terminal.Summary)
			result.Termination = "insufficient_evidence"
			result.UnknownID = terminal.UnknownID
			if cfg.OnTurn != nil {
				cfg.OnTurn(turn, action, nil)
			}
			return result, nil
		}
		result.Actions = append(result.Actions, action)
		if cfg.MaxToolCalls > 0 && len(result.Observations) >= cfg.MaxToolCalls {
			return result, ErrBudgetExceeded
		}
		observation := cfg.Execute(ctx, action)
		if observation.SchemaVersion == "" {
			observation.SchemaVersion = SchemaVersion
		}
		if observation.Tool == "" {
			observation.Tool = action.Tool
		}
		if cfg.OnTurn != nil {
			cfg.OnTurn(turn, action, &observation)
		}
		result.Observations = append(result.Observations, observation)
		if action.Tool == "record_reasoning" && observation.Status == "ok" {
			var reasoning struct {
				Record struct {
					ID     string `json:"id"`
					Kind   string `json:"kind"`
					Status string `json:"status"`
				} `json:"record"`
			}
			if json.Unmarshal(action.Arguments, &reasoning) == nil &&
				strings.EqualFold(strings.TrimSpace(reasoning.Record.Kind), "unknown") &&
				strings.EqualFold(strings.TrimSpace(reasoning.Record.Status), "unresolved") &&
				strings.TrimSpace(reasoning.Record.ID) != "" {
				unresolvedUnknowns[strings.TrimSpace(reasoning.Record.ID)] = struct{}{}
			}
		}
		if cfg.MaxObservationBytes > 0 {
			total := 0
			for _, item := range result.Observations {
				encoded, _ := json.Marshal(item)
				total += len(encoded)
			}
			if total > cfg.MaxObservationBytes {
				return result, ErrBudgetExceeded
			}
		}
		encoded, _ := json.Marshal(observation)
		messages = append(messages,
			Message{Role: "assistant", Content: response.Content},
			Message{Role: "user", Content: "Tool observation:\n" + string(encoded)},
		)
	}
	return result, ErrTurnLimit
}

func ParseAction(content string) (Action, error) {
	action, _, err := parseAction(content)
	return action, err
}

func parseAction(content string) (Action, bool, error) {
	raw := strings.TrimSpace(content)
	fallback := false
	if start := strings.Index(raw, "{"); start >= 0 {
		fallback = start != 0
		raw = raw[start:]
	}
	var action Action
	decoder := json.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(&action); err != nil {
		return Action{}, fallback, fmt.Errorf("%w: %v", ErrInvalidAction, err)
	}
	// Local models sometimes emit the requested action sequence as adjacent
	// top-level JSON objects. The runtime contract executes one action per turn,
	// so accept the first complete object and let the observation drive the next
	// model turn instead of rejecting otherwise valid typed output.
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		fallback = true
	}
	if action.SchemaVersion != SchemaVersion || !validTool(action.Tool) {
		return Action{}, fallback, ErrInvalidAction
	}
	if action.Tool != "finish" && len(action.Arguments) == 0 {
		return Action{}, fallback, ErrInvalidAction
	}
	return action, fallback, nil
}

func validTool(tool string) bool {
	switch tool {
	case "read", "search", "patch", "command", "record_reasoning", "stop_insufficient_evidence", "finish":
		return true
	default:
		return false
	}
}
