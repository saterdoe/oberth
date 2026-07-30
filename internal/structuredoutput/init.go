package structuredoutput

import (
	"context"
	"fmt"
	"log"

	"github.com/saterdoe/oberth/internal/config"
	"github.com/saterdoe/oberth/pkg/llm"
)

type Manager struct {
	Engine     Engine
	Logger     Logger
	Enabled    bool
	EngineName string
}

func NewManager(cfg config.StructuredOutputsConfig, provider llm.Provider) (*Manager, error) {
	var logger Logger
	if cfg.Traces.Enabled {
		l, err := NewFileLogger(cfg.Traces.Path)
		if err != nil {
			return nil, fmt.Errorf("init trace logger: %w", err)
		}
		logger = l
	} else {
		logger = NopLogger{}
	}

	defaultModel := "gpt-4o-mini"
	if provider != nil {
		defaultModel = "auto"
	}

	var engine Engine
	switch cfg.Engine {
	case "baml":
		engine = NewBAMLEngine(logger)
		log.Printf("[structuredoutput] using BAML engine (src: %s)", cfg.BAML.SrcPath)
	case "native_json", "":
		engine = NewNativeEngine(provider, logger, defaultModel)
	default:
		engine = NewNativeEngine(provider, logger, defaultModel)
	}

	if !cfg.Enabled {
		return &Manager{Enabled: false, Logger: logger, EngineName: cfg.Engine}, nil
	}

	return &Manager{
		Engine:     engine,
		Logger:     logger,
		Enabled:    true,
		EngineName: cfg.Engine,
	}, nil
}

// ClassifyTaskWithFallback tries the engine, falls back to raw LLM call if configured.
func (m *Manager) ClassifyTaskWithFallback(ctx context.Context, provider llm.Provider, input ClassifyTaskInput) (*ClassifyTaskOutput, error) {
	if !m.Enabled || m.Engine == nil {
		return fallbackClassifyTask(ctx, provider, input)
	}
	out, err := m.Engine.ClassifyTask(ctx, input)
	if err != nil {
		return fallbackClassifyTask(ctx, provider, input)
	}
	return out, nil
}

func fallbackClassifyTask(ctx context.Context, provider llm.Provider, input ClassifyTaskInput) (*ClassifyTaskOutput, error) {
	if provider == nil {
		return &ClassifyTaskOutput{
			TaskType:               input.TaskDescription,
			Confidence:             1.0,
			RiskLevel:              "low",
			RequiresGitChanges:     false,
			RequiresHumanApproval:  false,
			SuggestedExecutionMode: "direct",
		}, nil
	}
	e := NewNativeEngine(provider, NopLogger{}, "auto")
	return e.ClassifyTask(ctx, input)
}
