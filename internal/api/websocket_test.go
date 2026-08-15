package api

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHubBroadcastDeliversToAllClients(t *testing.T) {
	h := NewHub()
	ctx := context.Background()

	c1 := h.register(ctx)
	c2 := h.register(ctx)

	evt := Event{Type: EventSessionComplete, Payload: "test", Time: time.Now()}
	h.Broadcast(evt)

	select {
	case received := <-c1.ch:
		if received.Type != EventSessionComplete {
			t.Errorf("c1 expected EventSessionComplete, got %v", received.Type)
		}
	default:
		t.Fatal("c1 did not receive event")
	}

	select {
	case received := <-c2.ch:
		if received.Type != EventSessionComplete {
			t.Errorf("c2 expected EventSessionComplete, got %v", received.Type)
		}
	default:
		t.Fatal("c2 did not receive event")
	}
}

func TestTaskChunkPublishesDomainEvent(t *testing.T) {
	hub := NewHub()
	server := &Server{eventHub: hub}
	client := hub.register(context.Background())
	taskID, sessionID := uuid.New(), uuid.New()
	server.broadcastTaskChunk(taskID, sessionID, "partial")
	event := <-client.ch
	if event.Type != EventTaskChunk || event.AggregateID != sessionID.String() {
		t.Fatalf("unexpected event: %+v", event)
	}
	payload := event.Payload.(map[string]any)
	if payload["task_id"] != taskID.String() || payload["content"] != "partial" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestHubUnregisterStopsDelivery(t *testing.T) {
	h := NewHub()
	ctx := context.Background()

	c := h.register(ctx)
	h.unregister(c)

	// After unregister, the channel is closed. Reading should return zero value with ok=false.
	_, ok := <-c.ch
	if ok {
		t.Fatal("unregistered client channel should be closed")
	}
}

func TestHubBroadcastDoesNotBlockOnSlowClient(t *testing.T) {
	h := NewHub()
	ctx := context.Background()

	// Register a client but never read from its channel
	c := h.register(ctx)

	// Fill the channel buffer (64) then one more should be dropped
	for i := 0; i < 70; i++ {
		h.Broadcast(Event{Type: EventDaemonStatus, Time: time.Now()})
	}

	// Channel should have at most 64 items (buffer size)
	if len(c.ch) > 64 {
		t.Errorf("expected at most 64 buffered events, got %d", len(c.ch))
	}
	foundResync := false
	for len(c.ch) > 0 {
		if event := <-c.ch; event.Type == EventResyncRequired {
			foundResync = true
		}
	}
	if !foundResync {
		t.Fatal("slow consumer overflow must emit an explicit resync event")
	}
}

func TestHubMultipleEventTypes(t *testing.T) {
	h := NewHub()
	ctx := context.Background()

	c := h.register(ctx)

	types := []EventType{EventCostAlert, EventDaemonStatus, EventVaultChange, EventSessionComplete, EventADRPending}
	for _, et := range types {
		h.Broadcast(Event{Type: et, Time: time.Now()})
	}

	for _, expected := range types {
		select {
		case received := <-c.ch:
			if received.Type != expected {
				t.Errorf("expected %v, got %v", expected, received.Type)
			}
		default:
			t.Fatalf("missing event %v", expected)
		}
	}
}

func TestHubAddsVersionedOrderedEventEnvelope(t *testing.T) {
	hub := NewHub()
	client := hub.register(context.Background())
	hub.Broadcast(Event{Type: EventTaskStarted, AggregateID: "task-1", Payload: map[string]any{"status": "running"}})
	hub.Broadcast(Event{Type: EventTaskChunk, AggregateID: "task-1", Payload: map[string]any{"content": "hello"}})
	first := <-client.ch
	second := <-client.ch
	if first.Version != CurrentEventVersion || first.ID == "" {
		t.Fatalf("missing event envelope: %+v", first)
	}
	if first.Sequence == 0 || second.Sequence != first.Sequence+1 {
		t.Fatalf("events are not ordered: %d then %d", first.Sequence, second.Sequence)
	}
	if first.AggregateID != "task-1" || second.AggregateID != "task-1" {
		t.Fatal("aggregate ID was not preserved")
	}
}

func TestDurablePayloadRedactsStreamContent(t *testing.T) {
	payload := durableEventPayload(Event{Type: EventTaskChunk, Payload: map[string]any{"content": "private source"}})
	redacted, ok := payload.(map[string]any)
	if !ok || redacted["redacted"] != true {
		t.Fatalf("task chunks must be redacted before persistence: %#v", payload)
	}
	if _, leaked := redacted["content"]; leaked {
		t.Fatal("durable task chunk payload leaked streamed content")
	}
}

func TestProviderModelsEgressPolicyOnlyAllowsExplicitLocalTypes(t *testing.T) {
	for _, providerType := range []string{"openai", "anthropic", "google"} {
		if providerModelsEgressPolicy(providerType).AllowLoopback {
			t.Fatalf("%s must not allow loopback model discovery", providerType)
		}
	}
	for _, providerType := range []string{"ollama", "custom"} {
		if !providerModelsEgressPolicy(providerType).AllowLoopback {
			t.Fatalf("%s must allow explicitly configured local model discovery", providerType)
		}
	}
}
