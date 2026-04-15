package ingest

import "strings"

var companySuffixes = []string{
	", inc.",
	" inc.",
	", inc",
	" inc",
	", llc",
	" llc",
	", ltd.",
	" ltd.",
	", ltd",
	" ltd",
	", corp.",
	" corp.",
	", corporation",
	" corporation",
}

func normalizeLabelVariant(name string) string {
	trimmed := strings.TrimSpace(name)
	lower := strings.ToLower(trimmed)
	for _, suffix := range companySuffixes {
		if strings.HasSuffix(lower, suffix) {
			base := strings.TrimSpace(trimmed[:len(trimmed)-len(suffix)])
			if base != "" {
				return base
			}
		}
	}
	return trimmed
}
