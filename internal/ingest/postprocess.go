package ingest

import "strings"

// PostProcessExtraction tightens LLM output before write.
func PostProcessExtraction(result *ExtractionResult, knownEntities []string) *ExtractionResult {
	if result == nil {
		return &ExtractionResult{}
	}

	canonicalByLower := make(map[string]string, len(knownEntities))
	fullPersonNames := make([]string, 0, len(knownEntities))
	for _, name := range knownEntities {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		canonicalByLower[strings.ToLower(trimmed)] = trimmed
		canonicalByLower[strings.ToLower(normalizeLabelVariant(trimmed))] = trimmed
		if strings.Contains(trimmed, " ") {
			fullPersonNames = append(fullPersonNames, trimmed)
		}
	}

	entityCanonical := make(map[string]string)
	for i := range result.Entities {
		name := strings.TrimSpace(result.Entities[i].Name)
		if name == "" {
			continue
		}
		canonical := chooseCanonicalName(name, fullPersonNames, canonicalByLower)
		result.Entities[i].Name = canonical
		entityCanonical[strings.ToLower(name)] = canonical
		entityCanonical[strings.ToLower(normalizeLabelVariant(name))] = canonical
		entityCanonical[strings.ToLower(canonical)] = canonical
		entityCanonical[strings.ToLower(normalizeLabelVariant(canonical))] = canonical
	}

	seenEntities := make(map[string]bool, len(result.Entities))
	filteredEntities := make([]ExtractedEntity, 0, len(result.Entities))
	entitiesByName := make(map[string]ExtractedEntity, len(result.Entities))
	for _, ent := range result.Entities {
		nameLower := strings.ToLower(strings.TrimSpace(ent.Name))
		if !entityLooksWorthTracking(ent) {
			continue
		}
		key := nameLower + "|" + strings.TrimSpace(ent.Type)
		if key == "|" || seenEntities[key] {
			continue
		}
		seenEntities[key] = true
		filteredEntities = append(filteredEntities, ent)
		entitiesByName[nameLower] = ent
	}
	result.Entities = filteredEntities

	filteredRels := make([]ExtractedRelationship, 0, len(result.Relationships))
	for _, rel := range result.Relationships {
		rel.Relation = normalizeRelation(rel.Relation)
		rel.Source = canonicalizeRef(rel.Source, entityCanonical, fullPersonNames, canonicalByLower)
		rel.Target = canonicalizeRef(rel.Target, entityCanonical, fullPersonNames, canonicalByLower)
		if rel.Relation == "" || rel.Source == "" || rel.Target == "" || strings.EqualFold(rel.Source, rel.Target) {
			continue
		}
		if !relationshipLooksWorthTracking(rel, entitiesByName) {
			continue
		}
		filteredRels = append(filteredRels, rel)
	}
	result.Relationships = filteredRels

	filteredFacts := make([]ExtractedFact, 0, len(result.Facts))
	for _, fact := range result.Facts {
		fact.Subject = canonicalizeRef(fact.Subject, entityCanonical, fullPersonNames, canonicalByLower)
		fact.Property = normalizeFactProperty(fact.Property)
		if strings.TrimSpace(fact.Subject) == "" || strings.TrimSpace(fact.Property) == "" {
			continue
		}
		filteredFacts = append(filteredFacts, fact)
	}
	result.Facts = filteredFacts

	return result
}

func canonicalizeRef(name string, entityCanonical map[string]string, fullPersonNames []string, canonicalByLower map[string]string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	trimmed = normalizeLabelVariant(trimmed)
	if canonical, ok := entityCanonical[strings.ToLower(trimmed)]; ok {
		return canonical
	}
	return chooseCanonicalName(trimmed, fullPersonNames, canonicalByLower)
}

func chooseCanonicalName(name string, fullPersonNames []string, canonicalByLower map[string]string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	if canonical, ok := canonicalByLower[strings.ToLower(trimmed)]; ok {
		return canonical
	}

	normalized := normalizeLabelVariant(trimmed)
	if canonical, ok := canonicalByLower[strings.ToLower(normalized)]; ok {
		return canonical
	}

	lower := strings.ToLower(normalized)
	for _, full := range fullPersonNames {
		parts := strings.Fields(full)
		if len(parts) < 2 {
			continue
		}
		if strings.ToLower(parts[0]) == lower {
			return full
		}
	}

	return trimmed
}
