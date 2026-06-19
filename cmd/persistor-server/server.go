package main

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newMux builds the HTTP routing for the remote MCP daemon: the go-sdk
// Streamable HTTP transport at /mcp and a liveness probe at /healthz. The MCP
// server is shared across requests (one engine, one tenant in this phase); auth
// arrives in P2 as middleware in front of this mux, so identity never comes from
// the network path.
func newMux(server *mcp.Server) *http.ServeMux {
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server }, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", mcpHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}
