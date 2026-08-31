package rest

import (
	"encoding/json"
	"net/http"
)

// spec is the OpenAPI 3.1 document, built once at init time.
var spec []byte

func init() {
	doc := buildSpec()
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		panic("rest: failed to marshal OpenAPI spec: " + err.Error())
	}
	spec = b
}

// ServeSpec handles GET /openapi.json.
func ServeSpec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(spec)
}

// ── OpenAPI types ─────────────────────────────────────────────────────────────

type oaDocument struct {
	OpenAPI    string                 `json:"openapi"`
	Info       oaInfo                 `json:"info"`
	Servers    []oaServer             `json:"servers"`
	Paths      map[string]oaPathItem  `json:"paths"`
	Components oaComponents           `json:"components"`
}

type oaInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type oaServer struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type oaPathItem struct {
	Get    *oaOperation `json:"get,omitempty"`
	Post   *oaOperation `json:"post,omitempty"`
	Put    *oaOperation `json:"put,omitempty"`
	Delete *oaOperation `json:"delete,omitempty"`
}

type oaOperation struct {
	Summary     string               `json:"summary"`
	OperationID string               `json:"operationId"`
	Tags        []string             `json:"tags,omitempty"`
	Parameters  []oaParameter        `json:"parameters,omitempty"`
	RequestBody *oaRequestBody       `json:"requestBody,omitempty"`
	Responses   map[string]oaResponse `json:"responses"`
}

type oaParameter struct {
	Name        string   `json:"name"`
	In          string   `json:"in"`
	Description string   `json:"description,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Schema      oaSchema `json:"schema"`
}

type oaRequestBody struct {
	Required bool               `json:"required"`
	Content  map[string]oaMedia `json:"content"`
}

type oaMedia struct {
	Schema oaSchema `json:"schema"`
}

type oaResponse struct {
	Description string             `json:"description"`
	Content     map[string]oaMedia `json:"content,omitempty"`
}

type oaSchema struct {
	Ref        string              `json:"$ref,omitempty"`
	Type       string              `json:"type,omitempty"`
	Format     string              `json:"format,omitempty"`
	Properties map[string]oaSchema `json:"properties,omitempty"`
	Items      *oaSchema           `json:"items,omitempty"`
	Required   []string            `json:"required,omitempty"`
}

type oaComponents struct {
	Schemas map[string]oaSchema `json:"schemas"`
}

// ── helpers ───────────────────────────────────────────────────────────────────

func ref(name string) oaSchema { return oaSchema{Ref: "#/components/schemas/" + name} }

func jsonBody(schemaRef string) *oaRequestBody {
	return &oaRequestBody{
		Required: true,
		Content:  map[string]oaMedia{"application/json": {Schema: ref(schemaRef)}},
	}
}

func jsonResp(desc, schemaRef string) oaResponse {
	return oaResponse{
		Description: desc,
		Content:     map[string]oaMedia{"application/json": {Schema: ref(schemaRef)}},
	}
}

func errResp(desc string) oaResponse {
	return oaResponse{
		Description: desc,
		Content: map[string]oaMedia{
			"application/json": {Schema: ref("Error")},
		},
	}
}

// ── spec builder ──────────────────────────────────────────────────────────────

func buildSpec() oaDocument {
	tags := func(t ...string) []string { return t }

	return oaDocument{
		OpenAPI: "3.1.0",
		Info: oaInfo{
			Title:       "MIRA REST API",
			Description: "HTTP REST interface for the MIRA memory management system.",
			Version:     "1.0.0",
		},
		Servers: []oaServer{
			{URL: "/", Description: "Current host"},
		},
		Paths: map[string]oaPathItem{
			"/api/v1/memories": {
				Post: &oaOperation{
					Summary:     "Store a memory",
					OperationID: "storeMemory",
					Tags:        tags("memories"),
					RequestBody: jsonBody("StoreMemoryRequest"),
					Responses: map[string]oaResponse{
						"201": jsonResp("Memory stored", "StoreMemoryResponse"),
						"400": errResp("Bad request"),
						"422": errResp("Validation error"),
						"500": errResp("Internal error"),
					},
				},
				Delete: &oaOperation{
					Summary:     "Clear memories",
					OperationID: "clearMemories",
					Tags:        tags("memories"),
					RequestBody: jsonBody("ClearMemoriesRequest"),
					Responses: map[string]oaResponse{
						"200": jsonResp("Cleared", "ClearMemoriesResponse"),
						"500": errResp("Internal error"),
					},
				},
			},
			"/api/v1/memories/{id}": {
				Get: &oaOperation{
					Summary:     "Load a memory by ID",
					OperationID: "loadMemory",
					Tags:        tags("memories"),
					Parameters: []oaParameter{
						{Name: "id", In: "path", Required: true, Description: "Memory UUID", Schema: oaSchema{Type: "string", Format: "uuid"}},
					},
					Responses: map[string]oaResponse{
						"200": jsonResp("Memory found", "Verbatim"),
						"400": errResp("Bad request"),
						"404": errResp("Not found"),
						"500": errResp("Internal error"),
					},
				},
				Put: &oaOperation{
					Summary:     "Update a memory's content",
					OperationID: "updateMemory",
					Tags:        tags("memories"),
					Parameters: []oaParameter{
						{Name: "id", In: "path", Required: true, Description: "Memory UUID", Schema: oaSchema{Type: "string", Format: "uuid"}},
					},
					RequestBody: jsonBody("UpdateMemoryRequest"),
					Responses: map[string]oaResponse{
						"200": jsonResp("Updated", "Verbatim"),
						"400": errResp("Bad request"),
						"404": errResp("Not found"),
						"422": errResp("Validation error"),
						"500": errResp("Internal error"),
					},
				},
				Delete: &oaOperation{
					Summary:     "Delete a memory by ID",
					OperationID: "deleteMemory",
					Tags:        tags("memories"),
					Parameters: []oaParameter{
						{Name: "id", In: "path", Required: true, Description: "Memory UUID", Schema: oaSchema{Type: "string", Format: "uuid"}},
					},
					Responses: map[string]oaResponse{
						"204": {Description: "Deleted"},
						"400": errResp("Bad request"),
						"404": errResp("Not found"),
						"500": errResp("Internal error"),
					},
				},
			},
			"/api/v1/memories/recall": {
				Post: &oaOperation{
					Summary:     "Recall memories for a query",
					OperationID: "recallMemories",
					Tags:        tags("memories"),
					RequestBody: jsonBody("RecallRequest"),
					Responses: map[string]oaResponse{
						"200": jsonResp("Recalled memories", "RecallResponse"),
						"400": errResp("Bad request"),
						"422": errResp("Validation error"),
						"500": errResp("Internal error"),
					},
				},
			},
			"/api/v1/memories/search": {
				Post: &oaOperation{
					Summary:     "Semantic vector search",
					OperationID: "searchMemories",
					Tags:        tags("memories"),
					RequestBody: jsonBody("SearchRequest"),
					Responses: map[string]oaResponse{
						"200": jsonResp("Search results", "SearchResponse"),
						"400": errResp("Bad request"),
						"422": errResp("Validation error"),
						"500": errResp("Internal error"),
					},
				},
			},
			"/api/v1/memories/consolidate": {
				Post: &oaOperation{
					Summary:     "Consolidate redundant memories",
					OperationID: "consolidateMemories",
					Tags:        tags("memories"),
					RequestBody: jsonBody("ConsolidateRequest"),
					Responses: map[string]oaResponse{
						"200": jsonResp("Consolidation result", "ConsolidateResponse"),
						"400": errResp("Bad request"),
						"422": errResp("Validation error"),
						"500": errResp("Internal error"),
					},
				},
			},
			"/api/v1/timeline": {
				Get: &oaOperation{
					Summary:     "Get memory timeline",
					OperationID: "getTimeline",
					Tags:        tags("timeline"),
					Parameters: []oaParameter{
						{Name: "wing", In: "query", Schema: oaSchema{Type: "string"}},
						{Name: "room", In: "query", Schema: oaSchema{Type: "string"}},
						{Name: "type", In: "query", Schema: oaSchema{Type: "string"}},
						{Name: "since", In: "query", Schema: oaSchema{Type: "string"}},
						{Name: "until", In: "query", Schema: oaSchema{Type: "string"}},
						{Name: "limit", In: "query", Schema: oaSchema{Type: "integer"}},
						{Name: "cursor", In: "query", Schema: oaSchema{Type: "string"}},
					},
					Responses: map[string]oaResponse{
						"200": jsonResp("Timeline items", "TimelineResponse"),
						"500": errResp("Internal error"),
					},
				},
			},
			"/api/v1/archive": {
				Post: &oaOperation{
					Summary:     "Archive old memories",
					OperationID: "archiveMemories",
					Tags:        tags("maintenance"),
					Responses: map[string]oaResponse{
						"200": jsonResp("Archive result", "ArchiveResponse"),
						"500": errResp("Internal error"),
					},
				},
			},
			"/api/v1/causal/{id}": {
				Get: &oaOperation{
					Summary:     "Get causal chain for a memory",
					OperationID: "getCausalChain",
					Tags:        tags("causal"),
					Parameters: []oaParameter{
						{Name: "id", In: "path", Required: true, Description: "Memory UUID", Schema: oaSchema{Type: "string", Format: "uuid"}},
						{Name: "max_depth", In: "query", Schema: oaSchema{Type: "integer"}},
						{Name: "include_consequences", In: "query", Schema: oaSchema{Type: "boolean"}},
					},
					Responses: map[string]oaResponse{
						"200": jsonResp("Causal chain", "CausalChainResponse"),
						"400": errResp("Bad request"),
						"404": errResp("Not found"),
						"500": errResp("Internal error"),
					},
				},
			},
			"/api/v1/status": {
				Get: &oaOperation{
					Summary:     "Get system status",
					OperationID: "getStatus",
					Tags:        tags("system"),
					Responses: map[string]oaResponse{
						"200": jsonResp("System status", "StatusResponse"),
						"500": errResp("Internal error"),
					},
				},
			},
		},
		Components: oaComponents{
			Schemas: buildSchemas(),
		},
	}
}

func buildSchemas() map[string]oaSchema {
	str := func() oaSchema { return oaSchema{Type: "string"} }
	integer := func() oaSchema { return oaSchema{Type: "integer"} }
	number := func() oaSchema { return oaSchema{Type: "number"} }
	uuid := func() oaSchema { return oaSchema{Type: "string", Format: "uuid"} }
	arr := func(item oaSchema) oaSchema { return oaSchema{Type: "array", Items: &item} }

	return map[string]oaSchema{
		"Error": {
			Type:       "object",
			Properties: map[string]oaSchema{"error": str()},
			Required:   []string{"error"},
		},
		"Verbatim": {
			Type: "object",
			Properties: map[string]oaSchema{
				"id":          uuid(),
				"content":     str(),
				"wing":        str(),
				"room":        str(),
				"token_count": integer(),
				"created_at":  str(),
			},
		},
		"StoreMemoryRequest": {
			Type: "object",
			Properties: map[string]oaSchema{
				"content": str(),
				"wing":    str(),
				"room":    str(),
				"type":    str(),
			},
			Required: []string{"content", "wing"},
		},
		"StoreMemoryResponse": {
			Type: "object",
			Properties: map[string]oaSchema{
				"fingerprint_id": str(),
				"type":           str(),
				"fact_count":     integer(),
				"token_count":    integer(),
				"model_hash":     str(),
			},
		},
		"UpdateMemoryRequest": {
			Type:       "object",
			Properties: map[string]oaSchema{"content": str()},
			Required:   []string{"content"},
		},
		"RecallRequest": {
			Type: "object",
			Properties: map[string]oaSchema{
				"query":          str(),
				"budget":         integer(),
				"wing":           str(),
				"room":           str(),
				"fallback_wings": arr(str()),
				"session_id":     str(),
			},
			Required: []string{"query"},
		},
		"RecallResponse": {
			Type: "object",
			Properties: map[string]oaSchema{
				"memories":     arr(ref("SelectedMemory")),
				"total_tokens": integer(),
				"budget_used":  number(),
			},
		},
		"SelectedMemory": {
			Type: "object",
			Properties: map[string]oaSchema{
				"candidate_id": uuid(),
				"verbatim_id":  uuid(),
				"mode":         str(),
				"token_cost":   integer(),
				"rendered":     str(),
				"selected_at":  str(),
			},
		},
		"SearchRequest": {
			Type: "object",
			Properties: map[string]oaSchema{
				"query":     str(),
				"top_k":     integer(),
				"threshold": number(),
			},
			Required: []string{"query"},
		},
		"SearchResponse": {
			Type:       "object",
			Properties: map[string]oaSchema{"results": arr(ref("SearchResult"))},
		},
		"SearchResult": {
			Type: "object",
			Properties: map[string]oaSchema{
				"id":         uuid(),
				"content":    str(),
				"similarity": number(),
				"type":       str(),
				"wing":       str(),
				"room":       str(),
			},
		},
		"ConsolidateRequest": {
			Type: "object",
			Properties: map[string]oaSchema{
				"wing":                 str(),
				"similarity_threshold": number(),
			},
			Required: []string{"wing"},
		},
		"ConsolidateResponse": {
			Type: "object",
			Properties: map[string]oaSchema{
				"consolidated_count": integer(),
				"removed_count":      integer(),
			},
		},
		"ClearMemoriesRequest": {
			Type: "object",
			Properties: map[string]oaSchema{
				"mode": str(),
				"wing": str(),
				"room": str(),
			},
		},
		"ClearMemoriesResponse": {
			Type: "object",
			Properties: map[string]oaSchema{
				"deleted_count": integer(),
				"mode":          str(),
			},
		},
		"TimelineResponse": {
			Type: "object",
			Properties: map[string]oaSchema{
				"items":       arr(ref("TimelineItem")),
				"next_cursor": str(),
			},
		},
		"TimelineItem": {
			Type: "object",
			Properties: map[string]oaSchema{
				"id":        str(),
				"timestamp": str(),
				"type":      str(),
				"summary":   str(),
			},
		},
		"ArchiveResponse": {
			Type: "object",
			Properties: map[string]oaSchema{
				"result": ref("ArchiveResult"),
			},
		},
		"ArchiveResult": {
			Type: "object",
			Properties: map[string]oaSchema{
				"session_notes": integer(),
				"debug_logs":    integer(),
				"tokens_freed":  integer(),
			},
		},
		"CausalChainResponse": {
			Type: "object",
			Properties: map[string]oaSchema{
				"chain":        arr(ref("CausalNode")),
				"consequences": arr(ref("CausalNode")),
			},
		},
		"CausalNode": {
			Type: "object",
			Properties: map[string]oaSchema{
				"id":      uuid(),
				"content": str(),
				"type":    str(),
			},
		},
		"StatusResponse": {
			Type: "object",
			Properties: map[string]oaSchema{
				"stats":   ref("Stats"),
				"models":  arr(str()),
				"version": str(),
				"uptime":  str(),
			},
		},
		"Stats": {
			Type: "object",
			Properties: map[string]oaSchema{
				"verbatim_count":    integer(),
				"fingerprint_count": integer(),
				"embedding_count":   integer(),
				"causal_node_count": integer(),
				"causal_edge_count": integer(),
				"total_tokens":      integer(),
				"type_counts":       {Type: "object"},
				"active_wings":      arr(str()),
			},
		},
	}
}
