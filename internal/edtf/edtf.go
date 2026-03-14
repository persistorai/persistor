package edtf

import (
	"fmt"
	"strings"
	"time"
)

// Date represents a parsed EDTF date with computed bounds.
type Date struct {
	Raw       string     // Original EDTF string
	Lower     *time.Time // Computed lower bound (inclusive)
	Upper     *time.Time // Computed upper bound (inclusive)
	Qualifier string     // exact, approximate, estimated, before, after, range, decade, unknown
}

// Parse parses an EDTF date string and computes bounds.
func Parse(s string) (*Date, error) {
	if s == "" {
		return nil, nil
	}
	if strings.Contains(s, "/") {
		return parseRange(s)
	}
	if strings.HasSuffix(s, "x") {
		return parseDecade(s)
	}
	return parseQualified(s)
}

func parseRange(s string) (*Date, error) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("edtf: invalid range %q", s)
	}

	var lower, upper *time.Time

	if parts[0] != ".." && parts[0] != "" {
		d, err := parseQualified(parts[0])
		if err != nil {
			return nil, err
		}
		lower = d.Lower
	}

	if parts[1] != ".." && parts[1] != "" {
		d, err := parseQualified(parts[1])
		if err != nil {
			return nil, err
		}
		upper = d.Upper
	}

	return &Date{Raw: s, Lower: lower, Upper: upper, Qualifier: "range"}, nil
}

func parseQualified(s string) (*Date, error) {
	raw := s
	approx := strings.HasPrefix(s, "~")
	s = strings.TrimPrefix(s, "~")
	uncertain := strings.HasSuffix(s, "?")
	s = strings.TrimSuffix(s, "?")

	lo, hi, err := dateBounds(s)
	if err != nil {
		return nil, fmt.Errorf("edtf: %w", err)
	}

	qualifier, spread := qualifierAndSpread(approx, uncertain)
	if spread > 0 {
		lo = lo.AddDate(-spread, 0, 0)
		hi = hi.AddDate(spread, 0, 0)
	}

	return &Date{Raw: raw, Lower: &lo, Upper: &hi, Qualifier: qualifier}, nil
}

func qualifierAndSpread(approx, uncertain bool) (qualifier string, spread int) {
	switch {
	case approx && uncertain:
		return "estimated", 2
	case approx:
		return "approximate", 1
	case uncertain:
		return "estimated", 1
	default:
		return "exact", 0
	}
}

func parseDecade(s string) (*Date, error) {
	if len(s) != 4 || s[3] != 'x' {
		return nil, fmt.Errorf("edtf: invalid decade %q", s)
	}
	yearStr := s[:3] + "0"
	t, err := time.Parse("2006", yearStr)
	if err != nil {
		return nil, fmt.Errorf("edtf: parsing decade %q: %w", s, err)
	}
	lo := t
	hi := time.Date(t.Year()+9, 12, 31, 0, 0, 0, 0, time.UTC)
	return &Date{Raw: s, Lower: &lo, Upper: &hi, Qualifier: "decade"}, nil
}

func dateBounds(s string) (lo, hi time.Time, err error) {
	switch len(s) {
	case 10:
		return parseFullDate(s)
	case 7:
		return parseYearMonth(s)
	case 4:
		return parseYear(s)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unrecognized date format %q", s)
	}
}

func parseFullDate(s string) (lo, hi time.Time, err error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parsing date %q: %w", s, err)
	}
	return t, t, nil
}

func parseYearMonth(s string) (lo, hi time.Time, err error) {
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parsing year-month %q: %w", s, err)
	}
	upper := lastDayOfMonth(t)
	return t, upper, nil
}

func parseYear(s string) (lo, hi time.Time, err error) {
	t, err := time.Parse("2006", s)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("parsing year %q: %w", s, err)
	}
	upper := time.Date(t.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
	return t, upper, nil
}

func lastDayOfMonth(t time.Time) time.Time {
	first := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return first.AddDate(0, 0, -1)
}
