package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/google/uuid"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
)

// ============================================================================
// MOCKS
// ============================================================================

// mockStoreMemory mocks the StoreMemory interactor
type mockStoreMemory struct {
	executeFunc func(ctx context.Context, input interactors.StoreMemoryInput) (*interactors.StoreMemoryOutput, error)
}

func (m *mockStoreMemory) Execute(ctx context.Context, input interactors.StoreMemoryInput) (*interactors.StoreMemoryOutput, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	return &interactors.StoreMemoryOutput{
		FingerprintID: "test-id",
		Type:          "fact",
		FactCount:     3,
		TokenCount:    150,
		ModelHash:     "model-abc123",
	}, nil
}

// mockRecallMemory mocks the RecallMemory interactor
type mockRecallMemory struct {
	executeFunc func(ctx context.Context, input interactors.RecallMemoryInput) (*interactors.RecallMemoryOutput, error)
}

func (m *mockRecallMemory) Execute(ctx context.Context, input interactors.RecallMemoryInput) (*interactors.RecallMemoryOutput, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	memories := []*valueobjects.SelectedMemory{
		valueobjects.NewSelectedMemory(
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
			valueobjects.ModeFingerprint,
			50,
			"Test memory content",
		),
	}
	return &interactors.RecallMemoryOutput{
		Memories:    memories,
		TotalTokens: 50,
		BudgetUsed:  12.5,
	}, nil
}

// mockLoadMemory mocks the LoadMemory interactor
type mockLoadMemory struct {
	executeFunc func(ctx context.Context, input interactors.LoadMemoryInput) (*interactors.LoadMemoryOutput, error)
}

func (m *mockLoadMemory) Execute(ctx context.Context, input interactors.LoadMemoryInput) (*interactors.LoadMemoryOutput, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	room := "test-room"
	return &interactors.LoadMemoryOutput{
		Verbatim: &entities.Verbatim{
			ID:         input.ID,
			Content:    "Test verbatim content",
			TokenCount: 100,
			CreatedAt:  time.Now(),
			Wing:       "test-wing",
			Room:       &room,
		},
	}, nil
}

// mockGetCausalChain mocks the GetCausalChain interactor
type mockGetCausalChain struct {
	executeFunc func(ctx context.Context, input interactors.GetCausalChainInput) (*interactors.GetCausalChainOutput, error)
}

func (m *mockGetCausalChain) Execute(ctx context.Context, input interactors.GetCausalChainInput) (*interactors.GetCausalChainOutput, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	room := "test-room"
	return &interactors.GetCausalChainOutput{
		Chain: []*entities.CausalNode{
			entities.NewCausalNode(input.ID, "decision", "Root decision", "test-wing", &room),
		},
		Consequences: []*entities.CausalNode{
			entities.NewCausalNode(uuid.New(), "fact", "Consequence fact", "test-wing", &room),
		},
	}, nil
}

// mockGetTimeline mocks the GetTimeline interactor
type mockGetTimeline struct {
	executeFunc func(ctx context.Context, input interactors.GetTimelineInput) (*interactors.GetTimelineOutput, error)
}

func (m *mockGetTimeline) Execute(ctx context.Context, input interactors.GetTimelineInput) (*interactors.GetTimelineOutput, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	memType := valueobjects.MemoryType("fact")
	return &interactors.GetTimelineOutput{
		Items: []*valueobjects.TimelineItem{
			{
				ID:        "550e8400-e29b-41d4-a716-446655440000",
				Timestamp: "2024-01-01T00:00:00Z",
				Type:      memType,
				Summary:   "Test timeline item",
			},
		},
	}, nil
}

// mockGetStatus mocks the GetStatus interactor
type mockGetStatus struct {
	executeFunc func(ctx context.Context) (*interactors.GetStatusOutput, error)
}

func (m *mockGetStatus) Execute(ctx context.Context) (*interactors.GetStatusOutput, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx)
	}
	stats := valueobjects.NewStats()
	stats.VerbatimCount = 100
	stats.FingerprintCount = 95
	stats.EmbeddingCount = 95
	stats.CausalNodeCount = 50
	stats.CausalEdgeCount = 75
	stats.TotalTokens = 10000
	stats.TypeCounts["decision"] = 10
	stats.TypeCounts["fact"] = 50
	stats.TypeCounts["preference"] = 20
	stats.TypeCounts["session_note"] = 10
	stats.TypeCounts["debug_log"] = 5
	stats.ActiveWings = []string{"wing1", "wing2"}
	return &interactors.GetStatusOutput{
		Stats:  stats,
		Models: []string{"model1", "model2"},
	}, nil
}

// mockArchiveMemories mocks the ArchiveMemories interactor
type mockArchiveMemories struct {
	executeFunc func(ctx context.Context) (*interactors.ArchiveMemoriesOutput, error)
}

func (m *mockArchiveMemories) Execute(ctx context.Context) (*interactors.ArchiveMemoriesOutput, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx)
	}
	return &interactors.ArchiveMemoriesOutput{
		Result: &valueobjects.ArchiveResult{
			SessionNotes: 5,
			DebugLogs:    10,
			TokensFreed:  1500,
		},
	}, nil
}

// mockClearMemory mocks the ClearMemory interactor
type mockClearMemory struct {
	executeFunc func(ctx context.Context, input interactors.ClearMemoryInput) (*interactors.ClearMemoryOutput, error)
}

func (m *mockClearMemory) Execute(ctx context.Context, input interactors.ClearMemoryInput) (*interactors.ClearMemoryOutput, error) {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, input)
	}
	return &interactors.ClearMemoryOutput{DeletedCount: 0, Mode: input.Mode}, nil
}

// ============================================================================
// SUCCESS TESTS
// ============================================================================

// TestHandleStoreSuccess tests mira_store with a mock
func TestHandleStoreSuccess(t *testing.T) {
	mock := &mockStoreMemory{}
	controller := &Controller{storeMemory: mock}

	args := map[string]interface{}{
		"content": "Test content to store",
		"wing":    "test-wing",
		"room":    "test-room",
	}

	ctx := context.Background()
	result, err := controller.handleStore(ctx, args)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	// Verify the result is a CallToolResult with text content
	content := result.Content
	if len(content) == 0 {
		t.Fatal("Expected content in result")
	}

	textContent, ok := content[0].(mcptypes.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", content[0])
	}

	// Verify the result contains expected fields
	if !strings.Contains(textContent.Text, "test-id") {
		t.Errorf("Expected result to contain fingerprint ID 'test-id', got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "fact") {
		t.Errorf("Expected result to contain type 'fact', got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "3") {
		t.Errorf("Expected result to contain fact count '3', got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "150") {
		t.Errorf("Expected result to contain token count '150', got: %s", textContent.Text)
	}
}

// TestHandleRecallSuccess tests mira_recall with a mock
func TestHandleRecallSuccess(t *testing.T) {
	mock := &mockRecallMemory{}
	controller := &Controller{recallMemory: mock}

	args := map[string]interface{}{
		"query":  "test query",
		"budget": float64(500),
	}

	ctx := context.Background()
	result, err := controller.handleRecall(ctx, args)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	content := result.Content
	if len(content) == 0 {
		t.Fatal("Expected content in result")
	}

	textContent, ok := content[0].(mcptypes.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", content[0])
	}

	// Verify the result contains expected sections
	if !strings.Contains(textContent.Text, "MIRA CONTEXT") {
		t.Errorf("Expected result to contain 'MIRA CONTEXT', got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "test query") {
		t.Errorf("Expected result to contain query, got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "FINGERPRINT") {
		t.Errorf("Expected result to contain mode FINGERPRINT, got: %s", textContent.Text)
	}
}

// TestHandleLoadSuccess tests mira_load with a mock
func TestHandleLoadSuccess(t *testing.T) {
	mock := &mockLoadMemory{}
	controller := &Controller{loadMemory: mock}

	testID := "550e8400-e29b-41d4-a716-446655440000"
	args := map[string]interface{}{
		"id": testID,
	}

	ctx := context.Background()
	result, err := controller.handleLoad(ctx, args)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	content := result.Content
	if len(content) == 0 {
		t.Fatal("Expected content in result")
	}

	textContent, ok := content[0].(mcptypes.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", content[0])
	}

	// Verify the result contains expected content
	if !strings.Contains(textContent.Text, "Test verbatim content") {
		t.Errorf("Expected result to contain verbatim content, got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, testID) {
		t.Errorf("Expected result to contain ID '%s', got: %s", testID, textContent.Text)
	}
	if !strings.Contains(textContent.Text, "test-wing") {
		t.Errorf("Expected result to contain wing, got: %s", textContent.Text)
	}
}

// TestHandleCausalChainSuccess tests mira_causal_chain with a mock
func TestHandleCausalChainSuccess(t *testing.T) {
	mock := &mockGetCausalChain{}
	controller := &Controller{getCausalChain: mock}

	testID := "550e8400-e29b-41d4-a716-446655440000"
	args := map[string]interface{}{
		"id":        testID,
		"max_depth": float64(5),
	}

	ctx := context.Background()
	result, err := controller.handleCausalChain(ctx, args)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	content := result.Content
	if len(content) == 0 {
		t.Fatal("Expected content in result")
	}

	textContent, ok := content[0].(mcptypes.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", content[0])
	}

	// Verify the result contains expected sections
	if !strings.Contains(textContent.Text, "CAUSAL CHAIN") {
		t.Errorf("Expected result to contain 'CAUSAL CHAIN', got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "CONSEQUENCES") {
		t.Errorf("Expected result to contain 'CONSEQUENCES', got: %s", textContent.Text)
	}
}

// TestHandleTimelineSuccess tests mira_timeline with a mock
func TestHandleTimelineSuccess(t *testing.T) {
	mock := &mockGetTimeline{}
	controller := &Controller{getTimeline: mock}

	args := map[string]interface{}{
		"wing": "test-wing",
		"room": "test-room",
	}

	ctx := context.Background()
	result, err := controller.handleTimeline(ctx, args)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	content := result.Content
	if len(content) == 0 {
		t.Fatal("Expected content in result")
	}

	textContent, ok := content[0].(mcptypes.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", content[0])
	}

	// Verify the result contains expected content
	if !strings.Contains(textContent.Text, "TIMELINE") {
		t.Errorf("Expected result to contain 'TIMELINE', got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "test-wing") {
		t.Errorf("Expected result to contain wing, got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "Test timeline item") {
		t.Errorf("Expected result to contain timeline item, got: %s", textContent.Text)
	}
}

// TestHandleStatusSuccess tests mira_status with a mock
func TestHandleStatusSuccess(t *testing.T) {
	mock := &mockGetStatus{}
	controller := &Controller{getStatus: mock}

	ctx := context.Background()
	result, err := controller.handleStatus(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	content := result.Content
	if len(content) == 0 {
		t.Fatal("Expected content in result")
	}

	textContent, ok := content[0].(mcptypes.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", content[0])
	}

	// Verify the result contains expected sections
	if !strings.Contains(textContent.Text, "MIRA System Status") {
		t.Errorf("Expected result to contain 'MIRA System Status', got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "Verbatims: 100") {
		t.Errorf("Expected result to contain verbatim count, got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "wing1") {
		t.Errorf("Expected result to contain active wings, got: %s", textContent.Text)
	}
}

// TestHandleArchiveSuccess tests mira_archive with a mock
func TestHandleArchiveSuccess(t *testing.T) {
	mock := &mockArchiveMemories{}
	controller := &Controller{archiveMemories: mock}

	ctx := context.Background()
	result, err := controller.handleArchive(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if result == nil {
		t.Fatal("Expected result, got nil")
	}

	content := result.Content
	if len(content) == 0 {
		t.Fatal("Expected content in result")
	}

	textContent, ok := content[0].(mcptypes.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", content[0])
	}

	// Verify the result contains expected content
	if !strings.Contains(textContent.Text, "Archiving complete") {
		t.Errorf("Expected result to contain 'Archiving complete', got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "5") {
		t.Errorf("Expected result to contain session notes count, got: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "1500") {
		t.Errorf("Expected result to contain tokens freed, got: %s", textContent.Text)
	}
}

// TestHandleClearMemoryGlobalSuccess tests mira_clear_memory in global mode
func TestHandleClearMemoryGlobalSuccess(t *testing.T) {
	mock := &mockClearMemory{}
	controller := &Controller{clearMemory: mock}

	ctx := context.Background()
	result, err := controller.handleClearMemory(ctx, map[string]interface{}{"mode": "global"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	content := result.Content
	if len(content) == 0 {
		t.Fatal("Expected content in result")
	}

	textContent, ok := content[0].(mcptypes.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", content[0])
	}

	if !strings.Contains(textContent.Text, "All memories have been permanently deleted") {
		t.Errorf("Expected global clear confirmation, got: %s", textContent.Text)
	}
}

// TestHandleClearMemoryRoomSuccess tests mira_clear_memory in room mode
func TestHandleClearMemoryRoomSuccess(t *testing.T) {
	mock := &mockClearMemory{
		executeFunc: func(ctx context.Context, input interactors.ClearMemoryInput) (*interactors.ClearMemoryOutput, error) {
			return &interactors.ClearMemoryOutput{DeletedCount: 7, Mode: "room"}, nil
		},
	}
	controller := &Controller{clearMemory: mock}

	ctx := context.Background()
	result, err := controller.handleClearMemory(ctx, map[string]interface{}{
		"mode": "room",
		"wing": "auth-service",
		"room": "decisions",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	content := result.Content
	if len(content) == 0 {
		t.Fatal("Expected content in result")
	}

	textContent, ok := content[0].(mcptypes.TextContent)
	if !ok {
		t.Fatalf("Expected TextContent, got %T", content[0])
	}

	if !strings.Contains(textContent.Text, "Cleared 7 memories") {
		t.Errorf("Expected room clear confirmation with count, got: %s", textContent.Text)
	}
}

// TestHandleClearMemoryValidation tests validation errors for mira_clear_memory
func TestHandleClearMemoryValidation(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]interface{}
		expectedErrMsg string
	}{
		{
			name:           "missing mode",
			args:           map[string]interface{}{},
			expectedErrMsg: "mode is required",
		},
		{
			name: "invalid mode",
			args: map[string]interface{}{
				"mode": "invalid",
			},
			expectedErrMsg: "mode is required",
		},
		{
			name: "room mode missing wing",
			args: map[string]interface{}{
				"mode": "room",
			},
			expectedErrMsg: "wing is required",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &Controller{clearMemory: &mockClearMemory{}}
			_, err := controller.handleClearMemory(ctx, tt.args)
			if err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.expectedErrMsg)
				return
			}
			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

// ============================================================================
// VALIDATION TESTS (EXISTANTS)
// ============================================================================

// TestHandleStoreValidation tests the validation logic in mira_store handler
func TestHandleStoreValidation(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]interface{}
		expectedErrMsg string
	}{
		{
			name: "missing content",
			args: map[string]interface{}{
				"wing": "test-wing",
			},
			expectedErrMsg: "content is required",
		},
		{
			name: "missing wing",
			args: map[string]interface{}{
				"content": "Test content",
			},
			expectedErrMsg: "wing is required",
		},
		{
			name: "empty wing",
			args: map[string]interface{}{
				"content": "Test content",
				"wing":    "",
			},
			expectedErrMsg: "wing is required",
		},
		{
			name: "wing whitespace only",
			args: map[string]interface{}{
				"content": "Test content",
				"wing":    "   ",
			},
			expectedErrMsg: "wing is required",
		},
		{
			name: "content too long",
			args: map[string]interface{}{
				"content": strings.Repeat("a", MaxContentLength+1),
				"wing":    "test-wing",
			},
			expectedErrMsg: "exceeds maximum length",
		},
		{
			name: "wing too long",
			args: map[string]interface{}{
				"content": "test content",
				"wing":    strings.Repeat("a", MaxWingLength+1),
			},
			expectedErrMsg: "exceeds maximum length",
		},
		{
			name: "room too long",
			args: map[string]interface{}{
				"content": "test content",
				"wing":    "test-wing",
				"room":    strings.Repeat("a", MaxRoomLength+1),
			},
			expectedErrMsg: "exceeds maximum length",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use empty StoreMemory - validation happens before Execute is called
			controller := &Controller{storeMemory: &mockStoreMemory{}}

			_, err := controller.handleStore(ctx, tt.args)

			if err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.expectedErrMsg)
				return
			}

			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

// TestHandleStoreWithTypeValidation tests that valid type parameters pass validation
func TestHandleStoreWithTypeValidation(t *testing.T) {
	// Valid type parameters should pass validation (even if use case fails later)
	// This test documents the valid type values
	validTypes := []string{"decision", "fact", "preference", "session_note", "debug_log"}

	for _, typeVal := range validTypes {
		t.Run("type_"+typeVal, func(t *testing.T) {
			// Just verify the type is a valid MemoryType
			mt := valueobjects.MemoryType(typeVal)
			if !mt.IsValid() {
				t.Errorf("Type '%s' should be valid", typeVal)
			}
		})
	}

	// Test invalid types
	t.Run("invalid_type", func(t *testing.T) {
		mt := valueobjects.MemoryType("invalid_type")
		if mt.IsValid() {
			t.Error("Invalid type should not be valid")
		}
	})
}

// TestHandleRecallValidation tests the validation logic in mira_recall handler
func TestHandleRecallValidation(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]interface{}
		expectedErrMsg string
	}{
		{
			name: "query missing",
			args: map[string]interface{}{
				"budget": 500,
			},
			expectedErrMsg: "query is required",
		},
		{
			name: "query empty",
			args: map[string]interface{}{
				"query":  "",
				"budget": 500,
			},
			expectedErrMsg: "cannot be empty",
		},
		{
			name: "query whitespace only",
			args: map[string]interface{}{
				"query":  "   ",
				"budget": 500,
			},
			expectedErrMsg: "cannot be empty",
		},
		{
			name: "query too long",
			args: map[string]interface{}{
				"query":  strings.Repeat("a", MaxQueryLength+1),
				"budget": 500,
			},
			expectedErrMsg: "exceeds maximum length",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &Controller{recallMemory: &mockRecallMemory{}}

			_, err := controller.handleRecall(ctx, tt.args)

			if err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.expectedErrMsg)
				return
			}

			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

// TestHandleRecallParameters documents the valid parameter formats for mira_recall
func TestHandleRecallParameters(t *testing.T) {
	// This test documents the expected parameter formats
	// We can't test the full flow without proper use case initialization

	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{
			name: "valid query and budget",
			args: map[string]interface{}{
				"query":  "test query",
				"budget": float64(400),
			},
		},
		{
			name: "valid with wing filter",
			args: map[string]interface{}{
				"query":  "filtered query",
				"budget": float64(500),
				"wing":   "test-wing",
			},
		},
		{
			name: "valid with room filter",
			args: map[string]interface{}{
				"query":  "room filtered query",
				"budget": float64(500),
				"room":   "test-room",
			},
		},
		{
			name: "default budget",
			args: map[string]interface{}{
				"query": "query without budget",
			},
		},
		{
			name: "budget as int",
			args: map[string]interface{}{
				"query":  "query with int budget",
				"budget": 500,
			},
		},
		{
			name: "budget as string",
			args: map[string]interface{}{
				"query":  "query with string budget",
				"budget": "600",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Just verify the args can be processed without validation errors
			// (we can't actually call handleRecall without proper initialization)

			query, hasQuery := tt.args["query"].(string)
			if !hasQuery || query == "" {
				t.Error("Query should be a non-empty string")
			}

			// Verify budget parsing logic
			budget := 4000 // default
			if bArg, ok := tt.args["budget"]; ok {
				switch v := bArg.(type) {
				case float64:
					budget = int(v)
				case int:
					budget = v
				case string:
					// Would parse string in real code
				}
			}
			if budget <= 0 {
				t.Error("Budget should be positive")
			}
		})
	}
}

// TestHandleLoadValidation tests the validation logic in mira_load handler
func TestHandleLoadValidation(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]interface{}
		expectedErrMsg string
	}{
		{
			name: "missing id",
			args: map[string]interface{}{
				"other": "value",
			},
			expectedErrMsg: "id is required",
		},
		{
			name: "invalid uuid",
			args: map[string]interface{}{
				"id": "not-a-valid-uuid",
			},
			expectedErrMsg: "invalid UUID",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &Controller{loadMemory: &mockLoadMemory{}}

			_, err := controller.handleLoad(ctx, tt.args)

			if err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.expectedErrMsg)
				return
			}

			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

// TestHandleTimelineValidation tests the validation logic in mira_timeline handler
func TestHandleTimelineValidation(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]interface{}
		expectedErrMsg string
	}{
		{
			name: "missing wing",
			args: map[string]interface{}{
				"room": "test-room",
			},
			expectedErrMsg: "wing is required",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &Controller{getTimeline: &mockGetTimeline{}}

			_, err := controller.handleTimeline(ctx, tt.args)

			if err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.expectedErrMsg)
				return
			}

			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

// TestHandleCausalChainValidation tests the validation logic in mira_causal_chain handler
func TestHandleCausalChainValidation(t *testing.T) {
	tests := []struct {
		name           string
		args           map[string]interface{}
		expectedErrMsg string
	}{
		{
			name: "missing id",
			args: map[string]interface{}{
				"max_depth": float64(5),
			},
			expectedErrMsg: "id is required",
		},
		{
			name: "invalid uuid",
			args: map[string]interface{}{
				"id": "invalid-uuid",
			},
			expectedErrMsg: "invalid UUID",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &Controller{getCausalChain: &mockGetCausalChain{}}

			_, err := controller.handleCausalChain(ctx, tt.args)

			if err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.expectedErrMsg)
				return
			}

			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

// TestValidationErrorsCombined tests all validation errors in one place
func TestValidationErrorsCombined(t *testing.T) {
	tests := []struct {
		name           string
		handler        string
		args           map[string]interface{}
		expectedErrMsg string
	}{
		// Store validation errors
		{
			name:    "store - content too long",
			handler: "store",
			args: map[string]interface{}{
				"content": strings.Repeat("a", MaxContentLength+1),
				"wing":    "test-wing",
			},
			expectedErrMsg: "exceeds maximum length",
		},
		{
			name:    "store - wing missing",
			handler: "store",
			args: map[string]interface{}{
				"content": "test content",
			},
			expectedErrMsg: "wing is required",
		},

		// Recall validation errors
		{
			name:    "recall - query missing",
			handler: "recall",
			args: map[string]interface{}{
				"budget": 500,
			},
			expectedErrMsg: "query is required",
		},
		{
			name:    "recall - query empty",
			handler: "recall",
			args: map[string]interface{}{
				"query":  "",
				"budget": 500,
			},
			expectedErrMsg: "cannot be empty",
		},
		{
			name:    "recall - query whitespace only",
			handler: "recall",
			args: map[string]interface{}{
				"query":  "   ",
				"budget": 500,
			},
			expectedErrMsg: "cannot be empty",
		},
		{
			name:    "recall - query too long",
			handler: "recall",
			args: map[string]interface{}{
				"query":  strings.Repeat("a", MaxQueryLength+1),
				"budget": 500,
			},
			expectedErrMsg: "exceeds maximum length",
		},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			controller := &Controller{}
			var err error
			switch tt.handler {
			case "store":
				controller.storeMemory = &mockStoreMemory{}
				_, err = controller.handleStore(ctx, tt.args)
			case "recall":
				controller.recallMemory = &mockRecallMemory{}
				_, err = controller.handleRecall(ctx, tt.args)
			default:
				t.Fatalf("Unknown handler: %s", tt.handler)
			}

			if err == nil {
				t.Errorf("Expected error containing '%s', got nil", tt.expectedErrMsg)
				return
			}

			if !strings.Contains(err.Error(), tt.expectedErrMsg) {
				t.Errorf("Expected error containing '%s', got '%s'", tt.expectedErrMsg, err.Error())
			}
		})
	}
}

// TestAllHandlersExist tests that all handler validation functions work
func TestAllHandlersExist(t *testing.T) {
	ctx := context.Background()

	// Test store handler validation
	controller := &Controller{storeMemory: &mockStoreMemory{}}
	_, err := controller.handleStore(ctx, map[string]interface{}{
		"content": "",
		"wing":    "",
	})
	if err == nil {
		t.Error("store handler should return validation error")
	}

	// Test recall handler validation
	controller = &Controller{recallMemory: &mockRecallMemory{}}
	_, err = controller.handleRecall(ctx, map[string]interface{}{
		"query": "",
	})
	if err == nil {
		t.Error("recall handler should return validation error")
	}

	// Test load handler validation
	controller = &Controller{loadMemory: &mockLoadMemory{}}
	_, err = controller.handleLoad(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("load handler should return validation error")
	}

	// Test timeline handler validation
	controller = &Controller{getTimeline: &mockGetTimeline{}}
	_, err = controller.handleTimeline(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("timeline handler should return validation error")
	}

	// Test causal chain handler validation
	controller = &Controller{getCausalChain: &mockGetCausalChain{}}
	_, err = controller.handleCausalChain(ctx, map[string]interface{}{})
	if err == nil {
		t.Error("causal chain handler should return validation error")
	}

	// Note: handleStatus and handleArchive don't have validation,
	// they require properly initialized use cases to test
}
