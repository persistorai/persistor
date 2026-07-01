package mcpengine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer builds the Persistor MCP server over the engine and registers the
// memory tools. persistor-server wraps it in the Streamable HTTP transport —
// the only transport; there is no stdio binary.
func NewServer(e *Engine, version string) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "persistor", Title: "Persistor Memory", Version: version},
		&mcp.ServerOptions{Instructions: serverInstructions},
	)
	registerTools(server, e)
	return server
}

// serverInstructions is the orientation paragraph MCP clients show the model
// alongside the tool list.
const serverInstructions = "This is the user's long-term memory (Persistor): durable notes the " +
	"agent itself wrote — people, projects, decisions, preferences, events — that outlive any single " +
	"conversation. Search it when the user references something you lack context for, read full notes " +
	"with memory_get, and save genuinely durable facts back with memory_write (set `supersedes` only when " +
	"correcting a stale fact, never for a new point in a timeline). Treat results as the user's own memory."

const (
	searchDescription = "Search the user's long-term prose memory (full-text over the agent's own notes). " +
		"Use this FIRST whenever the user references people, projects, decisions, or past events you lack " +
		"context for. Returns ranked note summaries; superseded notes are excluded unless you ask for them. " +
		"Do NOT use it for information already in this conversation or for general world knowledge."
	getDescription = "Fetch one note's full prose body by id (as returned by memory_search). Returns the " +
		"note's current `version` — pass it back as `expected_version` to safely update or delete the note."
	listDescription = "List the user's stored notes as summaries (id, namespace, title, kind, tier, version) " +
		"WITHOUT bodies — the way to BROWSE or page memory when you don't have a search term. Optionally filter " +
		"to one `namespace` and page with `limit`/`offset`. Fetch a full body with memory_get. Superseded notes " +
		"are excluded unless you ask for them."
	namespacesDescription = "List the namespaces the user's memory is organized into, each with its note count " +
		"— the top-level map of where memory lives (e.g. demo, claude, work). Use it to discover namespaces " +
		"before listing or searching within one."
	writeDescription = "Save a durable note in prose. Provide the note body and an id (or a .md path it is " +
		"derived from). Omit `expected_version` to CREATE; to UPDATE an existing note pass its current " +
		"`version` (from memory_get) as `expected_version`. Set `supersedes` to the id of a note this " +
		"CORRECTS (kept as history but hidden from default retrieval); use a new note without `supersedes` " +
		"for a new point in a timeline."
	deleteDescription = "Tombstone a note by id so it leaves retrieval. History is preserved — memory_restore " +
		"can undo it. Pass the note's current `version` (from memory_get) as `expected_version`."
	restoreDescription = "Undo a delete or a bad overwrite by copying a prior version's content forward. Omit " +
		"`target_version` for the most recent non-delete version. Pass the current `version` as `expected_version`."
	briefDescription = "Assemble the bounded memory working-set: every always-loaded Core note plus the Tail " +
		"notes most relevant to the given seed/topic, under a token budget. Returns a markdown block."
)

// textResult serializes a tool's output value into a single JSON text content
// block and returns it as the tool result.
//
// Persistor deliberately does NOT declare output schemas or return
// structuredContent. Structured tool output is a newer MCP feature (2025-06-18)
// with uneven client support — the claude.ai web connector (BETA) errors on
// tool results that carry structuredContent / a declared outputSchema, even
// though the response is otherwise spec-correct. A prose/JSON memory tool loses
// nothing by returning its payload as text: every MCP client since the original
// spec understands a text content block, and broad client compatibility is the
// whole point of Persistor ("one memory across many LLMs"). Input schemas are
// still generated (from the typed In on each AddTool), which is what clients
// need to CALL the tools correctly.
//
// The any return is required by the SDK: mcp.AddTool only suppresses
// outputSchema generation when the output type parameter is exactly any.
func textResult(v any) (*mcp.CallToolResult, any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling tool output: %w", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: asciiSafeJSON(b)}},
	}, nil, nil
}

// asciiSafeJSON re-encodes already-valid JSON so every non-ASCII rune becomes a
// \uXXXX escape, yielding pure-ASCII JSON. This is a lossless transform — it
// decodes back to the identical value — but it sidesteps clients that mishandle
// raw multibyte UTF-8 in a tool result (the claude.ai web connector chokes on
// note bodies containing emoji / variation selectors like U+FE0F, even though
// the response is valid UTF-8 JSON). ASCII-safe JSON is also broadly the most
// portable wire form across heterogeneous MCP clients. Non-ASCII bytes only ever
// occur inside JSON string literals (all structural tokens are ASCII), so
// escaping runes in place keeps the document valid.
func asciiSafeJSON(b []byte) string {
	var out strings.Builder
	out.Grow(len(b))
	for _, r := range string(b) {
		switch {
		case r < 0x80:
			out.WriteByte(byte(r))
		case r > 0xFFFF:
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&out, "\\u%04x\\u%04x", hi, lo)
		default:
			fmt.Fprintf(&out, "\\u%04x", r)
		}
	}
	return out.String()
}

// registerTools adds the memory tools to the server. Each tool keeps its typed
// input (so the SDK generates an input schema) but returns a text result via
// textResult (no output schema, no structuredContent) for maximum client
// compatibility — see textResult.
func registerTools(server *mcp.Server, e *Engine) {
	mcp.AddTool(server, &mcp.Tool{Name: "memory_search", Description: searchDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, any, error) {
			out, err := e.Search(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_get", Description: getDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in GetInput) (*mcp.CallToolResult, any, error) {
			out, err := e.Get(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_list", Description: listDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ListInput) (*mcp.CallToolResult, any, error) {
			out, err := e.List(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_namespaces", Description: namespacesDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ NamespacesInput) (*mcp.CallToolResult, any, error) {
			out, err := e.Namespaces(ctx)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_write", Description: writeDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in WriteInput) (*mcp.CallToolResult, any, error) {
			out, err := e.Write(ctx, &in)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_delete", Description: deleteDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in DeleteInput) (*mcp.CallToolResult, any, error) {
			out, err := e.Delete(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_restore", Description: restoreDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in RestoreInput) (*mcp.CallToolResult, any, error) {
			out, err := e.Restore(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out)
		})
	mcp.AddTool(server, &mcp.Tool{Name: "brief", Description: briefDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in BriefInput) (*mcp.CallToolResult, any, error) {
			out, err := e.Brief(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return textResult(out)
		})
}
