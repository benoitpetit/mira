package mcp

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/benoitpetit/mira/budget"
	"github.com/benoitpetit/mira/causal"
	"github.com/benoitpetit/mira/extract"
	"github.com/benoitpetit/mira/store"
	"github.com/benoitpetit/mira/types"
)

// Server encapsulates MCP server
type Server struct {
	store     *store.Store
	allocator *budget.Allocator
	extractor *extract.Extractor
	causal    *causal.Graph
}

// NewServer creates a new MCP server
func NewServer(store *store.Store, allocator *budget.Allocator, extractor *extract.Extractor, causal *causal.Graph) *Server {
	return &Server{
		store:     store,
		allocator: allocator,
		extractor: extractor,
		causal:    causal,
	}
}

// RegisterTools registers all MCP tools
func (s *Server) RegisterTools(mcpServer server.MCPServer) {
	// Configure tool list handler
	mcpServer.HandleListTools(func(ctx context.Context, cursor *string) (*mcp.ListToolsResult, error) {
		tools := []mcp.Tool{
			{
				Name:        "mira_store",
				Description: "Store a memory in MIRA with deterministic extraction",
				InputSchema: mcp.ToolInputSchema{
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
				InputSchema: mcp.ToolInputSchema{
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
				InputSchema: mcp.ToolInputSchema{
					Type: "object",
					Properties: map[string]interface{}{
						"id": map[string]string{"type": "string", "description": "Verbatim UUID or T0:xxx reference"},
					},
				},
			},
			{
				Name:        "mira_causal_chain",
				Description: "Trace back causal chain of a decision or event",
				InputSchema: mcp.ToolInputSchema{
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
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}{},
				},
			},
			{
				Name:        "mira_timeline",
				Description: "Filtered chronological reconstruction",
				InputSchema: mcp.ToolInputSchema{
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
				InputSchema: mcp.ToolInputSchema{
					Type:       "object",
					Properties: map[string]interface{}{},
				},
			},
		}
		return &mcp.ListToolsResult{Tools: tools}, nil
	})

	// Configure tool call handler
	mcpServer.HandleCallTool(func(ctx context.Context, name string, arguments map[string]interface{}) (*mcp.CallToolResult, error) {
		switch name {
		case "mira_store":
			return s.handleStore(arguments)
		case "mira_recall":
			return s.handleRecall(arguments)
		case "mira_load":
			return s.handleLoad(arguments)
		case "mira_causal_chain":
			return s.handleCausalChain(arguments)
		case "mira_status":
			return s.handleStatus()
		case "mira_timeline":
			return s.handleTimeline(arguments)
		case "mira_archive":
			return s.handleArchive()
		default:
			return nil, fmt.Errorf("unknown tool: %s", name)
		}
	})
}

func (s *Server) handleStore(args map[string]interface{}) (*mcp.CallToolResult, error) {
	content, ok := args["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content is required")
	}
	wing, ok := args["wing"].(string)
	if !ok {
		return nil, fmt.Errorf("wing is required")
	}

	var room *string
	if r, ok := args["room"]; ok && r.(string) != "" {
		rs := r.(string)
		room = &rs
	}

	verbatim := &types.Verbatim{
		ID:        uuid.New(),
		Content:   content,
		Wing:      wing,
		Room:      room,
		CreatedAt: time.Now(),
	}

	// Pipeline extraction T0 → T1, T2
	fp, emb, err := s.extractor.ExtractPipeline(verbatim)
	if err != nil {
		return nil, fmt.Errorf("extraction failed: %w", err)
	}

	// Transactional storage
	tx, err := s.store.BeginTx()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := s.store.StoreVerbatimTx(tx, verbatim); err != nil {
		return nil, err
	}
	if err := s.store.StoreFingerprintTx(tx, fp); err != nil {
		return nil, err
	}
	if err := s.store.StoreEmbeddingTx(tx, emb); err != nil {
		return nil, err
	}

	// Create causal node
	node := s.causal.CreateCausalNodeFromFingerprint(fp, wing, room)
	if err := s.causal.AddNodeTx(tx, node); err != nil {
		fmt.Printf("Warning: failed to add causal node: %v\n", err)
	}

	// Causal detection
	recentFps, err := s.causal.GetRecentForCausalDetection(wing, fp.ID, 50)
	if err != nil {
		fmt.Printf("Warning: causal detection fetch failed: %v\n", err)
	}

	if len(recentFps) > 0 {
		edges, err := s.extractor.DetectCausalRelations(fp, recentFps, content)
		if err != nil {
			fmt.Printf("Warning: causal detection failed: %v\n", err)
		} else {
			for _, e := range edges {
				if err := s.causal.AddEdgeTx(tx, e); err != nil {
					fmt.Printf("Warning: failed to add edge %s->%s: %v\n", e.FromID, e.ToID, err)
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	result := fmt.Sprintf("Stored: %s\nType: %s\nFacts: %d\nTokens: %d\nModel: %s",
		fp.ID, fp.Type, fp.FactCount, verbatim.TokenCount, fp.ModelHash)

	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: result}},
	}, nil
}

func (s *Server) handleRecall(args map[string]interface{}) (*mcp.CallToolResult, error) {
	query, ok := args["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query is required")
	}

	b := budget.DefaultBudget
	if bArg, ok := args["budget"]; ok {
		switch v := bArg.(type) {
		case float64:
			b = int(v)
		case int:
			b = v
		case string:
			if bi, err := strconv.Atoi(v); err == nil {
				b = bi
			}
		}
	}

	var wing, room *string
	if w, ok := args["wing"]; ok && w.(string) != "" {
		ws := w.(string)
		wing = &ws
	}
	if r, ok := args["room"]; ok && r.(string) != "" {
		rs := r.(string)
		room = &rs
	}

	selected, err := s.allocator.Allocate(query, b, wing, room)
	if err != nil {
		return nil, fmt.Errorf("allocation failed: %w", err)
	}

	var parts []string
	totalTokens := 0

	parts = append(parts, "=== MIRA CONTEXT ===")
	parts = append(parts, fmt.Sprintf("Query: %s | Budget: %d", query, b))
	if wing != nil {
		parts = append(parts, fmt.Sprintf("Wing: %s", *wing))
	}
	parts = append(parts, "")

	for i, sel := range selected {
		parts = append(parts, fmt.Sprintf("--- [%d] %s (%d tokens) ---",
			i+1, modeString(sel.Mode), sel.TokenCost))
		parts = append(parts, sel.Rendered)
		parts = append(parts, "")
		totalTokens += sel.TokenCost
	}

	parts = append(parts, fmt.Sprintf("=== Total: %d/%d tokens (%.1f%%) ===",
		totalTokens, b, float64(totalTokens)/float64(b)*100))

	parts = append(parts, "")
	parts = append(parts, "INSTRUCTIONS:")
	parts = append(parts, "- HEADER: Reference only, use mira_load(id) for full content")
	parts = append(parts, "- FINGERPRINT: Essential extracted facts (informational density)")
	parts = append(parts, "- VERBATIM: Complete original content")

	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: strings.Join(parts, "\n")}},
	}, nil
}

func (s *Server) handleLoad(args map[string]interface{}) (*mcp.CallToolResult, error) {
	idStr, ok := args["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id is required")
	}

	idStr = strings.TrimPrefix(idStr, "T0:")
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID: %w", err)
	}

	verbatim, err := s.store.GetVerbatim(id)
	if err != nil {
		return nil, fmt.Errorf("not found: %w", err)
	}

	meta := fmt.Sprintf("[ID: %s | Wing: %s | Date: %s]\n\n",
		verbatim.ID, verbatim.Wing, verbatim.CreatedAt.Format(time.RFC3339))

	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: meta + verbatim.Content}},
	}, nil
}

func (s *Server) handleCausalChain(args map[string]interface{}) (*mcp.CallToolResult, error) {
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

	chain, err := s.causal.GetChain(id, maxDepth)
	if err != nil {
		return nil, err
	}

	var parts []string
	parts = append(parts, "=== CAUSAL CHAIN (Upstream) ===")

	for i, node := range chain {
		indent := strings.Repeat(" ", len(chain)-1-i)
		parts = append(parts, fmt.Sprintf("%s→ [%s] %s (%s)",
			indent, node.Type, node.Summary, node.Timestamp.Format("2006-01-02")))
	}

	if includeConsequences {
		consequences, err := s.causal.GetConsequences(id, maxDepth)
		if err == nil && len(consequences) > 0 {
			parts = append(parts, "")
			parts = append(parts, "=== CONSEQUENCES (Downstream) ===")
			for i, node := range consequences {
				indent := strings.Repeat(" ", i)
				parts = append(parts, fmt.Sprintf("%s→ [%s] %s",
					indent, node.Type, node.Summary))
			}
		}
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: strings.Join(parts, "\n")}},
	}, nil
}

func (s *Server) handleStatus() (*mcp.CallToolResult, error) {
	stats, err := s.store.GetStats()
	if err != nil {
		return nil, err
	}

	models, err := s.store.GetEmbeddingModels()
	if err != nil {
		models = []string{"unknown"}
	}

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
		models,
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

	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: result}},
	}, nil
}

func (s *Server) handleTimeline(args map[string]interface{}) (*mcp.CallToolResult, error) {
	wing, ok := args["wing"].(string)
	if !ok {
		return nil, fmt.Errorf("wing is required")
	}

	var room, memType *string
	var since, until *time.Time

	if r, ok := args["room"]; ok && r.(string) != "" {
		rs := r.(string)
		room = &rs
	}
	if t, ok := args["type"]; ok && t.(string) != "" {
		ts := t.(string)
		memType = &ts
	}
	if s, ok := args["since"]; ok && s.(string) != "" {
		if t, err := time.Parse(time.RFC3339, s.(string)); err == nil {
			since = &t
		}
	}
	if u, ok := args["until"]; ok && u.(string) != "" {
		if t, err := time.Parse(time.RFC3339, u.(string)); err == nil {
			until = &t
		}
	}

	items, err := s.store.GetTimeline(wing, room, memType, since, until)
	if err != nil {
		return nil, err
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("=== TIMELINE: %s ===", wing))
	if room != nil {
		parts = append(parts, fmt.Sprintf("Room: %s", *room))
	}
	parts = append(parts, "")

	for _, item := range items {
		parts = append(parts, fmt.Sprintf("[%s] %s: %s",
			item.Timestamp.Format("2006-01-02 15:04"),
			item.Type,
			item.Summary))
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: strings.Join(parts, "\n")}},
	}, nil
}

func (s *Server) handleArchive() (*mcp.CallToolResult, error) {
	archived, err := s.store.ArchiveOldMemories()
	if err != nil {
		return nil, err
	}

	result := fmt.Sprintf("Archiving complete:\n- Session notes > 30d: %d\n- Debug logs > 7d: %d\nTotal freed: %d tokens",
		archived.SessionNotes, archived.DebugLogs, archived.TokensFreed)

	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: result}},
	}, nil
}

func modeString(m types.RenderMode) string {
	switch m {
	case types.ModeHeader:
		return "HEADER"
	case types.ModeFingerprint:
		return "FINGERPRINT"
	case types.ModeVerbatim:
		return "VERBATIM"
	default:
		return "UNKNOWN"
	}
}
