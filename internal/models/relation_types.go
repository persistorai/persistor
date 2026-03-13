package models

import (
	"fmt"
	"sort"
	"sync"
)

// RelationType represents a named relation type with a human-readable description.
type RelationType struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// canonicalRelationTypes holds the built-in relation types that ship with Persistor.
var canonicalRelationTypes = map[string]string{
	"created":       "A built/founded/authored B",
	"founded":       "A founded/established B",
	"works_at":      "A is employed at B",
	"worked_at":     "A was formerly employed at B",
	"works_on":      "A is actively working on B",
	"leads":         "A leads/manages B",
	"owns":          "A owns/possesses B",
	"part_of":       "A is part of B",
	"product_of":    "A is a product of B",
	"deployed_on":   "A is deployed on B",
	"runs_on":       "A runs on B",
	"uses":          "A uses/utilizes B",
	"depends_on":    "A depends on B",
	"implements":    "A implements B",
	"extends":       "A extends/inherits from B",
	"replaced_by":   "A was replaced by B",
	"enables":       "A enables/powers B",
	"supports":      "A supports B",
	"parent_of":     "A is the parent of B",
	"child_of":      "A is the child of B",
	"sibling_of":    "A is a sibling of B",
	"married_to":    "A is married to B",
	"friend_of":     "A is a friend of B",
	"mentored":      "A mentored B",
	"located_in":    "A is located in B",
	"learned":       "A learned B",
	"decided":       "A decided B",
	"inspired":      "A was inspired by B",
	"prefers":       "A prefers B",
	"competes_with": "A competes with B",
	"acquired":      "A acquired B",
	"funded":        "A funded B",
	"partners_with": "A partners with B",
	"affected_by":   "A was affected by B",
	"achieved":      "A achieved B",
	"detected_in":   "A was detected in B",
	"experienced":   "A experienced B",
}

// mu guards customRelationTypes for concurrent access.
var mu sync.RWMutex

// customRelationTypes holds runtime-added relation types (not persisted to DB).
var customRelationTypes = map[string]string{}

// IsCanonicalRelation reports whether the given relation name is a canonical type.
func IsCanonicalRelation(rel string) bool {
	_, ok := canonicalRelationTypes[rel]
	return ok
}

// ListRelationTypes returns all known relation types (canonical + runtime custom),
// sorted alphabetically by name.
func ListRelationTypes() []RelationType {
	mu.RLock()
	defer mu.RUnlock()

	types := make([]RelationType, 0, len(canonicalRelationTypes)+len(customRelationTypes))

	for name, desc := range canonicalRelationTypes {
		types = append(types, RelationType{Name: name, Description: desc})
	}

	for name, desc := range customRelationTypes {
		types = append(types, RelationType{Name: name, Description: desc})
	}

	sort.Slice(types, func(i, j int) bool {
		return types[i].Name < types[j].Name
	})

	return types
}

// AddRelationType adds a new relation type to the runtime registry.
// Returns an error if name or description is empty, or if the type already exists.
func AddRelationType(name, description string) error {
	if name == "" {
		return fmt.Errorf("relation type name: %w", ErrMissingLabel)
	}

	if description == "" {
		return fmt.Errorf("relation type description: %w", ErrMissingLabel)
	}

	mu.Lock()
	defer mu.Unlock()

	if _, ok := canonicalRelationTypes[name]; ok {
		return fmt.Errorf("relation type %q: %w", name, ErrDuplicateKey)
	}

	if _, ok := customRelationTypes[name]; ok {
		return fmt.Errorf("relation type %q: %w", name, ErrDuplicateKey)
	}

	customRelationTypes[name] = description

	return nil
}

// ResetCustomRelationTypes clears runtime-added types. Exported for testing.
func ResetCustomRelationTypes() {
	mu.Lock()
	defer mu.Unlock()

	customRelationTypes = map[string]string{}
}
