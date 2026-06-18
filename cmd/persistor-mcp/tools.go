package main

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	getDescription   = "Fetch one note's full prose body by id (as returned by memory_search)."
	writeDescription = "Save a durable note in prose. Provide a relative .md path and the note body; set " +
		"`supersedes` to the id of a note this CORRECTS (the old note is kept as history but hidden from " +
		"default retrieval). Use a new note without `supersedes` for a new point in a timeline."
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
	mcp.AddTool(server, &mcp.Tool{Name: "memory_write", Description: writeDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in WriteInput) (*mcp.CallToolResult, WriteOutput, error) {
			out, err := e.Write(ctx, &in)
			return nil, out, err
		})
	mcp.AddTool(server, &mcp.Tool{Name: "brief", Description: briefDescription},
		func(ctx context.Context, _ *mcp.CallToolRequest, in BriefInput) (*mcp.CallToolResult, BriefOutput, error) {
			out, err := e.Brief(ctx, in)
			return nil, out, err
		})
}
