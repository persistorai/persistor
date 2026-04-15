package ingest

import (
	"context"
	"fmt"
)

// ExtractWithRetry retries once with a smaller prompt on parse failure.
func (e *Extractor) ExtractWithRetry(ctx context.Context, chunk string, knownEntities ...string) (*ExtractionResult, error) {
	result, err := e.Extract(ctx, chunk, knownEntities...)
	if err == nil {
		return result, nil
	}
	if !isParseError(err) {
		return nil, err
	}

	fallbackPrompt := "Return only compact valid JSON with minimal entities, relationships, and facts. Prefer fewer items over speculative ones. Text:\n\n" + chunk
	raw, chatErr := e.llm.Chat(ctx, fallbackPrompt)
	if chatErr != nil {
		return nil, err
	}
	parsed, parseErr := e.parseResponse(raw)
	if parseErr != nil {
		return nil, fmt.Errorf("retry parse failed after initial error %v: %w", err, parseErr)
	}
	return parsed, nil
}

func isParseError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg,
		"parsing extraction result",
		"invalid character",
		"unexpected end of JSON input",
	)
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && len(s) >= len(needle) && containsString(s, needle) {
			return true
		}
	}
	return false
}

func containsString(s, needle string) bool {
	for i := 0; i+len(needle) <= len(s); i++ {
		if s[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
