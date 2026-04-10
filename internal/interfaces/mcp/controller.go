// MCP controller - Interface adapter
package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
) *Controller {
	return &Controller{
		storeMemory:     storeMemory,
		recallMemory:    recallMemory,
		loadMemory:      loadMemory,
		getTimeline:     getTimeline,
		getStatus:       getStatus,
		getCausalChain:  getCausalChain,
		archiveMemories: archiveMemories,
	}
}

// RegisterTools registers all MCP tools
func (c *Controller) RegisterTools(mcpServer server.MCPServer) {
	mcpServer.HandleListTools(func(ctx context.Context, cursor *string) (*mcptypes.ListToolsResult, error) {
		tools := []mcptypes.Tool{
			{
				Name:        "mira_store",
				Description: "Store a memory in MIRA with deterministic extraction",
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
				Name:        "mira_recall",
				Description: "Retrieve optimal context for a query with budget",
				InputSchema: mcptypes.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"query":  map[string]string{"type": "string", "description": "Query/search"},
						"budget": map[string]string{"type": "number", "description": "Token budget (default: 4000)"},
						"wing":   map[string]string{"type": "string", "description": "Filter by wing"},
						"room":   map[string]string{"type": "string", "description": "Filter by room"},
					},
				},
			},
			{
				Name:        "mira_load",
				Description: "Load complete verbatim by ID (T0)",
				InputSchema: mcptypes.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"id": map[string]string{"type": "string", "description": "Verbatim UUID or T0:xxx reference"},
					},
				},
			},
			{
				Name:        "mira_causal_chain",
				Description: "Trace back causal chain of a decision or event",
				InputSchema: mcptypes.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"id":                   map[string]string{"type": "string", "description": "Fingerprint ID"},
						"max_depth":            map[string]string{"type": "number", "description": "Max depth (default: 5)"},
						"include_consequences": map[string]string{"type": "boolean", "description": "Include consequences (children)"},
					},
				},
			},
			{
				Name:        "mira_status",
				Description: "MIRA system stats and health",
				InputSchema: mcptypes.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}{},
				},
			},
			{
				Name:        "mira_timeline",
				Description: "Filtered chronological reconstruction",
				InputSchema: mcptypes.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"wing":  map[string]string{"type": "string", "description": "Required wing"},
						"room":  map[string]string{"type": "string", "description": "Filter by room"},
						"since": map[string]string{"type": "string", "description": "Start date ISO 8601"},
						"until": map[string]string{"type": "string", "description": "End date ISO 8601"},
						"type":  map[string]string{"type": "string", "description": "Filter by type"},
					},
				},
			},
			{
				Name:        "mira_archive",
				Description: "Archive and clean old memories according to decay rates",
				InputSchema: mcptypes.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}{},
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

	input := interactors.RecallMemoryInput{
		Query:  query,
		Budget: budget,
		Wing:   wing,
		Room:   room,
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
		parts = append(parts, fmt.Sprintf("--- [%d] %s (%d tokens) ---",
			i+1, sel.Mode.String(), sel.TokenCost))
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

	id, err := uuid.Parse(strings.TrimPrefix(idStr, "T0:"))
	if err != nil {
		return nil, err
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
		parts = append(parts, fmt.Sprintf("[%s] %s: %s",
			item.Timestamp, item.Type, item.Summary))
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
