package edtf_test

import (
	"testing"
	"time"

	"github.com/persistorai/persistor/internal/edtf"
)

func datePtr(year, month, day int) *time.Time {
	t := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return &t
}

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantNil   bool
		wantLower *time.Time
		wantUpper *time.Time
		wantQual  string
		wantErr   bool
	}{
		{
			name:    "empty string returns nil",
			input:   "",
			wantNil: true,
		},
		{
			name:      "exact date",
			input:     "2006-01-21",
			wantLower: datePtr(2006, 1, 21),
			wantUpper: datePtr(2006, 1, 21),
			wantQual:  "exact",
		},
		{
			name:      "year-month",
			input:     "2006-01",
			wantLower: datePtr(2006, 1, 1),
			wantUpper: datePtr(2006, 1, 31),
			wantQual:  "exact",
		},
		{
			name:      "year only",
			input:     "2006",
			wantLower: datePtr(2006, 1, 1),
			wantUpper: datePtr(2006, 12, 31),
			wantQual:  "exact",
		},
		{
			name:      "approximate year",
			input:     "~2006",
			wantLower: datePtr(2005, 1, 1),
			wantUpper: datePtr(2007, 12, 31),
			wantQual:  "approximate",
		},
		{
			name:      "uncertain year",
			input:     "2006?",
			wantLower: datePtr(2005, 1, 1),
			wantUpper: datePtr(2007, 12, 31),
			wantQual:  "estimated",
		},
		{
			name:      "approximate and uncertain year",
			input:     "~2006?",
			wantLower: datePtr(2004, 1, 1),
			wantUpper: datePtr(2008, 12, 31),
			wantQual:  "estimated",
		},
		{
			name:      "decade",
			input:     "198x",
			wantLower: datePtr(1980, 1, 1),
			wantUpper: datePtr(1989, 12, 31),
			wantQual:  "decade",
		},
		{
			name:      "open-ended start",
			input:     "2009/..",
			wantLower: datePtr(2009, 1, 1),
			wantUpper: nil,
			wantQual:  "range",
		},
		{
			name:      "open-ended end",
			input:     "../2022",
			wantLower: nil,
			wantUpper: datePtr(2022, 12, 31),
			wantQual:  "range",
		},
		{
			name:      "range",
			input:     "2009/2022",
			wantLower: datePtr(2009, 1, 1),
			wantUpper: datePtr(2022, 12, 31),
			wantQual:  "range",
		},
		{
			name:      "approximate range",
			input:     "~1989/~1992",
			wantLower: datePtr(1988, 1, 1),
			wantUpper: datePtr(1993, 12, 31),
			wantQual:  "range",
		},
		{
			name:      "range with full dates",
			input:     "2009-03-15/2022-11-30",
			wantLower: datePtr(2009, 3, 15),
			wantUpper: datePtr(2022, 11, 30),
			wantQual:  "range",
		},
		{
			name:      "february last day leap year",
			input:     "2024-02",
			wantLower: datePtr(2024, 2, 1),
			wantUpper: datePtr(2024, 2, 29),
			wantQual:  "exact",
		},
		{
			name:      "february last day non-leap year",
			input:     "2023-02",
			wantLower: datePtr(2023, 2, 1),
			wantUpper: datePtr(2023, 2, 28),
			wantQual:  "exact",
		},
		{
			name:      "raw field preserved for qualified",
			input:     "~2006?",
			wantLower: datePtr(2004, 1, 1),
			wantUpper: datePtr(2008, 12, 31),
			wantQual:  "estimated",
		},
		// Error cases
		{
			name:    "invalid format - too short",
			input:   "abc",
			wantErr: true,
		},
		{
			name:    "invalid year - non-numeric",
			input:   "abcd",
			wantErr: true,
		},
		{
			name:    "invalid date - bad month",
			input:   "2006-13-01",
			wantErr: true,
		},
		{
			name:    "invalid year-month",
			input:   "2006-99",
			wantErr: true,
		},
		{
			name:    "invalid decade - non-numeric prefix",
			input:   "19xx",
			wantErr: true,
		},
		{
			name:    "invalid decade - wrong length",
			input:   "20x",
			wantErr: true,
		},
		{
			name:    "invalid range - bad start date",
			input:   "notadate/2022",
			wantErr: true,
		},
		{
			name:    "invalid range - bad end date",
			input:   "2009/notadate",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := edtf.Parse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Parse(%q) expected error, got nil (result: %+v)", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("Parse(%q) = %+v, want nil", tt.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("Parse(%q) = nil, want non-nil Date", tt.input)
			}
			if got.Qualifier != tt.wantQual {
				t.Errorf("Parse(%q).Qualifier = %q, want %q", tt.input, got.Qualifier, tt.wantQual)
			}
			assertTimePtr(t, tt.input, "Lower", got.Lower, tt.wantLower)
			assertTimePtr(t, tt.input, "Upper", got.Upper, tt.wantUpper)
		})
	}
}

func TestParseRaw(t *testing.T) {
	inputs := []string{"2006-01-21", "~2006", "2006?", "~2006?", "198x", "2009/2022", "2009/..", "../2022"}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			got, err := edtf.Parse(input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Raw != input {
				t.Errorf("Raw = %q, want %q", got.Raw, input)
			}
		})
	}
}

func assertTimePtr(t *testing.T, input, field string, got, want *time.Time) {
	t.Helper()
	if want == nil {
		if got != nil {
			t.Errorf("Parse(%q).%s = %v, want nil", input, field, got)
		}
		return
	}
	if got == nil {
		t.Errorf("Parse(%q).%s = nil, want %v", input, field, want)
		return
	}
	if !got.Equal(*want) {
		t.Errorf("Parse(%q).%s = %v, want %v", input, field, *got, *want)
	}
}
