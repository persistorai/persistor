package ingest

import (
	"context"
	"strings"

	"github.com/persistorai/persistor/client"
)

// GraphClient abstracts the Persistor API operations needed by the writer.
type GraphClient interface {
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
	graph  GraphClient
	source string
}

// NewWriter creates a Writer that uses the given GraphClient and source tag.
func NewWriter(graph GraphClient, source string) *Writer {
	return &Writer{graph: graph, source: source}
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
