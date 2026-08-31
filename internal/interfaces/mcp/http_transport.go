// Package mcp provides HTTP transport adapter using mcp-go's built-in SSE server.
package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/server"
)

// MCPServerHandler wraps MCP SSE server for HTTP transport.
type MCPServerHandler struct {
	sseServer *server.SSEServer
}

// NewMCPServerHandler creates a new MCP HTTP handler using SSE transport.
func NewMCPServerHandler(mcpServer server.MCPServer, addr string) *MCPServerHandler {
	return &MCPServerHandler{
		sseServer: server.NewSSEServer(mcpServer, "http://"+addr),
	}
}

// Start starts the HTTP SSE server.
func (h *MCPServerHandler) Start(addr string) error {
	return h.sseServer.Start(addr)
}

// Shutdown gracefully shuts down the server.
func (h *MCPServerHandler) Shutdown(ctx context.Context) error {
	return h.sseServer.Shutdown(ctx)
}