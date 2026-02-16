package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func formatJSON(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "Error: encode json: %v\n", err)
		os.Exit(1)
	}
}

func formatTable(headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	printRow := func(cells []string) {
		parts := make([]string, len(cells))
		for i, cell := range cells {
			w := 0
			if i < len(widths) {
				w = widths[i]
			}
			parts[i] = fmt.Sprintf("%-*s", w, cell)
		}
		fmt.Println(strings.Join(parts, "  "))
	}

	printRow(headers)
	seps := make([]string, len(headers))
	for i, w := range widths {
		seps[i] = strings.Repeat("-", w)
		_ = i
	}
	printRow(seps)
	for _, row := range rows {
		printRow(row)
	}
}

func formatQuiet(id string) {
	fmt.Println(id)
}

func output(v any, quietVal string) {
	switch flagFmt {
	case "quiet":
		formatQuiet(quietVal)
	case "table":
		// Table requires caller to use formatTable directly.
		// Fallback to JSON for generic output.
		formatJSON(v)
	default:
		formatJSON(v)
	}
}
