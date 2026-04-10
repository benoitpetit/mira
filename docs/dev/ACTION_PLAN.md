# 🎯 Plan d'Action - Amélioration Projet MIRA

**Basé sur l'analyse**: PROJECT_ANALYSIS.md  
**Score actuel**: 70/100  
**Objectif**: 85/100  
**Durée estimée**: 3-4 semaines

---

## 📋 Vue d'Ensemble

```
PHASE 1: Fondations (Semaine 1)          ████████░░░░░░░░░░░░  40%
PHASE 2: Tests & Qualité (Semaine 2)     ██████████████░░░░░░  70%
PHASE 3: Robustesse (Semaine 3)          ████████████████░░░░  80%
PHASE 4: Optimisations (Semaine 4)       ████████████████████ 100%
```

---

## 🔴 PHASE 1: Fondations - Tests Critiques (Semaine 1)

### Objectif: Augmenter la couverture de 55.9% à 65%

### 1.1 Tests pour `internal/app` (Priorité: CRITIQUE)
**Fichier**: `internal/app/main_test.go` (créer)

```go
// Tests à implémenter:
- TestNewApplication          // Test création complète
- TestNewApplicationFromConfig // Test chargement config
- TestApplicationClose        // Test cleanup
- TestApplicationRun          // Test démarrage serveur
- TestDependencyInjection     // Test wiring
```

**Estimation**: 1 jour  
**Impact**: +5% couverture

### 1.2 Tests pour `internal/interfaces/mcp` (Priorité: CRITIQUE)
**Fichier**: `internal/interfaces/mcp/controller_test.go` (compléter)

```go
// Tests à ajouter:
- TestHandleStoreSuccess      // Test store complet
- TestHandleRecallSuccess     // Test recall complet
- TestHandleCausalChain       // Test graphe causal
- TestHandleTimeline          // Test timeline
- TestHandleStatus            // Test status
- TestHandleArchive           // Test archivage
```

**Estimation**: 1.5 jours  
**Impact**: +8% couverture

### 1.3 Tests pour `internal/adapters/vector` (Priorité: HAUTE)
**Fichiers**:
- `hnsw_store_test.go` (compléter)
- `sqlite_vector_store_test.go` (créer)
- `sqlite_overlap_cache_test.go` (créer)

```go
// Tests HNSW:
- TestHNSWStoreSaveLoad       // Test persistance
- TestHNSWStoreSearch         // Test recherche
- TestHNSWStoreAddDelete      // Test modifications

// Tests SQLiteVectorStore:
- TestSQLiteVectorStoreSearch // Test recherche
- TestSQLiteVectorStoreAdd    // Test ajout

// Tests OverlapCache:
- TestOverlapCacheGetSet      // Test cache
- TestOverlapCacheTTL         // Test expiration
```

**Estimation**: 1.5 jours  
**Impact**: +6% couverture

### 1.4 Nettoyage staticcheck (Priorité: MOYENNE)
**Fichiers à corriger**:
- `internal/adapters/async/processor.go:45` - champ `workers` inutilisé
- `internal/interfaces/mcp/controller_test.go:12` - fonction `getTextContent` inutilisée
- `internal/usecases/interactors/recall_memory_test.go:820` - fonction `createTestCandidate` inutilisée

**Estimation**: 0.5 jour

---

## 🟠 PHASE 2: Qualité & Architecture (Semaine 2)

### Objectif: Améliorer qualité du code et couverture à 70%

### 2.1 Ajout de context.Context (Priorité: HAUTE)
**Impact**: Tous les use cases et adapters

```go
// Modifier les signatures:
// AVANT:
func (uc *StoreMemory) Execute(input StoreMemoryInput) (*StoreMemoryOutput, error)

// APRÈS:
func (uc *StoreMemory) Execute(ctx context.Context, input StoreMemoryInput) (*StoreMemoryOutput, error)
```

**Fichiers à modifier**:
- Tous les interactors (7 fichiers)
- Tous les adapters (repository, vector store, etc.)
- Le contrôleur MCP

**Estimation**: 2 jours

### 2.2 Refactoring Interface Extractor (Priorité: MOYENNE)
**Problème**: Interface trop large (4 responsabilités)

```go
// AVANT:
type Extractor interface {
    ExtractPipeline(verbatim *entities.Verbatim, forcedType *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error)
    Encode(text string) ([]float32, error)
    ModelHash() string
    DetectCausalRelations(newFp, existing *entities.Fingerprint, content string) ([]*entities.CausalEdge, error)
}

// APRÈS:
type FingerprintExtractor interface {
    ExtractPipeline(verbatim *entities.Verbatim, forcedType *valueobjects.MemoryType) (*entities.Fingerprint, *entities.Embedding, error)
    ModelHash() string
}

type CausalRelationDetector interface {
    DetectCausalRelations(newFp, existing *entities.Fingerprint, content string) ([]*entities.CausalEdge, error)
}
```

**Estimation**: 1 jour

### 2.3 Correction DIP HNSWStore (Priorité: MOYENNE)
**Problème**: HNSWStore dépend directement de SQLiteRepository

```go
// Créer interface:
type EmbeddingSource interface {
    GetEmbeddingWithFingerprint(id uuid.UUID) (*entities.Embedding, *entities.Fingerprint, *entities.Verbatim, error)
    GetAllEmbeddings() ([]*entities.Embedding, error)
}

// Modifier HNSWStore:
type HNSWStore struct {
    source    EmbeddingSource  // Au lieu de *storage.SQLiteRepository
    // ...
}
```

**Estimation**: 1 jour

### 2.4 Tests des Use Cases Mineurs (Priorité: MOYENNE)
**Fichiers**:
- `get_causal_chain_test.go` (créer)
- `get_status_test.go` (créer)
- `get_timeline_test.go` (créer)
- `load_memory_test.go` (créer)
- `archive_memories_test.go` (créer)

**Estimation**: 1 jour  
**Impact**: +4% couverture

---

## 🟡 PHASE 3: Robustesse & Fiabilité (Semaine 3)

### Objectif: Corriger l'async processing et ajouter de la résilience

### 3.1 Correction Async Processing (Priorité: CRITIQUE)
**Options**:

**Option A: Implémenter correctement**
```go
// Modifier SimpleAsyncManager pour injecter dépendances
type SimpleAsyncManager struct {
    extractor  ports.Extractor      // AJOUTER
    repository ports.Repository     // AJOUTER
    // ...
}

func (m *SimpleAsyncManager) processJob(job *Job) {
    // Implémenter réellement l'extraction T1/T2
    verbatim := job.Payload.ToVerbatim()
    fp, emb, err := m.extractor.ExtractPipeline(verbatim, nil)
    // ...
}
```

**Option B: Retirer le code (recommandé si non utilisé)**
- Supprimer `internal/adapters/async`
- Retirer de la config
- Simplifier l'architecture

**Estimation**: 
- Option A: 2 jours
- Option B: 0.5 jour

### 3.2 Retry Logic (Priorité: MOYENNE)
**Fichier**: `internal/util/retry.go` (créer)

```go
package util

import (
    "context"
    "time"
)

// RetryConfig configures retry behavior
type RetryConfig struct {
    MaxAttempts int
    Delay       time.Duration
    MaxDelay    time.Duration
    Multiplier  float64
}

// Retry executes the given function with exponential backoff
func Retry(ctx context.Context, config RetryConfig, fn func() error) error {
    // Implémentation
}
```

**Utilisation**:
- Repository operations
- Webhook calls
- Vector store operations

**Estimation**: 1 jour

### 3.3 Circuit Breaker (Priorité: MOYENNE)
**Fichier**: `internal/util/circuit_breaker.go` (créer)

```go
package util

// CircuitBreaker prevents cascade failures
type CircuitBreaker struct {
    failureThreshold int
    successThreshold int
    timeout          time.Duration
    state            State
}

func (cb *CircuitBreaker) Execute(fn func() error) error {
    // Implémentation
}
```

**Utilisation**: Webhook manager

**Estimation**: 1 jour

### 3.4 HMAC Webhooks (Priorité: MOYENNE)
**Fichier**: `internal/adapters/webhook/manager.go`

```go
func (m *SimpleWebhookManager) sendWebhook(endpoint *WebhookEndpoint, event WebhookEvent) error {
    // ...
    if endpoint.Secret != "" {
        signature := computeHMAC(payload, endpoint.Secret)
        req.Header.Set("X-MIRA-Signature", "sha256="+signature)
    }
    // ...
}

func computeHMAC(payload []byte, secret string) string {
    mac := hmac.New(sha256.New, []byte(secret))
    mac.Write(payload)
    return hex.EncodeToString(mac.Sum(nil))
}
```

**Estimation**: 0.5 jour

---

## 🟢 PHASE 4: Optimisations & Documentation (Semaine 4)

### Objectif: Optimisations performances et documentation

### 4.1 Optimisation Calcul Overlap (Priorité: MOYENNE)
**Problème**: Complexité O(n²) dans selectGreedy

**Solution**: Lazy evaluation
```go
func (uc *RecallMemory) selectGreedy(candidates []*entities.Candidate, budget int) []*valueobjects.SelectedMemory {
    // Ne calculer l'overlap que si le score initial est prometteur
    if c.Score < uc.earlyPruningThreshold {
        continue // Skip overlap calculation
    }
    // Compute overlap only for promising candidates
}
```

**Estimation**: 0.5 jour

### 4.2 Persistance Complète HNSW (Priorité: BASSE)
**Fichier**: `internal/adapters/vector/hnsw_store.go`

```go
// Sauvegarder aussi le graphe HNSW (pas seulement les mappings)
func (h *HNSWStore) Save() error {
    // Sauvegarder le graphe avec gob
    // ...
}

func (h *HNSWStore) Load() error {
    // Charger le graphe si existe
    // Sinon, reconstruire
}
```

**Estimation**: 1 jour

### 4.3 Health Check Endpoint (Priorité: MOYENNE)
**Fichier**: `internal/app/health.go` (créer)

```go
package app

// HealthChecker provides health status
type HealthChecker struct {
    repo       ports.Repository
    vectorStore ports.VectorStore
}

func (h *HealthChecker) Check() HealthStatus {
    return HealthStatus{
        Database:    h.checkDatabase(),
        VectorStore: h.checkVectorStore(),
        Model:       h.checkModel(),
    }
}
```

**Estimation**: 0.5 jour

### 4.4 Documentation Architecture (ADR) (Priorité: MOYENNE)
**Fichiers à créer**:
- `docs/adr/001-clean-architecture.md`
- `docs/adr/002-cba-algorithm.md`
- `docs/adr/003-vector-search.md`
- `docs/adr/004-causal-graph.md`
- `docs/CONTRIBUTING.md`
- `docs/DEPLOYMENT.md`

**Estimation**: 1.5 jours

### 4.5 Prometheus Metrics (Priorité: BASSE)
**Fichier**: `internal/adapters/metrics/prometheus.go` (créer)

```go
package metrics

import "github.com/prometheus/client_golang/prometheus"

type PrometheusCollector struct {
    storeDuration   prometheus.Histogram
    recallDuration  prometheus.Histogram
    // ...
}

func (c *PrometheusCollector) StartServer(addr string) error {
    http.Handle("/metrics", promhttp.Handler())
    return http.ListenAndServe(addr, nil)
}
```

**Estimation**: 1 jour

---

## 📊 Planning Détaillé

### Semaine 1: Tests Fondamentaux
| Jour | Tâche | Fichier(s) | Estimé |
|------|-------|------------|--------|
| Lundi | Tests app | `main_test.go` | 1j |
| Mardi | Tests MCP controller | `controller_test.go` | 1j |
| Mercredi | Tests vector adapters | `*_test.go` | 1j |
| Jeudi | Tests vector suite + staticcheck | `*_test.go` | 0.5j |
| Vendredi | Revue + corrections | - | 0.5j |

**Livrable**: Couverture 65%+, 0 warning staticcheck

### Semaine 2: Architecture & Qualité
| Jour | Tâche | Fichier(s) | Estimé |
|------|-------|------------|--------|
| Lundi | Context.Context | 7+ fichiers | 1j |
| Mardi | Context.Context suite | 7+ fichiers | 1j |
| Mercredi | Refactoring Extractor | `services.go`, extractors | 1j |
| Jeudi | DIP HNSWStore | `hnsw_store.go` | 1j |
| Vendredi | Tests use cases mineurs | 5 fichiers | 1j |

**Livrable**: Couverture 70%+, code moderne avec context

### Semaine 3: Robustesse
| Jour | Tâche | Fichier(s) | Estimé |
|------|-------|------------|--------|
| Lundi | Async processing | `processor.go` | 1j |
| Mardi | Retry logic | `retry.go` | 1j |
| Mercredi | Circuit breaker | `circuit_breaker.go` | 1j |
| Jeudi | HMAC webhooks | `manager.go` | 0.5j |
| Vendredi | Revue + tests | - | 0.5j |

**Livrable**: Async fonctionnel ou retiré, retry/circuit breaker en place

### Semaine 4: Optimisations & Docs
| Jour | Tâche | Fichier(s) | Estimé |
|------|-------|------------|--------|
| Lundi | Optimisation overlap | `recall_memory.go` | 0.5j |
| Mardi | Persistance HNSW | `hnsw_store.go` | 1j |
| Mercredi | Health check | `health.go` | 0.5j |
| Jeudi | Documentation ADR | `docs/adr/*.md` | 1j |
| Vendredi | Prometheus metrics | `prometheus.go` | 1j |

**Livrable**: Documentation complète, optimisations actives

---

## 🎯 Critères de Succès par Phase

### Phase 1
- [ ] Couverture tests >= 65%
- [ ] 0 warning staticcheck
- [ ] Tests app passent
- [ ] Tests MCP > 50% couverture

### Phase 2
- [ ] Couverture tests >= 70%
- [ ] context.Context dans toutes les signatures
- [ ] Interface Extractor refactorisée
- [ ] HNSWStore DIP corrigé

### Phase 3
- [ ] Async processing fonctionnel ou retiré
- [ ] Retry logic implémenté
- [ ] Circuit breaker en place
- [ ] HMAC webhooks actif

### Phase 4
- [ ] Optimisation overlap O(n²) réduite
- [ ] HNSW persistance complète
- [ ] Health check disponible
- [ ] 4 ADR rédigés
- [ ] Prometheus endpoint exposé

---

## 📈 Impact sur le Score

| Phase | Améliorations | Score avant | Score après | Gain |
|-------|---------------|-------------|-------------|------|
| 1 | Tests + staticcheck | 70 | 75 | +5 |
| 2 | Context + Architecture | 75 | 80 | +5 |
| 3 | Robustesse | 80 | 84 | +4 |
| 4 | Optimisations + Docs | 84 | 88 | +4 |
| **TOTAL** | | **70** | **88** | **+18** |

---

## 🚨 Risques et Mitigations

| Risque | Probabilité | Impact | Mitigation |
|--------|-------------|--------|------------|
| Phase 1 prend plus de temps | Moyen | Haut | Prioriser tests app et MCP uniquement |
| Context.Context casse des tests | Élevé | Moyen | Migration graduelle, pas tout d'un coup |
| Async processing trop complexe | Moyen | Moyen | Choisir Option B (retrait) si délai dépassé |
| HNSW persistance incompatible | Faible | Moyen | Tests d'abord, rollback possible |

---

## 📝 Checklist de Validation Finale

Après les 4 phases, le projet doit avoir:

### Code
- [ ] Couverture >= 70%
- [ ] 0 warning `go vet`
- [ ] 0 warning `staticcheck`
- [ ] Tous les tests passent
- [ ] `go test -race` passe
- [ ] `go build` réussi

### Fonctionnalités
- [ ] Async fonctionnel ou retiré
- [ ] HMAC webhooks actif
- [ ] Retry logic en place
- [ ] Health check disponible
- [ ] Prometheus metrics exposés

### Documentation
- [ ] 4 ADR rédigés
- [ ] CONTRIBUTING.md
- [ ] DEPLOYMENT.md
- [ ] README à jour

---

## 🎓 Ressources Recommandées

### Tests
- [Go Testing Patterns](https://github.com/stretchr/testify)
- [Go Mock](https://github.com/golang/mock)
- [Testify Suite](https://github.com/stretchr/testify#suite-package)

### Architecture
- [Go Clean Architecture](https://github.com/bxcodec/go-clean-arch)
- [Standard Go Project Layout](https://github.com/golang-standards/project-layout)

### Performance
- [Go Profiling](https://go.dev/doc/diagnostics)
- [Go Benchmarks](https://pkg.go.dev/testing#hdr-Benchmarks)

---

*Plan créé le 10 Avril 2026*  
*Dernière mise à jour: 10 Avril 2026*
