package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/persistorai/persistor/internal/models"
)

// mockUnknownRelationStore implements UnknownRelationStore for testing.
type mockUnknownRelationStore struct {
	logFn     func(ctx context.Context, tenantID, relationType, sourceName, targetName, sourceText string) error
	listFn    func(ctx context.Context, tenantID string, opts models.UnknownRelationListOpts) ([]models.UnknownRelation, error)
	resolveFn func(ctx context.Context, tenantID, id, canonicalType string) error
}

func (m *mockUnknownRelationStore) LogUnknownRelation(
	ctx context.Context,
	tenantID, relationType, sourceName, targetName, sourceText string,
) error {
	return m.logFn(ctx, tenantID, relationType, sourceName, targetName, sourceText)
}

func (m *mockUnknownRelationStore) ListUnknownRelations(
	ctx context.Context,
	tenantID string,
	opts models.UnknownRelationListOpts,
) ([]models.UnknownRelation, error) {
	return m.listFn(ctx, tenantID, opts)
}

func (m *mockUnknownRelationStore) ResolveUnknownRelation(
	ctx context.Context,
	tenantID, id, canonicalType string,
) error {
	return m.resolveFn(ctx, tenantID, id, canonicalType)
}

func TestUnknownRelationService_Log(t *testing.T) {
	tests := []struct {
		name     string
		storeErr error
		wantErr  bool
	}{
		{name: "success"},
		{name: "store error", storeErr: errors.New("db fail"), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockUnknownRelationStore{
				logFn: func(_ context.Context, _, _, _, _, _ string) error {
					return tc.storeErr
				},
			}
			svc := NewUnknownRelationService(store, testLogger())

			err := svc.LogUnknownRelation(
				context.Background(), "t1", "MENTORS", "Alice", "Bob", "source",
			)

			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestUnknownRelationService_List(t *testing.T) {
	tests := []struct {
		name      string
		storeRet  []models.UnknownRelation
		storeErr  error
		wantCount int
		wantErr   bool
	}{
		{
			name: "success",
			storeRet: []models.UnknownRelation{
				{
					ID:           uuid.New(),
					RelationType: "MENTORS",
					SourceName:   "Alice",
					TargetName:   "Bob",
					Count:        3,
					FirstSeen:    time.Now(),
					LastSeen:     time.Now(),
				},
			},
			wantCount: 1,
		},
		{
			name:     "store error",
			storeErr: errors.New("db fail"),
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockUnknownRelationStore{
				listFn: func(_ context.Context, _ string, _ models.UnknownRelationListOpts) ([]models.UnknownRelation, error) {
					return tc.storeRet, tc.storeErr
				},
			}
			svc := NewUnknownRelationService(store, testLogger())

			results, err := svc.ListUnknownRelations(
				context.Background(), "t1", models.UnknownRelationListOpts{Limit: 50},
			)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(results) != tc.wantCount {
				t.Errorf("got %d results, want %d", len(results), tc.wantCount)
			}
		})
	}
}

func TestUnknownRelationService_Resolve(t *testing.T) {
	tests := []struct {
		name     string
		storeErr error
		wantErr  bool
	}{
		{name: "success"},
		{name: "not found", storeErr: models.ErrUnknownRelationNotFound, wantErr: true},
		{name: "store error", storeErr: errors.New("db fail"), wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			store := &mockUnknownRelationStore{
				resolveFn: func(_ context.Context, _, _, _ string) error {
					return tc.storeErr
				},
			}
			svc := NewUnknownRelationService(store, testLogger())

			err := svc.ResolveUnknownRelation(
				context.Background(), "t1", uuid.New().String(), "TEACHES",
			)

			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
