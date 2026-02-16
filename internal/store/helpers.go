package store

import (
	"strconv"
	"strings"
)

// maxListLimit is a defense-in-depth cap on limit values for list queries.
const maxListLimit = 1000

// formatEmbedding converts a float32 slice to the pgvector string format "[0.1,0.2,...]".
func formatEmbedding(embedding []float32) string {
	var b strings.Builder
	b.Grow(len(embedding)*8 + 2)
	b.WriteByte('[')

	for i, v := range embedding {
		if i > 0 {
			b.WriteByte(',')
		}

		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}

	b.WriteByte(']')

	return b.String()
}
