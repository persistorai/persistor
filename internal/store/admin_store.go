package store

// AdminStore composes admin-related store capabilities behind a single concrete type.
import (
	"context"

	"github.com/persistorai/persistor/internal/models"
)

type AdminStore struct {
	*EmbeddingStore
	retrievalFeedback *RetrievalFeedbackStore
}

// NewAdminStore creates an AdminStore.
func NewAdminStore(base Base) *AdminStore {
	return &AdminStore{
		EmbeddingStore:    NewEmbeddingStore(base),
		retrievalFeedback: NewRetrievalFeedbackStore(base),
	}
}

func (s *AdminStore) CreateRetrievalFeedback(ctx context.Context, tenantID string, req models.RetrievalFeedbackRequest) (*models.RetrievalFeedbackRecord, error) {
	return s.retrievalFeedback.CreateRetrievalFeedback(ctx, tenantID, req)
}

func (s *AdminStore) ListRetrievalFeedback(ctx context.Context, tenantID string, opts models.RetrievalFeedbackListOpts) ([]models.RetrievalFeedbackRecord, error) {
	return s.retrievalFeedback.ListRetrievalFeedback(ctx, tenantID, opts)
}
