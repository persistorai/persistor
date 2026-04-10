package store

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/persistorai/persistor/internal/models"
)

func applyFactConsolidation(existing, patch map[string]any) (map[string]any, map[string]any, error) {
	factUpdates, plainPatch, err := splitFactUpdates(patch)
	if err != nil {
		return nil, nil, err
	}

	merged := copyProperties(existing)
	merged = models.MergeProperties(merged, plainPatch)
	if len(factUpdates) == 0 {
		return merged, filterHistoryProperties(merged), nil
	}

	evidenceByProperty, err := extractFactEvidence(merged)
	if err != nil {
		return nil, nil, err
	}

	for key, update := range factUpdates {
		oldVal, existed := merged[key]
		conflicts := existed && !propertyValuesEqual(oldVal, update.Value)
		merged[key] = update.Value
		evidenceByProperty[key] = append(evidenceByProperty[key], models.FactEvidence{
			Value:                      update.Value,
			Source:                     update.Source,
			Timestamp:                  factTimestamp(update.Timestamp),
			Confidence:                 update.Confidence,
			ConflictsWithPrior:         conflicts,
			SupersedesPrior:            conflicts,
			PreviousValue:              priorValue(conflicts, oldVal),
			HistoricalEvidenceRetained: conflicts,
		})
	}

	if len(evidenceByProperty) > 0 {
		merged[models.FactEvidenceProperty] = evidenceByProperty
	}
	if beliefs := buildFactBeliefs(merged, evidenceByProperty); len(beliefs) > 0 {
		merged[models.FactBeliefsProperty] = beliefs
	} else {
		delete(merged, models.FactBeliefsProperty)
	}

	return merged, filterHistoryProperties(merged), nil
}

func splitFactUpdates(patch map[string]any) (map[string]models.FactUpdate, map[string]any, error) {
	plain := make(map[string]any, len(patch))
	for k, v := range patch {
		if k != models.FactUpdatesProperty {
			plain[k] = v
		}
	}

	raw, ok := patch[models.FactUpdatesProperty]
	if !ok || raw == nil {
		return nil, plain, nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("marshalling fact updates: %w", err)
	}

	updates := map[string]models.FactUpdate{}
	if err := json.Unmarshal(data, &updates); err != nil {
		return nil, nil, fmt.Errorf("unmarshalling fact updates: %w", err)
	}
	return updates, plain, nil
}

func extractFactEvidence(props map[string]any) (map[string][]models.FactEvidence, error) {
	raw, ok := props[models.FactEvidenceProperty]
	if !ok || raw == nil {
		return map[string][]models.FactEvidence{}, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshalling fact evidence: %w", err)
	}
	var evidence map[string][]models.FactEvidence
	if err := json.Unmarshal(data, &evidence); err != nil {
		return nil, fmt.Errorf("unmarshalling fact evidence: %w", err)
	}
	if evidence == nil {
		return map[string][]models.FactEvidence{}, nil
	}
	return evidence, nil
}

func filterHistoryProperties(props map[string]any) map[string]any {
	filtered := make(map[string]any, len(props))
	for k, v := range props {
		if k == models.FactEvidenceProperty || k == models.FactUpdatesProperty || k == models.FactBeliefsProperty {
			continue
		}
		filtered[k] = v
	}
	return filtered
}

func copyProperties(props map[string]any) map[string]any {
	if props == nil {
		return map[string]any{}
	}
	copied := make(map[string]any, len(props))
	for k, v := range props {
		copied[k] = v
	}
	return copied
}

func propertyValuesEqual(a, b any) bool {
	left, err := json.Marshal(a)
	if err != nil {
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
	right, err := json.Marshal(b)
	if err != nil {
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}
	return string(left) == string(right)
}

func factTimestamp(ts string) string {
	if ts != "" {
		return ts
	}
	return time.Now().UTC().Format(time.RFC3339)
}

func priorValue(conflicts bool, value any) any {
	if !conflicts {
		return nil
	}
	return value
}
