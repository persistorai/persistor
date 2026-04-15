package ingest

import (
	"context"
	"strings"
	"time"

	"github.com/persistorai/persistor/client"
)

// GraphClient abstracts the Persistor API operations needed by the writer.
type GraphClient interface {
	GetNode(ctx context.Context, id string) (*client.Node, error)
	GetNodeByLabel(ctx context.Context, label string) (*client.Node, error)
	SearchNodes(ctx context.Context, query string, limit int) ([]client.Node, error)
	CreateNode(ctx context.Context, req *client.CreateNodeRequest) (*client.Node, error)
	UpdateNode(ctx context.Context, id string, req *client.UpdateNodeRequest) (*client.Node, error)
	PatchNodeProperties(ctx context.Context, id string, properties map[string]any) (*client.Node, error)
	CreateEdge(ctx context.Context, req *client.CreateEdgeRequest) (*client.Edge, error)
	UpdateEdge(ctx context.Context, source, target, relation string, req *client.UpdateEdgeRequest) (*client.Edge, error)
}

// WriteReport summarizes the results of writing entities and edges.
type WriteReport struct {
	CreatedNodes     int
	UpdatedNodes     int
	SkippedNodes     int
	CreatedEdges     int
	SkippedEdges     int
	UnknownRelations []ExtractedRelationship
}

// Writer writes extracted entities, relationships, and facts to the graph.
type Writer struct {
	graph       GraphClient
	source      string
	episodic    EpisodicClient
	tenantID    string
	diagnostics *diagnosticsCollector
}

// NewWriter creates a Writer that uses the given GraphClient and source tag.
func NewWriter(graph GraphClient, source string) *Writer {
	return &Writer{graph: graph, source: source}
}

func (w *Writer) WithDiagnostics(diag *diagnosticsCollector) *Writer {
	if w == nil {
		return nil
	}
	w.diagnostics = diag
	return w
}

func (w *Writer) recordParseFailure() {
	if w != nil && w.diagnostics != nil {
		w.diagnostics.recordParseFailure()
	}
}

func (w *Writer) recordAPIFailure(err error) {
	if w != nil && w.diagnostics != nil {
		w.diagnostics.recordAPIFailure(err)
	}
}

func (w *Writer) recordEntityResolution(status entityResolutionStatus) {
	if w != nil && w.diagnostics != nil {
		w.diagnostics.recordEntityResolution(status)
	}
}

func (w *Writer) recordUnknownRelations(count int) {
	if w != nil && w.diagnostics != nil {
		w.diagnostics.recordUnknownRelations(count)
	}
}

func (w *Writer) recordChunkThroughput(chunkIndex int, duration time.Duration, entities, rels, facts int) {
	if w != nil && w.diagnostics != nil {
		w.diagnostics.recordChunkThroughput(chunkIndex, duration, entities, rels, facts)
	}
}

// WriteEntities creates or updates nodes for each entity, returning a name-to-ID map.
func (w *Writer) WriteEntities(
	ctx context.Context,
	entities []ExtractedEntity,
) (*WriteReport, map[string]string, error) {
	report := &WriteReport{}
	nodeMap := make(map[string]string, len(entities))

	for _, ent := range entities {
		id, action, err := w.writeEntity(ctx, ent)
		if err != nil {
			report.SkippedNodes++
			continue
		}
		nodeMap[strings.ToLower(ent.Name)] = id
		applyAction(report, action)
	}

	return report, nodeMap, nil
}

// entityAction describes what happened when writing an entity.
type entityAction int

const (
	actionCreated entityAction = iota
	actionUpdated
)

func applyAction(report *WriteReport, action entityAction) {
	switch action {
	case actionCreated:
		report.CreatedNodes++
	case actionUpdated:
		report.UpdatedNodes++
	}
}
