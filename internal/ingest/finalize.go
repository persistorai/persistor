package ingest

import "strings"

// FinalizeExtraction applies a final document-level normalization and compaction pass.
func FinalizeExtraction(
	entities []ExtractedEntity,
	rels []ExtractedRelationship,
	facts []ExtractedFact,
	knownEntities []string,
	documentText string,
) ([]ExtractedEntity, []ExtractedRelationship, []ExtractedFact) {
	result := PostProcessExtraction(&ExtractionResult{
		Entities:      entities,
		Relationships: rels,
		Facts:         facts,
	}, knownEntities)

	fullPersonNames := buildFullPersonNames(result.Entities, knownEntities)
	canonicalByLower := buildCanonicalNameMap(result.Entities, fullPersonNames)

	filteredEntities := dedupeEntities(result.Entities, fullPersonNames, canonicalByLower)
	entityTypes := makeEntityTypeMap(filteredEntities)
	birthdays := buildBirthdayMap(filteredEntities, result.Facts)
	filteredRels := dedupeRelationships(result.Relationships, fullPersonNames, canonicalByLower, entityTypes, birthdays)
	filteredFacts := dedupeFacts(result.Facts, fullPersonNames, canonicalByLower)

	docKind := detectDocumentKind(documentText)
	filteredEntities = compactEntitiesForDocument(filteredEntities, filteredRels, filteredFacts, documentText, docKind)
	entitySet := buildEntitySet(filteredEntities)
	filteredRels = filterRelationshipsToEntitySet(filteredRels, entitySet)
	filteredFacts = filterFactsToEntitySet(filteredFacts, entitySet)

	return filteredEntities, filteredRels, filteredFacts
}

func buildFullPersonNames(entities []ExtractedEntity, knownEntities []string) []string {
	fullPersonNames := make([]string, 0, len(entities)+len(knownEntities))
	for _, known := range knownEntities {
		trimmed := strings.TrimSpace(known)
		if strings.Contains(trimmed, " ") {
			fullPersonNames = append(fullPersonNames, trimmed)
		}
	}
	for _, ent := range entities {
		trimmed := strings.TrimSpace(ent.Name)
		if ent.Type == "person" && strings.Contains(trimmed, " ") {
			fullPersonNames = append(fullPersonNames, trimmed)
		}
	}
	return fullPersonNames
}

func buildCanonicalNameMap(entities []ExtractedEntity, fullPersonNames []string) map[string]string {
	canonicalByLower := make(map[string]string, len(entities)*2)
	for _, ent := range entities {
		trimmed := strings.TrimSpace(ent.Name)
		canonicalByLower[strings.ToLower(trimmed)] = ent.Name
		canonicalByLower[strings.ToLower(normalizeLabelVariant(trimmed))] = ent.Name
	}
	for _, full := range fullPersonNames {
		parts := strings.Fields(full)
		if len(parts) < 2 {
			continue
		}
		first := strings.ToLower(parts[0])
		if _, ok := canonicalByLower[first]; !ok {
			canonicalByLower[first] = full
		}
	}
	return canonicalByLower
}

func dedupeEntities(entities []ExtractedEntity, fullPersonNames []string, canonicalByLower map[string]string) []ExtractedEntity {
	filteredEntities := make([]ExtractedEntity, 0, len(entities))
	seenEntities := make(map[string]bool, len(entities))
	for _, ent := range entities {
		canonical := chooseCanonicalName(ent.Name, fullPersonNames, canonicalByLower)
		ent.Name = canonical
		key := strings.ToLower(strings.TrimSpace(ent.Name)) + "|" + strings.TrimSpace(ent.Type)
		if key == "|" || seenEntities[key] {
			continue
		}
		seenEntities[key] = true
		filteredEntities = append(filteredEntities, ent)
	}
	return filteredEntities
}

func dedupeRelationships(
	rels []ExtractedRelationship,
	fullPersonNames []string,
	canonicalByLower map[string]string,
	entityTypes map[string]string,
	birthdays map[string]string,
) []ExtractedRelationship {
	filteredRels := make([]ExtractedRelationship, 0, len(rels))
	seenRels := make(map[string]bool, len(rels))
	for _, rel := range rels {
		rel.Source = chooseCanonicalName(rel.Source, fullPersonNames, canonicalByLower)
		rel.Target = chooseCanonicalName(rel.Target, fullPersonNames, canonicalByLower)
		normalized, ok := normalizeRelationshipWithContext(rel, entityTypes, birthdays)
		if !ok {
			continue
		}
		key := relationshipKey(normalized)
		if seenRels[key] {
			continue
		}
		seenRels[key] = true
		filteredRels = append(filteredRels, normalized)
	}
	return filteredRels
}

func dedupeFacts(facts []ExtractedFact, fullPersonNames []string, canonicalByLower map[string]string) []ExtractedFact {
	filteredFacts := make([]ExtractedFact, 0, len(facts))
	seenFacts := make(map[string]bool, len(facts))
	for _, fact := range facts {
		fact.Subject = chooseCanonicalName(fact.Subject, fullPersonNames, canonicalByLower)
		key := strings.ToLower(fact.Subject) + "|" + fact.Property
		if fact.Subject == "" || seenFacts[key] {
			continue
		}
		seenFacts[key] = true
		filteredFacts = append(filteredFacts, fact)
	}
	return filteredFacts
}
