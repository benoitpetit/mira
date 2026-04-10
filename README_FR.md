# MIRA - Memory with Information-theoretic Relevance Allocation

**Version:** 0.3.0 | **Langage:** Go 1.23+ | **License:** MIT

Système de mémoire longue durée pour LLM avec allocation optimale de budget contextuel, recherche sémantique et cohérence temporelle. 100% local, déterministe, O(n log n).

📄 **Whitepaper Technique:** [WHITEPAPER.md](WHITEPAPER.md) (Anglais) - Architecture et détails d'implémentation  
📘 **Exemples API:** [API_EXAMPLES.md](API_EXAMPLES.md) (Anglais) - Exemples pratiques d'utilisation

---

## Table des Matières

1. [Installation](#installation)
2. [Architecture Système](#architecture-système)
3. [Modèle de Données](#modèle-de-données)
4. [Mathématiques du Scoring](#mathématiques-du-scoring)
5. [Pipeline d'Extraction](#pipeline-dextraction)
6. [Algorithme d'Allocation](#algorithme-dallocation)
7. [Graphe Causal](#graphe-causal)
8. [Performance & Complexité](#performance--complexité)
9. [Configuration](#configuration)
10. [API MCP](#api-mcp)
11. [Développement](#développement)

---

## Installation

### Depuis les sources (Go 1.23+)

```bash
go install github.com/benoitpetit/mira/cmd/mira@latest
```

### Depuis les releases binaires

Téléchargez les binaires pré-compilés depuis la page [Releases](https://github.com/benoitpetit/mira/releases) :

```bash
# Linux/macOS
tar -xzf mira-linux-amd64.tar.gz
sudo mv mira /usr/local/bin/
mira --version

# Windows
unzip mira-windows-amd64.zip
.\mira.exe --version
```

### Démarrage rapide

```bash
# 1. Copier et éditer la configuration
cp config.example.yaml config.yaml
# Éditer config.yaml selon votre environnement

# 2. Lancer le serveur MCP
mira
```

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
   │T0→T1,T2 │           │  + HNSW    │        │   (BFS)   │
   └────┬────┘           └────────────┘        └───────────┘
        │
   ┌────┴────────────────────────┐
   │  NLP Stack                  │
   │  • tiktoken (tokenization)  │
   │  • prose (NER/entities)     │
   │  • cybertron (embeddings)   │
   └─────────────────────────────┘
```

### Clean Architecture

MIRA v0.3.0 suit les principes de la Clean Architecture :

```
┌─────────────────────────────────────────────────────────────────┐
│  Frameworks & Drivers                                           │
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐           │
│  │  SQLite  │ │  Prose   │ │Cybertron │ │   MCP    │           │
│  │    DB    │ │   NLP    │ │  Embed   │ │  Server  │           │
│  └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘           │
└───────┼────────────┼────────────┼────────────┼─────────────────┘
        │            │            │            │
        └────────────┴────────────┴────────────┘
                         │
        ┌────────────────┴────────────────┐
        │      Interface Adapters         │
        │  ┌──────────────────────────┐   │
        │  │  SQLiteRepository        │   │
        │  │  ProseExtractor          │   │
        │  │  HNSWStore               │   │
        │  │  MCPController           │   │
        │  └──────────────────────────┘   │
        └────────────────┬────────────────┘
                         │
        ┌────────────────┴────────────────┐
        │         Use Cases               │
        │  ┌──────────────────────────┐   │
        │  │  StoreMemory             │   │
        │  │  RecallMemory (CBA)      │   │
        │  │  LoadMemory              │   │
        │  │  GetTimeline             │   │
        │  │  GetStatus               │   │
        │  │  GetCausalChain          │   │
        │  │  ArchiveMemories         │   │
        │  └──────────────────────────┘   │
        └────────────────┬────────────────┘
                         │
        ┌────────────────┴────────────────┐
        │           Domain                │
        │  ┌──────────────────────────┐   │
        │  │  Entities (Verbatim,     │   │
        │  │  Fingerprint, Embedding) │   │
        │  │  Value Objects           │   │
        │  └──────────────────────────┘   │
        └─────────────────────────────────┘
```

**Principes :**
- **Dependency Rule:** Les dépendances pointent vers l'intérieur uniquement
- **Domain:** Logique métier pure, sans dépendances externes
- **Use Cases:** Règles métier applicatives, orchestration des entités
- **Interface Adapters:** Conversion des données pour les frameworks
- **Frameworks:** Outils externes (DB, HTTP, NLP)

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
| `t₀`  | Σ* (UTF-8, max 64KB) | Verbatim - texte original           |
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
| -------------- | ----------- | ------ | ------------------- |
| < 100          | Header      | 2-5    | `[type|date|wing]`  |
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
| Recherche vectorielle | O(log n)   | HNSW approximate nearest neighbor   |
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

### Fichier de Configuration

Copiez le fichier d'exemple :

```bash
cp config.example.yaml config.yaml
```

Puis modifiez `config.yaml` selon votre environnement.

```yaml
system:
  version: "0.3.0"
  max_concurrent_queries: 10

storage:
  path: "./mira_data"
  sqlite:
    journal_mode: WAL
    synchronous: NORMAL
    cache_size: -64000
    mmap_size: 268435456
    temp_store: MEMORY

embeddings:
  current_model: "sentence-transformers/all-MiniLM-L6-v2"
  model_hash: "a2d8f3e9"
  dimension: 384
  batch_size: 32
  cache_size: 1000

# Configuration HNSW
hnsw:
  M: 16
  Ml: 0.25
  ef_construction: 200
  ef_search: 50

allocator:
  default_budget: 4000
  max_candidates: 100
  early_pruning_threshold: 0.6
  session_window_seconds: 7200
  session_boost_beta: 0.2
  causal_penalty_alpha: 0.15
  density_sigmoid:
    k: 2.0
    mu: 0.3

decay_rates:
  decision: 0.001
  fact: 0.005
  preference: 0.01
  session_note: 0.1
  debug_log: 0.5

archive_thresholds:
  session_note: 30
  debug_log: 7

extraction:
  max_verbatim_size: 65536
  max_sentence_length: 500
  min_entity_length: 2
  causal_lookback: 50
  causal_max_days: 30

mcp:
  name: "mira"
  version: "0.3.0"
  transport: "stdio"
  timeout_seconds: 30
  queue_size: 1000
  non_blocking: false

# Export de métriques
metrics:
  enabled: true
  prometheus_addr: ":9090"
  report_interval_seconds: 60

# Notifications webhook
webhooks:
  enabled: false
  workers: 3
  queue_size: 1000
  timeout_seconds: 30
  endpoints: []
```

---

## API MCP

### Outils Disponibles

MIRA expose 7 outils MCP :

| Outil | Description |
|-------|-------------|
| `mira_store` | Stocker une mémoire avec extraction T0→T1,T2 |
| `mira_recall` | Récupérer le contexte optimal avec budget |
| `mira_load` | Charger le verbatim complet par ID (T0) |
| `mira_causal_chain` | Remonter la chaîne causale |
| `mira_status` | Statistiques système et santé |
| `mira_timeline` | Reconstruction chronologique filtrée |
| `mira_archive` | Archiver et nettoyer les vieilles mémoires |

Voir [API_EXAMPLES.md](API_EXAMPLES.md) pour des exemples d'utilisation détaillés.

---

## Développement

### Structure du Code

```
internal/
├── domain/                  # Règles métier enterprise
│   ├── entities/            # Entités domaine (Verbatim, Fingerprint, etc.)
│   └── valueobjects/        # Value objects (MemoryType, RenderMode, etc.)
├── usecases/                # Règles métier applicatives
│   ├── ports/               # Interfaces repositories & services
│   └── interactors/         # Implémentations use cases
├── adapters/                # Adapters d'interface
│   ├── extraction/          # Implémentations NLP/embeddings
│   ├── metrics/             # Adapter collecte métriques
│   ├── storage/             # Implémentation SQLite
│   ├── vector/              # Implémentations vector store (HNSW, SQLite)
│   └── webhook/             # Adapter webhook
├── interfaces/              # Interfaces externes
│   └── mcp/                 # Contrôleur protocole MCP
├── config/                  # Configuration
└── app/                     # Racine de composition
    └── main.go              # Injection de dépendances
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
go test -bench=. -benchmem ./...
```

---

## Références

### Librairies Clés

- [tiktoken-go](https://github.com/pkoukk/tiktoken-go) - Tokenization OpenAI
- [prose](https://github.com/jdkato/prose) - NLP/NER en Go
- [cybertron](https://github.com/nlpodyssey/cybertron) - Embeddings transformers
- [hnsw](https://github.com/coder/hnsw) - Hierarchical Navigable Small World graphs
- [mcp-go](https://github.com/mark3labs/mcp-go) - Protocole MCP

### Modèle d'Embedding

- **Modèle:** sentence-transformers/all-MiniLM-L6-v2
- **Dimensions:** 384
- **Taille:** ~80MB
- **Performance:** ~1000 textes/sec sur CPU

---

## Changelog

### v0.3.0 (2026-04-10)

- 🚀 Nouvelle version 0.3.0

### v0.3.0 (2026-04-10)

- 🚀 Nouvelle version 0.3.0

### v0.3.0 (2026-04-09)

- ✅ **Clean Architecture Refactor**: Restructuration complète du codebase
- ✅ **Index Vectoriel HNSW**: Recherche approximative O(log n)
- ✅ **Embeddings Cybertron**: Embeddings transformers réels
- ✅ **Système de Métriques**: Métriques compatibles Prometheus
- ✅ **Notifications Webhook**: Callbacks HTTP pour les événements

### v0.2.0 (2026-04-09)

- ✅ Fondation Index HNSW
- ✅ Whitepaper technique

### v0.1.0 (2026-04-08)

- ✅ Version initiale
- ✅ Architecture mémoire T0/T1/T2
- ✅ Algorithme CBA (Context Budget Allocator)
- ✅ Stockage SQLite avec WAL mode
- ✅ Serveur MCP avec 7 outils
- ✅ Graphe causal avec 5 types de relations

---

**MIRA** - _Memory with Information-theoretic Relevance Allocation_

_"La mémoire est la sève de l'intelligence artificielle."_
