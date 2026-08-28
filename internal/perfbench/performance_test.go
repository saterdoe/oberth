package perfbench

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/saterdoe/oberth/internal/codeindex"
	semcontext "github.com/saterdoe/oberth/internal/context"
)

type measurement struct {
	Scenario string        `json:"scenario"`
	Size     string        `json:"size"`
	P50      time.Duration `json:"p50"`
	P95      time.Duration `json:"p95"`
	Budget   time.Duration `json:"budget"`
}

func TestRegressionBudgets(t *testing.T) {
	var report []measurement
	// Retain partial measurements even when a budget fails and calls FailNow.
	t.Cleanup(func() {
		if destination := strings.TrimSpace(os.Getenv("OBERTH_PERF_REPORT")); destination != "" {
			if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
				t.Error(err)
				return
			}
			encoded, _ := json.MarshalIndent(map[string]any{"schema_version": "1", "measurements": report}, "", "  ")
			if err := os.WriteFile(destination, encoded, 0o600); err != nil {
				t.Error(err)
			}
		}
	})
	for _, fixture := range []struct {
		name   string
		count  int
		budget time.Duration
	}{{"small", 25, 20 * time.Millisecond}, {"medium", 250, 75 * time.Millisecond}, {"large", 1000, 250 * time.Millisecond}} {
		sources := make([]semcontext.ContextSource, fixture.count)
		for i := range sources {
			sources[i] = semcontext.ContextSource{ID: fmt.Sprintf("src-%04d", i), Kind: "code", Content: strings.Repeat("func Handler() { return }\n", 8), Priority: i % 10, Relevance: float64(i%7) / 7}
		}
		pipeline := semcontext.NewPipeline(nil, nil)
		m := sample("context_compile", fixture.name, fixture.budget, func() {
			if _, err := pipeline.CompileWithOptions(context.Background(), "trace persistence flow", "implementation", semcontext.CompileOptions{MaxTokens: 4000, ReserveOutputTokens: 1000, RepoSources: sources}); err != nil {
				t.Fatal(err)
			}
		})
		report = append(report, m)
		assertBudget(t, m)

		lines := fixture.count
		content := strings.Repeat("func Handler() { return }\n", lines)
		file := codeindex.File{Path: "internal/service.go", Language: "go", Content: []byte(content), Hash: "fixture"}
		m = sample("code_chunk", fixture.name, fixture.budget, func() { _ = codeindex.ChunkFile("repo:fixture", file, codeindex.DefaultOptions(), "disabled") })
		report = append(report, m)
		assertBudget(t, m)

		root, dataDir := t.TempDir(), t.TempDir()
		for i := 0; i < fixture.count; i++ {
			path := filepath.Join(root, fmt.Sprintf("file-%04d.go", i))
			if err := os.WriteFile(path, []byte("package fixture\nfunc Handler() {}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		options := codeindex.DefaultOptions()
		options.MaxFiles, options.MaxChunks = fixture.count+1, fixture.count*4
		index, err := codeindex.Open(root, dataDir, nil, nil, options)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = index.Update(context.Background()); err != nil {
			t.Fatal(err)
		}
		m = sample("index_incremental", fixture.name, fixture.budget*2, func() {
			if _, updateErr := index.Update(context.Background()); updateErr != nil {
				t.Fatal(updateErr)
			}
		})
		report = append(report, m)
		assertBudget(t, m)
		m = sample("index_startup", fixture.name, fixture.budget, func() {
			if _, openErr := codeindex.Open(root, dataDir, nil, nil, options); openErr != nil {
				t.Fatal(openErr)
			}
		})
		report = append(report, m)
		assertBudget(t, m)

		payload := map[string]any{"data": sources, "count": len(sources), "cursor": "fixture"}
		handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(payload) })
		request := httptest.NewRequest(http.MethodGet, "/api/v1/benchmark", nil)
		m = sample("api_serialization", fixture.name, fixture.budget, func() {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("unexpected API status %d", recorder.Code)
			}
		})
		report = append(report, m)
		assertBudget(t, m)
	}
}

func sample(scenario, size string, budget time.Duration, operation func()) measurement {
	operation()
	samples := make([]time.Duration, 25)
	for i := range samples {
		start := time.Now()
		operation()
		samples[i] = time.Since(start)
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return measurement{Scenario: scenario, Size: size, P50: samples[len(samples)/2], P95: samples[(len(samples)*95+99)/100-1], Budget: budget}
}

func assertBudget(t *testing.T, value measurement) {
	t.Helper()
	if value.P95 > value.Budget {
		t.Fatalf("%s/%s p95 %s exceeds %s budget", value.Scenario, value.Size, value.P95, value.Budget)
	}
}
