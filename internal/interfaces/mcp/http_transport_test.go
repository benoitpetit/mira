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

func TestMCPHTTPTransportAuth(t *testing.T) {
	h := NewMCPServerHandlerWithAuth(server.NewDefaultServer("mira-test", "1.0.0"), "secret")
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)

	unauthorized := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	unauthorizedRec := httptest.NewRecorder()
	h.ServeHTTP(unauthorizedRec, unauthorized)
	if unauthorizedRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401", unauthorizedRec.Code)
	}

	authorized := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	authorized.Header.Set("Authorization", "Bearer secret")
	authorizedRec := httptest.NewRecorder()
	h.ServeHTTP(authorizedRec, authorized)
	if authorizedRec.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, want 200: %s", authorizedRec.Code, authorizedRec.Body.String())
	}
}
