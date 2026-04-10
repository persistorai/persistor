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
		if req.SearchText {
			searchText := models.BuildNodeSearchText(fullNode)
			if searchText != "" && searchText != node.CurrentSearchText {
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

// RunMaintenance performs an explicit operator-triggered maintenance pass for refresh and fact scans.
func (s *AdminService) RunMaintenance(ctx context.Context, tenantID string, req models.MaintenanceRunRequest) (*models.MaintenanceRunResult, error) {
	batchSize := req.BatchSize
	if batchSize <= 0 {
		batchSize = 100
	}

	nodes, err := s.store.ListNodesForMaintenance(ctx, tenantID, batchSize)
	if err != nil {
		return nil, err
	}

	result := &models.MaintenanceRunResult{Scanned: len(nodes)}
	for _, node := range nodes {
		fullNode := &models.Node{ID: node.ID, Type: node.Type, Label: node.Label, Properties: node.Properties}
		if req.RefreshSearchText {
			searchText := models.BuildNodeSearchText(fullNode)
			if searchText != "" && searchText != node.CurrentSearchText {
				if err := s.store.UpdateNodeSearchText(ctx, tenantID, node.ID, searchText); err != nil {
					return nil, err
				}
				result.UpdatedSearchText++
			}
		}
		if req.RefreshEmbeddings && node.NeedsEmbedding && s.embedWorker != nil {
			s.embedWorker.Enqueue(EmbedJob{
				TenantID: tenantID,
				NodeID:   node.ID,
				Text:     models.BuildNodeEmbeddingText(fullNode),
			})
			result.QueuedEmbeddings++
		}
		if req.ScanStaleFacts {
			if node.HasFactEvidence {
				result.StaleFactNodes++
			}
			if node.HasSupersededFacts || node.NodeSuperseded {
				result.SupersededNodes++
			}
		}
	}

	if req.IncludeDuplicateCandidates {
		pairs, err := s.store.ListDuplicateCandidatePairs(ctx, tenantID, "", batchSize)
		if err != nil {
			return nil, err
		}
		result.DuplicateCandidatePairs = len(pairs)
	}

	remainingSearchText, remainingEmbeddings, remainingTotal, err := s.store.CountNodesForReprocess(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	result.RemainingSearchText = remainingSearchText
	result.RemainingEmbeddings = remainingEmbeddings
	result.RemainingMaintenanceNodes = remainingTotal

	return result, nil
}
