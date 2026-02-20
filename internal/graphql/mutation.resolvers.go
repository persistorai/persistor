package graphql

// Mutation resolvers — split from schema.resolvers.go for maintainability.

import (
	"context"

	"github.com/persistorai/persistor/internal/models"
)

// CreateNode is the resolver for the createNode field.
func (r *mutationResolver) CreateNode(ctx context.Context, input CreateNodeInput) (*Node, error) {
	tid, err := TenantIDFromContext(ctx)
	if err != nil {
		return nil, gqlErr(ctx, err)
	}
	req := models.CreateNodeRequest{
		ID:         derefStr(input.ID),
		Type:       input.Type,
		Label:      input.Label,
		Properties: input.Properties,
	}
	if err := req.Validate(); err != nil {
		return nil, gqlErr(ctx, err)
	}
	n, err := r.NodeSvc.CreateNode(ctx, tid, req)
	if err != nil {
		return nil, gqlErr(ctx, err)
	}
	return nodeToGQL(n), nil
}

// UpdateNode is the resolver for the updateNode field.
func (r *mutationResolver) UpdateNode(ctx context.Context, id string, input UpdateNodeInput) (*Node, error) {
	tid, err := TenantIDFromContext(ctx)
	if err != nil {
		return nil, gqlErr(ctx, err)
	}
	req := models.UpdateNodeRequest{
		Type:       input.Type,
		Label:      input.Label,
		Properties: input.Properties,
	}
	if err := req.Validate(); err != nil {
		return nil, gqlErr(ctx, err)
	}
	n, err := r.NodeSvc.UpdateNode(ctx, tid, id, req)
	if err != nil {
		return nil, gqlErr(ctx, err)
	}
	return nodeToGQL(n), nil
}

// DeleteNode is the resolver for the deleteNode field.
func (r *mutationResolver) DeleteNode(ctx context.Context, id string) (bool, error) {
	tid, err := TenantIDFromContext(ctx)
	if err != nil {
		return false, gqlErr(ctx, err)
	}
	if err := r.NodeSvc.DeleteNode(ctx, tid, id); err != nil {
		return false, gqlErr(ctx, err)
	}
	return true, nil
}

// CreateEdge is the resolver for the createEdge field.
func (r *mutationResolver) CreateEdge(ctx context.Context, input CreateEdgeInput) (*Edge, error) {
	tid, err := TenantIDFromContext(ctx)
	if err != nil {
		return nil, gqlErr(ctx, err)
	}
	req := models.CreateEdgeRequest{
		Source:     input.Source,
		Target:     input.Target,
		Relation:   input.Relation,
		Properties: input.Properties,
		Weight:     input.Weight,
	}
	if err := req.Validate(); err != nil {
		return nil, gqlErr(ctx, err)
	}
	e, err := r.EdgeSvc.CreateEdge(ctx, tid, req)
	if err != nil {
		return nil, gqlErr(ctx, err)
	}
	return edgeToGQL(e), nil
}

// UpdateEdge is the resolver for the updateEdge field.
func (r *mutationResolver) UpdateEdge(ctx context.Context, source string, target string, relation string, input UpdateEdgeInput) (*Edge, error) {
	tid, err := TenantIDFromContext(ctx)
	if err != nil {
		return nil, gqlErr(ctx, err)
	}
	req := models.UpdateEdgeRequest{
		Properties: input.Properties,
		Weight:     input.Weight,
	}
	if err := req.Validate(); err != nil {
		return nil, gqlErr(ctx, err)
	}
	e, err := r.EdgeSvc.UpdateEdge(ctx, tid, source, target, relation, req)
	if err != nil {
		return nil, gqlErr(ctx, err)
	}
	return edgeToGQL(e), nil
}

// DeleteEdge is the resolver for the deleteEdge field.
func (r *mutationResolver) DeleteEdge(ctx context.Context, source string, target string, relation string) (bool, error) {
	tid, err := TenantIDFromContext(ctx)
	if err != nil {
		return false, gqlErr(ctx, err)
	}
	if err := r.EdgeSvc.DeleteEdge(ctx, tid, source, target, relation); err != nil {
		return false, gqlErr(ctx, err)
	}
	return true, nil
}

// BoostNode is the resolver for the boostNode field.
func (r *mutationResolver) BoostNode(ctx context.Context, id string) (*Node, error) {
	tid, err := TenantIDFromContext(ctx)
	if err != nil {
		return nil, gqlErr(ctx, err)
	}
	n, err := r.SalienceSvc.BoostNode(ctx, tid, id)
	if err != nil {
		return nil, gqlErr(ctx, err)
	}
	return nodeToGQL(n), nil
}

// SupersedeNode is the resolver for the supersedeNode field.
func (r *mutationResolver) SupersedeNode(ctx context.Context, oldID string, newID string) (bool, error) {
	tid, err := TenantIDFromContext(ctx)
	if err != nil {
		return false, gqlErr(ctx, err)
	}
	if err := r.SalienceSvc.SupersedeNode(ctx, tid, oldID, newID); err != nil {
		return false, gqlErr(ctx, err)
	}
	return true, nil
}

// RecalculateSalience is the resolver for the recalculateSalience field.
func (r *mutationResolver) RecalculateSalience(ctx context.Context) (int, error) {
	tid, err := TenantIDFromContext(ctx)
	if err != nil {
		return 0, gqlErr(ctx, err)
	}
	count, err := r.SalienceSvc.RecalculateSalience(ctx, tid)
	if err != nil {
		return 0, gqlErr(ctx, err)
	}
	return count, nil
}
