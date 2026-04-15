package ingest

import "strings"

var weakAbstractConcepts = map[string]bool{
	"trust":     true,
	"honesty":   true,
	"faith":     true,
	"mission":   true,
	"principle": true,
	"values":    true,
}

var weakPersonaTerms = map[string]bool{
	"voice":     true,
	"assistant": true,
	"avatar":    true,
	"persona":   true,
}

var discouragedRelations = map[string]bool{
	"partners_with": true,
	"prefers":       true,
	"trusts":        true,
	"learned":       true,
	"inspired":      true,
}

func entityLooksWorthTracking(ent ExtractedEntity) bool {
	name := strings.TrimSpace(ent.Name)
	if name == "" {
		return false
	}

	lower := strings.ToLower(name)
	wordCount := len(strings.Fields(name))

	switch ent.Type {
	case "person", "project", "company", "place", "animal", "event", "decision":
		return true
	case "technology", "service":
		if wordCount >= 2 {
			return true
		}
		return !weakPersonaTerms[lower]
	case "concept":
		if weakAbstractConcepts[lower] {
			return false
		}
		return wordCount >= 2
	default:
		return false
	}
}

func relationshipLooksWorthTracking(rel ExtractedRelationship, entitiesByName map[string]ExtractedEntity) bool {
	if discouragedRelations[strings.TrimSpace(rel.Relation)] {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(rel.Source), strings.TrimSpace(rel.Target)) {
		return false
	}

	source, sourceOK := entitiesByName[strings.ToLower(strings.TrimSpace(rel.Source))]
	target, targetOK := entitiesByName[strings.ToLower(strings.TrimSpace(rel.Target))]
	if sourceOK && targetOK {
		if rel.Relation == "works_on" && source.Type == "service" && target.Type == "person" {
			return false
		}
		if rel.Relation == "uses" && source.Type == "service" && target.Type == "service" {
			return false
		}
	}

	return true
}
