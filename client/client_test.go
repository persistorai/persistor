package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestServer creates a test server that routes to the given handler map.
// Keys are "METHOD /path", values are handler funcs.
func newTestServer(t *testing.T, routes map[string]http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, handler := range routes {
		mux.HandleFunc(pattern, handler)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, WithAPIKey("test-key"))
	return srv, c
}

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func TestHealth(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/health": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, HealthResponse{Status: "ok", Version: "0.7.0"})
		},
	})
	resp, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("got status %q, want ok", resp.Status)
	}
	if resp.Version != "0.7.0" {
		t.Errorf("got version %q, want 0.7.0", resp.Version)
	}
}

func TestStats(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/stats": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, StatsResponse{Nodes: 500, Edges: 500, EntityTypes: 10})
		},
	})
	resp, err := c.Stats(context.Background())
	if err != nil {
		t.Fatalf("Stats() error: %v", err)
	}
	if resp.Nodes != 500 {
		t.Errorf("got nodes %d, want 500", resp.Nodes)
	}
}

func TestNodesCRUD(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/nodes": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]any{"nodes": []Node{{ID: "n1", Label: "Test"}}, "has_more": false})
		},
		"POST /api/v1/nodes": func(w http.ResponseWriter, r *http.Request) {
			var req CreateNodeRequest
			json.NewDecoder(r.Body).Decode(&req) //nolint:errcheck
			jsonResponse(w, 201, Node{ID: req.ID, Type: req.Type, Label: req.Label})
		},
		"GET /api/v1/nodes/n1": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, Node{ID: "n1", Label: "Test"})
		},
		"PUT /api/v1/nodes/n1": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, Node{ID: "n1", Label: "Updated"})
		},
		"DELETE /api/v1/nodes/n1": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]bool{"deleted": true})
		},
	})

	ctx := context.Background()

	// List
	nodes, hasMore, err := c.Nodes.List(ctx, nil)
	if err != nil {
		t.Fatalf("List error: %v", err)
	}
	if len(nodes) != 1 || hasMore {
		t.Errorf("List: got %d nodes, hasMore=%v", len(nodes), hasMore)
	}

	// Create
	node, err := c.Nodes.Create(ctx, &CreateNodeRequest{ID: "n2", Type: "person", Label: "Big Jerry"})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if node.Label != "Big Jerry" {
		t.Errorf("Create: got label %q", node.Label)
	}

	// Get
	node, err = c.Nodes.Get(ctx, "n1")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if node.ID != "n1" {
		t.Errorf("Get: got id %q", node.ID)
	}

	// Update
	label := "Updated"
	node, err = c.Nodes.Update(ctx, "n1", &UpdateNodeRequest{Label: &label})
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}
	if node.Label != "Updated" {
		t.Errorf("Update: got label %q", node.Label)
	}

	// Delete
	if err := c.Nodes.Delete(ctx, "n1"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
}

func TestEdgesCRUD(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/edges": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]any{"edges": []Edge{{Source: "a", Target: "b", Relation: "knows"}}, "has_more": false})
		},
		"POST /api/v1/edges": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 201, Edge{Source: "a", Target: "b", Relation: "knows"})
		},
		"PUT /api/v1/edges/a/b/knows": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, Edge{Source: "a", Target: "b", Relation: "knows", Weight: 0.9})
		},
		"DELETE /api/v1/edges/a/b/knows": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]bool{"deleted": true})
		},
	})

	ctx := context.Background()

	edges, _, err := c.Edges.List(ctx, nil)
	if err != nil || len(edges) != 1 {
		t.Fatalf("List error: %v, len=%d", err, len(edges))
	}

	edge, err := c.Edges.Create(ctx, &CreateEdgeRequest{Source: "a", Target: "b", Relation: "knows"})
	if err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if edge.Source != "a" {
		t.Errorf("Create: got source %q", edge.Source)
	}

	w := 0.9
	edge, err = c.Edges.Update(ctx, "a", "b", "knows", &UpdateEdgeRequest{Weight: &w})
	if err != nil {
		t.Fatalf("Update error: %v", err)
	}

	if err := c.Edges.Delete(ctx, "a", "b", "knows"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
}

func TestSearch(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/search": func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("q") == "" {
				jsonResponse(w, 400, map[string]string{"code": "invalid_request", "message": "q required"})
				return
			}
			jsonResponse(w, 200, map[string]any{"nodes": []Node{{ID: "n1"}}, "total": 1})
		},
		"GET /api/v1/search/semantic": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]any{"nodes": []ScoredNode{{Node: Node{ID: "n1"}, Score: 0.95}}, "total": 1})
		},
		"GET /api/v1/search/hybrid": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]any{"nodes": []Node{{ID: "n1"}}, "total": 1})
		},
	})

	ctx := context.Background()

	nodes, err := c.Search.FullText(ctx, "deer", nil)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("FullText: err=%v, len=%d", err, len(nodes))
	}

	scored, err := c.Search.Semantic(ctx, "deer identification", 10)
	if err != nil || len(scored) != 1 {
		t.Fatalf("Semantic: err=%v, len=%d", err, len(scored))
	}
	if scored[0].Score != 0.95 {
		t.Errorf("Semantic score: got %f, want 0.95", scored[0].Score)
	}

	nodes, err = c.Search.Hybrid(ctx, "deer", nil)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("Hybrid: err=%v, len=%d", err, len(nodes))
	}
}

func TestGraph(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/graph/neighbors/n1": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, NeighborResult{Nodes: []Node{{ID: "n2"}}, Edges: []Edge{{Source: "n1", Target: "n2"}}})
		},
		"GET /api/v1/graph/traverse/n1": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, TraverseResult{Nodes: []Node{{ID: "n1"}, {ID: "n2"}}, Edges: []Edge{{Source: "n1", Target: "n2"}}})
		},
		"GET /api/v1/graph/context/n1": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, ContextResult{Node: Node{ID: "n1"}, Neighbors: []Node{{ID: "n2"}}})
		},
		"GET /api/v1/graph/path/n1/n3": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]any{"path": []Node{{ID: "n1"}, {ID: "n2"}, {ID: "n3"}}})
		},
	})

	ctx := context.Background()

	nb, err := c.Graph.Neighbors(ctx, "n1", 0)
	if err != nil || len(nb.Nodes) != 1 {
		t.Fatalf("Neighbors: err=%v", err)
	}

	tr, err := c.Graph.Traverse(ctx, "n1", 2)
	if err != nil || len(tr.Nodes) != 2 {
		t.Fatalf("Traverse: err=%v", err)
	}

	cr, err := c.Graph.Context(ctx, "n1")
	if err != nil || cr.Node.ID != "n1" {
		t.Fatalf("Context: err=%v", err)
	}

	path, err := c.Graph.ShortestPath(ctx, "n1", "n3")
	if err != nil || len(path) != 3 {
		t.Fatalf("ShortestPath: err=%v, len=%d", err, len(path))
	}
}

func TestSalience(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/v1/salience/boost/n1": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, Node{ID: "n1", Salience: 1.5})
		},
		"POST /api/v1/salience/supersede": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]bool{"superseded": true})
		},
		"POST /api/v1/salience/recalc": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]int{"updated": 42})
		},
	})

	ctx := context.Background()

	node, err := c.Salience.Boost(ctx, "n1")
	if err != nil || node.Salience != 1.5 {
		t.Fatalf("Boost: err=%v, salience=%f", err, node.Salience)
	}

	if err := c.Salience.Supersede(ctx, "old", "new"); err != nil {
		t.Fatalf("Supersede: %v", err)
	}

	count, err := c.Salience.Recalculate(ctx)
	if err != nil || count != 42 {
		t.Fatalf("Recalculate: err=%v, count=%d", err, count)
	}
}

func TestBulk(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/v1/bulk/nodes": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]any{
				"upserted": 2,
				"nodes": []map[string]any{
					{"id": "n1", "type": "t", "label": "l1"},
					{"id": "n2", "type": "t", "label": "l2"},
				},
			})
		},
		"POST /api/v1/bulk/edges": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]any{
				"upserted": 1,
				"edges": []map[string]any{
					{"source": "a", "target": "b", "relation": "r"},
				},
			})
		},
	})

	ctx := context.Background()

	nodes, err := c.Bulk.UpsertNodes(ctx, []CreateNodeRequest{{Type: "t", Label: "l"}})
	if err != nil || len(nodes) != 2 {
		t.Fatalf("UpsertNodes: err=%v, len=%d", err, len(nodes))
	}

	edges, err := c.Bulk.UpsertEdges(ctx, []CreateEdgeRequest{{Source: "a", Target: "b", Relation: "r"}})
	if err != nil || len(edges) != 1 {
		t.Fatalf("UpsertEdges: err=%v, len=%d", err, len(edges))
	}
}

func TestAudit(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/audit": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]any{"data": []AuditEntry{{ID: "a1", Action: "node.create"}}, "has_more": false})
		},
		"DELETE /api/v1/audit": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]any{"deleted": 10, "retention_days": 90})
		},
	})

	ctx := context.Background()

	entries, hasMore, err := c.Audit.Query(ctx, nil)
	if err != nil || len(entries) != 1 || hasMore {
		t.Fatalf("Query: err=%v, len=%d", err, len(entries))
	}

	deleted, err := c.Audit.Purge(ctx, 90)
	if err != nil || deleted != 10 {
		t.Fatalf("Purge: err=%v, deleted=%d", err, deleted)
	}
}

func TestAdmin(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"POST /api/v1/admin/backfill-embeddings": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 200, map[string]int{"queued": 25})
		},
	})

	queued, err := c.Admin.BackfillEmbeddings(context.Background())
	if err != nil || queued != 25 {
		t.Fatalf("BackfillEmbeddings: err=%v, queued=%d", err, queued)
	}
}

func TestAPIError(t *testing.T) {
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/nodes/missing": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 404, map[string]string{"code": "not_found", "message": "node not found"})
		},
		"POST /api/v1/nodes": func(w http.ResponseWriter, _ *http.Request) {
			jsonResponse(w, 409, map[string]string{"code": "conflict", "message": "duplicate"})
		},
	})

	ctx := context.Background()

	_, err := c.Nodes.Get(ctx, "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected not found, got: %v", err)
	}

	_, err = c.Nodes.Create(ctx, &CreateNodeRequest{ID: "dup", Type: "t", Label: "l"})
	if !IsConflict(err) {
		t.Errorf("expected conflict, got: %v", err)
	}
}

func TestAuthHeader(t *testing.T) {
	var gotAuth string
	_, c := newTestServer(t, map[string]http.HandlerFunc{
		"GET /api/v1/health": func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			jsonResponse(w, 200, HealthResponse{Status: "ok"})
		},
	})

	c.Health(context.Background()) //nolint:errcheck
	if gotAuth != "Bearer test-key" {
		t.Errorf("auth header: got %q, want %q", gotAuth, "Bearer test-key")
	}
}
