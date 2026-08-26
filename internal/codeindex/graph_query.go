package codeindex

import (
	"encoding/base64"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultGraphNodeLimit = 100
	MaxGraphNodeLimit     = 300
	MaxGraphEdgeLimit     = 1500
)

var (
	ErrGraphNodeNotFound = errors.New("graph node not found")
	ErrGraphCursorStale  = errors.New("graph cursor is stale")
	ErrGraphCursor       = errors.New("invalid graph cursor")
)

type GraphDirection string

const (
	GraphDirectionIncoming GraphDirection = "incoming"
	GraphDirectionOutgoing GraphDirection = "outgoing"
	GraphDirectionBoth     GraphDirection = "both"
)

type GraphCoverage struct {
	Languages map[string]int `json:"languages"`
	NodeCount int            `json:"node_count"`
	EdgeCount int            `json:"edge_count"`
}

type GraphQueryResult struct {
	SchemaVersion string        `json:"schema_version"`
	RepoID        string        `json:"repo_id"`
	Fingerprint   string        `json:"fingerprint"`
	Nodes         []GraphNode   `json:"nodes"`
	Edges         []GraphEdge   `json:"edges"`
	Coverage      GraphCoverage `json:"coverage"`
	Truncated     bool          `json:"truncated"`
	Remaining     int           `json:"remaining"`
	NextCursor    string        `json:"next_cursor,omitempty"`
	LastIndexed   time.Time     `json:"last_indexed"`
	Fresh         bool          `json:"fresh"`
}

func (i *Index) FindGraphNodes(query string, limit int, cursor string) (GraphQueryResult, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	graph := i.state.Graph
	query = strings.ToLower(strings.TrimSpace(query))
	ids := make([]string, 0, len(graph.Nodes))
	for id, node := range graph.Nodes {
		if query == "" || id == query || strings.Contains(strings.ToLower(node.Label), query) || strings.Contains(strings.ToLower(node.Path), query) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	result, err := pageGraph(graph, ids, nil, limit, cursor)
	return i.withGraphStatusLocked(result), err
}

func (i *Index) GraphNeighborhood(nodeID string, direction GraphDirection, limit int, cursor string) (GraphQueryResult, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	graph := i.state.Graph
	if _, ok := graph.Nodes[nodeID]; !ok {
		return GraphQueryResult{}, ErrGraphNodeNotFound
	}
	if direction != GraphDirectionIncoming && direction != GraphDirectionOutgoing && direction != GraphDirectionBoth {
		direction = GraphDirectionBoth
	}
	edgeIDs := make([]string, 0)
	for id, edge := range graph.Edges {
		if direction != GraphDirectionOutgoing && edge.TargetID == nodeID || direction != GraphDirectionIncoming && edge.SourceID == nodeID {
			edgeIDs = append(edgeIDs, id)
		}
	}
	sort.Strings(edgeIDs)
	result, err := pageGraph(graph, nil, edgeIDs, limit, cursor)
	return i.withGraphStatusLocked(result), err
}

func (i *Index) withGraphStatusLocked(result GraphQueryResult) GraphQueryResult {
	result.LastIndexed = i.state.LastIndexed
	result.Fresh = !result.LastIndexed.IsZero() && time.Since(result.LastIndexed) < 10*time.Minute
	return result
}

func pageGraph(graph GraphSnapshot, nodeIDs, edgeIDs []string, limit int, cursor string) (GraphQueryResult, error) {
	if limit <= 0 {
		limit = DefaultGraphNodeLimit
	}
	limit = min(limit, MaxGraphNodeLimit)
	offset, err := decodeGraphCursor(cursor, graph.Fingerprint)
	if err != nil {
		return GraphQueryResult{}, err
	}
	result := GraphQueryResult{SchemaVersion: graph.SchemaVersion, RepoID: graph.RepoID, Fingerprint: graph.Fingerprint, Coverage: graphCoverage(graph)}
	if edgeIDs == nil {
		end := min(offset+limit, len(nodeIDs))
		if offset > len(nodeIDs) {
			return GraphQueryResult{}, ErrGraphCursor
		}
		for _, id := range nodeIDs[offset:end] {
			result.Nodes = append(result.Nodes, graph.Nodes[id])
		}
		result.Remaining = len(nodeIDs) - end
		result.Truncated = result.Remaining > 0
		if result.Truncated {
			result.NextCursor = encodeGraphCursor(graph.Fingerprint, end)
		}
		return result, nil
	}

	edgeLimit := min(limit*5, MaxGraphEdgeLimit)
	end := min(offset+edgeLimit, len(edgeIDs))
	if offset > len(edgeIDs) {
		return GraphQueryResult{}, ErrGraphCursor
	}
	nodeSet := make(map[string]bool)
	for _, id := range edgeIDs[offset:end] {
		edge := graph.Edges[id]
		result.Edges = append(result.Edges, edge)
		nodeSet[edge.SourceID] = true
		nodeSet[edge.TargetID] = true
	}
	orderedNodes := make([]string, 0, len(nodeSet))
	for id := range nodeSet {
		orderedNodes = append(orderedNodes, id)
	}
	sort.Strings(orderedNodes)
	if len(orderedNodes) > MaxGraphNodeLimit {
		orderedNodes = orderedNodes[:MaxGraphNodeLimit]
	}
	for _, id := range orderedNodes {
		result.Nodes = append(result.Nodes, graph.Nodes[id])
	}
	result.Remaining = len(edgeIDs) - end
	result.Truncated = result.Remaining > 0
	if result.Truncated {
		result.NextCursor = encodeGraphCursor(graph.Fingerprint, end)
	}
	return result, nil
}

func graphCoverage(graph GraphSnapshot) GraphCoverage {
	coverage := GraphCoverage{Languages: make(map[string]int), NodeCount: len(graph.Nodes), EdgeCount: len(graph.Edges)}
	for _, node := range graph.Nodes {
		if node.Kind == GraphNodeFile && node.Language != "" {
			coverage.Languages[node.Language]++
		}
	}
	return coverage
}

func encodeGraphCursor(fingerprint string, offset int) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fingerprint + ":" + strconv.Itoa(offset)))
}

func decodeGraphCursor(cursor, fingerprint string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, ErrGraphCursor
	}
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 2 {
		return 0, ErrGraphCursor
	}
	if parts[0] != fingerprint {
		return 0, ErrGraphCursorStale
	}
	offset, err := strconv.Atoi(parts[1])
	if err != nil || offset < 0 {
		return 0, ErrGraphCursor
	}
	return offset, nil
}
