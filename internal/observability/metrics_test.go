package observability

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestBoundedConcurrentTelemetry(t *testing.T) {
	c := &Collector{}
	ctx := WithCollector(context.Background(), c, Correlation{RunID: "run", TaskID: "task", SessionID: "session"})
	var group sync.WaitGroup
	for i := 0; i < 300; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			done := Start(ctx, "provider", fmt.Sprint(i))
			done(context.Canceled)
			done(nil)
		}(i)
	}
	group.Wait()
	snapshot := c.Snapshot()
	if len(snapshot.Metrics) != capacity || len(snapshot.Traces) != capacity || snapshot.Dropped != 44 {
		t.Fatalf("unbounded snapshot: %d %d %d", len(snapshot.Metrics), len(snapshot.Traces), snapshot.Dropped)
	}
	for _, trace := range snapshot.Traces {
		if trace.RunID != "run" || trace.Outcome != "cancelled" {
			t.Fatalf("bad correlation/outcome: %+v", trace)
		}
	}
	snapshot.Traces[0].RunID = "mutated"
	if c.Snapshot().Traces[0].RunID != "run" {
		t.Fatal("snapshot aliases collector")
	}
}

func TestNoCollectorAndLatencyBuckets(t *testing.T) {
	Start(context.Background(), "context", "")(nil)
	c := &Collector{}
	for _, value := range []float64{1, 100, 101, 1000, 1001, 10000, 10001, 60000, 60001} {
		c.record(Trace{Stage: "context", DurationMS: value, Outcome: "ok"})
	}
	m := c.Snapshot().Metrics[0]
	if m.Count != 9 || m.Errors != 0 || m.MaxMS != 60001 || m.Buckets != [5]uint64{2, 2, 2, 2, 1} {
		t.Fatalf("bad metric: %+v", m)
	}
}
