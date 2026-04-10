package service

import (
	"context"

	"github.com/persistorai/persistor/internal/models"
)

// ReprocessNodes rewrites search text and/or requeues embeddings for a batch of nodes.
func (s *AdminService) ReprocessNodes(ctx context.Context, tenantID string, req models.ReprocessNodesRequest) (*models.ReprocessNodesResult, error) {
	batchSize := req.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	nodes, err := s.store.ListNodesForReprocess(ctx, tenantID, batchSize)
	if err != nil {
		return nil, err
	}

	result := &models.ReprocessNodesResult{Scanned: len(nodes)}
	for _, node := range nodes {
		fullNode := &models.Node{ID: node.ID, Type: node.Type, Label: node.Label, Properties: node.Properties}
		if req.SearchText && node.NeedsSearchText {
			searchText := models.BuildNodeSearchText(fullNode)
			if searchText != "" {
				if err := s.store.UpdateNodeSearchText(ctx, tenantID, node.ID, searchText); err != nil {
					return nil, err
				}
				result.UpdatedSearch++
			}
		}
		if req.Embeddings && node.NeedsEmbedding && s.embedWorker != nil {
			s.embedWorker.Enqueue(EmbedJob{
				TenantID: tenantID,
				NodeID:   node.ID,
				Text:     models.BuildNodeEmbeddingText(fullNode),
			})
			result.QueuedEmbed++
		}
	}

	remainingSearchText, remainingEmbeddings, remainingTotal, err := s.store.CountNodesForReprocess(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result.RemainingSearchText = remainingSearchText
	result.RemainingEmbeddings = remainingEmbeddings
	result.RemainingTotal = remainingTotal

	return result, nil
}
