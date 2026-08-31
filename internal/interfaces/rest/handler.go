// Package rest provides the optional HTTP REST API interface adapter.
package rest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/benoitpetit/mira/internal/domain/valueobjects"
	"github.com/benoitpetit/mira/internal/usecases/interactors"
	"github.com/benoitpetit/mira/internal/usecases/ports"
	"github.com/google/uuid"
)

// ── Executor interfaces ───────────────────────────────────────────────────────

type (
	// StoreMemoryExecutor stores a memory.
	StoreMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.StoreMemoryInput) (*interactors.StoreMemoryOutput, error)
	}

	// RecallMemoryExecutor recalls memories.
	RecallMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.RecallMemoryInput) (*interactors.RecallMemoryOutput, error)
	}

	// LoadMemoryExecutor loads a single memory by ID.
	LoadMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.LoadMemoryInput) (*interactors.LoadMemoryOutput, error)
	}

	// UpdateMemoryExecutor updates a memory's content.
	UpdateMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.UpdateMemoryInput) (*interactors.UpdateMemoryOutput, error)
	}

	// DeleteMemoryExecutor deletes a memory by ID.
	DeleteMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.DeleteMemoryInput) error
	}

	// SearchSemanticExecutor performs pure vector search.
	SearchSemanticExecutor interface {
		Execute(ctx context.Context, input interactors.SearchSemanticInput) ([]*interactors.SearchSemanticResult, error)
	}

	// ConsolidateMemoriesExecutor consolidates redundant memories.
	ConsolidateMemoriesExecutor interface {
		Execute(ctx context.Context, input interactors.ConsolidateMemoriesInput) (*interactors.ConsolidateMemoriesOutput, error)
	}

	// ClearMemoryExecutor clears memories by mode.
	ClearMemoryExecutor interface {
		Execute(ctx context.Context, input interactors.ClearMemoryInput) (*interactors.ClearMemoryOutput, error)
	}

	// GetTimelineExecutor returns timeline events.
	GetTimelineExecutor interface {
		Execute(ctx context.Context, input interactors.GetTimelineInput) (*interactors.GetTimelineOutput, error)
	}

	// ArchiveMemoriesExecutor archives old memories.
	ArchiveMemoriesExecutor interface {
		Execute(ctx context.Context) (*interactors.ArchiveMemoriesOutput, error)
	}

	// GetCausalChainExecutor returns the causal chain for a memory.
	GetCausalChainExecutor interface {
		Execute(ctx context.Context, input interactors.GetCausalChainInput) (*interactors.GetCausalChainOutput, error)
	}

	// GetStatusExecutor returns system status.
	GetStatusExecutor interface {
		Execute(ctx context.Context) (*interactors.GetStatusOutput, error)
	}

	// SoulStatusQuerier provides SOUL identity data for the status endpoint.
	// The handler field is nil when SOUL is disabled — checked before each call.
	SoulStatusQuerier interface {
		QueryStatus(ctx context.Context) (*interactors.SoulStatusSummary, error)
	}
)

// ── Handler ───────────────────────────────────────────────────────────────────

// Handler is the REST API controller.
type Handler struct {
	store       StoreMemoryExecutor
	recall      RecallMemoryExecutor
	load        LoadMemoryExecutor
	update      UpdateMemoryExecutor
	del         DeleteMemoryExecutor
	search      SearchSemanticExecutor
	consolidate ConsolidateMemoriesExecutor
	clear       ClearMemoryExecutor
	timeline    GetTimelineExecutor
	archive     ArchiveMemoriesExecutor
	causal      GetCausalChainExecutor
	status      GetStatusExecutor
	audit       ports.AuditRepository
	policy      ports.PolicyRepository
	soulStatus  SoulStatusQuerier // nil when SOUL is disabled
}

// NewHandler creates a Handler with all dependencies wired.
func NewHandler(
	store StoreMemoryExecutor,
	recall RecallMemoryExecutor,
	load LoadMemoryExecutor,
	update UpdateMemoryExecutor,
	del DeleteMemoryExecutor,
	search SearchSemanticExecutor,
	consolidate ConsolidateMemoriesExecutor,
	clear ClearMemoryExecutor,
	timeline GetTimelineExecutor,
	archive ArchiveMemoriesExecutor,
	causal GetCausalChainExecutor,
	status GetStatusExecutor,
	audit ports.AuditRepository,
	policy ports.PolicyRepository,
) *Handler {
	return &Handler{
		store:       store,
		recall:      recall,
		load:        load,
		update:      update,
		del:         del,
		search:      search,
		consolidate: consolidate,
		clear:       clear,
		timeline:    timeline,
		archive:     archive,
		causal:      causal,
		status:      status,
		audit:       audit,
		policy:      policy,
	}
}

// SetSoulQuerier injects an optional SOUL status provider.
// Call this after NewHandler when SOUL is enabled; passing nil is a no-op.
func (h *Handler) SetSoulQuerier(q SoulStatusQuerier) {
	if q != nil {
		h.soulStatus = q
	}
}

// RegisterRoutes mounts all routes on mux using Go 1.22+ method+path patterns.
// More-specific fixed paths (e.g. /recall, /search) are registered before the
// wildcard {id} pattern so they take precedence automatically.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Fixed sub-paths first (must beat the {id} wildcard)
	mux.HandleFunc("POST /api/v1/memories/recall", h.handleRecall)
	mux.HandleFunc("POST /api/v1/memories/search", h.handleSearch)
	mux.HandleFunc("POST /api/v1/memories/consolidate", h.handleConsolidate)

	// CRUD
	mux.HandleFunc("POST /api/v1/memories", h.handleStore)
	mux.HandleFunc("GET /api/v1/memories/{id}", h.handleLoad)
	mux.HandleFunc("PUT /api/v1/memories/{id}", h.handleUpdate)
	mux.HandleFunc("DELETE /api/v1/memories/{id}", h.handleDelete)
	mux.HandleFunc("DELETE /api/v1/memories", h.handleClear)

	// Other resources
	mux.HandleFunc("GET /api/v1/timeline", h.handleTimeline)
	mux.HandleFunc("POST /api/v1/archive", h.handleArchive)
	mux.HandleFunc("GET /api/v1/causal/{id}", h.handleCausal)
	mux.HandleFunc("GET /api/v1/status", h.handleStatus)
	mux.HandleFunc("GET /openapi.json", ServeSpec)
}

// ── JSON helpers ──────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("rest: json encode error", "error", err)
	}
}

type errorBody struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

func parseUUID(s string) (uuid.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid UUID %q: %w", s, err)
	}
	return id, nil
}

// ── Handlers ──────────────────────────────────────────────────────────────────

// POST /api/v1/memories
func (h *Handler) handleStore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string         `json:"content"`
		Wing    string         `json:"wing"`
		Room    *string        `json:"room"`
		Type    *string        `json:"type"`
		Metrics map[string]any `json:"metrics"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	input := interactors.StoreMemoryInput{
		Content: body.Content,
		Wing:    body.Wing,
		Room:    body.Room,
		Metrics: body.Metrics,
	}
	if body.Type != nil {
		mt := valueobjects.MemoryType(*body.Type)
		input.Type = &mt
	}

	if err := input.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	out, err := h.store.Execute(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

// GET /api/v1/memories/{id}
func (h *Handler) handleLoad(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	out, err := h.load.Execute(r.Context(), interactors.LoadMemoryInput{ID: id})
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "memory not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, out.Verbatim)
}

// PUT /api/v1/memories/{id}
func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Content == "" {
		writeError(w, http.StatusUnprocessableEntity, "content is required")
		return
	}

	out, err := h.update.Execute(r.Context(), interactors.UpdateMemoryInput{
		ID:      id,
		Content: body.Content,
	})
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "memory not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, out.Verbatim)
}

// DELETE /api/v1/memories/{id}
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.del.Execute(r.Context(), interactors.DeleteMemoryInput{ID: id}); err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "memory not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/memories/recall
func (h *Handler) handleRecall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query         string   `json:"query"`
		Budget        int      `json:"budget"`
		Wing          *string  `json:"wing"`
		Room          *string  `json:"room"`
		FallbackWings []string `json:"fallback_wings"`
		SessionID     *string  `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Query == "" {
		writeError(w, http.StatusUnprocessableEntity, "query is required")
		return
	}

	out, err := h.recall.Execute(r.Context(), interactors.RecallMemoryInput{
		Query:         body.Query,
		Budget:        body.Budget,
		Wing:          body.Wing,
		Room:          body.Room,
		FallbackWings: body.FallbackWings,
		SessionID:     body.SessionID,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/v1/memories/search
func (h *Handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Query     string  `json:"query"`
		TopK      int     `json:"top_k"`
		Threshold float64 `json:"threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Query == "" {
		writeError(w, http.StatusUnprocessableEntity, "query is required")
		return
	}

	results, err := h.search.Execute(r.Context(), interactors.SearchSemanticInput{
		Query:     body.Query,
		TopK:      body.TopK,
		Threshold: body.Threshold,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if results == nil {
		results = []*interactors.SearchSemanticResult{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// POST /api/v1/memories/consolidate
func (h *Handler) handleConsolidate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Wing                string  `json:"wing"`
		SimilarityThreshold float64 `json:"similarity_threshold"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Wing == "" {
		writeError(w, http.StatusUnprocessableEntity, "wing is required")
		return
	}

	out, err := h.consolidate.Execute(r.Context(), interactors.ConsolidateMemoriesInput{
		Wing:                body.Wing,
		SimilarityThreshold: body.SimilarityThreshold,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// DELETE /api/v1/memories  (body: {"mode":"all|wing|room","wing":"...","room":"..."})
func (h *Handler) handleClear(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string  `json:"mode"`
		Wing string  `json:"wing"`
		Room *string `json:"room"`
	}
	// Allow empty body → mode "all"
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Mode == "" {
		body.Mode = "all"
	}

	out, err := h.clear.Execute(r.Context(), interactors.ClearMemoryInput{
		Mode: body.Mode,
		Wing: body.Wing,
		Room: body.Room,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/v1/timeline?wing=...&room=...&type=...&since=...&until=...&limit=...&cursor=...
func (h *Handler) handleTimeline(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	input := interactors.GetTimelineInput{
		Wing: q.Get("wing"),
	}
	if v := q.Get("room"); v != "" {
		input.Room = &v
	}
	if v := q.Get("type"); v != "" {
		mt := valueobjects.MemoryType(v)
		input.Type = &mt
	}
	if v := q.Get("since"); v != "" {
		input.Since = &v
	}
	if v := q.Get("until"); v != "" {
		input.Until = &v
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			input.Limit = n
		}
	}
	if input.Limit <= 0 {
		input.Limit = 100
	}
	if input.Limit > 1000 {
		input.Limit = 1000
	}
	if v := q.Get("cursor"); v != "" {
		input.Cursor = &v
	}

	out, err := h.timeline.Execute(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/v1/archive
func (h *Handler) handleArchive(w http.ResponseWriter, r *http.Request) {
	out, err := h.archive.Execute(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/v1/causal/{id}?max_depth=5&include_consequences=true
func (h *Handler) handleCausal(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	q := r.URL.Query()
	input := interactors.GetCausalChainInput{
		ID:       id,
		MaxDepth: 10,
	}
	if v := q.Get("max_depth"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			input.MaxDepth = n
		}
	}
	if v := q.Get("include_consequences"); v == "true" || v == "1" {
		input.IncludeConsequences = true
	}

	out, err := h.causal.Execute(r.Context(), input)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "memory not found")
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/v1/status
func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	out, err := h.status.Execute(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.soulStatus != nil {
		if soul, err := h.soulStatus.QueryStatus(r.Context()); err == nil {
			out.Soul = soul
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// ── helpers ───────────────────────────────────────────────────────────────────

// isNotFound returns true for "not found" errors emitted by the repositories.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var notFound interface{ IsNotFound() bool }
	if errors.As(err, &notFound) {
		return notFound.IsNotFound()
	}
	return false
}

// NewServer builds an *http.Server pre-configured with the Handler and
// the middleware chain (recovery → logging → optional auth).
// masterToken is the full-access bearer token (empty = disabled).
// wingTokens maps per-token bearer strings to the set of wings they may access.
func NewServer(h *Handler, addr, masterToken string, wingTokens map[string][]string, readTimeout, writeTimeout time.Duration) *http.Server {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	ServeDashboard(mux)

	var rootHandler http.Handler = mux
	if h.audit != nil {
		rootHandler = auditMiddleware(h.audit, rootHandler)
	}
	if masterToken != "" || len(wingTokens) > 0 || h.policy != nil {
		rootHandler = authMiddleware(masterToken, wingTokens, h.policy, rootHandler)
	}
	rootHandler = rateLimitMiddleware(100, 1*time.Minute, rootHandler) // 100 requests per minute per IP
	rootHandler = loggingMiddleware(rootHandler)
	rootHandler = recoveryMiddleware(rootHandler)

	return &http.Server{
		Addr:         addr,
		Handler:      rootHandler,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
	}
}
