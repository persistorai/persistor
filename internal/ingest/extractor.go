package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	"service":    true,
}

// entityTypeAliases maps common LLM-generated types to canonical types.
var entityTypeAliases = map[string]string{
	"location":     "place",
	"lesson":       "concept",
	"milestone":    "event",
	"component":    "technology",
	"endpoint":     "technology",
	"tool":         "technology",
	"software":     "technology",
	"framework":    "technology",
	"library":      "technology",
	"language":     "technology",
	"organization": "company",
	"org":          "company",
	"institution":  "company",
	"skill":        "concept",
	"pattern":      "concept",
	"principle":    "concept",
	"idea":         "concept",
	"feature":      "concept",
	"goal":         "concept",
	"metric":       "concept",
	"process":      "concept",
	"strategy":     "concept",
	"creature":     "animal",
	"pet":          "animal",
	"wildlife":     "animal",
	"media":        "concept",
	"model":        "technology",
	"database":     "technology",
	"platform":     "technology",
	"protocol":     "technology",
	"api":          "technology",
	"plugin":       "technology",
	"extension":    "technology",
	"agent":        "project",
	"product":      "project",
	"application":  "project",
	"app":          "project",
	"equipment":    "technology",
	"machine":      "technology",
	"hardware":     "technology",
	"device":       "technology",
	"sensor":       "technology",
	"entity":       "concept",
	"data":         "concept",
	"dataset":      "concept",
	"resource":     "concept",
	"domain":       "concept",
}

// Extract processes a text chunk and returns structured extraction results.
// knownEntities is an optional list of entity names to guide consistent naming.
func (e *Extractor) Extract(ctx context.Context, chunk string, knownEntities ...string) (*ExtractionResult, error) {
	prompt := buildPrompt(chunk, knownEntities)

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

		// Normalize aliases to canonical types
		if canonical, ok := entityTypeAliases[entityType]; ok {
			entityType = canonical
		}

		if !allowedEntityTypes[entityType] {
			log.Warnf("filtering entity %q: invalid type %q", ent.Name, ent.Type)
			continue
		}

		ent.Type = entityType
		valid = append(valid, ent)
	}

	return valid
}

func buildPrompt(chunk string, knownEntities []string) string {
	prompt := extractionPrompt

	// Inject known entities section if available
	textSection := ""
	if len(knownEntities) > 0 {
		textSection = "KNOWN ENTITIES (use these exact names if they appear in the text):\n"
		for _, name := range knownEntities {
			textSection += "- " + name + "\n"
		}
		textSection += "\n"
	}
	today := time.Now().UTC().Format("2006-01-02")
	prompt = strings.Replace(prompt, "{today}", today, 1)
	prompt = strings.Replace(prompt, "{text_section}", textSection, 1)
	prompt = strings.Replace(prompt, "{text}", chunk, 1)
	return prompt
}

const extractionPrompt = `You are a knowledge graph extraction engine. Extract entities, relationships, and facts from text into structured JSON.

Today's date: {today}

CRITICAL RULES FOR ENTITY NAMES:
- Use SHORT, CLEAN names only. Never put descriptions in names.
  GOOD: "PostgreSQL", "DeerPrint", "Brian Colinger"
  BAD: "PostgreSQL — relational database", "DeerPrint — AI deer identification system"
- Use the FULL PROPER NAME of a person, not just first name. "Brian Colinger" not "Brian".
- Use the CANONICAL name of a project/product. "DeerPrint" not "DeerPrint Platform" or "DeerPrint Production".
- Do NOT create separate entities for aspects of the same thing:
  BAD: "DeerPrint API", "DeerPrint Frontend", "DeerPrint Database" (these are parts, not entities)
  GOOD: "DeerPrint" (one entity, with facts about its components)
- Service names and systemd units are NOT entities. "persistor.service" is just how Persistor runs.
- Only extract entities that are INDEPENDENTLY NOTABLE — something you would write a reference page about.

Output ONLY valid JSON:
{
  "entities": [
    {"name": "Short Name", "type": "person|project|company|technology|event|decision|concept|place|animal|service", "properties": {}, "description": "One sentence"}
  ],
  "relationships": [
    {"source": "Entity A", "target": "Entity B", "relation": "type", "confidence": 0.9, "date_start": "EDTF date or null", "date_end": "EDTF date or null", "is_current": true}
  ],
  "facts": [
    {"subject": "Entity Name", "property": "key", "value": "value"}
  ]
}

ENTITY NAME CONSISTENCY:
If the text mentions the same entity by different names or variations, pick ONE canonical name and use it everywhere. For example, if text says "Brian" and "Brian Colinger", always use "Brian Colinger".

ENTITY TYPES must be one of: person, project, company, technology, event, decision, concept, place, animal, service

RELATIONSHIP TYPES — use ONLY these:
created, founded, works_at, worked_at, works_on, leads, owns, part_of, product_of, deployed_on, runs_on, uses, depends_on, implements, extends, replaced_by, enables, supports, parent_of, child_of, sibling_of, married_to, friend_of, mentored, located_in, learned, decided, inspired, prefers, competes_with, acquired, funded, partners_with, affected_by, achieved, detected_in, experienced

TEMPORAL DATA ON RELATIONSHIPS:
Always extract dates when the text mentions when a relationship started, ended, or whether it is ongoing.

EDTF date format rules (use the most precise format the text supports):
  Exact date:       "2019-10-15"
  Month precision:  "2009-05"
  Year precision:   "1983"
  Approximate:      "~1983"   (use when text says "around", "circa", "roughly")
  Decade:           "199X"    (use when text says "in the 1990s" or "the nineties")
  Unknown:          ".."      (use when one bound is explicitly unknown)

Examples of temporal extraction:
  "worked at Acme from 2009 to 2022"         → date_start: "2009",    date_end: "2022",    is_current: false
  "married since 1983"                        → date_start: "1983",    date_end: null,      is_current: true
  "lives in Seattle (current)"               → date_start: null,      date_end: null,      is_current: true
  "joined Google in May 2012, still there"   → date_start: "2012-05", date_end: null,      is_current: true
  "CEO until 2019-10-15"                     → date_start: null,      date_end: "2019-10-15", is_current: false
  "grew up in London in the nineties"        → date_start: "199X",    date_end: "199X",    is_current: false
  "started around 1983"                      → date_start: "~1983",   date_end: null,      is_current: false

Rules:
- Set is_current: true when the relationship is described as ongoing/present/current/still active
- Set is_current: false when the relationship has ended
- Omit is_current entirely when temporal status is unknown
- If no temporal info exists, omit date_start, date_end, and is_current entirely (do not output null fields)
- Use today's date ({today}) to decide if a present-tense relationship is current

WHAT TO EXTRACT:
- Real people with names (not "the team" or "users")
- Named projects, products, companies, organizations
- Specific technologies, languages, frameworks (PostgreSQL, Go, PyTorch — not "the database")
- Named places (cities, states, properties — not "the office")
- Significant events with names or dates
- Concrete decisions with consequences
- Key facts: dates, amounts, measurements, versions, statuses

WHAT NOT TO EXTRACT:
- Generic nouns: "code", "the system", "the project", "production", "the database"
- Subcomponents of a named entity (use facts instead): "DeerPrint's API" → fact on DeerPrint
- Process descriptions: "deploying", "testing", "refactoring"
- Temporary states: "the bug", "the fix", "the PR"
- Obvious/trivial relationships: a project uses its own components

Maximum 10 entities. Quality over quantity. Every entity should be something worth remembering.
Confidence: 0.9+ for explicit statements, 0.7-0.85 for implied/inferred, skip if below 0.6.
Output ONLY the JSON object — no text before or after it.

{text_section}
Text:
---
{text}
---`
