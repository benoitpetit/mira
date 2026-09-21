// Package mcp provides the stateless Streamable HTTP transport adapter.
package mcp

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/server"
)

// MCPServerHandler exposes JSON-RPC requests at POST /mcp.
type MCPServerHandler struct {
	mcpServer server.MCPServer
	authToken string
	srv       *http.Server
}

func NewMCPServerHandler(mcpServer server.MCPServer, _ string) *MCPServerHandler {
	return NewMCPServerHandlerWithAuth(mcpServer, "")
}

// NewMCPServerHandlerWithAuth creates a stateless HTTP transport. When a token
// is configured, every request must provide Authorization: Bearer <token>.
func NewMCPServerHandlerWithAuth(mcpServer server.MCPServer, authToken string) *MCPServerHandler {
	return &MCPServerHandler{mcpServer: mcpServer, authToken: authToken}
}

func (h *MCPServerHandler) Start(addr string) error {
	h.srv = &http.Server{
		Addr:         addr,
		Handler:      h,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return h.srv.ListenAndServe()
}

func (h *MCPServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.authToken != "" && !authorizedBearer(r, h.authToken) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="mira-mcp"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var request server.JSONRPCRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&request); err != nil {
		writeHTTPRPCError(w, nil, http.StatusBadRequest, -32700, "Parse error")
		return
	}
	if request.JSONRPC != "2.0" || request.Method == "" {
		writeHTTPRPCError(w, request.ID, http.StatusBadRequest, -32600, "Invalid Request")
		return
	}

	result, err := h.mcpServer.Request(r.Context(), request.Method, request.Params)
	if err != nil {
		code := -32603
		if strings.HasPrefix(err.Error(), "method not found:") {
			code = -32601
		}
		writeHTTPRPCError(w, request.ID, http.StatusOK, code, err.Error())
		return
	}

	if request.ID == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(server.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      request.ID,
		Result:  result,
	})
}

func authorizedBearer(r *http.Request, expected string) bool {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) < len(prefix) || header[:len(prefix)] != prefix {
		return false
	}
	got := header[len(prefix):]
	if len(got) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(expected)) == 1
}

func writeHTTPRPCError(w http.ResponseWriter, id interface{}, status, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	response := struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      interface{} `json:"id,omitempty"`
		Error   struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{JSONRPC: "2.0", ID: id}
	response.Error.Code = code
	response.Error.Message = message
	_ = json.NewEncoder(w).Encode(response)
}

func (h *MCPServerHandler) Shutdown(ctx context.Context) error {
	if h.srv == nil {
		return nil
	}
	if err := h.srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown MCP HTTP server: %w", err)
	}
	return nil
}
