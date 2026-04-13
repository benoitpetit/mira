// MCP controller - Interface adapter
package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/google/uuid"
	mcptypes "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Validation constants
const (
	MaxContentLength = 100000
	MaxWingLength    = 100
	MaxRoomLength    = 100
	MaxQueryLength   = 10000
)

// Interfaces for dependency injection and testing
type (
	// StoreMemoryExecutor stores memories
	StoreMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.StoreMemoryInput) (*interactors.StoreMemoryOutput, error)
	}

	// RecallMemoryExecutor recalls memories
	RecallMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.RecallMemoryInput) (*interactors.RecallMemoryOutput, error)
	}

	// LoadMemoryExecutor loads memories
	LoadMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.LoadMemoryInput) (*interactors.LoadMemoryOutput, error)
	}

	// GetTimelineExecutor gets timeline
	GetTimelineExecutor interface {
		Execute(ctx context.Context, input interactors.GetTimelineInput) (*interactors.GetTimelineOutput, error)
	}

	// GetStatusExecutor gets system status
	GetStatusExecutor interface {
		Execute(ctx context.Context) (*interactors.GetStatusOutput, error)
	}

	// GetCausalChainExecutor gets causal chain
	GetCausalChainExecutor interface {
		Execute(ctx context.Context, input interactors.GetCausalChainInput) (*interactors.GetCausalChainOutput, error)
	}

	// ArchiveMemoriesExecutor archives memories
	ArchiveMemoriesExecutor interface {
		Execute(ctx context.Context) (*interactors.ArchiveMemoriesOutput, error)
	}

	// ClearMemoryExecutor clears memories
	ClearMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.ClearMemoryInput) (*interactors.ClearMemoryOutput, error)
	}

	// FingerprintLookup provides read-only access to fingerprint lookups
	FingerprintLookup interface {
		GetFingerprintByVerbatimID(ctx context.Context, verbatimID uuid.UUID) (*entities.Fingerprint, error)
	}
)

// Controller handles MCP tool calls
type Controller struct {
	storeMemory     StoreMemoryExecutor
	recallMemory    RecallMemoryExecutor
	loadMemory      LoadMemoryExecutor
	getTimeline     GetTimelineExecutor
	getStatus       GetStatusExecutor
	getCausalChain  GetCausalChainExecutor
	archiveMemories ArchiveMemoriesExecutor
	clearMemory     ClearMemoryExecutor
	fingerprintRepo FingerprintLookup
}

// NewController creates a new MCP controller
func NewController(
	storeMemory *interactors.StoreMemory,
	recallMemory *interactors.RecallMemory,
	loadMemory *interactors.LoadMemory,
	getTimeline *interactors.GetTimeline,
	getStatus *interactors.GetStatus,
	getCausalChain *interactors.GetCausalChain,
	archiveMemories *interactors.ArchiveMemories,
	clearMemory *interactors.ClearMemory,
	fingerprintRepo FingerprintLookup,
) *Controller {
	return &Controller{
		storeMemory:     storeMemory,
		recallMemory:    recallMemory,
		loadMemory:      loadMemory,
		getTimeline:     getTimeline,
		getStatus:       getStatus,
		getCausalChain:  getCausalChain,
		archiveMemories: archiveMemories,
		clearMemory:     clearMemory,
		fingerprintRepo: fingerprintRepo,
	}
}

// RegisterTools registers all MCP tools
func (c *Controller) RegisterTools(mcpServer server.MCPServer) {
	mcpServer.HandleListTools(func(ctx context.Context, cursor *string) (*mcptypes.ListToolsResult, error) {
		tools := []mcptypes.Tool{
			{
				Name: "mira_store",
				Description: `Store a memory in MIRA with automatic entity extraction and fingerprinting.

The content is analyzed to extract entities, create a semantic fingerprint for similarity matching,
and link to existing causal chains if applicable.

Parameters:
  - content: The text to store (required)
  - wing: Namespace/project for organization (e.g., "auth-service", "frontend", "infra")
  - room: Sub-category within the wing (e.g., "decisions", "bugs", "architecture")
  - type: Memory type - auto-detected if empty. Values: decision|fact|preference|session_note|debug_log

Examples:
  Store a decision:  {"content": "Use JWT tokens with RS256 for OAuth2", "wing": "auth-service", "room": "decisions", "type": "decision"}
  Store a debug log: {"content": "Fixed nil pointer in user.go:42", "wing": "api", "room": "debug", "type": "debug_log"}
  Store a fact:      {"content": "Database connection pool max is 100", "wing": "infra", "type": "fact"}`,
				InputSchema: mcptypes.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"content": map[string]string{"type": "string", "description": "Text content to store"},
						"wing":    map[string]string{"type": "string", "description": "Namespace/project (e.g., 'auth-service')"},
						"room":    map[string]string{"type": "string", "description": "Sub-category (e.g., 'migration')"},
						"type":    map[string]string{"type": "string", "description": "Forced type: decision|fact|preference|session_note|debug_log (auto-detect if empty)"},
					},
				},
			},
			{
				Name: "mira_recall",
				Description: `Retrieve relevant memories for a query using semantic similarity and session-aware ranking.

Supports multilingual queries (English, French, Spanish, Italian, German, etc.) through cross-lingual embeddings.
If the initial search yields sparse results, MIRA automatically broadens the search with relaxed thresholds.

Returns the most relevant verbatims within the specified token budget, ranked by:
1. Semantic similarity to the query (embedding-based, multilingual)
2. Session recency boost (recent items in current session)
3. Causal relevance (items linked in decision chains)

Parameters:
  - query: Search text or question (works in any language)
  - budget: Max tokens to return (default: 4000)
  - wing: Filter to specific namespace/project
  - room: Filter to specific sub-category
  - fallback_wings: Comma-separated fallback wings to search if primary wing yields no results

Examples:
  General recall (EN): {"query": "What was decided about authentication?", "budget": 2000}
  Filtered recall:     {"query": "database migration", "wing": "infra", "room": "decisions"}
  Multilingual (FR):   {"query": "règles de langue français anglais", "wing": "general"}
  Multilingual (ES):   {"query": "reglas de idioma español inglés", "wing": "general"}`,
				InputSchema: mcptypes.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"query":          map[string]string{"type": "string", "description": "Query/search text (any language supported)"},
						"budget":         map[string]string{"type": "number", "description": "Token budget (default: 4000)"},
						"wing":           map[string]string{"type": "string", "description": "Filter by wing/namespace"},
						"room":           map[string]string{"type": "string", "description": "Filter by room/sub-category"},
						"fallback_wings": map[string]string{"type": "string", "description": "Comma-separated fallback wings to search if primary wing yields no results"},
					},
				},
			},
			{
				Name: "mira_load",
				Description: `Load a complete memory verbatim by its ID.

Retrieves the full content including metadata (creation time, type, wing, room, entities).
Use when you have a verbatim ID from a previous recall or causal chain and need the complete details.

Parameters:
  - id: Verbatim UUID or T0 reference (e.g., "T0:auth-123" or full UUID)

Examples:
  Load by T0 ref:    {"id": "T0:auth-service-abc123"}
  Load by UUID:      {"id": "550e8400-e29b-41d4-a716-446655440000"}`,
				InputSchema: mcptypes.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"id": map[string]string{"type": "string", "description": "Verbatim UUID or T0:xxx reference"},
					},
				},
			},
			{
				Name: "mira_causal_chain",
				Description: `Trace the causal chain of a decision or event through linked memories.

Shows how decisions evolved over time by following parent-child relationships between memories.
Useful for understanding the context and reasoning behind important decisions.

Parameters:
  - id: Fingerprint ID (full UUID) or T0:verbatim-reference. The ID shown in recall/timeline results.
  - max_depth: How far back to trace (default: 5)
  - include_consequences: Also show downstream effects (children)

Examples:
  Trace decision:    {"id": "550e8400-e29b-41d4-a716-446655440000", "max_depth": 3}
  Full chain:        {"id": "550e8400-e29b-41d4-a716-446655440000", "max_depth": 10, "include_consequences": true}`,
				InputSchema: mcptypes.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"id":                   map[string]string{"type": "string", "description": "Fingerprint ID (full UUID) or T0:verbatim-ref"},
						"max_depth":            map[string]string{"type": "number", "description": "Max depth (default: 5)"},
						"include_consequences": map[string]string{"type": "boolean", "description": "Include consequences/children"},
					},
				},
			},
			{
				Name: "mira_status",
				Description: `Get MIRA system statistics and health information.

Returns:
  - Total memories stored
  - Vector index status (HNSW)
  - Memory type distribution
  - Archive status
  - Storage usage

No parameters required. Use to check system health before operations.`,
				InputSchema: mcptypes.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}{},
				},
			},
			{
				Name: "mira_timeline",
				Description: `Reconstruct a chronological timeline of memories filtered by criteria.

Returns memories in chronological order, useful for seeing how a project or topic evolved over time.
All filters are optional - use combinations to narrow results.

Parameters:
  - wing: Filter to specific namespace/project (required for large databases)
  - room: Filter to sub-category
  - since: Start date (ISO 8601, e.g., "2024-01-15")
  - until: End date (ISO 8601)
  - type: Filter by memory type (decision|fact|preference|session_note|debug_log)

Examples:
  Project timeline:  {"wing": "auth-service", "since": "2024-01-01"}
  Sprint decisions:  {"wing": "frontend", "room": "sprint-5", "type": "decision"}
  Recent debug:      {"wing": "api", "type": "debug_log", "since": "2024-04-01"}`,
				InputSchema: mcptypes.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"wing":  map[string]string{"type": "string", "description": "Required wing/namespace"},
						"room":  map[string]string{"type": "string", "description": "Filter by room/sub-category"},
						"since": map[string]string{"type": "string", "description": "Start date ISO 8601 (e.g., 2024-01-15)"},
						"until": map[string]string{"type": "string", "description": "End date ISO 8601"},
						"type":  map[string]string{"type": "string", "description": "Filter by type: decision|fact|preference|session_note|debug_log"},
					},
				},
			},
			{
				Name: "mira_archive",
				Description: `Archive and clean old memories according to configured decay rates.

Memories are archived based on:
  - Age (older memories decay faster)
  - Access patterns (unused memories archive sooner)
  - Type-specific thresholds (debug_logs archive faster than decisions)

This operation is safe - archived memories can be restored if needed.
Returns statistics about what was archived.

No parameters required. Use periodically to maintain database size.`,
				InputSchema: mcptypes.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}{},
				},
			},
			{
				Name: "mira_clear_memory",
				Description: `Permanently delete all memories. Use with caution.

Supports two modes:
  - global: Deletes every memory across all wings and rooms. Requires no additional filters.
  - room: Deletes only memories within a specific wing and optional room.

Parameters:
  - mode: "global" or "room" (required)
  - wing: Required when mode is "room"
  - room: Optional sub-category when mode is "room"

Examples:
  Clear everything: {"mode": "global"}
  Clear one room:   {"mode": "room", "wing": "auth-service", "room": "decisions"}
  Clear whole wing: {"mode": "room", "wing": "auth-service"}`,
				InputSchema: mcptypes.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"mode": map[string]string{"type": "string", "description": "Clear mode: 'global' or 'room'"},
						"wing": map[string]string{"type": "string", "description": "Wing/namespace (required for room mode)"},
						"room": map[string]string{"type": "string", "description": "Room/sub-category (optional for room mode)"},
					},
				},
			},
		}
		return &mcptypes.ListToolsResult{Tools: tools}, nil
	})

	mcpServer.HandleCallTool(func(ctx context.Context, name string, arguments map[string]interface{}) (*mcptypes.CallToolResult, error) {
		switch name {
		case "mira_store":
			return c.handleStore(ctx, arguments)
		case "mira_recall":
			return c.handleRecall(ctx, arguments)
		case "mira_load":
			return c.handleLoad(ctx, arguments)
		case "mira_causal_chain":
			return c.handleCausalChain(ctx, arguments)
		case "mira_status":
			return c.handleStatus(ctx)
		case "mira_timeline":
			return c.handleTimeline(ctx, arguments)
		case "mira_archive":
			return c.handleArchive(ctx)
		case "mira_clear_memory":
			return c.handleClearMemory(ctx, arguments)
		default:
			return nil, fmt.Errorf("unknown tool: %s", name)
		}
	})
}

func (c *Controller) handleStore(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	content, ok := args["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content is required")
	}

	if utf8.RuneCountInString(content) > MaxContentLength {
		return nil, fmt.Errorf("content exceeds maximum length of %d characters", MaxContentLength)
	}

	wing, ok := args["wing"].(string)
	if !ok || strings.TrimSpace(wing) == "" {
		return nil, fmt.Errorf("wing is required")
	}

	if utf8.RuneCountInString(wing) > MaxWingLength {
		return nil, fmt.Errorf("wing exceeds maximum length of %d characters", MaxWingLength)
	}

	var room *string
	if r, ok := args["room"]; ok {
		if rs, ok := r.(string); ok && rs != "" {
			if utf8.RuneCountInString(rs) > MaxRoomLength {
				return nil, fmt.Errorf("room exceeds maximum length of %d characters", MaxRoomLength)
			}
			room = &rs
		}
	}

	var memType *valueobjects.MemoryType
	if t, ok := args["type"]; ok {
		if ts, ok := t.(string); ok && ts != "" {
			mt := valueobjects.MemoryType(ts)
			if mt.IsValid() {
				memType = &mt
			}
		}
	}

	input := interactors.StoreMemoryInput{
		Content: content,
		Wing:    wing,
		Room:    room,
		Type:    memType,
	}

	output, err := c.storeMemory.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	result := fmt.Sprintf("Stored: %s\nType: %s\nFacts: %d\nTokens: %d\nModel: %s",
		output.FingerprintID, output.Type, output.FactCount, output.TokenCount, output.ModelHash)

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: result}},
	}, nil
}

func (c *Controller) handleRecall(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	query, ok := args["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query is required")
	}

	if utf8.RuneCountInString(query) > MaxQueryLength {
		return nil, fmt.Errorf("query exceeds maximum length of %d characters", MaxQueryLength)
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}

	budget := 4000
	if bArg, ok := args["budget"]; ok {
		switch v := bArg.(type) {
		case float64:
			budget = int(v)
		case int:
			budget = v
		case string:
			if bi, err := strconv.Atoi(v); err == nil {
				budget = bi
			}
		}
	}

	if budget <= 0 || budget > 100000 {
		budget = 4000
	}

	var wing, room *string
	if w, ok := args["wing"]; ok {
		if ws, ok := w.(string); ok && ws != "" {
			wing = &ws
		}
	}
	if r, ok := args["room"]; ok {
		if rs, ok := r.(string); ok && rs != "" {
			room = &rs
		}
	}

	var fallbackWings []string
	if fw, ok := args["fallback_wings"]; ok {
		if fws, ok := fw.(string); ok && fws != "" {
			fallbackWings = strings.Split(fws, ",")
			for i := range fallbackWings {
				fallbackWings[i] = strings.TrimSpace(fallbackWings[i])
			}
		}
	}

	input := interactors.RecallMemoryInput{
		Query:         query,
		Budget:        budget,
		Wing:          wing,
		Room:          room,
		FallbackWings: fallbackWings,
	}

	output, err := c.recallMemory.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	var parts []string
	totalTokens := 0

	parts = append(parts, "=== MIRA CONTEXT ===")
	parts = append(parts, fmt.Sprintf("Query: %s | Budget: %d", query, budget))
	if wing != nil {
		parts = append(parts, fmt.Sprintf("Wing: %s", *wing))
	}
	parts = append(parts, "")

	for i, sel := range output.Memories {
		parts = append(parts, fmt.Sprintf("--- [%d] %s (%d tokens) | ID: %s ---",
			i+1, sel.Mode.String(), sel.TokenCost, sel.CandidateID.String()))
		parts = append(parts, sel.Rendered)
		parts = append(parts, "")
		totalTokens += sel.TokenCost
	}

	parts = append(parts, fmt.Sprintf("=== Total: %d/%d tokens (%.1f%%) ===",
		totalTokens, budget, output.BudgetUsed))

	parts = append(parts, "")
	parts = append(parts, "INSTRUCTIONS:")
	parts = append(parts, "- HEADER: Reference only, use mira_load(id) for full content")
	parts = append(parts, "- FINGERPRINT: Essential extracted facts (informational density)")
	parts = append(parts, "- VERBATIM: Complete original content")

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: strings.Join(parts, "\n")}},
	}, nil
}

func (c *Controller) handleLoad(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	idStr, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is required")
	}

	idStr = strings.TrimPrefix(idStr, "T0:")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID: %w", err)
	}

	input := interactors.LoadMemoryInput{ID: id}
	output, err := c.loadMemory.Execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("not found: %w", err)
	}

	meta := fmt.Sprintf("[ID: %s | Wing: %s | Date: %s]\n\n",
		output.Verbatim.ID, output.Verbatim.Wing, output.Verbatim.CreatedAt.Format(time.RFC3339))

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: meta + output.Verbatim.Content}},
	}, nil
}

func (c *Controller) handleCausalChain(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	idStr, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is required")
	}

	isT0Ref := strings.HasPrefix(idStr, "T0:")
	parsedID, err := uuid.Parse(strings.TrimPrefix(idStr, "T0:"))
	if err != nil {
		return nil, fmt.Errorf("invalid ID format: %w", err)
	}

	id := parsedID
	if isT0Ref {
		if c.fingerprintRepo == nil {
			return nil, fmt.Errorf("T0 references are not supported without a repository")
		}
		fp, err := c.fingerprintRepo.GetFingerprintByVerbatimID(ctx, parsedID)
		if err != nil {
			return nil, fmt.Errorf("could not resolve T0 reference to fingerprint: %w", err)
		}
		id = fp.ID
	}

	maxDepth := 5
	if d, ok := args["max_depth"]; ok {
		switch v := d.(type) {
		case float64:
			maxDepth = int(v)
		case int:
			maxDepth = v
		}
	}

	includeConsequences := false
	if ic, ok := args["include_consequences"]; ok {
		includeConsequences, _ = ic.(bool)
	}

	input := interactors.GetCausalChainInput{
		ID:                  id,
		MaxDepth:            maxDepth,
		IncludeConsequences: includeConsequences,
	}

	output, err := c.getCausalChain.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	var parts []string
	parts = append(parts, "=== CAUSAL CHAIN (Upstream) ===")

	for i, node := range output.Chain {
		indent := strings.Repeat(" ", len(output.Chain)-1-i)
		parts = append(parts, fmt.Sprintf("%s→ [%s] %s (%s)",
			indent, node.Type, node.Summary, node.Timestamp.Format("2006-01-02")))
	}

	if len(output.Consequences) > 0 {
		parts = append(parts, "")
		parts = append(parts, "=== CONSEQUENCES (Downstream) ===")
		for i, node := range output.Consequences {
			indent := strings.Repeat(" ", i)
			parts = append(parts, fmt.Sprintf("%s→ [%s] %s",
				indent, node.Type, node.Summary))
		}
	}

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: strings.Join(parts, "\n")}},
	}, nil
}

func (c *Controller) handleStatus(ctx context.Context) (*mcptypes.CallToolResult, error) {
	output, err := c.getStatus.Execute(ctx)
	if err != nil {
		return nil, err
	}

	stats := output.Stats

	result := fmt.Sprintf(`MIRA System Status:
═══════════════════════════════════════
Storage:
  Verbatims: %d
  Fingerprints: %d
  Embeddings: %d (models: %v)
  Causal Nodes: %d
  Causal Edges: %d
  Total Tokens: %d

Memory Distribution:
  Decisions: %d
  Facts: %d
  Preferences: %d
  Session Notes: %d
  Debug Logs: %d

Active Wings: %v
═══════════════════════════════════════`,
		stats.VerbatimCount,
		stats.FingerprintCount,
		stats.EmbeddingCount,
		output.Models,
		stats.CausalNodeCount,
		stats.CausalEdgeCount,
		stats.TotalTokens,
		stats.TypeCounts["decision"],
		stats.TypeCounts["fact"],
		stats.TypeCounts["preference"],
		stats.TypeCounts["session_note"],
		stats.TypeCounts["debug_log"],
		stats.ActiveWings,
	)

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: result}},
	}, nil
}

func (c *Controller) handleTimeline(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	wing, ok := args["wing"].(string)
	if !ok {
		return nil, fmt.Errorf("wing is required")
	}

	var room, memType *string
	var since, until *string

	if r, ok := args["room"]; ok {
		if rs, ok := r.(string); ok && rs != "" {
			room = &rs
		}
	}
	if t, ok := args["type"]; ok {
		if ts, ok := t.(string); ok && ts != "" {
			memType = &ts
		}
	}
	if sArg, ok := args["since"]; ok {
		if ss, ok := sArg.(string); ok && ss != "" {
			since = &ss
		}
	}
	if u, ok := args["until"]; ok {
		if us, ok := u.(string); ok && us != "" {
			until = &us
		}
	}

	var mt *valueobjects.MemoryType
	if memType != nil {
		t := valueobjects.MemoryType(*memType)
		mt = &t
	}

	input := interactors.GetTimelineInput{
		Wing:  wing,
		Room:  room,
		Type:  mt,
		Since: since,
		Until: until,
	}

	output, err := c.getTimeline.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("=== TIMELINE: %s ===", wing))
	if room != nil {
		parts = append(parts, fmt.Sprintf("Room: %s", *room))
	}
	parts = append(parts, "")

	for _, item := range output.Items {
		parts = append(parts, fmt.Sprintf("[%s] %s: %s (ID: %s)",
			item.Timestamp, item.Type, item.Summary, item.ID))
	}

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: strings.Join(parts, "\n")}},
	}, nil
}

func (c *Controller) handleArchive(ctx context.Context) (*mcptypes.CallToolResult, error) {
	output, err := c.archiveMemories.Execute(ctx)
	if err != nil {
		return nil, err
	}

	result := fmt.Sprintf("Archiving complete:\n- Session notes > 30d: %d\n- Debug logs > 7d: %d\nTotal freed: %d tokens",
		output.Result.SessionNotes, output.Result.DebugLogs, output.Result.TokensFreed)

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: result}},
	}, nil
}

func (c *Controller) handleClearMemory(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	mode, ok := args["mode"].(string)
	if !ok || (mode != "global" && mode != "room") {
		return nil, fmt.Errorf("mode is required and must be 'global' or 'room'")
	}

	input := interactors.ClearMemoryInput{Mode: mode}

	if mode == "room" {
		wing, ok := args["wing"].(string)
		if !ok || strings.TrimSpace(wing) == "" {
			return nil, fmt.Errorf("wing is required when mode is 'room'")
		}
		input.Wing = wing

		if r, ok := args["room"]; ok {
			if rs, ok := r.(string); ok && rs != "" {
				input.Room = &rs
			}
		}
	}

	output, err := c.clearMemory.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	var result string
	if output.Mode == "global" {
		result = "All memories have been permanently deleted."
	} else {
		roomLabel := "(no room)"
		if input.Room != nil {
			roomLabel = *input.Room
		}
		result = fmt.Sprintf("Cleared %d memories in wing '%s' / room '%s'.", output.DeletedCount, input.Wing, roomLabel)
	}

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: result}},
	}, nil
}
