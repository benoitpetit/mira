# MIRA - Memory with Information-theoretic Relevance Allocation

**Version:** 0.2.0 | **Langage:** Go 1.21+ | **License:** MIT

Système de mémoire longue durée pour LLM avec allocation optimale de budget contextuel, garanties d'approximation et cohérence temporelle. 100% local, déterministe, O(n log n).

---

## Table des Matières

1. [Architecture Système](#architecture-système)
2. [Modèle de Données](#modèle-de-données)
3. [Mathématiques du Scoring](#mathématiques-du-scoring)
4. [Pipeline d'Extraction](#pipeline-dextraction)
5. [Algorithme d'Allocation](#algorithme-dallocation)
6. [Graphe Causal](#graphe-causal)
7. [Performance & Complexité](#performance--complexité)
8. [Configuration](#configuration)
9. [API MCP](#api-mcp)
10. [Développement](#développement)

---

## Architecture Système

### Vue d'Ensemble

```
┌─────────────────────────────────────────────────────────────────────────┐
│                              MCP Server                                 │
│  ┌──────────┐ ┌───────────┐ ┌──────────┐ ┌─────────────────────────┐    │
│  │mira_store│ │mira_recall│ │mira_load │ │   mira_causal_chain     │    │
│  └────┬─────┘ └─────┬──────┘└───┬──────┘ └────────────┬────────────┘    │
│       └─────────────┴───────────┴─────────────────────┘                 │
│                                 │                                       │
└─────────────────────────────────┼───────────────────────────────────────┘
                                  │
                    ┌─────────────┴───────┐
                    │  Budget Allocator   │
                    │   (CBA Algorithm)   │
                    │   O(n log n)        │
                    └──────────┬──────────┘
                               │
        ┌──────────────────────┼────────────────────┐
        │                      │                    │
   ┌────┴────┐           ┌─────┴──────┐        ┌────┴──────┐
   │ Extract │           │   Store    │        │  Causal   │
   │ Pipeline│           │  (SQLite)  │        │   Graph   │
   │T0→T1,T2 │           │            │        │   (BFS)   │
   └────┬────┘           └────────────┘        └───────────┘
        │
   ┌────┴────────────────────────┐
   │  NLP Stack                  │
   │  • tiktoken (tokenization)  │
   │  • prose (NER/entities)     │
   │  • cybertron (embeddings)   │
   └─────────────────────────────┘
```

### Flux de Données

```
┌─────────┐  Extraction   ┌─────────────┐  Stockage   ┌──────────┐
│  Input  │ ─────────────→│  Triple T   │ ───────────→│  SQLite  │
│  Texte  │   T0→T1→T2    │  (3 niveaux)│  WAL Mode   │  + WAL   │
└─────────┘               └─────────────┘             └──────────┘
                                   │                         │
                                   │ Requête                 │
                                   ↓                         ↓
┌─────────┐  Scoring       ┌─────────────┐  Allocation ┌──────────┐
│Requête  │ ←───────────── │  CBA Score  │ ←───────────│  Budget  │
│Vecteur  │   ρ×δ×η×τ×σ×χ  │  Composite  │  Greedy     │  4000tk  │
└─────────┘                └─────────────┘             └──────────┘
```

---

## Modèle de Données

### Espace de Représentation

Chaque mémoire `m ∈ ℳ` est un tuple:

```
m = (id, t₀, t₁, t₂, c, τ, ω, γ, δ, ν)
```

| Champ | Type                  | Description                         |
| ----- | --------------------- | ----------------------------------- |
| `id`  | UUIDv4 (128 bits)     | Identifiant unique                  |
| `t₀`  | Σ\* (UTF-8, max 64KB) | Verbatim - texte original           |
| `t₁`  | JSON canonique        | Fingerprint structuré               |
| `t₂`  | ℝ³⁸⁴                  | Embedding vector (all-MiniLM-L6-v2) |
| `c`   | ℕ                     | Token count (tiktoken cl100k_base)  |
| `τ`   | ℝ⁺                    | Timestamp UNIX (secondes)           |
| `ω`   | Ω                     | Type de mémoire (enum)              |
| `γ`   | Γ                     | Graphe causal (nœud + arêtes)       |
| `δ`   | ℝ⁺                    | Decay rate λ_ω selon le type        |
| `ν`   | {0,1}³²               | Hash du modèle d'embedding          |

### Types de Mémoire et Decay Rates

| Type ω         | λ_ω (jour⁻¹) | Demi-vie  | Archive auto | Usage                     |
| -------------- | ------------ | --------- | ------------ | ------------------------- |
| `decision`     | 0.001        | 693 jours | Non          | Décisions architecturales |
| `fact`         | 0.005        | 139 jours | Non          | Faits, connaissances      |
| `preference`   | 0.01         | 69 jours  | Non          | Préférences utilisateur   |
| `session_note` | 0.1          | 7 jours   | 30 jours     | Notes de session          |
| `debug_log`    | 0.5          | 1.4 jours | 7 jours      | Logs de debug             |

### Schéma SQL (SQLite)

```sql
-- Métadonnées des modèles (versioning embeddings)
CREATE TABLE embedding_models (
    model_hash TEXT PRIMARY KEY,  -- SHA256 truncated
    model_name TEXT NOT NULL,     -- "all-MiniLM-L6-v2"
    dimension INTEGER NOT NULL,
    created_at REAL NOT NULL,
    metadata BLOB                 -- JSON config
);

-- T0: Verbatim Store (append-only, WAL mode)
CREATE TABLE verbatim (
    id BLOB PRIMARY KEY,          -- 16 bytes UUID
    content TEXT NOT NULL,        -- UTF-8, max 64KB
    token_count INTEGER NOT NULL, -- tiktoken cl100k_base
    created_at REAL NOT NULL,     -- UNIX timestamp
    wing TEXT NOT NULL,           -- namespace/project
    room TEXT,                    -- sous-catégorie
    metadata BLOB                 -- msgpack compressé
);

-- T1: Fingerprint Index (JSON canonique)
CREATE TABLE fingerprints (
    id BLOB PRIMARY KEY,
    verbatim_id BLOB NOT NULL REFERENCES verbatim(id),
    ftype TEXT NOT NULL,          -- decision|fact|preference|...
    extracted_at REAL NOT NULL,
    entities TEXT,                -- JSON array
    subjects TEXT,                -- JSON array
    decision TEXT,
    related_to TEXT,              -- JSON array d'IDs
    data BLOB NOT NULL,           -- JSON minifié
    fact_count INTEGER DEFAULT 0,
    token_estimate INTEGER DEFAULT 0,
    model_hash TEXT
);

-- T2: Vector Index avec versioning
CREATE TABLE embeddings (
    id BLOB PRIMARY KEY,
    model_hash TEXT NOT NULL,
    dim INTEGER NOT NULL,         -- 384
    vector BLOB NOT NULL,         -- 384 * 4 = 1536 bytes
    normalized BOOLEAN DEFAULT 1,
    created_at REAL NOT NULL
);

-- Causal Graph (DAG)
CREATE TABLE causal_nodes (
    id BLOB PRIMARY KEY,
    node_type TEXT NOT NULL,
    summary TEXT NOT NULL,
    timestamp REAL NOT NULL,
    wing TEXT NOT NULL,
    room TEXT
);

CREATE TABLE causal_edges (
    from_id BLOB NOT NULL,
    to_id BLOB NOT NULL,
    relation TEXT NOT NULL,       -- BECAUSE|TRIGGERED|CONTRADICTS|UPDATES|RESOLVES
    weight REAL DEFAULT 1.0,
    detected_at REAL NOT NULL,
    PRIMARY KEY (from_id, to_id, relation)
);

-- Cache d'overlap avec TTL (30 jours)
CREATE TABLE overlap_cache (
    id_a BLOB NOT NULL,
    id_b BLOB NOT NULL,
    similarity REAL NOT NULL,
    computed_at REAL NOT NULL,
    ttl REAL NOT NULL DEFAULT (unixepoch() + 2592000),
    PRIMARY KEY (id_a, id_b)
);
```

---

## Mathématiques du Scoring

### Fonction de Score Composite

Pour une requête `q` avec embedding `e_q ∈ ℝ³⁸⁴`, le score d'un candidat `m` étant donné l'ensemble déjà sélectionné `S`:

```
┌─────────────────────────────────────────────────────────────────────────┐
│                                                                         │
│   S(m|q,S,θ) = ρ(m,q) × δ_sig(m) × η(m) × τ_session(m,S)                │
│                × (1 - max_s∈S σ(m,s)) × χ_penalty(m,S)                  │
│                × 𝟙[ρ > θ_min]                                           │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### 1. Pertinence Sémantique ρ

```
              e_m · e_q
ρ(m,q) = ───────────────── ∈ [0, 1]
          ‖e_m‖ ‖e_q‖

Avec normalisation L2 préalable:
ρ(m,q) = (1 + cosim(e_m, e_q)) / 2
```

**Early pruning:** Seuls les m avec `ρ > θ_min = 0.6` sont considérés.

### 2. Densité Informationnelle δ_sig

```
               |facts(t₁(m))|
δ_raw(m) = ─────────────────
               √c(m)

δ_sig(m) = 2/(1 + e^(-2(δ_raw - 0.3))) - 1 ∈ [0, 1]
```

Paramètres sigmoïde:

- `μ = 0.3` : centre (5 facts/100 tokens = neutre)
- `k = 2.0` : pente

**Objectif:** Éviter la sur-valorisation des micro-mémoires.

### 3. Poids Temporel (Recency) η

```
η(m) = exp(-λ_ω · (t_now - τ(m)))

Où:
- λ_ω : decay rate selon le type
- t_now - τ(m) : âge en jours
```

### 4. Boost de Cohérence Temporelle τ_session

```
τ_session(m,S) = 1 + β · 𝟙[∃s ∈ S : |τ(m) - τ(s)| < θ_session]

Paramètres:
- β = 0.2 (20% boost)
- θ_session = 7200s (2 heures)
```

**Objectif:** Favoriser la cohérence narrative dans une session.

### 5. Pénalité d'Overlap σ

```
              e_m · e_s
σ(m,s) = ───────────────── ∈ [-1, 1]
          ‖e_m‖ ‖e_s‖

Pénalité appliquée: (1 - max_s∈S σ(m,s))
```

### 6. Pénalité Causale χ_penalty

```
χ_penalty(m,S) = exp(-α · |{s ∈ S : causal_link(s,m)}|)

Paramètre:
- α = 0.15

Objectif: Éviter la sur-sélection de chaînes causales longues.
```

---

## Pipeline d'Extraction

### T0 → T1: Extraction Structurée

```go
// Patterns regex UTF-8 aware
var decisionPatterns = []*regexp.Regexp{
    regexp.MustCompile(`(?i)([\p{L}\p{N}]+)\s+a\s+(?:décidé|choisi|sélectionné)...`),
    regexp.MustCompile(`(?i)(?:décision|choice)\s*:\s*(.+?)(?:\.|\n|$)`),
    regexp.MustCompile(`(?i)on\s+(?:va|vais|allons)\s+(?:utiliser|prendre|adopter)...`),
}

// Extraction NER avec prose
entities := extractEntities(doc)  // PERSON, ORG, GPE

// Détection type avec priorité stricte
if matchDecision(content) → TypeDecision
else if matchPreference(content) → TypePreference
else if matchFact(content) → TypeFact
else → TypeSessionNote
```

### Structure T1 (Fingerprint)

```json
{
  "id": "uuid",
  "type": "decision|fact|preference|session_note|debug_log",
  "date": "2026-04-08T09:18:01Z",
  "entities": ["PostgreSQL", "API", "Auth"],
  "subject": ["database-migration"],
  "decision": "Use PostgreSQL",
  "rejected": ["MySQL", "MongoDB"],
  "reason": ["Better ACID", "Team expertise"],
  "validated_by": "CTO",
  "assignee": "John",
  "deadline": "Sprint 5",
  "causal_parent": null,
  "verbatim_ref": "T0:uuid"
}
```

### T0 → T2: Génération d'Embedding

```python
# Modèle: sentence-transformers/all-MiniLM-L6-v2
Dimension: 384
Normalization: L2 préalable

vector = model.encode(text)  # ℝ³⁸⁴
vector = normalize_L2(vector)
```

---

## Algorithme d'Allocation

### Context Budget Allocator (CBA) v2

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         CBA Algorithm                                   │
│                         O(n log n)                                      │
├─────────────────────────────────────────────────────────────────────────┤
│  Input: query q, budget B, wing w, room r                               │
│  Output: liste de mémoires sélectionnées avec mode de rendu             │
├─────────────────────────────────────────────────────────────────────────┤
│  1. Embedding avec cache LRU                                            │
│     e_q ← Embed(q)                                                      │
│                                                                         │
│  2. Recherche vectorielle                                               │
│     C ← VectorSearch(e_q, N=100, w, r)                                  │
│                                                                         │
│  3. Early Pruning                                                       │
│     C' ← {c ∈ C : ρ(c,q) > 0.6}                                         │
│     if C' = ∅: C' ← top-5(C)                                           │
│                                                                         │
│  4. Scoring initial                                                     │
│     ∀c ∈ C':                                                            │
│       c.score ← ρ(c) × δ_sig(c) × η(c)                                  │
│                                                                         │
│  5. Sélection Greedy avec renormalisation dynamique                     │
│     S ← ∅, tokens_used ← 0                                             │
│     PQ ← MaxHeap(C')  # par score initial                               │
│                                                                         │
│     while PQ ≠ ∅ AND tokens_used < B:                                  │
│       c ← Pop(PQ)                                                       │
│                                                                         │
│       # Recalcul dynamique                                              │
│       c.max_overlap ← max_{s∈S} σ(c,s)                                  │
│       c.causal_penalty ← exp(-0.15 × |links(c,S)|)                      │
│       c.session_boost ← 1.2 if |τ(c)-τ(S)| < 2h else 1.0                │
│                                                                         │
│       adjusted_score ← c.score × (1-c.max_overlap)                      │
│                           × c.causal_penalty                            │
│                           × c.session_boost                             │
│                                                                         │
│       # Vérifier si prochain a meilleur score                           │
│       if PQ[0].score × 0.8 > adjusted_score:                            │
│          Push(PQ, c) with adjusted_score                                │
│          continue                                                       │
│                                                                         │
│       # Déterminer mode de rendu selon budget RESTANT                   │
│       remaining ← B - tokens_used                                       │
│       mode ← DetermineRenderMode(c, remaining)                          │
│       cost ← CalculateTokenCost(c, mode)                                │
│                                                                         │
│       # Downgrade si nécessaire                                         │
│       if tokens_used + cost > B:                                        │
│          mode ← downgrade(mode)                                         │
│          cost ← recalculate(mode)                                       │
│          if tokens_used + cost > B: continue                            │
│                                                                         │
│       S ← S ∪ {c}, tokens_used ← tokens_used + cost                     │
│                                                                         │
│  6. Return S                                                            │
└─────────────────────────────────────────────────────────────────────────┘
```

### Modes de Rendu

Le mode dépend uniquement du **budget restant**, pas de l'overlap:

| Budget Restant | Mode        | Tokens | Contenu             |
| -------------- | ----------- | ------ | ------------------- | ---- | -------------- |
| < 100          | Header      | 2-5    | `[type              | date | wing] → T0:id` |
| < 1000         | Fingerprint | ~15%   | Faits essentiels T1 |
| ≥ 1000         | Verbatim    | 100%   | Texte original T0   |

---

## Graphe Causal

### Relations Supportées

| Relation      | Direction | Sémantique              |
| ------------- | --------- | ----------------------- |
| `BECAUSE`     | A ← B     | B explique A            |
| `TRIGGERED`   | A ← B     | B a déclenché A         |
| `CONTRADICTS` | A ↔ B     | A et B en contradiction |
| `UPDATES`     | A ← B     | B remplace/actualise A  |
| `RESOLVES`    | A ← B     | B résout le problème A  |

### Détection de Relations Causales

```
Input: new_fp, recent_fps[50], verbatim_content
Output: liste d'arêtes causales

for each existing in recent_fps:
    if time_diff > 30 days: continue

    for each (relation, pattern) in causal_patterns:
        if pattern.match(verbatim_content):
            # Vérifier référence implicite
            if hasOverlap(new_fp.entities, existing.entities) OR
               hasOverlap(new_fp.subjects, existing.subjects) OR
               content.contains(existing.id[:8]):

               edge ← CausalEdge{
                   from: existing.id,
                   to: new_fp.id,
                   relation: relation,
                   weight: 0.7
               }
               edges.add(edge)
```

### Navigation BFS

```go
// GetChain: remonter les causes (parents)
func (g *Graph) GetChain(nodeID UUID, maxDepth int) []*CausalNode

// GetConsequences: descendre les effets (enfants)
func (g *Graph) GetConsequences(nodeID UUID, maxDepth int) []*CausalNode
```

---

## Performance & Complexité

### Complexités Algorithmiques

| Opération             | Complexité | Notes                               |
| --------------------- | ---------- | ----------------------------------- |
| Stockage (T0→T1,T2)   | O(1)       | Amorti, insertion unique            |
| Recherche vectorielle | O(n)       | SQLite linear scan (HNSW: O(log n)) |
| Scoring               | O(n)       | n = nombre de candidats             |
| Allocation Greedy     | O(n log n) | Max-heap operations                 |
| BFS Graphe causal     | O(V + E)   | V=nœuds, E=arêtes                   |
| Total Recall          | O(n log n) | Bottleneck: heap operations         |

### Constantes de Performance

| Paramètre         | Valeur         | Justification                |
| ----------------- | -------------- | ---------------------------- |
| `MaxCandidates`   | 100            | Early pruning inclus         |
| `EmbeddingCache`  | 1000 entrées   | LRU pour query embeddings    |
| `CausalLookback`  | 50 derniers FP | Fenêtre temporelle: 30 jours |
| `OverlapCacheTTL` | 30 jours       | Éviter explosion O(n²)       |
| `SessionWindow`   | 2 heures       | Cohérence conversationnelle  |

### Benchmarks (estimés)

```
BenchmarkCosineSimilarity-384      50M ops/sec
BenchmarkNormalizeL2-384           20M ops/sec
BenchmarkAllocateWithCache-1000    ~5ms/query
BenchmarkAllocateNoCache-1000      ~50ms/query
```

---

## Configuration

### Fichier config.yaml

```yaml
system:
  version: "0.2.0"
  max_concurrent_queries: 10

storage:
  path: "./mira_data"
  sqlite:
    journal_mode: WAL # Write-Ahead Logging
    synchronous: NORMAL # Équilibre perf/sécurité
    cache_size: -64000 # 64MB
    mmap_size: 268435456 # 256MB
    temp_store: MEMORY

embeddings:
  current_model: "sentence-transformers/all-MiniLM-L6-v2"
  model_hash: "a2d8f3e9" # SHA256 truncated
  dimension: 384
  batch_size: 32
  cache_size: 1000 # LRU cache

allocator:
  default_budget: 4000 # tokens
  max_candidates: 100
  early_pruning_threshold: 0.6 # ρ_min
  session_window_seconds: 7200 # 2h
  session_boost_beta: 0.2 # 20%
  causal_penalty_alpha: 0.15
  density_sigmoid:
    k: 2.0
    mu: 0.3

decay_rates:
  decision: 0.001 # ~2 ans demi-vie
  fact: 0.005 # ~5 mois
  preference: 0.01 # ~2 mois
  session_note: 0.1 # ~1 semaine
  debug_log: 0.5 # ~1.4 jour

archive_thresholds:
  session_note: 30 # jours
  debug_log: 7 # jours

extraction:
  max_verbatim_size: 65536 # 64KB
  max_sentence_length: 500
  min_entity_length: 2
  causal_lookback: 50
  causal_max_days: 30
```

---

## API MCP

### Outils Disponibles

#### `mira_store`

Stocke une mémoire avec extraction automatique T0→T1,T2.

```json
{
  "content": "On a décidé d'utiliser PostgreSQL pour la DB",
  "wing": "backend-service",
  "room": "database-migration"
}
```

**Réponse:**

```
Stored: 550e8400-e29b-41d4-a716-446655440000
Type: decision
Facts: 3
Tokens: 42
Model: a2d8f3e9
```

#### `mira_recall`

Récupère contexte optimal avec budget.

```json
{
  "query": "Quelle base de données?",
  "budget": 4000,
  "wing": "backend-service"
}
```

**Réponse:**

```
=== MIRA CONTEXT ===
Query: Quelle base de données? | Budget: 4000
Wing: backend-service

--- [1] VERBATIM (42 tokens) ---
On a décidé d'utiliser PostgreSQL pour la DB

=== Total: 42/4000 tokens (1.1%) ===
```

#### `mira_causal_chain`

Remonte chaîne causale.

```json
{
  "id": "550e8400...",
  "max_depth": 5,
  "include_consequences": true
}
```

**Réponse:**

```
=== CAUSAL CHAIN (Upstream) ===
 → [decision] Évaluer options DB (2026-04-01)
  → [fact] Benchmark PostgreSQL vs MySQL (2026-03-28)

=== CONSEQUENCES (Downstream) ===
→ [decision] Configurer connexion pool (2026-04-09)
```

---

## Développement

### Structure du Code

```
mira/
├── cmd/mira/           # Entry point
├── types/              # Domain models
├── store/              # SQLite persistence
├── extract/            # T0→T1,T2 pipeline
├── budget/             # CBA algorithm
├── causal/             # Graph operations
├── mcp/                # MCP server
├── config/             # Configuration
└── vector/             # Vector search adapter
```

### Commandes Make

```bash
make build      # Compiler
make test       # Tests unitaires
make bench      # Benchmarks
make migrate    # Migrations DB
make clean      # Nettoyer
```

### Tests

```bash
# Tests unitaires
go test -v ./...

# Avec race detector
go test -race ./...

# Benchmarks
go test -bench=. -benchmem ./budget
```

---

## Références

### Librairies Clés

- [tiktoken-go](https://github.com/pkoukk/tiktoken-go) - Tokenization OpenAI
- [prose](https://github.com/jdkato/prose) - NLP/NER en Go
- [cybertron](https://github.com/nlpodyssey/cybertron) - Embeddings transformers
- [mcp-go](https://github.com/mark3labs/mcp-go) - Protocole MCP

### Modèle d'Embedding

- **Modèle:** sentence-transformers/all-MiniLM-L6-v2
- **Dimensions:** 384
- **Taille:** ~80MB
- **Performance:** ~1000 textes/sec sur CPU

---

## Changelog

### v0.2.0 (2026-04-08)

- ✅ Extraction UTF-8 avec patterns `\p{L}\p{N}`
- ✅ Densité sigmoïde (k=2, μ=0.3)
- ✅ Session boost (2h window)
- ✅ Causal penalty (pas boost)
- ✅ Greedy avec renormalisation dynamique
- ✅ Rendu selon budget restant uniquement
- ✅ Versioning des modèles d'embedding

---

**MIRA** - _Memory with Information-theoretic Relevance Allocation_

_"La mémoire est la sève de l'intelligence artificielle."_
