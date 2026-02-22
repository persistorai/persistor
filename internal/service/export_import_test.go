package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/persistorai/persistor/internal/models"
)

// --- Export tests ---

func TestExport_EmptyTenant(t *testing.T) {
	svc := newTestService(&mockExportImportStore{})
	ctx := context.Background()

	got, err := svc.Export(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if got.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q, want %q", got.TenantID, "tenant-1")
	}

	if got.Stats.NodeCount != 0 {
		t.Errorf("NodeCount = %d, want 0", got.Stats.NodeCount)
	}

	if got.Stats.EdgeCount != 0 {
		t.Errorf("EdgeCount = %d, want 0", got.Stats.EdgeCount)
	}

	if got.PersistorVersion != "test-0.0.1" {
		t.Errorf("PersistorVersion = %q, want %q", got.PersistorVersion, "test-0.0.1")
	}

	if got.ExportedAt.IsZero() {
		t.Error("ExportedAt should not be zero")
	}
}

func TestExport_WithData(t *testing.T) {
	store := &mockExportImportStore{
		nodes: []models.ExportNode{
			{ID: "n1", Type: "person", Label: "Alice"},
			{ID: "n2", Type: "concept", Label: "Go"},
		},
		edges: []models.ExportEdge{
			{Source: "n1", Target: "n2", Relation: "uses"},
		},
	}
	svc := newTestService(store)
	ctx := context.Background()

	got, err := svc.Export(ctx, "tenant-2")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	if got.Stats.NodeCount != 2 {
		t.Errorf("NodeCount = %d, want 2", got.Stats.NodeCount)
	}

	if got.Stats.EdgeCount != 1 {
		t.Errorf("EdgeCount = %d, want 1", got.Stats.EdgeCount)
	}

	if len(got.Nodes) != 2 || len(got.Edges) != 1 {
		t.Errorf("unexpected node/edge slice lengths: %d/%d", len(got.Nodes), len(got.Edges))
	}
}

func TestExport_StoreError(t *testing.T) {
	store := &mockExportImportStore{errOnExport: errors.New("db down")}
	svc := newTestService(store)

	_, err := svc.Export(context.Background(), "tenant-x")
	if err == nil {
		t.Fatal("expected error from Export, got nil")
	}
}

// --- ValidateImport tests ---

func TestValidateImport_Valid(t *testing.T) {
	now := time.Now().UTC()
	store := &mockExportImportStore{}
	svc := newTestService(store)
	ctx := context.Background()

	data := &models.ExportFormat{
		SchemaVersion: 0, // always <= current (even if 0)
		Nodes: []models.ExportNode{
			{ID: "a", Type: "t", Label: "A", CreatedAt: now, UpdatedAt: now},
			{ID: "b", Type: "t", Label: "B", CreatedAt: now, UpdatedAt: now},
		},
		Edges: []models.ExportEdge{
			{Source: "a", Target: "b", Relation: "rel"},
		},
	}

	errs, err := svc.ValidateImport(ctx, "t1", data)
	if err != nil {
		t.Fatalf("ValidateImport: %v", err)
	}

	if len(errs) != 0 {
		t.Errorf("expected no validation errors, got: %v", errs)
	}
}

func TestValidateImport_EmptyNodeID(t *testing.T) {
	store := &mockExportImportStore{}
	svc := newTestService(store)
	ctx := context.Background()

	data := &models.ExportFormat{
		Nodes: []models.ExportNode{
			{ID: "", Type: "t", Label: "Bad"},
		},
	}

	errs, err := svc.ValidateImport(ctx, "t1", data)
	if err != nil {
		t.Fatalf("ValidateImport: %v", err)
	}

	if len(errs) == 0 {
		t.Error("expected validation error for empty node ID, got none")
	}
}

func TestValidateImport_EdgeMissingNode(t *testing.T) {
	store := &mockExportImportStore{} // no existing DB nodes
	svc := newTestService(store)
	ctx := context.Background()

	data := &models.ExportFormat{
		Nodes: []models.ExportNode{
			{ID: "exists", Type: "t", Label: "X"},
		},
		Edges: []models.ExportEdge{
			{Source: "exists", Target: "ghost", Relation: "r"},
		},
	}

	errs, err := svc.ValidateImport(ctx, "t1", data)
	if err != nil {
		t.Fatalf("ValidateImport: %v", err)
	}

	if len(errs) == 0 {
		t.Error("expected validation error for missing edge target, got none")
	}
}

func TestValidateImport_EdgeTargetInDB(t *testing.T) {
	// "ghost" exists in the DB — edge should be valid.
	store := &mockExportImportStore{
		nodes: []models.ExportNode{{ID: "ghost"}},
	}
	svc := newTestService(store)
	ctx := context.Background()

	data := &models.ExportFormat{
		Nodes: []models.ExportNode{
			{ID: "exists", Type: "t", Label: "X"},
		},
		Edges: []models.ExportEdge{
			{Source: "exists", Target: "ghost", Relation: "r"},
		},
	}

	errs, err := svc.ValidateImport(ctx, "t1", data)
	if err != nil {
		t.Fatalf("ValidateImport: %v", err)
	}

	if len(errs) != 0 {
		t.Errorf("expected no errors when edge target exists in DB, got: %v", errs)
	}
}
