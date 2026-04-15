package ingest

import "testing"

func TestFinalizeExtraction_ProfileFiltersHallucinatedKnownEntities(t *testing.T) {
	doc := `# SOUL.md - Who I Am

I'm Scout.

Brian is the engineer and visionary.
DeerPrint is the project.`

	entities := []ExtractedEntity{
		{Name: "Scout", Type: "service"},
		{Name: "Brian Colinger", Type: "person"},
		{Name: "DeerPrint", Type: "project"},
		{Name: "Type 1 Diabetes", Type: "concept"},
		{Name: "Decatur, Arkansas", Type: "place"},
	}
	rels := []ExtractedRelationship{
		{Source: "Scout", Target: "DeerPrint", Relation: "works_on", Confidence: 0.9},
		{Source: "AI Assistant", Target: "DeerPrint", Relation: "works_on", Confidence: 0.8},
	}
	facts := []ExtractedFact{{Subject: "Brian Colinger", Property: "faith", Value: "Christian"}}

	gotEntities, gotRels, gotFacts := FinalizeExtraction(entities, rels, facts, []string{"Brian Colinger"}, doc)

	if len(gotEntities) != 3 {
		t.Fatalf("expected 3 kept entities, got %d: %#v", len(gotEntities), gotEntities)
	}
	assertEntityNames(t, gotEntities, "Scout", "Brian Colinger", "DeerPrint")
	if len(gotRels) != 1 {
		t.Fatalf("expected 1 kept relationship, got %d: %#v", len(gotRels), gotRels)
	}
	if gotRels[0].Source != "Scout" || gotRels[0].Target != "DeerPrint" {
		t.Fatalf("unexpected relationship kept: %#v", gotRels[0])
	}
	if len(gotFacts) != 1 || gotFacts[0].Subject != "Brian Colinger" {
		t.Fatalf("unexpected facts after finalize: %#v", gotFacts)
	}
}

func TestFinalizeExtraction_NormalizesDirectionAndPastEmployment(t *testing.T) {
	doc := `# USER.md - About Brian
- Brian Colinger
- Terry McClure
- Rebuy, Inc.
- DeerPrint`

	entities := []ExtractedEntity{
		{Name: "Brian Colinger", Type: "person", Properties: map[string]any{"birthday": "1982-08-14"}},
		{Name: "Terry McClure", Type: "person", Properties: map[string]any{"birthday": "1961-01-01"}},
		{Name: "Rebuy, Inc.", Type: "company"},
		{Name: "DeerPrint", Type: "project"},
	}
	rels := []ExtractedRelationship{
		{Source: "Brian Colinger", Target: "Terry McClure", Relation: "parent_of", Confidence: 0.9},
		{Source: "Brian Colinger", Target: "Rebuy, Inc.", Relation: "works_at", Confidence: 0.9, IsCurrent: boolPtr(false)},
		{Source: "Brian Colinger", Target: "DeerPrint", Relation: "product_of", Confidence: 0.9},
		{Source: "Laura", Target: "Brian Colinger", Relation: "married_to", Confidence: 0.9},
	}
	facts := []ExtractedFact{}

	_, gotRels, _ := FinalizeExtraction(entities, rels, facts, nil, doc)

	assertHasRelation(t, gotRels, "Brian Colinger", "Terry McClure", "child_of")
	assertHasRelation(t, gotRels, "Brian Colinger", "Rebuy, Inc.", "worked_at")
	assertHasRelation(t, gotRels, "Brian Colinger", "DeerPrint", "created")
	assertNoRelation(t, gotRels, "Laura", "Brian Colinger", "married_to")
}

func assertEntityNames(t *testing.T, entities []ExtractedEntity, expected ...string) {
	t.Helper()
	seen := map[string]bool{}
	for _, ent := range entities {
		seen[ent.Name] = true
	}
	for _, name := range expected {
		if !seen[name] {
			t.Fatalf("expected entity %q in %#v", name, entities)
		}
	}
}

func assertHasRelation(t *testing.T, rels []ExtractedRelationship, source, target, relation string) {
	t.Helper()
	for _, rel := range rels {
		if rel.Source == source && rel.Target == target && rel.Relation == relation {
			return
		}
	}
	t.Fatalf("expected relation %s -> %s (%s) in %#v", source, target, relation, rels)
}

func assertNoRelation(t *testing.T, rels []ExtractedRelationship, source, target, relation string) {
	t.Helper()
	for _, rel := range rels {
		if rel.Source == source && rel.Target == target && rel.Relation == relation {
			t.Fatalf("unexpected relation %s -> %s (%s) in %#v", source, target, relation, rels)
		}
	}
}

func boolPtr(v bool) *bool {
	return &v
}
