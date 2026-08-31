package rest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/benoitpetit/mira/internal/domain/entities"
	"github.com/benoitpetit/mira/internal/usecases/ports"
)

// rateLimiter tracks request rates per IP for token bucket limiting.
type rateLimiter struct {
	mu        sync.Mutex
	interval  time.Duration
	limit       int // max requests per interval
	intervals map[string][]time.Time // IP -> recent request timestamps
}

// newRateLimiter creates a new rate limiter with the given limit and interval.
func newRateLimiter(limit int, interval time.Duration) *rateLimiter {
	return &rateLimiter{
		interval:  interval,
		limit:     limit,
		intervals: make(map[string][]time.Time),
	}
}

// allowRequest checks if a request from the given IP is within the rate limit.
// Returns true if allowed, false if rate limited.
func (rl *rateLimiter) allowRequest(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	// Clean up old timestamps outside the interval
	timestamps := rl.intervals[ip]
	valid := timestamps[:0]
	for _, t := range timestamps {
		if now.Sub(t) < rl.interval {
			valid = append(valid, t)
		}
	}
	rl.intervals[ip] = valid

	// Check if under the limit
	if len(valid) >= rl.limit {
		rl.intervals[ip] = valid
		return false
	}

	// Add current timestamp
	rl.intervals[ip] = append(valid, now)
	return true
}

// rateLimitMiddleware enforces per-IP rate limiting.
func rateLimitMiddleware(limit int, interval time.Duration, next http.Handler) http.Handler {
	rl := newRateLimiter(limit, interval)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		// If behind a proxy, extract real IP from X-Forwarded-For
		if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
			ip = strings.Split(forwardedFor, ",")[0]
		}

		if !rl.allowRequest(ip) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── Context keys ──────────────────────────────────────────────────────────────

type ctxKey int

const (
	// CtxKeyWing is the context key under which the resolved wing name is stored
	// after successful wing-token authentication (e.g. "read", "write", …).
	CtxKeyWing ctxKey = iota
)

// ── Wing definitions ──────────────────────────────────────────────────────────

// Defined wings and the (method, path-prefix) pairs they cover.
// The public wing is not a real token wing — it covers /openapi.json and is
// always allowed regardless of auth.
const (
	WingRead   = "read"
	WingWrite  = "write"
	WingDelete = "delete"
	WingAdmin  = "admin"
)

// wingForRequest returns the wing name for a given HTTP method + path.
// Returns "" for public paths that are always allowed (/openapi.json).
func wingForRequest(method, path string) string {
	// Always-public endpoint
	if path == "/openapi.json" {
		return ""
	}

	switch {
	// read wing
	case method == http.MethodGet:
		return WingRead
	case method == http.MethodPost && (strings.HasSuffix(path, "/recall") || strings.HasSuffix(path, "/search")):
		return WingRead

	// write wing
	case method == http.MethodPost && strings.HasPrefix(path, "/api/v1/memories"):
		// POST /api/v1/memories (store) — not recall/search/consolidate
		return WingWrite
	case method == http.MethodPut:
		return WingWrite

	// delete wing
	case method == http.MethodDelete:
		return WingDelete

	// admin wing
	case method == http.MethodPost && strings.HasSuffix(path, "/consolidate"):
		return WingAdmin
	case method == http.MethodPost && strings.HasPrefix(path, "/api/v1/archive"):
		return WingAdmin
	}

	// Default fallback: treat as write (conservative)
	return WingWrite
}

// tokenHasWing reports whether tokenWings contains the required wing.
func tokenHasWing(tokenWings []string, required string) bool {
	for _, w := range tokenWings {
		if w == required {
			return true
		}
	}
	return false
}

// ── Middleware ────────────────────────────────────────────────────────────────

// recoveryMiddleware catches panics and returns 500 instead of crashing.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("rest: panic recovered",
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// loggingMiddleware logs every request with method, path, status and latency.
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		slog.Info("rest",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"latency_ms", time.Since(start).Milliseconds(),
		)
	})
}

// authMiddleware enforces Bearer token authentication.
//
// Access rules (evaluated in order):
//  1. /openapi.json is always public.
//  2. If masterToken is non-empty and the request carries it → full access.
//  3. If PolicyRepository is provided, the token hash is looked up in DB.
//  4. If wingTokens is non-nil, the token is looked up in static config.
//  5. If masterToken is empty AND wingTokens is nil/empty AND repo is nil → no auth, allow all.
//  6. Otherwise → 401 Unauthorized.
func authMiddleware(masterToken string, wingTokens map[string][]string, repo ports.PolicyRepository, next http.Handler) http.Handler {
	noAuth := masterToken == "" && len(wingTokens) == 0 && repo == nil
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always-public endpoint
		if r.URL.Path == "/openapi.json" {
			next.ServeHTTP(w, r)
			return
		}

		// No auth configured at all — allow everything
		if noAuth {
			next.ServeHTTP(w, r)
			return
		}

		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")

		// Master token → full access
		if masterToken != "" && bearer == masterToken {
			next.ServeHTTP(w, r)
			return
		}

		// 1. Check PolicyRepository (DB)
		if repo != nil && bearer != "" {
			hash := sha256.Sum256([]byte(bearer))
			hashStr := hex.EncodeToString(hash[:])
			policy, err := repo.GetPolicyByTokenHash(r.Context(), hashStr)
			if err == nil {
				required := wingForRequest(r.Method, r.URL.Path)
				if required == "" || policy.HasWing(required) {
					ctx := context.WithValue(r.Context(), CtxKeyWing, required)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
				writeError(w, http.StatusForbidden, "insufficient permissions for this wing (db policy)")
				return
			}
		}

		// 2. Check static wingTokens (Config)
		if wings, ok := wingTokens[bearer]; ok {
			required := wingForRequest(r.Method, r.URL.Path)
			if required == "" || tokenHasWing(wings, required) {
				ctx := context.WithValue(r.Context(), CtxKeyWing, required)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			// Token exists but lacks the required wing
			writeError(w, http.StatusForbidden, "insufficient permissions for this wing (static config)")
			return
		}

		writeError(w, http.StatusUnauthorized, "unauthorized")
	})
}

// auditMiddleware logs operations to the AuditRepository.
func auditMiddleware(repo ports.AuditRepository, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only audit API paths
		if !strings.HasPrefix(r.URL.Path, "/api/v1") {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)

		// Extract actor from context (set by authMiddleware) or Authorization header
		actor := "anonymous"
		if wing, ok := r.Context().Value(CtxKeyWing).(string); ok {
			actor = "wing:" + wing
		} else if bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "); bearer != "" {
			// Use a short hash of the token if available to avoid logging secrets
			if len(bearer) > 8 {
				actor = "token:" + bearer[:8] + "..."
			} else {
				actor = "token:present"
			}
		}

		action := r.Method + " " + r.URL.Path
		resource := r.PathValue("id")
		if resource == "" {
			resource = r.URL.Query().Get("wing") // Fallback to wing for timeline etc.
		}

		metadata := map[string]any{
			"latency_ms": time.Since(start).Milliseconds(),
			"query":      r.URL.RawQuery,
			"ip":         r.RemoteAddr,
		}
		metaJSON, _ := json.Marshal(metadata)

		logEntry := &entities.AuditLog{
			Timestamp: start,
			Action:    action,
			Actor:     actor,
			Resource:  resource,
			Status:    rw.status,
			Metadata:  string(metaJSON),
		}

		// Save asynchronously to not block the response
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := repo.SaveAuditLog(ctx, logEntry); err != nil {
				slog.Error("audit: failed to save log", "error", err)
			}
		}()
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.status = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}
