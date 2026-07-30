package toolrunner

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/saterdoe/oberth/internal/permission"
)

type Effect string

const (
	EffectRead    Effect = "read"
	EffectWrite   Effect = "write"
	EffectExecute Effect = "execute"
)

type Schema map[string]string
type Input map[string]any

type Result struct {
	Data any `json:"data,omitempty"`
}

type Handler func(context.Context, Input) (Result, error)

type Tool struct {
	Name       string
	Schema     Schema
	Permission string
	Effect     Effect
	Timeout    time.Duration
	Handler    Handler
}

type Registry struct {
	mu        sync.RWMutex
	tools     map[string]Tool
	policy    *permission.Engine
	auditSink func(AuditRecord)
}

type AuditRecord struct {
	Tool       string        `json:"tool"`
	Permission string        `json:"permission"`
	Effect     Effect        `json:"effect"`
	Target     string        `json:"target,omitempty"`
	Status     string        `json:"status"`
	Duration   time.Duration `json:"duration"`
	Error      string        `json:"error,omitempty"`
}

func NewRegistry(policy *permission.Engine) *Registry {
	return &Registry{tools: map[string]Tool{}, policy: policy}
}

func (r *Registry) SetAuditSink(sink func(AuditRecord)) {
	r.mu.Lock()
	r.auditSink = sink
	r.mu.Unlock()
}

func (r *Registry) Register(tool Tool) error {
	if tool.Name == "" || len(tool.Schema) == 0 || tool.Permission == "" || tool.Effect == "" || tool.Timeout <= 0 || tool.Handler == nil {
		return fmt.Errorf("tool contract requires name, schema, permission, effect, timeout and handler")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[tool.Name]; exists {
		return fmt.Errorf("tool %q already registered", tool.Name)
	}
	r.tools[tool.Name] = tool
	return nil
}

func (r *Registry) Execute(ctx context.Context, name string, input Input) (result Result, err error) {
	started := time.Now()
	r.mu.RLock()
	tool, ok := r.tools[name]
	auditSink := r.auditSink
	r.mu.RUnlock()
	target, _ := input["target"].(string)
	defer func() {
		if auditSink == nil {
			return
		}
		status := "passed"
		if errors.Is(err, ErrApprovalRequired) {
			status = "denied"
		} else if err != nil {
			status = "failed"
		}
		duration := time.Since(started)
		if duration <= 0 {
			duration = time.Nanosecond
		}
		record := AuditRecord{Tool: name, Permission: tool.Permission, Effect: tool.Effect, Target: target, Status: status, Duration: duration}
		if err != nil {
			record.Error = err.Error()
		}
		auditSink(record)
	}()
	if !ok {
		return Result{}, fmt.Errorf("tool %q is not registered", name)
	}
	for key := range tool.Schema {
		if _, exists := input[key]; !exists {
			return Result{}, fmt.Errorf("missing required input %q", key)
		}
	}
	if r.policy != nil && tool.Effect != EffectRead {
		decision, _ := r.policy.Evaluate(permission.Request{Operation: tool.Permission, Target: target})
		if decision != permission.Allow {
			return Result{}, ErrApprovalRequired
		}
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, tool.Timeout)
	defer cancel()
	return tool.Handler(timeoutCtx, input)
}
