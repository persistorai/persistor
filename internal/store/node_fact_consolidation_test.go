package store_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/persistorai/persistor/internal/models"
	"github.com/persistorai/persistor/internal/store"
)

func TestPatchNodeProperties_FactConflictPreservesEvidence(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	hs := store.NewHistoryStore(base)
	ctx := context.Background()

	req := models.CreateNodeRequest{Type: "person", Label: "Alice", Properties: map[string]any{"city": "Austin"}}
	_ = req.Validate()
	node, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	confidence := 0.82
	timestamp := "2026-04-10T17:00:00Z"
	patched, err := ns.PatchNodeProperties(ctx, tenantID, node.ID, models.PatchPropertiesRequest{
		Properties: map[string]any{
			models.FactUpdatesProperty: map[string]models.FactUpdate{
				"city": {Value: "Chicago", Source: "journal", Confidence: &confidence, Timestamp: timestamp},
			},
		},
	})
	if err != nil {
		t.Fatalf("PatchNodeProperties: %v", err)
	}

	if got := patched.Properties["city"]; got != "Chicago" {
		t.Fatalf("city = %v, want Chicago", got)
	}

	evidence := decodeEvidence(t, patched.Properties[models.FactEvidenceProperty])
	entries := evidence["city"]
	if len(entries) != 1 {
		t.Fatalf("city evidence len = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Source != "journal" || entry.Timestamp != timestamp {
		t.Fatalf("entry metadata = %#v, want source/timestamp preserved", entry)
	}
	if entry.Confidence == nil || *entry.Confidence != confidence {
		t.Fatalf("entry confidence = %#v, want %v", entry.Confidence, confidence)
	}
	if !entry.ConflictsWithPrior || !entry.SupersedesPrior || !entry.HistoricalEvidenceRetained {
		t.Fatalf("entry conflict flags = %#v, want true", entry)
	}
	if entry.PreviousValue != "Austin" {
		t.Fatalf("entry previous_value = %v, want Austin", entry.PreviousValue)
	}

	beliefs := decodeBeliefs(t, patched.Properties[models.FactBeliefsProperty])
	belief := beliefs["city"]
	if belief.Status != models.FactBeliefStatusSupported {
		t.Fatalf("belief status = %q, want %q", belief.Status, models.FactBeliefStatusSupported)
	}
	if belief.PreferredValue != "Chicago" {
		t.Fatalf("belief preferred value = %v, want Chicago", belief.PreferredValue)
	}
	if belief.EvidenceCount != 1 || len(belief.Claims) != 1 {
		t.Fatalf("belief counts = %+v, want 1 evidence and 1 claim", belief)
	}
	if !belief.Claims[0].Preferred || belief.Claims[0].Status != models.FactBeliefStatusSupported {
		t.Fatalf("belief claim = %+v, want preferred supported claim", belief.Claims[0])
	}

	changes, _, err := hs.GetPropertyHistory(ctx, tenantID, node.ID, "", 10, 0)
	if err != nil {
		t.Fatalf("GetPropertyHistory: %v", err)
	}
	if len(changes) != 1 {
		t.Fatalf("history len = %d, want 1", len(changes))
	}
	if changes[0].PropertyKey != "city" {
		t.Fatalf("history property_key = %q, want city", changes[0].PropertyKey)
	}
}

func TestPatchNodeProperties_FactCorroborationAddsEvidenceWithoutHistoryChange(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	hs := store.NewHistoryStore(base)
	ctx := context.Background()

	req := models.CreateNodeRequest{Type: "person", Label: "Alice", Properties: map[string]any{"city": "Austin"}}
	_ = req.Validate()
	node, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	patched, err := ns.PatchNodeProperties(ctx, tenantID, node.ID, models.PatchPropertiesRequest{
		Properties: map[string]any{
			models.FactUpdatesProperty: map[string]models.FactUpdate{
				"city": {Value: "Austin", Source: "calendar"},
			},
		},
	})
	if err != nil {
		t.Fatalf("PatchNodeProperties: %v", err)
	}

	evidence := decodeEvidence(t, patched.Properties[models.FactEvidenceProperty])
	entries := evidence["city"]
	if len(entries) != 1 {
		t.Fatalf("city evidence len = %d, want 1", len(entries))
	}
	if entries[0].ConflictsWithPrior || entries[0].SupersedesPrior || entries[0].PreviousValue != nil {
		t.Fatalf("unexpected corroboration entry = %#v", entries[0])
	}

	beliefs := decodeBeliefs(t, patched.Properties[models.FactBeliefsProperty])
	belief := beliefs["city"]
	if belief.Status != models.FactBeliefStatusSupported || belief.PreferredValue != "Austin" {
		t.Fatalf("belief = %+v, want supported Austin", belief)
	}
	if belief.EvidenceCount != 1 || len(belief.Claims) != 1 {
		t.Fatalf("belief counts = %+v, want 1 evidence and 1 claim", belief)
	}

	changes, _, err := hs.GetPropertyHistory(ctx, tenantID, node.ID, "", 10, 0)
	if err != nil {
		t.Fatalf("GetPropertyHistory: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("history len = %d, want 0", len(changes))
	}
}

func TestPatchNodeProperties_FactConflictCanBecomeContested(t *testing.T) {
	base, tenantID := setupTestBase(t)
	ns := store.NewNodeStore(base)
	ctx := context.Background()

	req := models.CreateNodeRequest{Type: "person", Label: "Alice", Properties: map[string]any{"city": "Austin"}}
	_ = req.Validate()
	node, err := ns.CreateNode(ctx, tenantID, req)
	if err != nil {
		t.Fatalf("CreateNode: %v", err)
	}

	confidence := 0.9
	for _, value := range []string{"Austin", "Chicago", "Austin", "Chicago"} {
		patched, err := ns.PatchNodeProperties(ctx, tenantID, node.ID, models.PatchPropertiesRequest{
			Properties: map[string]any{
				models.FactUpdatesProperty: map[string]models.FactUpdate{
					"city": {Value: value, Source: "note", Confidence: &confidence},
				},
			},
		})
		if err != nil {
			t.Fatalf("PatchNodeProperties(%q): %v", value, err)
		}
		node = patched
	}

	beliefs := decodeBeliefs(t, node.Properties[models.FactBeliefsProperty])
	belief := beliefs["city"]
	if belief.Status != models.FactBeliefStatusContested {
		t.Fatalf("belief status = %q, want %q", belief.Status, models.FactBeliefStatusContested)
	}
	if len(belief.Claims) != 2 {
		t.Fatalf("belief claims len = %d, want 2", len(belief.Claims))
	}
	if belief.Claims[0].Value != "Chicago" || !belief.Claims[0].Preferred {
		t.Fatalf("preferred claim = %+v, want preferred Chicago", belief.Claims[0])
	}
	if belief.Claims[0].Status != models.FactBeliefStatusContested || belief.Claims[1].Status != models.FactBeliefStatusContested {
		t.Fatalf("claim statuses = %+v, want contested top claims", belief.Claims)
	}
	if belief.Claims[1].Value != "Austin" {
		t.Fatalf("runner-up claim = %+v, want Austin", belief.Claims[1])
	}
}

func decodeEvidence(t *testing.T, raw any) map[string][]models.FactEvidence {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	var evidence map[string][]models.FactEvidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		t.Fatalf("unmarshal evidence: %v", err)
	}
	return evidence
}

func decodeBeliefs(t *testing.T, raw any) map[string]models.FactBeliefState {
	t.Helper()
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal beliefs: %v", err)
	}
	var beliefs map[string]models.FactBeliefState
	if err := json.Unmarshal(data, &beliefs); err != nil {
		t.Fatalf("unmarshal beliefs: %v", err)
	}
	return beliefs
}
