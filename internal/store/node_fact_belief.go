package store

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/persistorai/persistor/internal/models"
)

const contestedConfidenceDelta = 0.15

type factClaimAggregate struct {
	value          any
	confidenceSum  float64
	evidenceCount  int
	sourceWeight   float64
	lastObservedAt string
	sources        map[string]struct{}
}

func buildFactBeliefs(props map[string]any, evidenceByProperty map[string][]models.FactEvidence) map[string]models.FactBeliefState {
	beliefs := make(map[string]models.FactBeliefState, len(evidenceByProperty))
	for key, entries := range evidenceByProperty {
		state := buildFactBeliefState(props[key], propertyExists(props, key), entries)
		if state.EvidenceCount == 0 {
			continue
		}
		beliefs[key] = state
	}
	if len(beliefs) == 0 {
		return nil
	}
	return beliefs
}

func buildFactBeliefState(currentValue any, hasCurrentValue bool, entries []models.FactEvidence) models.FactBeliefState {
	aggregates := make(map[string]*factClaimAggregate)
	for _, entry := range entries {
		key := canonicalFactValue(entry.Value)
		agg, ok := aggregates[key]
		if !ok {
			agg = &factClaimAggregate{value: entry.Value, sources: map[string]struct{}{}}
			aggregates[key] = agg
		}
		confidence := 1.0
		if entry.Confidence != nil {
			confidence = *entry.Confidence
		}
		weight := sourceWeight(entry.Source)
		agg.confidenceSum += confidence * weight
		agg.evidenceCount++
		agg.sourceWeight += weight
		if entry.Timestamp > agg.lastObservedAt {
			agg.lastObservedAt = entry.Timestamp
		}
		if entry.Source != "" {
			agg.sources[entry.Source] = struct{}{}
		}
	}

	claims := make([]models.FactBeliefClaim, 0, len(aggregates))
	for _, agg := range aggregates {
		claims = append(claims, models.FactBeliefClaim{
			Value:          agg.value,
			Confidence:     clamp01(agg.confidenceSum / float64(agg.evidenceCount)),
			EvidenceCount:  agg.evidenceCount,
			SourceWeight:   agg.sourceWeight / float64(agg.evidenceCount),
			LastObservedAt: agg.lastObservedAt,
			Sources:        sortedSources(agg.sources),
		})
	}

	sort.Slice(claims, func(i, j int) bool {
		if claims[i].Confidence != claims[j].Confidence {
			return claims[i].Confidence > claims[j].Confidence
		}
		if claims[i].EvidenceCount != claims[j].EvidenceCount {
			return claims[i].EvidenceCount > claims[j].EvidenceCount
		}
		if currentMatchesClaim(hasCurrentValue, currentValue, claims[i].Value) != currentMatchesClaim(hasCurrentValue, currentValue, claims[j].Value) {
			return currentMatchesClaim(hasCurrentValue, currentValue, claims[i].Value)
		}
		if claims[i].LastObservedAt != claims[j].LastObservedAt {
			return claims[i].LastObservedAt > claims[j].LastObservedAt
		}
		return canonicalFactValue(claims[i].Value) < canonicalFactValue(claims[j].Value)
	})

	preferredIdx := 0
	if hasCurrentValue {
		for i, claim := range claims {
			if propertyValuesEqual(currentValue, claim.Value) {
				preferredIdx = i
				break
			}
		}
	}
	claims[0], claims[preferredIdx] = claims[preferredIdx], claims[0]

	state := models.FactBeliefState{EvidenceCount: len(entries), Claims: claims}
	preferred := &state.Claims[0]
	preferred.Preferred = true
	state.PreferredValue = preferred.Value
	state.PreferredConfidence = preferred.Confidence
	state.Status = models.FactBeliefStatusSupported

	if !hasCurrentValue {
		state.Status = models.FactBeliefStatusSuperseded
	}
	if len(state.Claims) > 1 {
		runnerUp := state.Claims[1]
		if hasCurrentValue && preferred.Confidence-runnerUp.Confidence < contestedConfidenceDelta {
			state.Status = models.FactBeliefStatusContested
		}
	}
	for i := range state.Claims {
		claim := &state.Claims[i]
		switch {
		case i == 0 && hasCurrentValue:
			claim.Status = state.Status
		case state.Status == models.FactBeliefStatusContested && i <= 1:
			claim.Status = models.FactBeliefStatusContested
		default:
			claim.Status = models.FactBeliefStatusSuperseded
		}
	}
	return state
}

func extractFactBeliefs(props map[string]any) (map[string]models.FactBeliefState, error) {
	raw, ok := props[models.FactBeliefsProperty]
	if !ok || raw == nil {
		return map[string]models.FactBeliefState{}, nil
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("marshalling fact beliefs: %w", err)
	}
	var beliefs map[string]models.FactBeliefState
	if err := json.Unmarshal(data, &beliefs); err != nil {
		return nil, fmt.Errorf("unmarshalling fact beliefs: %w", err)
	}
	if beliefs == nil {
		return map[string]models.FactBeliefState{}, nil
	}
	return beliefs, nil
}

func propertyExists(props map[string]any, key string) bool {
	_, ok := props[key]
	return ok
}

func currentMatchesClaim(hasCurrentValue bool, currentValue, claimValue any) bool {
	return hasCurrentValue && propertyValuesEqual(currentValue, claimValue)
}

func canonicalFactValue(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(data)
}

func sortedSources(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	sources := make([]string, 0, len(set))
	for source := range set {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

func sourceWeight(source string) float64 {
	if source == "" {
		return 1.0
	}
	return 1.0
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
