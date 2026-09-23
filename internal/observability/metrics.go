// Package observability records bounded, content-free local runtime telemetry.
package observability

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

const capacity = 256

type Correlation struct {
	RunID     string `json:"run_id"`
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
}
type Trace struct {
	Correlation
	Stage      string    `json:"stage"`
	ProviderID string    `json:"provider_id,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	DurationMS float64   `json:"duration_ms"`
	Outcome    string    `json:"outcome"`
}
type Metric struct {
	Stage      string  `json:"stage"`
	ProviderID string  `json:"provider_id,omitempty"`
	Count      uint64  `json:"count"`
	Errors     uint64  `json:"errors"`
	TotalMS    float64 `json:"total_ms"`
	MaxMS      float64 `json:"max_ms"`
	// Non-cumulative counts: <=100ms, <=1s, <=10s, <=60s, >60s.
	Buckets [5]uint64 `json:"latency_buckets"`
}
type Snapshot struct {
	SchemaVersion string   `json:"schema_version"`
	Metrics       []Metric `json:"metrics"`
	Traces        []Trace  `json:"traces"`
	Dropped       uint64   `json:"dropped_series"`
}
type Collector struct {
	mu      sync.Mutex
	metrics map[string]Metric
	traces  []Trace
	dropped uint64
}
type contextKey struct{}
type binding struct {
	collector   *Collector
	correlation Correlation
}

func WithCollector(ctx context.Context, collector *Collector, correlation Correlation) context.Context {
	return context.WithValue(ctx, contextKey{}, binding{collector, correlation})
}

// Start accepts only internal stage names and opaque provider IDs. It deliberately
// has no field for prompts, paths, model output, URLs or error messages.
func Start(ctx context.Context, stage, providerID string) func(error) {
	bound, _ := ctx.Value(contextKey{}).(binding)
	if bound.collector == nil {
		return func(error) {}
	}
	started := time.Now()
	var once sync.Once
	return func(err error) {
		once.Do(func() {
			outcome := "ok"
			if err != nil {
				outcome = "error"
			}
			if errors.Is(err, context.Canceled) {
				outcome = "cancelled"
			}
			if errors.Is(err, context.DeadlineExceeded) {
				outcome = "timeout"
			}
			bound.collector.record(Trace{Correlation: bound.correlation, Stage: stage, ProviderID: providerID, StartedAt: started.UTC(), DurationMS: float64(time.Since(started)) / float64(time.Millisecond), Outcome: outcome})
		})
	}
}

func (c *Collector) record(trace Trace) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.metrics == nil {
		c.metrics = make(map[string]Metric)
	}
	key := trace.Stage + "\x00" + trace.ProviderID
	metric, exists := c.metrics[key]
	if !exists && len(c.metrics) >= capacity {
		c.dropped++
	} else {
		metric.Stage, metric.ProviderID = trace.Stage, trace.ProviderID
		metric.Count++
		metric.TotalMS += trace.DurationMS
		if trace.Outcome != "ok" {
			metric.Errors++
		}
		if trace.DurationMS > metric.MaxMS {
			metric.MaxMS = trace.DurationMS
		}
		bucket := sort.SearchFloat64s([]float64{100, 1000, 10000, 60000}, trace.DurationMS)
		metric.Buckets[bucket]++
		c.metrics[key] = metric
	}
	if len(c.traces) == capacity {
		copy(c.traces, c.traces[1:])
		c.traces = c.traces[:capacity-1]
	}
	c.traces = append(c.traces, trace)
}

func (c *Collector) Snapshot() Snapshot {
	result := Snapshot{SchemaVersion: "1", Metrics: []Metric{}, Traces: []Trace{}}
	if c == nil {
		return result
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, metric := range c.metrics {
		result.Metrics = append(result.Metrics, metric)
	}
	sort.Slice(result.Metrics, func(i, j int) bool {
		return result.Metrics[i].Stage+result.Metrics[i].ProviderID < result.Metrics[j].Stage+result.Metrics[j].ProviderID
	})
	result.Traces = append(result.Traces, c.traces...)
	result.Dropped = c.dropped
	return result
}
