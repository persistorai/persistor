package ingest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/persistorai/persistor/internal/models"
)

type documentKind string

const (
	documentKindGeneral documentKind = "general"
	documentKindProfile documentKind = "profile"
)

type entitySupport struct {
	relationCount int
	factCount     int
	propertyCount int
	score         int
}

func detectDocumentKind(text string) documentKind {
	sample := strings.ToLower(text)
	if len(sample) > 2000 {
		sample = sample[:2000]
	}
	if strings.Contains(sample, "# user.md") || strings.Contains(sample, "# soul.md") || strings.Contains(sample, "# identity.md") {
		return documentKindProfile
	}
	if strings.Count(sample, "- **") >= 6 {
		return documentKindProfile
	}
	return documentKindGeneral
}

func compactEntitiesForDocument(
	entities []ExtractedEntity,
	rels []ExtractedRelationship,
	facts []ExtractedFact,
	documentText string,
	kind documentKind,
) []ExtractedEntity {
	if len(entities) == 0 {
		return nil
	}

	support := buildEntitySupport(entities, rels, facts)
	filtered := make([]ExtractedEntity, 0, len(entities))
	for _, ent := range entities {
		if !shouldKeepEntity(ent, support[strings.ToLower(strings.TrimSpace(ent.Name))], documentText, kind) {
			continue
		}
		filtered = append(filtered, ent)
	}

	maxEntities := maxEntitiesForDocument(kind)
	filtered = collapseDuplicateEntityNames(filtered, support)
	if maxEntities > 0 && len(filtered) > maxEntities {
		sort.SliceStable(filtered, func(i, j int) bool {
			left := support[strings.ToLower(strings.TrimSpace(filtered[i].Name))].score
			right := support[strings.ToLower(strings.TrimSpace(filtered[j].Name))].score
			if left != right {
				return left > right
			}
			return strings.ToLower(filtered[i].Name) < strings.ToLower(filtered[j].Name)
		})
		filtered = filtered[:maxEntities]
	}

	return filtered
}

func collapseDuplicateEntityNames(entities []ExtractedEntity, support map[string]entitySupport) []ExtractedEntity {
	bestByName := make(map[string]ExtractedEntity, len(entities))
	for _, ent := range entities {
		key := strings.ToLower(strings.TrimSpace(ent.Name))
		existing, ok := bestByName[key]
		if !ok {
			bestByName[key] = ent
			continue
		}
		currentScore := support[key].score + typeWeight(ent.Type)
		existingScore := support[key].score + typeWeight(existing.Type)
		if currentScore > existingScore {
			bestByName[key] = ent
		}
	}
	collapsed := make([]ExtractedEntity, 0, len(bestByName))
	for _, ent := range bestByName {
		collapsed = append(collapsed, ent)
	}
	sort.SliceStable(collapsed, func(i, j int) bool {
		return strings.ToLower(collapsed[i].Name) < strings.ToLower(collapsed[j].Name)
	})
	return collapsed
}

func buildEntitySupport(
	entities []ExtractedEntity,
	rels []ExtractedRelationship,
	facts []ExtractedFact,
) map[string]entitySupport {
	support := make(map[string]entitySupport, len(entities))
	for _, ent := range entities {
		key := strings.ToLower(strings.TrimSpace(ent.Name))
		s := support[key]
		s.propertyCount += len(ent.Properties)
		s.score += typeWeight(ent.Type) + len(ent.Properties)
		if strings.TrimSpace(ent.Description) != "" {
			s.score++
		}
		lowerDesc := strings.ToLower(ent.Description)
		if strings.Contains(lowerDesc, "potential acquisition target") {
			s.score -= 3
		}
		if strings.Contains(lowerDesc, "voice model") {
			s.score -= 2
		}
		support[key] = s
	}

	for _, rel := range rels {
		for _, name := range []string{rel.Source, rel.Target} {
			key := strings.ToLower(strings.TrimSpace(name))
			s := support[key]
			s.relationCount++
			s.score += 3
			support[key] = s
		}
	}

	for _, fact := range facts {
		key := strings.ToLower(strings.TrimSpace(fact.Subject))
		s := support[key]
		s.factCount++
		s.score += 2
		support[key] = s
	}

	return support
}

func shouldKeepEntity(ent ExtractedEntity, support entitySupport, documentText string, kind documentKind) bool {
	if !entityMentionedInText(ent, documentText) {
		return false
	}
	name := strings.TrimSpace(ent.Name)
	if name == "" {
		return false
	}
	lowerDesc := strings.ToLower(ent.Description)

	if kind != documentKindProfile {
		return support.score >= 2
	}

	switch ent.Type {
	case "person":
		return support.relationCount > 0 || support.factCount > 0 || support.propertyCount > 0
	case "project", "company", "place", "decision", "event", "concept":
		if strings.Contains(lowerDesc, "potential acquisition target") && support.factCount == 0 {
			return false
		}
		return support.score >= 5
	case "service":
		if strings.EqualFold(name, "Scout") {
			return true
		}
		return support.relationCount >= 2 || (support.factCount >= 3 && !strings.Contains(lowerDesc, "voice model"))
	case "technology":
		return support.relationCount >= 2 || support.factCount >= 3
	case "animal":
		return len(strings.Fields(name)) >= 2 && (support.relationCount >= 2 || support.factCount >= 2)
	default:
		return false
	}
}

func maxEntitiesForDocument(kind documentKind) int {
	if kind == documentKindProfile {
		return 12
	}
	return 0
}

func typeWeight(entityType string) int {
	switch entityType {
	case "person":
		return 6
	case "project", "company":
		return 5
	case "place", "decision", "event", "concept":
		return 4
	case "service":
		return 3
	case "technology":
		return 2
	case "animal":
		return 1
	default:
		return 0
	}
}

func entityMentionedInText(ent ExtractedEntity, text string) bool {
	if strings.TrimSpace(text) == "" {
		return true
	}
	normalizedText := normalizeMentionText(text)
	for _, variant := range entityMentionVariants(ent) {
		if strings.Contains(normalizedText, normalizeMentionText(variant)) {
			return true
		}
	}
	return false
}

func entityMentionVariants(ent ExtractedEntity) []string {
	variants := []string{ent.Name, normalizeLabelVariant(ent.Name)}
	if ent.Type == "person" {
		parts := strings.Fields(strings.TrimSpace(ent.Name))
		if len(parts) >= 2 {
			variants = append(variants, parts[0])
		}
	}
	return variants
}

func normalizeMentionText(text string) string {
	replacer := strings.NewReplacer(
		",", " ",
		".", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		":", " ",
		";", " ",
		"\n", " ",
		"\t", " ",
		"-", " ",
	)
	return strings.Join(strings.Fields(strings.ToLower(replacer.Replace(text))), " ")
}

func makeEntityTypeMap(entities []ExtractedEntity) map[string]string {
	entityTypes := make(map[string]string, len(entities))
	for _, ent := range entities {
		entityTypes[strings.ToLower(strings.TrimSpace(ent.Name))] = ent.Type
	}
	return entityTypes
}

func buildBirthdayMap(entities []ExtractedEntity, facts []ExtractedFact) map[string]string {
	birthdays := make(map[string]string, len(entities)+len(facts))
	for _, ent := range entities {
		key := strings.ToLower(strings.TrimSpace(ent.Name))
		for _, prop := range []string{"birthday", "birth_date"} {
			if raw, ok := ent.Properties[prop]; ok {
				if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
					birthdays[key] = value
				}
			}
		}
	}
	for _, fact := range facts {
		if fact.Property != "birthday" && fact.Property != "birth_date" {
			continue
		}
		birthdays[strings.ToLower(strings.TrimSpace(fact.Subject))] = strings.TrimSpace(toStringValue(fact.Value))
	}
	return birthdays
}

func normalizeRelationshipWithContext(
	rel ExtractedRelationship,
	entityTypes map[string]string,
	birthdays map[string]string,
) (ExtractedRelationship, bool) {
	rel.Relation = normalizeRelation(rel.Relation)
	if rel.Source == "" || rel.Target == "" || strings.EqualFold(rel.Source, rel.Target) {
		return rel, false
	}

	sourceType := entityTypes[strings.ToLower(strings.TrimSpace(rel.Source))]
	targetType := entityTypes[strings.ToLower(strings.TrimSpace(rel.Target))]
	if rel.Relation == "product_of" && sourceType == "person" && targetType == "project" {
		rel.Relation = "created"
	}
	if rel.Relation == "founded" && sourceType == "person" && targetType == "project" {
		rel.Relation = "created"
	}
	if rel.Relation == "created" && sourceType == "project" && targetType == "person" {
		rel.Source, rel.Target = rel.Target, rel.Source
	}
	if rel.Relation == "works_at" && rel.IsCurrent != nil && !*rel.IsCurrent {
		rel.Relation = "worked_at"
	}
	if rel.Relation == "works_at" && rel.DateEnd != nil && strings.TrimSpace(*rel.DateEnd) != "" {
		rel.Relation = "worked_at"
	}
	if rel.Relation == "parent_of" {
		sourceBirthday := birthdays[strings.ToLower(strings.TrimSpace(rel.Source))]
		targetBirthday := birthdays[strings.ToLower(strings.TrimSpace(rel.Target))]
		if sourceBirthday != "" && targetBirthday != "" && sourceBirthday > targetBirthday {
			rel.Relation = "child_of"
		}
	}
	if rel.Relation == "child_of" {
		sourceBirthday := birthdays[strings.ToLower(strings.TrimSpace(rel.Source))]
		targetBirthday := birthdays[strings.ToLower(strings.TrimSpace(rel.Target))]
		if sourceBirthday != "" && targetBirthday != "" && sourceBirthday < targetBirthday {
			rel.Relation = "parent_of"
		}
	}
	if !models.IsCanonicalRelation(rel.Relation) {
		return rel, false
	}
	return rel, true
}

func relationshipKey(rel ExtractedRelationship) string {
	left := strings.ToLower(strings.TrimSpace(rel.Source))
	right := strings.ToLower(strings.TrimSpace(rel.Target))
	if isSymmetricRelation(rel.Relation) && left > right {
		left, right = right, left
	}
	return left + "|" + right + "|" + rel.Relation
}

func isSymmetricRelation(rel string) bool {
	switch rel {
	case "married_to", "sibling_of", "friend_of", "partners_with", "competes_with":
		return true
	default:
		return false
	}
}

func buildEntitySet(entities []ExtractedEntity) map[string]bool {
	entitySet := make(map[string]bool, len(entities))
	for _, ent := range entities {
		entitySet[strings.ToLower(strings.TrimSpace(ent.Name))] = true
	}
	return entitySet
}

func filterRelationshipsToEntitySet(rels []ExtractedRelationship, entitySet map[string]bool) []ExtractedRelationship {
	filtered := make([]ExtractedRelationship, 0, len(rels))
	seen := make(map[string]bool, len(rels))
	for _, rel := range rels {
		if !entitySet[strings.ToLower(strings.TrimSpace(rel.Source))] || !entitySet[strings.ToLower(strings.TrimSpace(rel.Target))] {
			continue
		}
		key := relationshipKey(rel)
		if seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, rel)
	}
	return filtered
}

func filterFactsToEntitySet(facts []ExtractedFact, entitySet map[string]bool) []ExtractedFact {
	filtered := make([]ExtractedFact, 0, len(facts))
	seen := make(map[string]bool, len(facts))
	for _, fact := range facts {
		if !entitySet[strings.ToLower(strings.TrimSpace(fact.Subject))] {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(fact.Subject)) + "|" + fact.Property
		if seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, fact)
	}
	return filtered
}

func toStringValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}
