package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
)

// LLMClient is the interface for LLM interaction (for testing).
type LLMClient interface {
	Chat(ctx context.Context, prompt string) (string, error)
}

// Extractor extracts entities and relationships from text using an LLM.
type Extractor struct {
	llm LLMClient
	log *logrus.Logger
}

// NewExtractor creates an Extractor with the given LLM client.
func NewExtractor(llm LLMClient) *Extractor {
	return &Extractor{
		llm: llm,
		log: logrus.StandardLogger(),
	}
}

// allowedEntityTypes defines valid entity types for extraction.
var allowedEntityTypes = map[string]bool{
	"person":     true,
	"project":    true,
	"company":    true,
	"technology": true,
	"event":      true,
	"decision":   true,
	"concept":    true,
	"place":      true,
	"animal":     true,
}

// Extract processes a text chunk and returns structured extraction results.
func (e *Extractor) Extract(ctx context.Context, chunk string) (*ExtractionResult, error) {
	prompt := buildPrompt(chunk)

	raw, err := e.llm.Chat(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("calling LLM: %w", err)
	}

	if strings.TrimSpace(raw) == "" {
		return &ExtractionResult{}, nil
	}

	return e.parseResponse(raw)
}

func (e *Extractor) parseResponse(raw string) (*ExtractionResult, error) {
	repaired := RepairJSON(raw)

	var result ExtractionResult
	if err := json.Unmarshal([]byte(repaired), &result); err != nil {
		return nil, fmt.Errorf("parsing extraction result: %w", err)
	}

	result.Entities = filterValidEntities(result.Entities, e.log)

	return &result, nil
}

func filterValidEntities(entities []ExtractedEntity, log *logrus.Logger) []ExtractedEntity {
	valid := make([]ExtractedEntity, 0, len(entities))

	for _, ent := range entities {
		entityType := strings.ToLower(ent.Type)
		if !allowedEntityTypes[entityType] {
			log.Warnf("filtering entity %q: invalid type %q", ent.Name, ent.Type)
			continue
		}

		ent.Type = entityType
		valid = append(valid, ent)
	}

	return valid
}

func buildPrompt(chunk string) string {
	return strings.Replace(extractionPrompt, "{text}", chunk, 1)
}

const extractionPrompt = `Extract entities, relationships, and facts from the following text.

Output ONLY valid JSON (no markdown fences, no explanation, no thinking):
{
  "entities": [
    {
      "name": "Entity Name",
      "type": "person|project|company|technology|event|decision|concept|place|animal",
      "properties": {},
      "description": "One sentence description"
    }
  ],
  "relationships": [
    {
      "source": "Entity A name",
      "target": "Entity B name",
      "relation": "relationship_type",
      "confidence": 0.9
    }
  ],
  "facts": [
    {
      "subject": "Entity Name",
      "property": "key",
      "value": "value"
    }
  ]
}

Relationship types — use the MOST SPECIFIC one that fits:
- "created" — A built/founded/authored B
- "founded" — A founded/established B (for organizations)
- "works_at" — A is currently employed by B
- "worked_at" — A was formerly employed by B
- "works_on" — A works on project/task B (NOT employment)
- "leads" — A leads/directs B
- "owns" — A owns B
- "part_of" — A is a component/member of B
- "product_of" — A is a product of company B
- "deployed_on" — A is deployed/hosted on B
- "runs_on" — A runs on platform/infra B
- "uses" — A uses/utilizes B
- "depends_on" — A requires/depends on B
- "implements" — A implements pattern/interface B
- "extends" — A extends/builds upon B
- "replaced_by" — A was replaced/superseded by B
- "enables" — A enables/powers B
- "supports" — A supports/is compatible with B
- "parent_of" — A is parent of B
- "child_of" — A is child of B
- "sibling_of" — A is sibling of B
- "married_to" — A is married to B
- "friend_of" — A is friend of B
- "mentored" — A mentored/taught B
- "located_in" — A is physically located in B
- "learned" — A learned/studied B
- "decided" — A made decision B
- "inspired" — A inspired/motivated B
- "prefers" — A prefers/favors B
- "competes_with" — A competes with B
- "acquired" — A acquired/purchased B
- "funded" — A gave money/resources to B
- "partners_with" — A partners/collaborates with B
- "affected_by" — A was impacted by event/thing B
- "achieved" — A achieved milestone/goal B
- "detected_in" — A was detected/found in B
- "experienced" — A experienced event B

Rules:
- Maximum 15 entities. Prioritize people, then projects, then companies, then events.
- Only extract specific, named entities — not generic nouns like "the project" or "code"
- Entity types MUST be one of: person, project, company, technology, event, decision, concept, place, animal
- Keep descriptions to one short sentence
- Include dates in properties when mentioned (e.g. "date": "2026-03-12")
- Relationships must reference entities by exact name from your entities list
- Direction matters: "Alice created ProjectX" not "ProjectX created Alice"
- Do NOT use "related_to" — always pick a more specific relationship type
- Confidence: 0.9+ for explicit statements, 0.6-0.8 for implied/inferred
- Facts should capture specific values: amounts, dates, measurements, statuses
- Output ONLY the JSON object — no text before or after it

Text:
---
{text}
---`
