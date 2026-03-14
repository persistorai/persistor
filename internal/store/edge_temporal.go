package store

import (
	"fmt"
	"time"

	"github.com/persistorai/persistor/internal/edtf"
)

// parseTemporalBounds parses EDTF date strings and returns computed lower/upper bounds and qualifier.
// Returns nils for all when both inputs are nil.
func parseTemporalBounds(dateStart, dateEnd *string) (lower, upper *time.Time, qualifier *string, err error) {
	var startDate, endDate *edtf.Date

	if dateStart != nil {
		startDate, err = edtf.Parse(*dateStart)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("date_start: %w", err)
		}
	}

	if dateEnd != nil {
		endDate, err = edtf.Parse(*dateEnd)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("date_end: %w", err)
		}
	}

	if startDate != nil {
		lower = startDate.Lower
	}

	if endDate != nil {
		upper = endDate.Upper
	}

	q := deriveQualifier(startDate, endDate)
	if q != "" {
		qualifier = &q
	}

	return lower, upper, qualifier, nil
}

// deriveQualifier selects a date_qualifier from parsed EDTF dates.
func deriveQualifier(start, end *edtf.Date) string {
	switch {
	case start != nil && end != nil:
		return "range"
	case start != nil:
		return start.Qualifier
	case end != nil:
		return end.Qualifier
	default:
		return ""
	}
}
