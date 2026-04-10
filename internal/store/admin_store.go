package store

// AdminStore composes admin-related store capabilities behind a single concrete type.
type AdminStore struct {
	*EmbeddingStore
}

// NewAdminStore creates an AdminStore.
func NewAdminStore(base Base) *AdminStore {
	return &AdminStore{EmbeddingStore: NewEmbeddingStore(base)}
}
