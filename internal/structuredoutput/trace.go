package structuredoutput

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type TraceEntry struct {
	Function      string `json:"function"`
	SchemaVersion string `json:"schema_version"`
	Timestamp     string `json:"timestamp"`
	Input         any    `json:"input"`
	Output        any    `json:"output"`
	Error         string `json:"error,omitempty"`
	LatencyMs     int64  `json:"latency_ms"`
	TokensIn      int    `json:"tokens_in"`
	TokensOut     int    `json:"tokens_out"`
	Engine        string `json:"engine"`
}

type FileLogger struct {
	dir     string
	mu      sync.Mutex
	counter int
}

func NewFileLogger(dir string) (*FileLogger, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create trace dir: %w", err)
	}
	return &FileLogger{dir: dir}, nil
}

func (l *FileLogger) LogTrace(function string, input, output any, err error, latencyMs int64, tokensIn, tokensOut int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.counter++

	errStr := ""
	if err != nil {
		errStr = err.Error()
	}

	entry := TraceEntry{
		Function:      function,
		SchemaVersion: "0.1",
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		Input:         input,
		Output:        output,
		Error:         errStr,
		LatencyMs:     latencyMs,
		TokensIn:      tokensIn,
		TokensOut:     tokensOut,
		Engine:        "native_json",
	}

	data, _ := json.MarshalIndent(entry, "", "  ")
	date := time.Now().UTC().Format("2006-01-02")
	name := fmt.Sprintf("%s-%s-%03d.json", function, date, l.counter)
	fp := filepath.Join(l.dir, name)
	_ = os.WriteFile(fp, data, 0644)
}

type NopLogger struct{}

func (NopLogger) LogTrace(string, any, any, error, int64, int, int) {}
