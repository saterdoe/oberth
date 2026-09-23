package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/saterdoe/oberth/internal/observability"
	"github.com/saterdoe/oberth/pkg/llm"
)

// ExecutorConfig holds configuration for the step executor.
type ExecutorConfig struct {
	// DefaultTimeout is the per-provider attempt timeout.
	// If zero, 30 seconds is used.
	DefaultTimeout time.Duration

	// ResolveProvider is called when a provider is not already registered.
	ResolveProvider func(ctx context.Context, providerID string) (llm.Provider, error)

	// OnFallback is called whenever execution moves from a failed attempt to the
	// next provider/model in the fallback chain.
	OnFallback                func(ctx context.Context, event FallbackEvent)
	MaxConcurrencyPerProvider int
	MaxRetries                int
	CircuitFailureThreshold   int
	CircuitOpenDuration       time.Duration
	MaxAdaptiveTimeout        time.Duration
	OnTimeoutDecision         func(ctx context.Context, event TimeoutDecision)
}

type auditContextKey string

const auditSessionIDKey auditContextKey = "gateway.audit_session_id"

// WithAuditSessionID attaches a session ID to the context for gateway audit hooks.
func WithAuditSessionID(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, auditSessionIDKey, sessionID)
}

// AuditSessionID returns the session ID attached with WithAuditSessionID.
func AuditSessionID(ctx context.Context) string {
	v, _ := ctx.Value(auditSessionIDKey).(string)
	return v
}

// Attempt records a single provider attempt during execution.
type Attempt struct {
	Provider string
	Model    string
	Err      error
}

type FallbackEvent struct {
	StepID       string
	FromProvider string
	FromModel    string
	ToProvider   string
	ToModel      string
	Attempt      int
	Error        string
}

type TimeoutDecision struct {
	StepID     string
	ProviderID string
	Model      string
	Provider   string
	Decision   string
	Reason     string
	Elapsed    time.Duration
	Deadline   time.Duration
}

// FallbackError is returned when all providers in the fallback chain fail.
type FallbackError struct {
	Tried []Attempt
}

func (e *FallbackError) Error() string {
	var b strings.Builder
	b.WriteString("all providers in fallback chain failed:")
	for i, a := range e.Tried {
		if i > 0 {
			b.WriteString(";")
		}
		fmt.Fprintf(&b, " [%s/%s: %s]", a.Provider, a.Model, a.Err)
	}
	return b.String()
}

// Unwrap preserves the typed causes from every provider attempt so callers can
// classify timeout, authentication and availability failures without parsing
// the human-readable aggregate message.
func (e *FallbackError) Unwrap() []error {
	causes := make([]error, 0, len(e.Tried))
	for _, attempt := range e.Tried {
		if attempt.Err != nil {
			causes = append(causes, attempt.Err)
		}
	}
	return causes
}

// StepExecutor executes a single step with fallback chain support.
type StepExecutor struct {
	llmProviders map[string]llm.Provider
	config       ExecutorConfig
	mu           sync.RWMutex
	gates        map[string]chan struct{}
	failures     map[string]int
	circuitUntil map[string]time.Time
	latencies    map[string]time.Duration
}

// NewStepExecutor creates a new StepExecutor.
func NewStepExecutor(providers map[string]llm.Provider, config ExecutorConfig) *StepExecutor {
	if providers == nil {
		providers = make(map[string]llm.Provider)
	}
	return &StepExecutor{
		llmProviders: providers,
		config:       config,
		gates:        map[string]chan struct{}{},
		failures:     map[string]int{},
		circuitUntil: map[string]time.Time{},
		latencies:    map[string]time.Duration{},
	}
}

func (e *StepExecutor) timeoutFor(providerID, model string, provider llm.Provider) time.Duration {
	base := e.config.DefaultTimeout
	if base <= 0 {
		base = 30 * time.Second
	}
	if provider.Name() == "ollama" {
		return base
	}
	e.mu.RLock()
	observed := e.latencies[providerID+"\x00"+model]
	e.mu.RUnlock()
	if observed > 0 && observed*3 > base {
		base = observed * 3
	}
	if maximum := e.config.MaxAdaptiveTimeout; maximum > 0 && base > maximum {
		base = maximum
	}
	return base
}

func (e *StepExecutor) recordLatency(providerID, model string, elapsed time.Duration) {
	key := providerID + "\x00" + model
	e.mu.Lock()
	if previous := e.latencies[key]; previous > 0 {
		e.latencies[key] = (previous*3 + elapsed) / 4
	} else {
		e.latencies[key] = elapsed
	}
	e.mu.Unlock()
}

func (e *StepExecutor) executeAdaptive(ctx context.Context, stepID, providerID, model string, provider llm.Provider, request llm.ChatRequest) (*llm.ChatResponse, error) {
	soft := e.timeoutFor(providerID, model, provider)
	hard := soft
	if provider.Name() == "ollama" {
		hard = soft * 4
		if maximum := e.config.MaxAdaptiveTimeout; maximum > 0 && hard > maximum {
			hard = maximum
		}
	}
	attemptCtx, cancel := context.WithTimeout(ctx, hard)
	defer cancel()
	type result struct {
		response *llm.ChatResponse
		err      error
	}
	done := make(chan result, 1)
	started := time.Now()
	go func() {
		response, err := e.executeChat(attemptCtx, providerID, provider, request)
		done <- result{response: response, err: err}
	}()
	timer := time.NewTimer(soft)
	defer timer.Stop()
	extensions := 0
	for {
		select {
		case outcome := <-done:
			if outcome.err == nil {
				e.recordLatency(providerID, model, time.Since(started))
			}
			return outcome.response, outcome.err
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-attemptCtx.Done():
			if _, local := provider.(llm.ActivityProber); !local {
				e.emitTimeoutDecision(ctx, TimeoutDecision{
					StepID: stepID, ProviderID: providerID, Model: model, Provider: provider.Name(),
					Decision: "timeout", Reason: "cloud request produced no observable progress before its adaptive deadline",
					Elapsed: time.Since(started), Deadline: soft,
				})
			}
			return nil, fmt.Errorf("%w: hard deadline reached after %s", llm.ErrTimeout, time.Since(started).Round(time.Millisecond))
		case <-timer.C:
			prober, local := provider.(llm.ActivityProber)
			if !local {
				e.emitTimeoutDecision(ctx, TimeoutDecision{
					StepID: stepID, ProviderID: providerID, Model: model, Provider: provider.Name(),
					Decision: "timeout", Reason: "cloud request produced no observable progress before its adaptive deadline",
					Elapsed: time.Since(started), Deadline: soft,
				})
				cancel()
				return nil, fmt.Errorf("%w: cloud provider produced no observable progress for %s", llm.ErrTimeout, soft)
			}
			activity := prober.ProbeActivity(ctx, model)
			extensions++
			if activity.Active || (activity.Reachable && extensions == 1) {
				e.emitTimeoutDecision(ctx, TimeoutDecision{
					StepID: stepID, ProviderID: providerID, Model: model, Provider: provider.Name(),
					Decision: "extend", Reason: activity.State, Elapsed: time.Since(started), Deadline: hard,
				})
				timer.Reset(soft)
				continue
			}
			e.emitTimeoutDecision(ctx, TimeoutDecision{
				StepID: stepID, ProviderID: providerID, Model: model, Provider: provider.Name(),
				Decision: "timeout", Reason: activity.State, Elapsed: time.Since(started), Deadline: hard,
			})
			cancel()
			return nil, fmt.Errorf("%w: local provider is not active (%s)", llm.ErrTimeout, activity.State)
		}
	}
}

func (e *StepExecutor) emitTimeoutDecision(ctx context.Context, event TimeoutDecision) {
	if e.config.OnTimeoutDecision != nil {
		e.config.OnTimeoutDecision(ctx, event)
	}
}

func (e *StepExecutor) executeChat(ctx context.Context, providerID string, provider llm.Provider, request llm.ChatRequest) (*llm.ChatResponse, error) {
	gate, err := e.acquireProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	defer func() { <-gate }()
	retries := e.config.MaxRetries
	if retries <= 0 {
		retries = 2
	}
	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if err := e.checkCircuit(providerID); err != nil {
			return nil, err
		}
		finishMetric := observability.Start(ctx, "provider.chat", providerID)
		response, err := provider.Chat(ctx, request)
		finishMetric(err)
		if err == nil {
			e.recordProviderSuccess(providerID)
			return response, nil
		}
		lastErr = err
		e.recordProviderFailure(providerID)
		if attempt == retries || !isRetryableProviderError(err) {
			break
		}
		jitter := rand.Intn(75)
		delay := time.Duration(100*(1<<attempt)+jitter) * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func (e *StepExecutor) acquireProvider(ctx context.Context, providerID string) (chan struct{}, error) {
	e.mu.Lock()
	gate := e.gates[providerID]
	if gate == nil {
		limit := e.config.MaxConcurrencyPerProvider
		if limit <= 0 {
			limit = 2
		}
		gate = make(chan struct{}, limit)
		e.gates[providerID] = gate
	}
	e.mu.Unlock()
	select {
	case gate <- struct{}{}:
		return gate, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (e *StepExecutor) checkCircuit(providerID string) error {
	e.mu.RLock()
	until := e.circuitUntil[providerID]
	e.mu.RUnlock()
	if time.Now().Before(until) {
		return fmt.Errorf("provider circuit is open until %s", until.UTC().Format(time.RFC3339))
	}
	return nil
}

func (e *StepExecutor) recordProviderSuccess(providerID string) {
	e.mu.Lock()
	delete(e.failures, providerID)
	delete(e.circuitUntil, providerID)
	e.mu.Unlock()
}

func (e *StepExecutor) recordProviderFailure(providerID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.failures[providerID]++
	threshold := e.config.CircuitFailureThreshold
	if threshold <= 0 {
		threshold = 5
	}
	if e.failures[providerID] >= threshold {
		duration := e.config.CircuitOpenDuration
		if duration <= 0 {
			duration = 30 * time.Second
		}
		e.circuitUntil[providerID] = time.Now().Add(duration)
	}
}

func isRetryableProviderError(err error) bool {
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "429") || strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "500") || strings.Contains(lower, "502") ||
		strings.Contains(lower, "503") || strings.Contains(lower, "504") ||
		strings.Contains(lower, "timeout") || strings.Contains(lower, "temporar")
}

// ExecuteStep runs a single step with fallback chain.
// It tries the primary provider/model first. If it fails due to:
//   - Timeout (context deadline or 30s default)
//   - Provider error (5xx)
//   - Rate limit (429)
//   - Budget exceeded
//
// Then it tries the next fallback in the chain.
// If all fallbacks fail, returns the last error.
func (e *StepExecutor) ExecuteStep(ctx context.Context, step Step, messages []llm.Message) (*llm.ChatResponse, error) {
	chain := make([]struct {
		providerID string
		model      string
	}, 0, 1+len(step.Fallbacks))

	chain = append(chain, struct {
		providerID string
		model      string
	}{step.ProviderID, step.Model})

	for _, fb := range step.Fallbacks {
		chain = append(chain, struct {
			providerID string
			model      string
		}{fb.ProviderID, fb.Model})
	}

	var attempts []Attempt

	for i, link := range chain {
		if err := ctx.Err(); err != nil {
			attempts = append(attempts, Attempt{
				Provider: link.providerID,
				Model:    link.model,
				Err:      err,
			})
			return nil, &FallbackError{Tried: attempts}
		}

		provider, err := e.providerFor(ctx, link.providerID)
		if err != nil {
			err := fmt.Errorf("provider %q not available: %w", link.providerID, err)
			slog.Warn("step execution failed, attempting fallback",
				"provider", link.providerID,
				"model", link.model,
				"error", err,
			)
			attempts = append(attempts, Attempt{
				Provider: link.providerID,
				Model:    link.model,
				Err:      err,
			})
			e.emitFallback(ctx, step.ID, chain, i, err)
			continue
		}
		attemptMessages, attemptMaxTokens, attemptTools := prepareAttempt(step, i, messages)
		resp, err := e.executeAdaptive(ctx, step.ID, link.providerID, link.model, provider, llm.ChatRequest{
			Model:     link.model,
			Messages:  attemptMessages,
			Tools:     attemptTools,
			MaxTokens: attemptMaxTokens,
		})

		if err != nil {
			slog.Warn("step execution failed, attempting fallback",
				"provider", link.providerID,
				"model", link.model,
				"error", err,
			)
			attempts = append(attempts, Attempt{
				Provider: link.providerID,
				Model:    link.model,
				Err:      err,
			})
			e.emitFallback(ctx, step.ID, chain, i, err)
			continue
		}

		normalizeTokenUsage(resp, attemptMessages)
		return resp, nil
	}

	return nil, &FallbackError{Tried: attempts}
}

// ExecuteStepStream executes a step through the provider streaming contract and
// emits each text delta while preserving the same timeout and fallback policy.
func (e *StepExecutor) ExecuteStepStream(ctx context.Context, step Step, messages []llm.Message, onChunk func(string)) (*llm.ChatResponse, error) {
	if len(step.Tools) > 0 {
		response, err := e.ExecuteStep(ctx, step, messages)
		if err == nil && onChunk != nil && response.Content != "" {
			onChunk(response.Content)
		}
		return response, err
	}
	timeout := e.config.DefaultTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	chain := make([]struct{ providerID, model string }, 0, 1+len(step.Fallbacks))
	chain = append(chain, struct{ providerID, model string }{step.ProviderID, step.Model})
	for _, fallback := range step.Fallbacks {
		chain = append(chain, struct{ providerID, model string }{fallback.ProviderID, fallback.Model})
	}
	var attempts []Attempt
	for index, link := range chain {
		if err := ctx.Err(); err != nil {
			attempts = append(attempts, Attempt{Provider: link.providerID, Model: link.model, Err: err})
			return nil, &FallbackError{Tried: attempts}
		}
		provider, err := e.providerFor(ctx, link.providerID)
		if err != nil {
			attempts = append(attempts, Attempt{Provider: link.providerID, Model: link.model, Err: err})
			e.emitFallback(ctx, step.ID, chain, index, err)
			continue
		}
		if circuitErr := e.checkCircuit(link.providerID); circuitErr != nil {
			attempts = append(attempts, Attempt{Provider: link.providerID, Model: link.model, Err: circuitErr})
			e.emitFallback(ctx, step.ID, chain, index, circuitErr)
			continue
		}
		gate, gateErr := e.acquireProvider(ctx, link.providerID)
		if gateErr != nil {
			attempts = append(attempts, Attempt{Provider: link.providerID, Model: link.model, Err: gateErr})
			continue
		}
		attemptCtx, cancel := context.WithCancel(ctx)
		finishMetric := observability.Start(ctx, "provider.stream", link.providerID)
		type streamResult struct {
			stream <-chan llm.StreamEvent
			err    error
		}
		streamReady := make(chan streamResult, 1)
		attemptMessages, attemptMaxTokens, _ := prepareAttempt(step, index, messages)
		go func() {
			stream, streamErr := provider.ChatStream(attemptCtx, llm.ChatRequest{Model: link.model, Messages: attemptMessages, MaxTokens: attemptMaxTokens})
			streamReady <- streamResult{stream: stream, err: streamErr}
		}()
		inactivity := time.NewTimer(timeout)
		var stream <-chan llm.StreamEvent
		select {
		case ready := <-streamReady:
			stream, err = ready.stream, ready.err
		case <-inactivity.C:
			err = fmt.Errorf("%w: provider sent no response headers for %s", llm.ErrTimeout, timeout)
		case <-ctx.Done():
			err = ctx.Err()
		}
		if err != nil {
			<-gate
			finishMetric(err)
			cancel()
			if !inactivity.Stop() {
				select {
				case <-inactivity.C:
				default:
				}
			}
			e.recordProviderFailure(link.providerID)
			attempts = append(attempts, Attempt{Provider: link.providerID, Model: link.model, Err: err})
			e.emitFallback(ctx, step.ID, chain, index, err)
			continue
		}
		resetTimer(inactivity, timeout)
		var content strings.Builder
		var streamErr error
	streamLoop:
		for {
			select {
			case <-ctx.Done():
				streamErr = ctx.Err()
				break streamLoop
			case <-inactivity.C:
				streamErr = fmt.Errorf("%w: stream made no progress for %s", llm.ErrTimeout, timeout)
				break streamLoop
			case event, ok := <-stream:
				if !ok || event.Done {
					break streamLoop
				}
				if event.Error != nil {
					streamErr = event.Error
					break streamLoop
				}
				if event.Content != "" {
					content.WriteString(event.Content)
					resetTimer(inactivity, timeout)
					if onChunk != nil {
						onChunk(event.Content)
					}
				}
			}
		}
		cancel()
		if !inactivity.Stop() {
			select {
			case <-inactivity.C:
			default:
			}
		}
		<-gate
		finishMetric(streamErr)
		if streamErr != nil {
			e.recordProviderFailure(link.providerID)
			attempts = append(attempts, Attempt{Provider: link.providerID, Model: link.model, Err: streamErr})
			e.emitFallback(ctx, step.ID, chain, index, streamErr)
			continue
		}
		if content.Len() == 0 {
			fallbackCtx, fallbackCancel := context.WithTimeout(ctx, timeout)
			response, fallbackErr := e.executeChat(fallbackCtx, link.providerID, provider, llm.ChatRequest{Model: link.model, Messages: attemptMessages, MaxTokens: attemptMaxTokens})
			fallbackCancel()
			if fallbackErr != nil {
				attempts = append(attempts, Attempt{Provider: link.providerID, Model: link.model, Err: fallbackErr})
				e.emitFallback(ctx, step.ID, chain, index, fallbackErr)
				continue
			}
			normalizeTokenUsage(response, attemptMessages)
			if onChunk != nil && response.Content != "" {
				onChunk(response.Content)
			}
			return response, nil
		}
		e.recordProviderSuccess(link.providerID)
		response := &llm.ChatResponse{Model: link.model, Content: content.String()}
		normalizeTokenUsage(response, attemptMessages)
		return response, nil
	}
	return nil, &FallbackError{Tried: attempts}
}

func prepareAttempt(step Step, index int, messages []llm.Message) ([]llm.Message, int, []llm.ToolDefinition) {
	candidate := step
	if index > 0 && index-1 < len(step.Fallbacks) {
		candidate = step.Fallbacks[index-1]
	}
	maxTokens, tools := candidate.MaxTokens, candidate.Tools
	if maxTokens <= 0 {
		maxTokens = step.MaxTokens
	}
	if len(tools) == 0 {
		tools = step.Tools
	}
	if candidate.Budget == nil {
		return messages, maxTokens, tools
	}
	if candidate.Budget.ReservedOutputTokens > 0 && (maxTokens <= 0 || maxTokens > candidate.Budget.ReservedOutputTokens) {
		maxTokens = candidate.Budget.ReservedOutputTokens
	}
	return fitAttemptMessages(messages, candidate.Budget.SafePromptTokens), maxTokens, tools
}

func fitAttemptMessages(messages []llm.Message, safeTokens int) []llm.Message {
	if safeTokens <= 0 {
		return messages
	}
	total := 0
	for _, message := range messages {
		total += len([]rune(message.Content))/4 + 4
	}
	if total <= safeTokens {
		return messages
	}
	result := make([]llm.Message, 0, 3)
	remainingChars := (safeTokens - 16) * 4
	if remainingChars < 128 {
		remainingChars = 128
	}
	if len(messages) > 0 && messages[0].Role == "system" {
		system := messages[0]
		systemRunes := []rune(system.Content)
		systemLimit := remainingChars / 2
		if len(systemRunes) > systemLimit {
			system.Content = string(systemRunes[:systemLimit]) + "\n[System contract truncated for fallback budget; attempt is capability-degraded.]"
		}
		result = append(result, system)
		remainingChars -= len([]rune(system.Content))
	}
	latest := messages[len(messages)-1]
	limit := remainingChars - 96
	if limit < 128 {
		limit = 128
	}
	runes := []rune(latest.Content)
	if len(runes) > limit {
		latest.Content = "[Context reduced for fallback model.]\n" + string(runes[len(runes)-limit:])
	}
	if len(messages) > 2 && remainingChars-len([]rune(latest.Content)) >= 96 {
		result = append(result, llm.Message{Role: "user", Content: "[Earlier messages omitted for fallback budget; see run evidence.]"})
	}
	result = append(result, latest)
	return result
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

func (e *StepExecutor) emitFallback(ctx context.Context, stepID string, chain []struct {
	providerID string
	model      string
}, failedIndex int, err error) {
	if e.config.OnFallback == nil || failedIndex+1 >= len(chain) {
		return
	}
	next := chain[failedIndex+1]
	failed := chain[failedIndex]
	e.config.OnFallback(ctx, FallbackEvent{
		StepID:       stepID,
		FromProvider: failed.providerID,
		FromModel:    failed.model,
		ToProvider:   next.providerID,
		ToModel:      next.model,
		Attempt:      failedIndex + 1,
		Error:        err.Error(),
	})
}

func normalizeTokenUsage(resp *llm.ChatResponse, messages []llm.Message) {
	if resp == nil {
		return
	}
	if resp.InputTokens <= 0 {
		total := 0
		for _, msg := range messages {
			total += estimateTokens(msg.Role) + estimateTokens(msg.Content) + 4
		}
		resp.InputTokens = total
	}
	if resp.OutputTokens <= 0 {
		resp.OutputTokens = estimateTokens(resp.Content)
	}
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	n := len([]rune(text)) / 4
	if n < 1 {
		return 1
	}
	return n
}

func (e *StepExecutor) providerFor(ctx context.Context, providerID string) (llm.Provider, error) {
	e.mu.RLock()
	provider, ok := e.llmProviders[providerID]
	e.mu.RUnlock()
	if ok {
		return provider, nil
	}

	if e.config.ResolveProvider == nil {
		return nil, fmt.Errorf("provider %q not registered", providerID)
	}

	provider, err := e.config.ResolveProvider(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if provider == nil {
		return nil, fmt.Errorf("provider %q resolved to nil", providerID)
	}

	e.mu.Lock()
	e.llmProviders[providerID] = provider
	e.mu.Unlock()
	return provider, nil
}
