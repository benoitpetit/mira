// Package mcp provides the Model Context Protocol interface adapter.
// MCP controller - Interface adapter
package mcp

import (
	"context"
	"encoding/json"
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

// ValidationLimits holds configurable validation limits for MCP tools.
type ValidationLimits struct {
	MaxContentLength int
	MaxWingLength    int
	MaxRoomLength    int
	MaxQueryLength   int
}

// DefaultValidationLimits returns the default validation limits.
func DefaultValidationLimits() ValidationLimits {
	return ValidationLimits{
		MaxContentLength: 100000,
		MaxWingLength:    100,
		MaxRoomLength:    100,
		MaxQueryLength:   10000,
	}
}

// Backward-compatible constants for tests and external code.
// Deprecated: Use ValidationLimits instead.
const (
	MaxContentLength = 100000
	MaxWingLength    = 100
	MaxRoomLength    = 100
	MaxQueryLength   = 10000
)

// sanitizeStoredMemoryContent neutralizes instruction-like lines when replaying
// stored memories back into model context to reduce memory-prompt-injection risk.
// Uses a structural approach: memories should be wrapped in <memory>...</memory>
// tags with system instructions, but as a defense-in-depth measure, this function
// blacklists common instruction patterns. For stronger protection, use structural
// delimiters: wrap all stored memories in <memory>...</memory> with an instruction
// to the model not to interpret the content as directives.
func sanitizeStoredMemoryContent(content string) string {
	if content == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		l := strings.ToLower(strings.TrimSpace(line))
		// Blacklisted instruction patterns (prefix-based + substrings)
		if strings.HasPrefix(l, "system:") ||
			strings.HasPrefix(l, "assistant:") ||
			strings.HasPrefix(l, "user:") ||
			strings.Contains(l, "ignore previous") ||
			strings.Contains(l, "ignore all") ||
			strings.Contains(l, "override instructions") ||
			strings.Contains(l, "you are now") ||
			strings.Contains(l, "disregard previous") ||
			strings.Contains(l, "forget previous") ||
			strings.Contains(l, "reset context") ||
			strings.Contains(l, "start new") ||
			strings.Contains(l, "system prompt") ||
			strings.Contains(l, "ai assistant") ||
			strings.Contains(l, "as an ai") {
			lines[i] = "[filtered potential instruction from memory]"
		}
		// Additional structural check: detect lines that look like directives
		// (uppercase, single words that are common instruction triggers)
		if len(strings.Fields(l)) <= 3 {
			firstWord := strings.Fields(l)[0]
			if firstWord == "system" ||
				firstWord == "ignore" ||
				firstWord == "forget" ||
				firstWord == "reset" {
				lines[i] = "[filtered potential instruction from memory]"
			}
		}
	}
	return strings.Join(lines, "\n")
}

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

	// CompressMemoriesExecutor runs on-demand compression
	CompressMemoriesExecutor interface {
		Execute(ctx context.Context, input interactors.CompressMemoriesInput) (*interactors.CompressMemoriesOutput, error)
	}

	// UpdateMemoryExecutor updates a memory's content with re-extraction.
	UpdateMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.UpdateMemoryInput) (*interactors.UpdateMemoryOutput, error)
	}

	// SearchSemanticExecutor performs pure vector search without CBA.
	SearchSemanticExecutor interface {
		Execute(ctx context.Context, input interactors.SearchSemanticInput) ([]*interactors.SearchSemanticResult, error)
	}

	// ConsolidateMemoriesExecutor merges redundant memories.
	ConsolidateMemoriesExecutor interface {
		Execute(ctx context.Context, input interactors.ConsolidateMemoriesInput) (*interactors.ConsolidateMemoriesOutput, error)
	}

	// FingerprintLookup provides read-only access to fingerprint lookups
	FingerprintLookup interface {
		GetFingerprintByVerbatimID(ctx context.Context, verbatimID uuid.UUID) (*entities.Fingerprint, error)
	}
)

// Controller handles MCP tool calls
type Controller struct {
	storeMemory         StoreMemoryExecutor
	recallMemory        RecallMemoryExecutor
	loadMemory          LoadMemoryExecutor
	getTimeline         GetTimelineExecutor
	getStatus           GetStatusExecutor
	getCausalChain      GetCausalChainExecutor
	archiveMemories     ArchiveMemoriesExecutor
	clearMemory         ClearMemoryExecutor
	compressMemories    CompressMemoriesExecutor
	updateMemory        UpdateMemoryExecutor
	searchSemantic      SearchSemanticExecutor
	consolidateMemories ConsolidateMemoriesExecutor
	fingerprintRepo     FingerprintLookup
	limits              ValidationLimits
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
	compressMemories *interactors.CompressMemories,
	updateMemory *interactors.UpdateMemory,
	searchSemantic *interactors.SearchSemantic,
	consolidateMemories *interactors.ConsolidateMemories,
) *Controller {
	return &Controller{
		storeMemory:         storeMemory,
		recallMemory:        recallMemory,
		loadMemory:          loadMemory,
		getTimeline:         getTimeline,
		getStatus:           getStatus,
		getCausalChain:      getCausalChain,
		archiveMemories:     archiveMemories,
		clearMemory:         clearMemory,
		fingerprintRepo:     fingerprintRepo,
		compressMemories:    compressMemories,
		updateMemory:        updateMemory,
		searchSemantic:      searchSemantic,
		consolidateMemories: consolidateMemories,
		limits:              DefaultValidationLimits(),
	}
}

// NewControllerWithLimits creates a new MCP controller with custom validation limits
func NewControllerWithLimits(
	storeMemory *interactors.StoreMemory,
	recallMemory *interactors.RecallMemory,
	loadMemory *interactors.LoadMemory,
	getTimeline *interactors.GetTimeline,
	getStatus *interactors.GetStatus,
	getCausalChain *interactors.GetCausalChain,
	archiveMemories *interactors.ArchiveMemories,
	clearMemory *interactors.ClearMemory,
	fingerprintRepo FingerprintLookup,
	compressMemories *interactors.CompressMemories,
	updateMemory *interactors.UpdateMemory,
	searchSemantic *interactors.SearchSemantic,
	consolidateMemories *interactors.ConsolidateMemories,
	limits ValidationLimits,
) *Controller {
	c := NewController(
		storeMemory, recallMemory, loadMemory, getTimeline, getStatus,
		getCausalChain, archiveMemories, clearMemory, fingerprintRepo,
		compressMemories, updateMemory, searchSemantic, consolidateMemories,
	)
	c.limits = limits
	return c
}

// RegisterTools registers all MCP tools
func (c *Controller) RegisterTools(mcpServer server.MCPServer) {
	tools := c.ToolDefinitions()
	mcpServer.HandleListTools(func(ctx context.Context, cursor *string) (*mcptypes.ListToolsResult, error) {
		return &mcptypes.ListToolsResult{Tools: tools}, nil
	})
	mcpServer.HandleCallTool(func(ctx context.Context, name string, arguments map[string]interface{}) (*mcptypes.CallToolResult, error) {
		return c.Call(ctx, name, arguments)
	})
}

// ToolDefinitions returns the MIRA tool definitions.
// Used for combined registration when SOUL is embedded.
func (c *Controller) ToolDefinitions() []mcptypes.Tool {
	return []mcptypes.Tool{
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
  - kind: Business role. Values: identity|user|project|task|knowledge|history
  - valid_from / valid_until: Optional RFC3339 bounds for temporal facts

Examples:
  Store a decision:  {"content": "Use JWT tokens with RS256 for OAuth2", "wing": "auth-service", "room": "decisions", "type": "decision"}
  Store a debug log: {"content": "Fixed nil pointer in user.go:42", "wing": "api", "room": "debug", "type": "debug_log"}
  Store a fact:      {"content": "Database connection pool max is 100", "wing": "infra", "type": "fact"}`,
			InputSchema: mcptypes.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"content":     map[string]string{"type": "string", "description": "Text content to store"},
					"wing":        map[string]string{"type": "string", "description": "Namespace/project (e.g., 'auth-service')"},
					"room":        map[string]string{"type": "string", "description": "Sub-category (e.g., 'migration')"},
					"type":        map[string]string{"type": "string", "description": "Forced type: decision|fact|preference|session_note|debug_log (auto-detect if empty)"},
					"kind":        map[string]string{"type": "string", "description": "Business role: identity|user|project|task|knowledge|history"},
					"valid_from":  map[string]string{"type": "string", "description": "RFC3339 start of the fact validity interval"},
					"valid_until": map[string]string{"type": "string", "description": "RFC3339 end of the fact validity interval"},
				},
			},
		},
		{
			Name: "mira_ingest",
			Description: `Extract history memories from a structured conversation using MIRA's normal storage pipeline.

By default only substantive user messages are captured. Set include_assistant to
true to include assistant replies as well. Tool and system messages are never
captured. Every selected message is stored with kind=history and is still
processed by the usual extraction and duplicate protection pipeline.

Parameters:
  - messages: Array of {role, content} conversation messages (required)
  - wing: Namespace/project for the extracted memories (required)
  - room: Optional sub-category
  - include_assistant: Also capture assistant replies (default: false)
  - min_chars: Minimum Unicode character count per captured message (default: 20)
  - dry_run: Preview the number of selected messages without persisting (default: false)`,
			InputSchema: mcptypes.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"messages":          map[string]string{"type": "array", "description": "Conversation messages, each with role and content"},
					"wing":              map[string]string{"type": "string", "description": "Namespace/project for extracted memories"},
					"room":              map[string]string{"type": "string", "description": "Optional room/sub-category"},
					"include_assistant": map[string]string{"type": "boolean", "description": "Also capture assistant replies"},
					"min_chars":         map[string]string{"type": "number", "description": "Minimum character count (default: 20)"},
					"dry_run":           map[string]string{"type": "boolean", "description": "Preview without storing"},
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
	  - kind: Filter to a business role: identity|user|project|task|knowledge|history
  - fallback_wings: Comma-separated fallback wings to search if primary wing yields no results
  - include_global: Also search the shared "general" wing if the project has no results
  - session_id: Optional session identifier for multi-turn memory injection (boosts memories recalled in previous turns by +30%)

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
					"kind":           map[string]string{"type": "string", "description": "Filter by business role"},
					"fallback_wings": map[string]string{"type": "string", "description": "Comma-separated fallback wings to search if primary wing yields no results"},
					"include_global": map[string]string{"type": "boolean", "description": "Fall back to the shared general wing if the primary wing has no results"},
					"session_id":     map[string]string{"type": "string", "description": "Session ID for multi-turn memory injection (optional)"},
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

IMPORTANT: You must use the exact Fingerprint ID returned by mira_recall or mira_timeline.
Do not invent or guess IDs. If you only have a T0: reference, it must be a valid UUID (e.g., T0:550e8400-e29b-41d4-a716-446655440000).

Parameters:
  - id: Exact Fingerprint ID from a previous mira_recall / mira_timeline result
  - max_depth: How far back to trace (default: 5)
  - include_consequences: Also show downstream effects (children)

Examples:
  Trace decision:    {"id": "550e8400-e29b-41d4-a716-446655440000", "max_depth": 3}
  Full chain:        {"id": "550e8400-e29b-41d4-a716-446655440000", "max_depth": 10, "include_consequences": true}`,
			InputSchema: mcptypes.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"id":                   map[string]string{"type": "string", "description": "Exact Fingerprint ID from mira_recall or mira_timeline"},
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
  - limit: Max items to return (default: 100, max: 1000)
  - cursor: Pagination cursor from a previous mira_timeline call

Examples:
  Project timeline:  {"wing": "auth-service", "since": "2024-01-01"}
  Sprint decisions:  {"wing": "frontend", "room": "sprint-5", "type": "decision"}
  Recent debug:      {"wing": "api", "type": "debug_log", "since": "2024-04-01"}`,
			InputSchema: mcptypes.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"wing":   map[string]string{"type": "string", "description": "Required wing/namespace"},
					"room":   map[string]string{"type": "string", "description": "Filter by room/sub-category"},
					"since":  map[string]string{"type": "string", "description": "Start date ISO 8601 (e.g., 2024-01-15)"},
					"until":  map[string]string{"type": "string", "description": "End date ISO 8601"},
					"type":   map[string]string{"type": "string", "description": "Filter by type: decision|fact|preference|session_note|debug_log"},
					"limit":  map[string]string{"type": "number", "description": "Max items to return (default: 100, max: 1000)"},
					"cursor": map[string]string{"type": "string", "description": "Pagination cursor from previous call"},
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
		{
			Name: "mira_health",
			Description: `Quick JSON health check for MIRA system.

Returns lightweight JSON status for liveness/readiness probes:
  - status: "healthy" | "degraded"
  - db_connected: true | false
  - memory_count: Total memories stored
  - vector_index_ready: true | false

No parameters required. Use for lightweight probes versus mira_status for full stats.`,
			InputSchema: mcptypes.ToolInputSchema{
				Type:       "object",
				Properties: map[string]interface{}{},
			},
		},
		{
			Name: "mira_compress",
			Description: `Run rule-based context compression over session_note verbatims.

Generates condensed summaries (stored alongside originals) that the recall engine
surfaces automatically when the token budget is tight. No LLM required — compression
is deterministic and instant.

Parameters:
  - wing:       Limit compression to a specific wing (optional; default: all wings)
  - min_tokens: Only compress verbatims with at least this many tokens (optional; default: 100)
  - dry_run:    Count candidates without persisting summaries (optional; default: false)

Examples:
  Compress everything:       {}
  Compress one wing:         {"wing": "auth-service"}
  Preview without saving:    {"dry_run": true}
  Only long notes:           {"min_tokens": 200}`,
			InputSchema: mcptypes.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"wing":       map[string]string{"type": "string", "description": "Limit to this wing (optional)"},
					"min_tokens": map[string]string{"type": "number", "description": "Minimum token count to qualify (default: 100)"},
					"dry_run":    map[string]string{"type": "boolean", "description": "Preview without persisting (default: false)"},
				},
			},
		},
		{
			Name: "mira_update",
			Description: `Update a memory's content and regenerate its fingerprint and embedding.

The existing verbatim is replaced atomically: fingerprint, embedding, and vector
index entries are all regenerated from the new content. Use when a stored memory
needs correction or enrichment.

Parameters:
  - id:      Verbatim UUID or T0 reference (e.g., "T0:auth-123" or full UUID) (required)
  - content: New text content to replace the existing content (required)

Examples:
  Correct a fact:   {"id": "T0:auth-service-abc123", "content": "Updated: API rate limit is 5000 req/min"}
  Enrich a decision: {"id": "550e8400-e29b-41d4-a716-446655440000", "content": "Decision: Use PostgreSQL 16 for v2. Key reasons: JSONB support, partitioning, pgvector."}`,
			InputSchema: mcptypes.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"id":      map[string]string{"type": "string", "description": "Verbatim UUID or T0:xxx reference"},
					"content": map[string]string{"type": "string", "description": "New content for this memory"},
				},
			},
		},
		{
			Name: "mira_search",
			Description: `Pure vector search without CBA (Context Budget Allocation).

Returns raw semantic matches ranked by cosine similarity. Unlike mira_recall,
this does not apply session boost, causal penalties, diversity weighting, or
token budgeting — useful for diagnostics, data exploration, and building
custom selection logic.

Parameters:
  - query:     Search text (required)
  - top_k:     Maximum results to return (default: 10)
  - threshold: Minimum similarity score (0.0–1.0, default: 0.3)

Examples:
  Quick search:    {"query": "authentication JWT"}
  High precision:  {"query": "database migration plan", "threshold": 0.7, "top_k": 5}`,
			InputSchema: mcptypes.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"query":     map[string]string{"type": "string", "description": "Search text"},
					"top_k":     map[string]string{"type": "number", "description": "Max results (default: 10)"},
					"threshold": map[string]string{"type": "number", "description": "Min similarity 0.0–1.0 (default: 0.3)"},
				},
			},
		},
		{
			Name: "mira_consolidate",
			Description: `Merge redundant session notes within a wing into synthesized facts.

Scans session notes, clusters highly similar items (default threshold: 0.92),
creates a synthetic fact for each cluster, and removes the originals. Useful
for compressing accumulated session noise into concise, permanent knowledge.

Parameters:
  - wing:                  Target wing (required)
  - similarity_threshold:  Min cosine similarity to merge (0.0–1.0, default: 0.92)

Examples:
  Consolidate a wing:          {"wing": "auth-service"}
  Aggressive merge:            {"wing": "api", "similarity_threshold": 0.85}`,
			InputSchema: mcptypes.ToolInputSchema{
				Type: "object",
				Properties: map[string]interface{}{
					"wing":                 map[string]string{"type": "string", "description": "Target wing to consolidate"},
					"similarity_threshold": map[string]string{"type": "number", "description": "Min similarity to merge (0.0–1.0, default: 0.92)"},
				},
			},
		},
	}
}

// Call dispatches a mira_* tool call. Returns an error for unknown tool names.
// Used for combined registration when SOUL is embedded.
func (c *Controller) Call(ctx context.Context, name string, arguments map[string]interface{}) (*mcptypes.CallToolResult, error) {
	switch name {
	case "mira_store":
		return c.handleStore(ctx, arguments)
	case "mira_ingest":
		return c.handleIngest(ctx, arguments)
	case "mira_recall":
		return c.handleRecall(ctx, arguments)
	case "mira_load":
		return c.handleLoad(ctx, arguments)
	case "mira_causal_chain":
		return c.handleCausalChain(ctx, arguments)
	case "mira_health":
		return c.handleHealth(ctx)
	case "mira_status":
		return c.handleStatus(ctx)
	case "mira_timeline":
		return c.handleTimeline(ctx, arguments)
	case "mira_archive":
		return c.handleArchive(ctx)
	case "mira_clear_memory":
		return c.handleClearMemory(ctx, arguments)
	case "mira_compress":
		return c.handleCompress(ctx, arguments)
	case "mira_update":
		return c.handleUpdate(ctx, arguments)
	case "mira_search":
		return c.handleSearch(ctx, arguments)
	case "mira_consolidate":
		return c.handleConsolidate(ctx, arguments)
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func (c *Controller) handleIngest(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	rawMessages, ok := args["messages"]
	if !ok {
		return nil, fmt.Errorf("messages is required")
	}
	raw, err := json.Marshal(rawMessages)
	if err != nil {
		return nil, fmt.Errorf("messages must be JSON serializable: %w", err)
	}
	messages, err := interactors.ParseConversationMessages(raw)
	if err != nil {
		return nil, err
	}
	wing, ok := args["wing"].(string)
	if !ok || strings.TrimSpace(wing) == "" {
		return nil, fmt.Errorf("wing is required")
	}
	var room *string
	if value, ok := args["room"].(string); ok && strings.TrimSpace(value) != "" {
		room = &value
	}
	includeAssistant, _ := args["include_assistant"].(bool)
	minChars := 20
	if value, ok := args["min_chars"].(float64); ok {
		minChars = int(value)
	}
	inputs, err := interactors.ConversationMemoryInputs(messages, wing, room, includeAssistant, minChars)
	if err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("no messages matched the selected roles and min_chars=%d", minChars)
	}
	if dryRun, _ := args["dry_run"].(bool); dryRun {
		return mcpTextResult(fmt.Sprintf("Dry-run: %d of %d conversation messages would be extracted as history memories.", len(inputs), len(messages))), nil
	}

	stored, failed := 0, 0
	for _, input := range inputs {
		if _, err := c.storeMemory.Execute(ctx, input); err != nil {
			failed++
			continue
		}
		stored++
	}
	return mcpTextResult(fmt.Sprintf("Conversation ingest complete: %d stored, %d failed (selected: %d)", stored, failed, len(inputs))), nil
}

func mcpTextResult(text string) *mcptypes.CallToolResult {
	return &mcptypes.CallToolResult{Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: text}}}
}

func (c *Controller) handleStore(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	content, ok := args["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content is required")
	}

	if utf8.RuneCountInString(content) > c.limits.MaxContentLength {
		return nil, fmt.Errorf("content exceeds maximum length of %d characters", c.limits.MaxContentLength)
	}

	wing, ok := args["wing"].(string)
	if !ok || strings.TrimSpace(wing) == "" {
		return nil, fmt.Errorf("wing is required")
	}

	if utf8.RuneCountInString(wing) > c.limits.MaxWingLength {
		return nil, fmt.Errorf("wing exceeds maximum length of %d characters", c.limits.MaxWingLength)
	}
	if !interactors.WingRoomRe.MatchString(wing) {
		return nil, fmt.Errorf("wing must be alphanumeric, hyphens or underscores only")
	}

	var room *string
	if r, ok := args["room"]; ok {
		if rs, ok := r.(string); ok && rs != "" {
			if utf8.RuneCountInString(rs) > c.limits.MaxRoomLength {
				return nil, fmt.Errorf("room exceeds maximum length of %d characters", c.limits.MaxRoomLength)
			}
			if !interactors.WingRoomRe.MatchString(rs) {
				return nil, fmt.Errorf("room must be alphanumeric, hyphens or underscores only")
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
	var memoryKind *valueobjects.MemoryKind
	if k, ok := args["kind"]; ok {
		if ks, ok := k.(string); ok && ks != "" {
			kind := valueobjects.MemoryKind(ks)
			if !kind.IsValid() {
				return nil, fmt.Errorf("invalid kind %q; valid values: identity, user, project, task, knowledge, history", ks)
			}
			memoryKind = &kind
		}
	}

	var metrics map[string]any
	if m, ok := args["metrics"]; ok {
		if ms, ok := m.(string); ok && ms != "" {
			if err := json.Unmarshal([]byte(ms), &metrics); err != nil {
				return nil, fmt.Errorf("metrics must be valid JSON: %w", err)
			}
		}
	}
	validFrom, err := temporalArgument(args, "valid_from")
	if err != nil {
		return nil, err
	}
	validUntil, err := temporalArgument(args, "valid_until")
	if err != nil {
		return nil, err
	}

	input := interactors.StoreMemoryInput{
		Content:    content,
		Wing:       wing,
		Room:       room,
		Type:       memType,
		Kind:       memoryKind,
		Metrics:    metrics,
		ValidFrom:  validFrom,
		ValidUntil: validUntil,
	}

	output, err := c.storeMemory.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	result := fmt.Sprintf("Stored: %s\nType: %s\nKind: %s\nFacts: %d\nTokens: %d\nModel: %s",
		output.FingerprintID, output.Type, output.Kind, output.FactCount, output.TokenCount, output.ModelHash)

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: result}},
	}, nil
}

func temporalArgument(args map[string]interface{}, name string) (*time.Time, error) {
	value, ok := args[name]
	if !ok || value == nil || value == "" {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, fmt.Errorf("%s must be an RFC3339 string", name)
	}
	parsed, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return nil, fmt.Errorf("%s must be an RFC3339 timestamp: %w", name, err)
	}
	return &parsed, nil
}

func (c *Controller) handleRecall(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	query, ok := args["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query is required")
	}

	if utf8.RuneCountInString(query) > c.limits.MaxQueryLength {
		return nil, fmt.Errorf("query exceeds maximum length of %d characters", c.limits.MaxQueryLength)
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
	var memoryKind *valueobjects.MemoryKind
	if k, ok := args["kind"]; ok {
		if ks, ok := k.(string); ok && ks != "" {
			kind := valueobjects.MemoryKind(ks)
			if !kind.IsValid() {
				return nil, fmt.Errorf("invalid kind %q; valid values: identity, user, project, task, knowledge, history", ks)
			}
			memoryKind = &kind
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
	if includeGlobal, ok := args["include_global"].(bool); ok && includeGlobal {
		if wing == nil || *wing != "general" {
			fallbackWings = append(fallbackWings, "general")
		}
	}

	var sessionID *string
	if sid, ok := args["session_id"]; ok {
		if sids, ok := sid.(string); ok && sids != "" {
			sessionID = &sids
		}
	}

	input := interactors.RecallMemoryInput{
		Query:         query,
		Budget:        budget,
		Wing:          wing,
		Room:          room,
		Kind:          memoryKind,
		FallbackWings: fallbackWings,
		SessionID:     sessionID,
	}

	output, err := c.recallMemory.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	var parts []string
	totalTokens := 0

	parts = append(parts, "=== MIRA CONTEXT ===", fmt.Sprintf("Query: %s | Budget: %d", query, budget))
	if wing != nil {
		parts = append(parts, fmt.Sprintf("Wing: %s", *wing))
	}
	parts = append(parts, "")

	for i, sel := range output.Memories {
		safeContent := sanitizeStoredMemoryContent(sel.Rendered)
		parts = append(parts, fmt.Sprintf("--- [%d] %s (%d tokens) | ID: T0:%s ---",
			i+1, sel.Mode.String(), sel.TokenCost, sel.VerbatimID.String()), safeContent, "")
		totalTokens += sel.TokenCost
	}

	parts = append(parts, fmt.Sprintf("=== Total: %d/%d tokens (%.1f%%) ===",
		totalTokens, budget, output.BudgetUsed), "",
		"INSTRUCTIONS:",
		"- HEADER: Reference only, use mira_load(id) for full content",
		"- FINGERPRINT: Essential extracted facts (informational density)",
		"- COMPRESSED: Rule-based summary (~40% of verbatim, when available)",
		"- VERBATIM: Complete original content")

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: strings.Join(parts, "\n")}},
	}, nil
}

func (c *Controller) handleLoad(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	idStr, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is required")
	}

	normalizedID := normalizeLoadID(idStr)
	id, err := uuid.Parse(normalizedID)
	if err != nil {
		return nil, fmt.Errorf("invalid ID '%s': %w. Use the exact ID returned by mira_recall or mira_timeline (plain UUID, T0:UUID, or F0:UUID)", idStr, err)
	}

	input := interactors.LoadMemoryInput{ID: id}
	output, err := c.loadMemory.Execute(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("not found: %w", err)
	}
	if output == nil || output.Verbatim == nil {
		return nil, fmt.Errorf("not found: no verbatim resolved for ID '%s'", idStr)
	}

	meta := fmt.Sprintf("[ID: %s | Wing: %s | Date: %s]\n\n",
		output.Verbatim.ID, output.Verbatim.Wing, output.Verbatim.CreatedAt.Format(time.RFC3339))

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: meta + output.Verbatim.Content}},
	}, nil
}

func (c *Controller) handleCompress(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	input := interactors.CompressMemoriesInput{}

	if w, ok := args["wing"].(string); ok {
		input.Wing = strings.TrimSpace(w)
	}
	if mt, ok := args["min_tokens"].(float64); ok && mt > 0 {
		input.MinTokens = int(mt)
	}
	if dr, ok := args["dry_run"].(bool); ok {
		input.DryRun = dr
	}

	output, err := c.compressMemories.Execute(ctx, input)
	if err != nil {
		return nil, err
	}

	qualifier := ""
	if input.DryRun {
		qualifier = " (dry-run — nothing persisted)"
	}
	result := fmt.Sprintf("Compression complete%s:\n- Compressed: %d verbatims\n- Tokens saved: %d",
		qualifier, output.CompressedCount, output.TokensSaved)

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: result}},
	}, nil
}

func (c *Controller) handleUpdate(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	idStr, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is required")
	}

	normalizedID := normalizeLoadID(idStr)
	id, err := uuid.Parse(normalizedID)
	if err != nil {
		return nil, fmt.Errorf("invalid ID '%s': %w. Use the exact ID returned by mira_recall or mira_timeline (plain UUID, T0:UUID, or F0:UUID)", idStr, err)
	}

	content, ok := args["content"].(string)
	if !ok || strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("content is required and cannot be empty")
	}

	if utf8.RuneCountInString(content) > c.limits.MaxContentLength {
		return nil, fmt.Errorf("content exceeds maximum length of %d characters", c.limits.MaxContentLength)
	}

	output, err := c.updateMemory.Execute(ctx, interactors.UpdateMemoryInput{
		ID:      id,
		Content: content,
	})
	if err != nil {
		return nil, fmt.Errorf("update failed: %w", err)
	}

	result := fmt.Sprintf("Updated: %s\nNew content stored with regenerated fingerprint and embedding.", output.Verbatim.ID)
	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: result}},
	}, nil
}

func (c *Controller) handleSearch(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	query, ok := args["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}

	if utf8.RuneCountInString(query) > c.limits.MaxQueryLength {
		return nil, fmt.Errorf("query exceeds maximum length of %d characters", c.limits.MaxQueryLength)
	}

	topK := 10
	if t, ok := args["top_k"]; ok {
		switch v := t.(type) {
		case float64:
			topK = int(v)
		case int:
			topK = v
		}
	}
	if topK <= 0 {
		topK = 10
	}
	if topK > 100 {
		topK = 100
	}

	threshold := 0.3
	if th, ok := args["threshold"]; ok {
		if v, ok := th.(float64); ok {
			threshold = v
		}
	}
	if threshold < 0 {
		threshold = 0
	}
	if threshold > 1 {
		threshold = 1
	}

	results, err := c.searchSemantic.Execute(ctx, interactors.SearchSemanticInput{
		Query:     query,
		TopK:      topK,
		Threshold: threshold,
	})
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		return &mcptypes.CallToolResult{
			Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: "No results found."}},
		}, nil
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("=== VECTOR SEARCH RESULTS (%d) ===", len(results)),
		fmt.Sprintf("Query: %s | Threshold: %.2f", query, threshold), "")
	for i, r := range results {
		safeContent := sanitizeStoredMemoryContent(r.Content)
		if len(safeContent) > 200 {
			safeContent = safeContent[:200] + "..."
		}
		parts = append(parts, fmt.Sprintf("[%d] T0:%s (%.3f) [%s] wing=%s",
			i+1, r.ID, r.Similarity, r.Type, r.Wing), safeContent, "")
	}

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: strings.Join(parts, "\n")}},
	}, nil
}

func (c *Controller) handleConsolidate(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	wing, ok := args["wing"].(string)
	if !ok || strings.TrimSpace(wing) == "" {
		return nil, fmt.Errorf("wing is required")
	}

	threshold := 0.92
	if th, ok := args["similarity_threshold"]; ok {
		if v, ok := th.(float64); ok {
			threshold = v
		}
	}
	if threshold <= 0 || threshold > 1 {
		threshold = 0.92
	}

	output, err := c.consolidateMemories.Execute(ctx, interactors.ConsolidateMemoriesInput{
		Wing:                wing,
		SimilarityThreshold: threshold,
	})
	if err != nil {
		return nil, fmt.Errorf("consolidation failed: %w", err)
	}

	result := fmt.Sprintf("Consolidation complete for wing '%s':\n- Consolidated clusters: %d\n- Original notes removed: %d",
		wing, output.ConsolidatedCount, output.RemovedCount)
	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: result}},
	}, nil
}

func normalizeLoadID(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimSpace(strings.TrimPrefix(s, "ID:"))

	upper := strings.ToUpper(s)
	for _, prefix := range []string{"T0:", "F0:", "V0:", "FP:"} {
		if strings.HasPrefix(upper, prefix) {
			return strings.TrimSpace(s[len(prefix):])
		}
	}

	return s
}

func (c *Controller) handleCausalChain(ctx context.Context, args map[string]interface{}) (*mcptypes.CallToolResult, error) {
	idStr, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is required")
	}

	isT0Ref := strings.HasPrefix(idStr, "T0:")
	refBody := strings.TrimPrefix(idStr, "T0:")
	parsedID, err := uuid.Parse(refBody)
	if err != nil {
		return nil, fmt.Errorf("invalid ID '%s': %w. Use the exact Fingerprint ID returned by mira_recall or mira_timeline. Do not invent T0: references.", idStr, err)
	}

	id := parsedID
	if isT0Ref {
		if c.fingerprintRepo == nil {
			return nil, fmt.Errorf("T0 references are not supported without a repository")
		}
		fp, err := c.fingerprintRepo.GetFingerprintByVerbatimID(ctx, parsedID)
		if err != nil {
			return nil, fmt.Errorf("could not resolve T0 reference '%s' to a fingerprint: %w. Use the exact Fingerprint ID from mira_recall or mira_timeline", idStr, err)
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
		parts = append(parts, "", "=== CONSEQUENCES (Downstream) ===")
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

func (c *Controller) handleHealth(ctx context.Context) (*mcptypes.CallToolResult, error) {
	output, err := c.getStatus.Execute(ctx)
	if err != nil {
		return &mcptypes.CallToolResult{
			Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: `{"status":"degraded","db_connected":false}`}},
		}, nil
	}

	status := "healthy"
	if output.Stats.VerbatimCount == 0 {
		status = "healthy (empty)"
	}

	result := fmt.Sprintf(`{"status":"%s","db_connected":true,"memory_count":%d}`, //nolint:gocritic // JSON value, %q would break output
		status, output.Stats.VerbatimCount)

	return &mcptypes.CallToolResult{
		Content: []mcptypes.Content{mcptypes.TextContent{Type: "text", Text: result}},
	}, nil
}

func (c *Controller) handleStatus(ctx context.Context) (*mcptypes.CallToolResult, error) {
	output, err := c.getStatus.Execute(ctx)
	if err != nil {
		return nil, err
	}

	stats := output.Stats

	result := fmt.Sprintf(`MIRA System Status
═══════════════════════════════════════
Version: %s
Uptime: %s

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
		output.Version,
		output.Uptime,
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
	var since, until, cursor *string
	limit := 100

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
	if c, ok := args["cursor"]; ok {
		if cs, ok := c.(string); ok && cs != "" {
			cursor = &cs
		}
	}
	if l, ok := args["limit"]; ok {
		switch v := l.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		}
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	var mt *valueobjects.MemoryType
	if memType != nil {
		t := valueobjects.MemoryType(*memType)
		mt = &t
	}

	input := interactors.GetTimelineInput{
		Wing:   wing,
		Room:   room,
		Type:   mt,
		Since:  since,
		Until:  until,
		Limit:  limit,
		Cursor: cursor,
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
		parts = append(parts, fmt.Sprintf("[%s] %s: %s (ID: T0:%s)",
			item.Timestamp, item.Type, item.Summary, item.ID))
	}

	if output.NextCursor != nil {
		parts = append(parts, "", fmt.Sprintf("next_cursor: %s", *output.NextCursor))
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
