package mcp

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/benoitpetit/mira/budget"
	"github.com/benoitpetit/mira/causal"
	"github.com/benoitpetit/mira/extract"
	"github.com/benoitpetit/mira/store"
	"github.com/benoitpetit/mira/types"
	"github.com/benoitpetit/mira/vector"
	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// resultText extracts text from a CallToolResult
func resultText(t *testing.T, result *mcpgo.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		t.Fatal("Expected non-nil result with content")
	}
	tc, ok := result.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

// setupTestServer creates a real Server with SQLite store.
func setupTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "mira-mcp-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	dbPath := tmpDir + "/test.db"

	st, err := store.NewWithOptions(dbPath, store.StoreOptions{})
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create store: %v", err)
	}

	embedder := extract.NewSimpleEmbedder(384)
	ext, err := extract.NewExtractorWithOptions("test-model", embedder, extract.ExtractorOptions{})
	if err != nil {
		st.Close()
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to create extractor: %v", err)
	}

	model := &types.EmbeddingModel{
		ModelHash: ext.ModelHash(),
		ModelName: "test-model",
		Dimension: 384,
		CreatedAt: time.Now(),
	}
	if err := st.RegisterModel(model); err != nil {
		t.Logf("Warning: failed to register model: %v", err)
	}

	cg := causal.New(st.DB())
	vs := vector.NewSQLiteAdapter(st)
	oc := vector.NewSQLiteOverlapCache(st.DB())
	alloc := budget.NewAllocatorWithOptions(vs, oc, cg, ext, budget.AllocatorOptions{})

	srv := NewServer(st, alloc, ext, cg)
	cleanup := func() {
		st.Close()
		os.RemoveAll(tmpDir)
	}

	return srv, cleanup
}

// --- handleStore ---

func TestHandleStore_Success(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	args := map[string]interface{}{
		"content": "The team decided to use PostgreSQL for the new auth service database.",
		"wing":    "backend",
	}

	result, err := srv.handleStore(args)
	if err != nil {
		t.Fatalf("handleStore failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Stored:") {
		t.Errorf("Expected 'Stored:' in result, got: %s", text)
	}
	if !strings.Contains(text, "Type:") {
		t.Errorf("Expected 'Type:' in result, got: %s", text)
	}
}

func TestHandleStore_WithRoom(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	args := map[string]interface{}{
		"content": "Decided to migrate from MySQL to PostgreSQL.",
		"wing":    "backend",
		"room":    "migration",
	}

	result, err := srv.handleStore(args)
	if err != nil {
		t.Fatalf("handleStore with room failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Stored:") {
		t.Errorf("Expected 'Stored:' in result, got: %s", text)
	}
}

func TestHandleStore_WithForcedType(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	args := map[string]interface{}{
		"content": "The server runs on port 8080.",
		"wing":    "infra",
		"type":    "fact",
	}

	result, err := srv.handleStore(args)
	if err != nil {
		t.Fatalf("handleStore with forced type failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Type: fact") {
		t.Errorf("Expected forced type 'fact' in result, got: %s", text)
	}
}

func TestHandleStore_MissingContent(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleStore(map[string]interface{}{"wing": "backend"})
	if err == nil {
		t.Fatal("Expected error for missing content")
	}
	if !strings.Contains(err.Error(), "content is required") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleStore_MissingWing(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleStore(map[string]interface{}{"content": "Some content"})
	if err == nil {
		t.Fatal("Expected error for missing wing")
	}
	if !strings.Contains(err.Error(), "wing is required") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleStore_EmptyWing(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleStore(map[string]interface{}{"content": "Some content", "wing": "   "})
	if err == nil {
		t.Fatal("Expected error for empty wing")
	}
	if !strings.Contains(err.Error(), "wing cannot be empty") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleStore_ContentTooLong(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleStore(map[string]interface{}{
		"content": strings.Repeat("x", MaxContentLength+1),
		"wing":    "test",
	})
	if err == nil {
		t.Fatal("Expected error for content too long")
	}
	if !strings.Contains(err.Error(), "exceeds maximum length") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleStore_WingTooLong(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleStore(map[string]interface{}{
		"content": "Some content",
		"wing":    strings.Repeat("w", MaxWingLength+1),
	})
	if err == nil {
		t.Fatal("Expected error for wing too long")
	}
	if !strings.Contains(err.Error(), "wing exceeds maximum length") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleStore_RoomTooLong(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleStore(map[string]interface{}{
		"content": "Some content",
		"wing":    "test",
		"room":    strings.Repeat("r", MaxRoomLength+1),
	})
	if err == nil {
		t.Fatal("Expected error for room too long")
	}
	if !strings.Contains(err.Error(), "room exceeds maximum length") {
		t.Errorf("Unexpected error: %v", err)
	}
}

// --- handleLoad ---

func TestHandleLoad_MissingID(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleLoad(map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing ID")
	}
	if !strings.Contains(err.Error(), "id is required") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleLoad_InvalidUUID(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleLoad(map[string]interface{}{"id": "not-a-uuid"})
	if err == nil {
		t.Fatal("Expected error for invalid UUID")
	}
	if !strings.Contains(err.Error(), "invalid UUID") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleLoad_T0Prefix(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	id := uuid.New()
	_, err := srv.handleLoad(map[string]interface{}{"id": "T0:" + id.String()})
	// Should not fail with "invalid UUID" — T0: prefix must be stripped
	if err != nil && strings.Contains(err.Error(), "invalid UUID") {
		t.Errorf("T0: prefix should be stripped, but got: %v", err)
	}
}

func TestHandleLoad_NotFound(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	id := uuid.New()
	_, err := srv.handleLoad(map[string]interface{}{"id": id.String()})
	if err == nil {
		t.Fatal("Expected error for non-existent ID")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}
}

// --- handleRecall ---

func TestHandleRecall_MissingQuery(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleRecall(map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing query")
	}
	if !strings.Contains(err.Error(), "query is required") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleRecall_EmptyQuery(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleRecall(map[string]interface{}{"query": "   "})
	if err == nil {
		t.Fatal("Expected error for empty query")
	}
	if !strings.Contains(err.Error(), "query cannot be empty") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleRecall_QueryTooLong(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleRecall(map[string]interface{}{
		"query": strings.Repeat("q", MaxQueryLength+1),
	})
	if err == nil {
		t.Fatal("Expected error for query too long")
	}
	if !strings.Contains(err.Error(), "exceeds maximum length") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleRecall_WingTooLong(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleRecall(map[string]interface{}{
		"query": "test",
		"wing":  strings.Repeat("w", MaxWingLength+1),
	})
	if err == nil {
		t.Fatal("Expected error for wing too long in recall")
	}
	if !strings.Contains(err.Error(), "wing exceeds maximum length") {
		t.Errorf("Unexpected error: %v", err)
	}
}

// --- handleCausalChain ---

func TestHandleCausalChain_MissingID(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleCausalChain(map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing ID")
	}
	if !strings.Contains(err.Error(), "id is required") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleCausalChain_EmptyGraph(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	id := uuid.New()
	result, err := srv.handleCausalChain(map[string]interface{}{"id": id.String()})
	if err != nil {
		t.Fatalf("handleCausalChain failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "CAUSAL CHAIN") {
		t.Errorf("Expected 'CAUSAL CHAIN' in result, got: %s", text)
	}
}

func TestHandleCausalChain_WithConsequences(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	id := uuid.New()
	result, err := srv.handleCausalChain(map[string]interface{}{
		"id":                   id.String(),
		"max_depth":            float64(3),
		"include_consequences": true,
	})
	if err != nil {
		t.Fatalf("handleCausalChain with consequences failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "CAUSAL CHAIN") {
		t.Errorf("Expected 'CAUSAL CHAIN', got: %s", text)
	}
}

func TestHandleCausalChain_MaxDepthInt(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	id := uuid.New()
	_, err := srv.handleCausalChain(map[string]interface{}{
		"id":        id.String(),
		"max_depth": 2,
	})
	if err != nil {
		t.Fatalf("handleCausalChain with int max_depth failed: %v", err)
	}
}

// --- handleStatus ---

func TestHandleStatus(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	result, err := srv.handleStatus()
	if err != nil {
		t.Fatalf("handleStatus failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "MIRA System Status") {
		t.Errorf("Expected 'MIRA System Status', got: %s", text)
	}
	if !strings.Contains(text, "Verbatims:") {
		t.Errorf("Expected 'Verbatims:', got: %s", text)
	}
}

// --- handleTimeline ---

func TestHandleTimeline_MissingWing(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleTimeline(map[string]interface{}{})
	if err == nil {
		t.Fatal("Expected error for missing wing")
	}
	if !strings.Contains(err.Error(), "wing is required") {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestHandleTimeline_EmptyResult(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	result, err := srv.handleTimeline(map[string]interface{}{"wing": "nonexistent"})
	if err != nil {
		t.Fatalf("handleTimeline failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "TIMELINE: nonexistent") {
		t.Errorf("Expected timeline header, got: %s", text)
	}
}

func TestHandleTimeline_WithFilters(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	result, err := srv.handleTimeline(map[string]interface{}{
		"wing":  "backend",
		"room":  "migration",
		"type":  "decision",
		"since": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		"until": time.Now().Add(1 * time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("handleTimeline with filters failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "TIMELINE: backend") {
		t.Errorf("Expected timeline header, got: %s", text)
	}
	if !strings.Contains(text, "Room: migration") {
		t.Errorf("Expected room filter, got: %s", text)
	}
}

// --- handleArchive ---

func TestHandleArchive(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	result, err := srv.handleArchive()
	if err != nil {
		t.Fatalf("handleArchive failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Archiving complete") {
		t.Errorf("Expected 'Archiving complete', got: %s", text)
	}
}

// --- modeString ---

func TestModeString(t *testing.T) {
	tests := []struct {
		mode     types.RenderMode
		expected string
	}{
		{types.ModeHeader, "HEADER"},
		{types.ModeFingerprint, "FINGERPRINT"},
		{types.ModeVerbatim, "VERBATIM"},
		{types.RenderMode(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		result := modeString(tt.mode)
		if result != tt.expected {
			t.Errorf("modeString(%d) = %s, want %s", tt.mode, result, tt.expected)
		}
	}
}

// --- Integration: store + load round-trip ---

func TestStoreAndLoad_RoundTrip(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	content := "The auth service must use OAuth2 with PKCE flow."
	args := map[string]interface{}{
		"content": content,
		"wing":    "auth",
	}

	storeResult, err := srv.handleStore(args)
	if err != nil {
		t.Fatalf("handleStore failed: %v", err)
	}
	storeText := resultText(t, storeResult)
	if !strings.Contains(storeText, "Stored:") {
		t.Fatalf("Expected 'Stored:' in result, got: %s", storeText)
	}

	// Extract UUID from "Stored: <uuid>" line
	lines := strings.Split(storeText, "\n")
	storedID := strings.TrimPrefix(lines[0], "Stored: ")

	// Load back via handleLoad
	loadResult, err := srv.handleLoad(map[string]interface{}{"id": storedID})
	if err != nil {
		t.Fatalf("handleLoad failed: %v", err)
	}
	loadText := resultText(t, loadResult)
	if !strings.Contains(loadText, content) {
		t.Errorf("Expected loaded content to contain original, got: %s", loadText)
	}
	if !strings.Contains(loadText, "Wing: auth") {
		t.Errorf("Expected wing metadata, got: %s", loadText)
	}
}

// --- Integration: store + recall ---

func TestStoreAndRecall(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Store a memory
	_, err := srv.handleStore(map[string]interface{}{
		"content": "We decided to use Redis for session caching because of its TTL support.",
		"wing":    "backend",
		"room":    "caching",
	})
	if err != nil {
		t.Fatalf("handleStore failed: %v", err)
	}

	// Recall should work (even if no vector search — tests CBA path with empty overlap/vector)
	recallResult, err := srv.handleRecall(map[string]interface{}{
		"query":  "session caching",
		"wing":   "backend",
		"budget": float64(4000),
	})
	if err != nil {
		t.Fatalf("handleRecall failed: %v", err)
	}
	text := resultText(t, recallResult)
	if !strings.Contains(text, "MIRA CONTEXT") {
		t.Errorf("Expected 'MIRA CONTEXT', got: %s", text)
	}
}

// --- Integration: store + timeline ---

func TestStoreAndTimeline(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleStore(map[string]interface{}{
		"content": "Decided to migrate from MongoDB to PostgreSQL for the user service.",
		"wing":    "data",
		"room":    "migration",
	})
	if err != nil {
		t.Fatalf("handleStore failed: %v", err)
	}

	result, err := srv.handleTimeline(map[string]interface{}{"wing": "data"})
	if err != nil {
		t.Fatalf("handleTimeline failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "TIMELINE: data") {
		t.Errorf("Expected timeline header, got: %s", text)
	}
}

// --- Integration: store + status ---

func TestStoreAndStatus(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	_, err := srv.handleStore(map[string]interface{}{
		"content": "API rate limit set to 100 req/min per client.",
		"wing":    "api",
	})
	if err != nil {
		t.Fatalf("handleStore failed: %v", err)
	}

	result, err := srv.handleStatus()
	if err != nil {
		t.Fatalf("handleStatus failed: %v", err)
	}
	text := resultText(t, result)
	if !strings.Contains(text, "Verbatims: 1") {
		t.Errorf("Expected 1 verbatim after store, got: %s", text)
	}
}

// --- Integration: store + causal chain ---

func TestStoreAndCausalChain(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	storeResult, err := srv.handleStore(map[string]interface{}{
		"content": "The team decided to use gRPC for inter-service communication.",
		"wing":    "infra",
	})
	if err != nil {
		t.Fatalf("handleStore failed: %v", err)
	}

	// Extract the stored ID
	storeText := resultText(t, storeResult)
	lines := strings.Split(storeText, "\n")
	storedID := strings.TrimPrefix(lines[0], "Stored: ")

	// Causal chain on a real stored node
	chainResult, err := srv.handleCausalChain(map[string]interface{}{
		"id":                   storedID,
		"include_consequences": true,
	})
	if err != nil {
		t.Fatalf("handleCausalChain failed: %v", err)
	}
	text := resultText(t, chainResult)
	if !strings.Contains(text, "CAUSAL CHAIN") {
		t.Errorf("Expected 'CAUSAL CHAIN', got: %s", text)
	}
}
