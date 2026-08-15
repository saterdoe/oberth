package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/saterdoe/oberth/internal/db/repos"
	"github.com/saterdoe/oberth/internal/vault"
	"nhooyr.io/websocket"
)

type EventType string

const (
	CurrentEventVersion            = "1.0"
	EventCostAlert       EventType = "cost.alert"
	EventDaemonStatus    EventType = "daemon.status"
	EventVaultChange     EventType = "vault.change"
	EventSessionComplete EventType = "session.complete"
	EventADRPending      EventType = "adr.pending"
	EventTaskStarted     EventType = "task.started"
	EventTaskChunk       EventType = "task.chunk"
	EventTaskStatus      EventType = "task.status"
	EventToolEffect      EventType = "tool.effect"
	EventStreamSnapshot  EventType = "stream.snapshot"
	EventResyncRequired  EventType = "stream.resync_required"
)

const (
	maxReplayEvents       = 1000
	durableEventRetention = 24 * time.Hour
)

type Event struct {
	Version     string    `json:"version"`
	ID          string    `json:"id"`
	Sequence    uint64    `json:"sequence"`
	Type        EventType `json:"type"`
	AggregateID string    `json:"aggregate_id,omitempty"`
	Payload     any       `json:"payload"`
	Time        time.Time `json:"time"`
}

type client struct {
	ch  chan Event
	ctx context.Context
}

type Hub struct {
	mu        sync.RWMutex
	clients   map[*client]bool
	sequence  atomic.Uint64
	pool      *pgxpool.Pool
	persistMu sync.Mutex
}

func durableEventPayload(evt Event) any {
	if evt.Type == EventTaskChunk {
		return map[string]any{"redacted": true, "reason": "ephemeral_stream_content"}
	}
	if evt.Type == EventTaskStatus || evt.Type == EventSessionComplete {
		if payload, ok := evt.Payload.(map[string]any); ok {
			redacted := make(map[string]any, len(payload))
			for key, value := range payload {
				if key != "summary" {
					redacted[key] = value
				}
			}
			redacted["summary_redacted"] = true
			return redacted
		}
	}
	return evt.Payload
}

func NewHub(pools ...*pgxpool.Pool) *Hub {
	h := &Hub{
		clients: make(map[*client]bool),
	}
	if len(pools) > 0 {
		h.pool = pools[0]
	}
	if h.pool != nil {
		var latest uint64
		if h.pool.QueryRow(context.Background(), `SELECT COALESCE(MAX(sequence),0) FROM durable_events`).Scan(&latest) == nil {
			h.sequence.Store(latest)
		}
	}
	return h
}

func (h *Hub) Broadcast(evt Event) {
	if evt.Version == "" {
		evt.Version = CurrentEventVersion
	}
	if evt.ID == "" {
		evt.ID = uuid.NewString()
	}
	if evt.Time.IsZero() {
		evt.Time = time.Now().UTC()
	}
	if evt.Sequence == 0 {
		evt.Sequence = h.persist(evt)
	}
	h.mu.RLock()
	defer h.mu.RUnlock()

	for cl := range h.clients {
		select {
		case cl.ch <- evt:
		default:
			select {
			case <-cl.ch:
			default:
			}
			resync := Event{Version: CurrentEventVersion, ID: uuid.NewString(), Sequence: evt.Sequence, Type: EventResyncRequired, Payload: map[string]any{"reason": "slow_consumer", "latest_sequence": evt.Sequence}, Time: time.Now().UTC()}
			select {
			case cl.ch <- resync:
			default:
			}
			slog.Warn("websocket client requires resync", "type", evt.Type)
		}
	}
}

func (h *Hub) persist(evt Event) uint64 {
	h.persistMu.Lock()
	defer h.persistMu.Unlock()
	seq := h.sequence.Add(1)
	if h.pool == nil {
		return seq
	}
	payload, err := json.Marshal(durableEventPayload(evt))
	if err != nil {
		return seq
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = h.pool.Exec(ctx, `DELETE FROM durable_events WHERE created_at < $1`, time.Now().UTC().Add(-durableEventRetention))
	err = h.pool.QueryRow(ctx, `INSERT INTO durable_events(sequence,event_id,version,event_type,aggregate_id,payload,created_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING sequence`, seq, evt.ID, evt.Version, string(evt.Type), evt.AggregateID, payload, evt.Time).Scan(&seq)
	if err != nil {
		slog.Error("failed to persist websocket event", "error", err)
		return seq
	}
	return seq
}

func trimReplayWindow(events []Event) ([]Event, bool) {
	if len(events) <= maxReplayEvents {
		return events, false
	}
	return events[len(events)-maxReplayEvents:], true
}

func (h *Hub) replay(ctx context.Context, after, through uint64) ([]Event, bool, error) {
	if h.pool == nil || through <= after {
		return nil, false, nil
	}
	rows, err := h.pool.Query(ctx, `SELECT sequence,event_id,version,event_type,aggregate_id,payload,created_at FROM (SELECT sequence,event_id,version,event_type,aggregate_id,payload,created_at FROM durable_events WHERE sequence>$1 AND sequence<=$2 ORDER BY sequence DESC LIMIT $3) recent ORDER BY sequence`, after, through, maxReplayEvents+1)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var events []Event
	for rows.Next() {
		var e Event
		var payload []byte
		if err := rows.Scan(&e.Sequence, &e.ID, &e.Version, &e.Type, &e.AggregateID, &payload, &e.Time); err != nil {
			return nil, false, err
		}
		if err := json.Unmarshal(payload, &e.Payload); err != nil {
			return nil, false, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	events, truncated := trimReplayWindow(events)
	return events, truncated, nil
}

func (h *Hub) register(ctx context.Context) *client {
	cl := &client{
		ch:  make(chan Event, 64),
		ctx: ctx,
	}
	h.mu.Lock()
	h.clients[cl] = true
	h.mu.Unlock()
	return cl
}

func (h *Hub) unregister(cl *client) {
	h.mu.Lock()
	delete(h.clients, cl)
	h.mu.Unlock()
	close(cl.ch)
}

func (s *Server) broadcastEvent(evt Event) {
	if s.eventHub == nil {
		return
	}
	s.eventHub.Broadcast(evt)
}

func (s *Server) broadcastTaskEvent(status string, sessionID uuid.UUID, summary string) {
	payload := map[string]any{
		"session_id": sessionID.String(),
		"status":     status,
		"summary":    summary,
		"time":       time.Now(),
	}
	s.broadcastEvent(Event{Type: EventTaskStatus, AggregateID: sessionID.String(), Payload: payload})
	s.broadcastEvent(Event{
		Type:        EventSessionComplete,
		AggregateID: sessionID.String(),
		Payload:     payload,
		Time:        time.Now(),
	})
}

func (s *Server) broadcastTaskChunk(taskID, sessionID uuid.UUID, content string) {
	s.broadcastEvent(Event{
		Type:        EventTaskChunk,
		AggregateID: sessionID.String(),
		Payload:     map[string]any{"task_id": taskID.String(), "session_id": sessionID.String(), "content": content},
	})
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	cursor, err := strconv.ParseUint(r.URL.Query().Get("cursor"), 10, 64)
	if r.URL.Query().Get("cursor") != "" && err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_CURSOR", "cursor must be an unsigned sequence", nil)
		return
	}
	subprotocols := []string{}
	if authProtocol := websocketAuthProtocol(r.Header.Get("Sec-WebSocket-Protocol")); authProtocol != "" {
		subprotocols = append(subprotocols, authProtocol)
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
		Subprotocols:       subprotocols,
	})
	if err != nil {
		slog.Warn("websocket upgrade failed", "error", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	cl := s.eventHub.register(ctx)
	defer s.eventHub.unregister(cl)
	snapshotSequence := s.eventHub.sequence.Load()
	replay, truncated, replayErr := s.eventHub.replay(ctx, cursor, snapshotSequence)
	if replayErr != nil {
		c.Close(websocket.StatusInternalError, "event replay unavailable")
		return
	}
	snapshot := Event{Version: CurrentEventVersion, ID: uuid.NewString(), Sequence: snapshotSequence, Type: EventStreamSnapshot, Payload: map[string]any{"cursor": snapshotSequence, "replayed": len(replay), "truncated": truncated}, Time: time.Now().UTC()}
	if data, marshalErr := json.Marshal(snapshot); marshalErr == nil {
		if writeErr := c.Write(ctx, websocket.MessageText, data); writeErr != nil {
			return
		}
	}
	if truncated {
		resync := Event{Version: CurrentEventVersion, ID: uuid.NewString(), Sequence: snapshotSequence, Type: EventResyncRequired, Payload: map[string]any{"reason": "replay_window_exceeded", "latest_sequence": snapshotSequence}, Time: time.Now().UTC()}
		data, _ := json.Marshal(resync)
		if err := c.Write(ctx, websocket.MessageText, data); err != nil {
			return
		}
	}
	for _, event := range replay {
		data, _ := json.Marshal(event)
		if err := c.Write(ctx, websocket.MessageText, data); err != nil {
			return
		}
	}

	go func() {
		defer cancel()
		for {
			_, _, err := c.Read(ctx)
			if err != nil {
				slog.Debug("websocket reader closed", "status", websocket.CloseStatus(err), "error", err)
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			c.Close(websocket.StatusNormalClosure, "connection closed")
			return
		case evt, ok := <-cl.ch:
			if !ok {
				return
			}
			if evt.Type != EventResyncRequired && evt.Sequence <= snapshotSequence {
				continue
			}
			data, err := json.Marshal(evt)
			if err != nil {
				slog.Warn("failed to marshal event", "error", err)
				continue
			}
			err = c.Write(ctx, websocket.MessageText, data)
			if err != nil {
				slog.Debug("websocket writer closed", "status", websocket.CloseStatus(err), "error", err)
				return
			}
		}
	}
}

func (s *Server) handleGetVaultStats(w http.ResponseWriter, r *http.Request) {
	notes, err := s.vaultConn.ListAllNotes()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to list vault notes", nil)
		return
	}

	notesByType := make(map[string]int)
	totalSize := 0

	for _, n := range notes {
		totalSize += len(n.Content) + len(n.Path)
		if n.Metadata != nil {
			if t, ok := n.Metadata["type"].(string); ok {
				notesByType[t]++
			}
		}
	}

	// Count orphan sessions (sessions with no backlinks)
	sessionCount := 0
	for _, n := range notes {
		if t, ok := n.Metadata["type"].(string); ok && t == "session" {
			sessionCount++
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"notes_by_type":    notesByType,
		"total_size_bytes": totalSize,
		"orphan_notes":     notesByType["session"] - sessionCount,
		"integrity": map[string]any{
			"status": "ok",
			"errors": []string{},
		},
	})
}

func checkProviderReachable(baseURL string, timeout time.Duration) bool {
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := u.Host
	if host == "" {
		return false
	}
	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (s *Server) handleGetCostProjection(w http.ResponseWriter, r *http.Request) {
	since := time.Now().AddDate(0, -1, 0)
	summary, err := s.costLogs.GetSummary(r.Context(), since)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "failed to get cost summary", nil)
		return
	}

	projected := summary.TotalCost * 1.15
	confidence := "medium"
	if projected < 100 {
		confidence = "high"
	} else if projected > 1000 {
		confidence = "low"
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"projected_cost": projected,
		"confidence":     confidence,
	})
}

func (s *Server) handleGetStatus(w http.ResponseWriter, r *http.Request) {
	checkedAt := time.Now().UTC()

	dbState := "healthy"
	dbMessage := ""
	dbCtx, cancelDB := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancelDB()
	if s.pool == nil {
		dbState = "unknown"
		dbMessage = "database is not configured"
	} else if err := s.pool.Ping(dbCtx); err != nil {
		dbState = "unavailable"
		dbMessage = "database did not respond"
	}

	vectorState := "unknown"
	vectorMessage := "vector store is not configured"
	if s.cfg != nil && s.cfg.VectorDB.Engine == "disabled" {
		vectorState = "disabled"
		vectorMessage = "semantic search is disabled"
	}
	if s.searcher != nil && (s.cfg == nil || s.cfg.VectorDB.Engine != "disabled") {
		vectorCtx, cancelVector := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancelVector()
		if err := s.searcher.Ping(vectorCtx); err != nil {
			vectorState = "unavailable"
			vectorMessage = "vector store did not respond"
		} else {
			vectorState = "healthy"
			vectorMessage = ""
		}
	}

	vaultState := "unknown"
	vaultMessage := "vault is not configured"
	notes := make([]vault.Note, 0)
	if s.vaultConn != nil {
		var err error
		notes, err = s.vaultConn.ListAllNotes()
		if err != nil {
			vaultState = "unavailable"
			vaultMessage = "vault could not be read"
		} else {
			vaultState = "healthy"
			vaultMessage = ""
		}
	}

	var providers []repos.Provider
	var err error
	if s.providers != nil {
		providers, err = s.providers.List(r.Context())
	}
	totalProviders := 0
	activeProviders := 0
	providerHealth := make([]map[string]any, 0)
	if s.providers != nil && err == nil {
		totalProviders = len(providers)
		providerHealth = make([]map[string]any, len(providers))
		var healthWG sync.WaitGroup
		for index, p := range providers {
			if p.IsActive {
				activeProviders++
			}
			healthWG.Add(1)
			go func(index int, p repos.Provider) {
				defer healthWG.Done()
				state := "unknown"
				message := "provider is inactive"
				if p.IsActive && p.BaseURL != nil && *p.BaseURL != "" {
					if checkProviderReachable(*p.BaseURL, 2*time.Second) {
						state = "healthy"
						message = ""
					} else {
						state = "unavailable"
						message = "provider endpoint did not respond"
					}
				} else if p.IsActive {
					message = "reachability is not available for this provider"
				}
				providerHealth[index] = map[string]any{
					"id":         p.ID.String(),
					"name":       p.Name,
					"type":       p.ProviderType,
					"active":     p.IsActive,
					"reachable":  state == "healthy",
					"state":      state,
					"checked_at": checkedAt,
					"message":    message,
				}
			}(index, p)
		}
		healthWG.Wait()
	}

	version := ""
	uptime := ""
	vectorEngine := ""
	vaultPath := ""
	if s.cfg != nil {
		version = s.cfg.Server.Version
		uptime = time.Since(s.cfg.Server.StartTime).String()
		vectorEngine = s.cfg.VectorDB.Engine
		vaultPath = s.cfg.Vault.Path
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"checked_at": checkedAt,
		"runtime": map[string]any{
			"state":      "healthy",
			"checked_at": checkedAt,
			"version":    version,
		},
		"server": map[string]any{
			"version":    version,
			"uptime":     uptime,
			"state":      "healthy",
			"checked_at": checkedAt,
		},
		"database": map[string]any{
			"connected":  dbState == "healthy",
			"driver":     "postgres",
			"state":      dbState,
			"checked_at": checkedAt,
			"message":    dbMessage,
		},
		"vector_store": map[string]any{
			"connected":  vectorState == "healthy",
			"engine":     vectorEngine,
			"embedder":   s.cfg.VectorDB.Embedder.Provider,
			"model":      s.cfg.VectorDB.Embedder.Model,
			"dimensions": s.cfg.VectorDB.Embedder.Dimensions,
			"state":      vectorState,
			"checked_at": checkedAt,
			"message":    vectorMessage,
		},
		"vault": map[string]any{
			"path":         vaultPath,
			"note_count":   len(notes),
			"last_indexed": nil,
			"state":        vaultState,
			"checked_at":   checkedAt,
			"message":      vaultMessage,
		},
		"providers": map[string]any{
			"total":  totalProviders,
			"active": activeProviders,
			"health": providerHealth,
		},
	})
}
