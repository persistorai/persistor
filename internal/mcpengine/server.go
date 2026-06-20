package mcpengine

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewServer builds the Persistor MCP server over the engine and registers the
// memory tools. Both transports (stdio and Streamable HTTP) wrap the same
// server, so the tool surface stays identical across them.
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

// registerTools adds the memory tools to the server.
func registerTools(server *mcp.Server, e *Engine) {
	mcp.AddTool(server, &mcp.Tool{Name: "memory_search", Description: searchDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, SearchOutput, error) {
			out, err := e.Search(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_get", Description: getDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in GetInput) (*mcp.CallToolResult, GetOutput, error) {
			out, err := e.Get(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_list", Description: listDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ListInput) (*mcp.CallToolResult, ListOutput, error) {
			out, err := e.List(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_namespaces", Description: namespacesDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, _ NamespacesInput) (*mcp.CallToolResult, NamespacesOutput, error) {
			out, err := e.Namespaces(ctx)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_write", Description: writeDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in WriteInput) (*mcp.CallToolResult, WriteOutput, error) {
			out, err := e.Write(ctx, &in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_delete", Description: deleteDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in DeleteInput) (*mcp.CallToolResult, MutationOutput, error) {
			out, err := e.Delete(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "memory_restore", Description: restoreDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in RestoreInput) (*mcp.CallToolResult, MutationOutput, error) {
			out, err := e.Restore(ctx, in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "brief", Description: briefDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in BriefInput) (*mcp.CallToolResult, BriefOutput, error) {
			out, err := e.Brief(ctx, in)
			return nil, out, err
		})
}
