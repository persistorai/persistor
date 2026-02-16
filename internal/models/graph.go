package models

// NeighborResult holds nodes directly connected to a given node plus their edges.
type NeighborResult struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// TraverseResult holds a subgraph discovered by BFS traversal.
type TraverseResult struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// ContextResult holds a node with its immediate neighborhood.
type ContextResult struct {
	Node      Node   `json:"node"`
	Neighbors []Node `json:"neighbors"`
	Edges     []Edge `json:"edges"`
}
