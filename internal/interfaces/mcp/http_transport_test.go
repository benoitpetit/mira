package mcp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/server"
)

func TestMCPHTTPTransportInitialize(t *testing.T) {
	h := NewMCPServerHandler(server.NewDefaultServer("mira-test", "1.0.0"), "unused")
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response server.JSONRPCResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.JSONRPC != "2.0" || response.Error != nil || response.Result == nil {
		t.Fatalf("unexpected JSON-RPC response: %+v", response)
	}
}

func TestMCPHTTPTransportRejectsSSEPath(t *testing.T) {
	h := NewMCPServerHandler(server.NewDefaultServer("mira-test", "1.0.0"), "unused")
	req := httptest.NewRequest(http.MethodGet, "/sse", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
