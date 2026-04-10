<div align="center">
  <img src="./logo.png" alt="MIRA Logo" width="800">
  
  # MIRA
  ### Memory with Information-theoretic Relevance Allocation
  
  **Système de Mémoire Long-Terme pour LLMs avec Allocation Optimale de Budget Contextuel**
  
  [![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
  [![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
  [![Version](https://img.shields.io/badge/Version-0.3.0-blue?style=flat-square)]()
  [![Tests](https://img.shields.io/badge/Tests-77%25-brightgreen?style=flat-square)]()
  
  *100% Local • Déterministe • O(n log n) • Clean Architecture*
  
  [📘 Référence API](docs/API_REFERENCES.md) • [📝 Changelog](CHANGELOG.md) • [🇬🇧 English](README.md)
  
</div>

---

## 📋 Table des Matières

- [Qu'est-ce que MIRA ?](#quest-ce-que-mira-)
- [La Révolution de la Mémoire pour LLMs](#la-révolution-de-la-mémoire-pour-llms)
- [Fonctionnement](#fonctionnement)
- [Architecture à 3 Niveaux (T0/T1/T2)](#architecture-à-3-niveaux-t0t1t2)
- [L'Algorithme CBA](#lalgorithme-cba)
- [Graphe Causal](#graphe-causal)
- [Installation](#installation)
- [Démarrage Rapide](#démarrage-rapide)
- [Configuration](#configuration)
- [API MCP](#api-mcp)
- [Performance](#performance)
- [Architecture Technique](#architecture-technique)
- [Développement](#développement)
- [Changelog](#changelog)

---

## Qu'est-ce que MIRA ?

**MIRA** est un système de mémoire long-terme sophistiqué conçu spécifiquement pour les **Large Language Models (LLMs)**. Contrairement aux systèmes de mémoire traditionnels qui se contentent de stocker et récupérer, MIRA utilise une **allocation informationnelle** pour optimiser chaque token de la fenêtre de contexte.

### Le Problème que MIRA Résout

Les LLMs modernes (GPT-4, Claude, Llama, etc.) souffrent d'un problème fondamental : **la fenêtre de contexte est limitée** (4K-128K tokens), mais les conversations et projets s'étendent sur des milliers d'interactions. Comment décider quoi garder dans le contexte ?

**Les approches traditionnelles échouent :**

- ❌ RAG simple : Récupération basée uniquement sur la similarité, ignore la densité d'information
- ❌ Fenêtre glissante : Perd les informations critiques du début
- ❌ Résumé statique : Ne s'adapte pas à la requête actuelle
- ❌ Vector DB basique : Complexité O(n), pas de gestion de budget

**MIRA apporte la solution :**

- ✅ **Allocation de Budget Contextuel** : Optimise chaque token sur 6 dimensions
- ✅ **Densité Informationnelle** : Privilégie les mémoires riches en faits
- ✅ **Cohérence Temporelle** : Maintient la continuité narrative
- ✅ **Graphe Causal** : Comprend les relations cause-effet
- ✅ **Recherche O(log n)** : HNSW pour des millions de mémoires
- ✅ **Clean Architecture** : Maintenable, testable, extensible

---

## La Révolution de la Mémoire pour LLMs

### Ce que MIRA Apporte de Nouveau

#### 1. **Allocation Informationnelle (CBA)**

Au lieu de simplement récupérer les "plus similaires", MIRA résout un **problème d'optimisation sous contrainte** : maximiser l'information utile dans un budget de tokens fixe.

```
Score(m) = Pertinence × Densité × Récence × (1-Chevauchement) × Cohérence × PénalitéCausale
```

#### 2. **Triple Représentation (T0/T1/T2)**

Chaque mémoire existe sous 3 formes pour différents usages :

- **T0 (Verbatim)** : Texte original complet
- **T1 (Fingerprint)** : Faits structurés extraits (~15% des tokens)
- **T2 (Embedding)** : Vecteur sémantique 384D pour la recherche

#### 3. **Graphe Causal Intégré**

Détection automatique des relations (BECAUSE, TRIGGERED, CONTRADICTS, UPDATES, RESOLVES) pour tracer la chaîne de raisonnement.

#### 4. **Rendu Adaptatif**

Selon le budget restant, MIRA choisit intelligemment le niveau de détail :

- **Header** (5 tokens) : Référence seule
- **Fingerprint** (~15% tokens) : Faits essentiels
- **Verbatim** (100% tokens) : Texte complet

---

## Fonctionnement

### Vue d'Ensemble du Flux

```
┌─────────────────────────────────────────────────────────────────────────┐
│                         STOCKAGE D'UNE MÉMOIRE                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   Texte Input        Extraction T1,T2         Stockage Atomique         │
│   ┌─────────┐       ┌──────────────┐          ┌─────────────────┐       │
│   │"Nous    │──────→│  Fingerprint │─────────→│  SQLite + HNSW  │       │
│   │ avons   │       │  + Embedding │          │  (WAL Mode)     │       │
│   │ décidé  │       └──────────────┘          └─────────────────┘       │
│   │ d'      │              │                       │                    │
│   │ utiliser│              ↓                       ↓                    │
│   │PostgreSQL"          T1: {                 Index Vectoriel           │
│   └─────────┘            - decision: "PostgreSQL"  ℝ³⁸⁴                 │
│                          - rejected: ["MySQL",     HNSW O(log n)        │
│                                      "MongoDB"]                         │
│                          - reason: ["ACID", "Exp"]                      │
│                          - type: DECISION                               │
│                                                                         │
│                         T2: [0.23, -0.15, 0.89, ...] 384D               │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────┐
│                         RÉCUPÉRATION (RECALL)                           │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│   Requête "Pourquoi PostgreSQL ?"                                       │
│       │                                                                 │
│       ▼                                                                 │
│   ┌─────────────┐    ┌─────────────────┐    ┌──────────────────────┐    │
│   │ Embedding   │───→│  HNSW Search    │───→│  Scoring Composite   │    │
│   │ Requête     │    │  Top 100        │    │  CBA Algorithm       │    │
│   │ ℝ³⁸⁴        │    │  O(log n)       │    │  O(n log n)          │    │
│   └─────────────┘    └─────────────────┘    └──────────────────────┘    │
│                                                        │                │
│                                                        ▼                │
│                                              Sélection Gloutonne        │
│                                              avec Budget 4000 tokens    │
│                                                        │                │
│       ┌────────────────────────────────────────────────┘                │
│       ▼                                                                 │
│   Résultat Optimisé :                                                   │
│   ┌──────────────────────────────────────────────────────────────┐      │
│   │ [1] Fingerprint: "Décision PostgreSQL (ACID, expertise)" 45tk│      │
│   │ [2] Verbatim: "Réunion 15/04 - discussion DB..."        120tk│      │
│   │ [3] Header: "Sprint 5 deadline"                           5tk│      │
│   │ ...                                                          │      │
│   │ Total: 3987/4000 tokens (99.7% utilisation)                  │      │
│   └──────────────────────────────────────────────────────────────┘      │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

### Le Score Composite CBA

Pour chaque mémoire candidate, MIRA calcule un **score multidimensionnel** :

```
┌─────────────────────────────────────────────────────────────────────┐
│                     FORMULE DE SCORE CBA                            │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   S(m) = ρ × δ × η × (1-σ) × τ × χ × 𝟙[ρ>θ]                         │
│                                                                     │
│   où :                                                              │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━    │
│   ρ (rho)    = Pertinence Sémantique    cos(embedding_m, embedding_q)│
│   δ (delta)  = Densité Informationnelle  sigmoïde(faits/√tokens)    │
│   η (eta)    = Poids Temporel            exp(-λ × âge)              │
│   σ (sigma)  = Chevauchement Max         sim(m, déjà_sélectionnés)  │
│   τ (tau)    = Boost Session             +20% si même session       │
│   χ (chi)    = Pénalité Causale          évite chaînes longues      │
│   𝟙[ρ>θ]     = Seuil de pertinence       élimine si ρ < 0.6         │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Architecture à 3 Niveaux (T0/T1/T2)

### Pourquoi 3 Niveaux ?

Le cerveau humain n'enregistre pas tout avec la même fidélité. MIRA imite cette hiérarchie :

```
┌─────────────────────────────────────────────────────────────────────┐
│                        HIÉRARCHIE T0/T1/T2                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   NIVEAU T0 - VERBATIM (Mémoire Épisodique)                         │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   │
│   "Réunion du 15 avril 2024 à 14h30.                                │
│    Participants: Marie (Tech Lead), Jean (DevOps), Sophie (PO)      │
│    Marie: 'Je propose qu'on migre vers PostgreSQL pour la v2'       │
│    Jean: 'Ça demande de la formation, mais c'est plus robuste'      │
│    Sophie: 'Le client valide pour le Sprint 5'                      │
│    Décision finale: Migration PostgreSQL validée"                   │
│                                                                     │
│    • Stockage : Texte UTF-8 complet (max 64KB)                      │
│    • Usage : Contexte riche quand le budget le permet               │
│    • Coût : ~200 tokens                                             │
│                                                                     │
│                              ↓ Extraction NLP                       │
│                                                                     │
│   NIVEAU T1 - FINGERPRINT (Mémoire Sémantique)                      │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   │
│   {                                                                 │
│     "type": "decision",                                             │
│     "decision": "Migration vers PostgreSQL",                        │
│     "rejected": ["MySQL", "MongoDB"],                               │
│     "reason": ["Robustesse ACID", "Validation client"],             │
│     "assignee": "Jean",                                             │
│     "deadline": "Sprint 5",                                         │
│     "validated_by": "Sophie (PO)"                                   │
│   }                                                                 │
│                                                                     │
│    • Stockage : JSON canonique structuré                            │
│    • Usage : Contexte dense quand le budget est moyen               │
│    • Coût : ~30 tokens (15% de T0)                                  │
│                                                                     │
│                              ↓ Embedding                            │
│                                                                     │
│   NIVEAU T2 - EMBEDDING (Mémoire Procédurale)                       │
│   ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   │
│   [0.23, -0.15, 0.89, -0.42, 0.67, ...]  // 384 dimensions          │
│                                                                     │
│    • Stockage : Vecteur float32[384]                                │
│    • Usage : Recherche vectorielle O(log n)                         │
│    • Coût : 0 tokens (recherche uniquement)                         │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Types de Mémoire et Décroissance

| Type           | λ (jour⁻¹) | Demi-vie   | Auto-Archive | Usage                     |
| -------------- | ---------- | ---------- | ------------ | ------------------------- |
| `decision`     | 0.001      | ~693 jours | ❌           | Décisions architecturales  |
| `fact`         | 0.005      | ~139 jours | ❌           | Connaissances, faits       |
| `preference`   | 0.01       | ~69 jours  | ❌           | Préférences utilisateur    |
| `session_note` | 0.1        | ~7 jours   | 30 jours     | Notes de session          |
| `debug_log`    | 0.5        | ~1.4 jours | 7 jours      | Logs de debug             |

---

## L'Algorithme CBA

### Context Budget Allocator v2

```
┌─────────────────────────────────────────────────────────────────────┐
│                    ALGORITHME CBA - O(n log n)                      │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ENTRÉE :  Requête q, Budget B (tokens), Wing w, Room r             │
│  SORTIE :  Liste de mémoires avec mode de rendu                     │
│                                                                     │
│  ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━   │
│                                                                     │
│  1. EMBEDDING                                                       │
│     e_q ← Embed(q) avec cache LRU (1000 entrées)                    │
│                                                                     │
│  2. RECHERCHE VECTORIELLE                                           │
│     C ← HNSW_Search(e_q, N=100, w, r)        # O(log n)             │
│     Si HNSW non prêt : C ← SQLite_Search(e_q, N=100)  # Fallback    │
│                                                                     │
│  3. ÉLAGAGE PRÉCOCE                                                 │
│     C' ← {c ∈ C : ρ(c,q) > 0.6}                                     │
│     Si C' = ∅ : C' ← top-5(C) par ρ                                │
│                                                                     │
│  4. SCORING INITIAL                                                 │
│     Pour chaque c ∈ C' :                                            │
│        c.score ← ρ(c) × δ_sigmoïde(c) × η_récence(c)                │
│                                                                     │
│  5. SÉLECTION GLOUTONNE AVEC RENORMALISATION DYNAMIQUE              │
│     S ← ∅, tokens_utilisés ← 0                                     │
│     PQ ← MaxHeap(C')  # par score initial                           │
│                                                                     │
│     TANT QUE PQ ≠ ∅ ET tokens_utilisés < B :                       │
│        c ← Pop(PQ)                                                  │
│                                                                     │
│        # Recalcul dynamique (dépend de S déjà sélectionné)          │
│        c.σ_max ← max_{s∈S} similarité(c,s)                          │
│        c.χ ← exp(-0.15 × |liens_causaux(c,S)|)                      │
│        c.τ ← 1.2 si |temps(c) - temps(S)| < 2h sinon 1.0            │
│                                                                     │
│        score_ajusté ← c.score × (1-c.σ_max) × c.χ × c.τ             │
│                                                                     │
│        # Vérifier si le suivant a un meilleur score                 │
│        Si PQ[0].score × 0.8 > score_ajusté :                        │
│           Push(PQ, c) avec score_ajusté                             │
│           continuer                                                 │
│                                                                     │
│        # Déterminer mode selon BUDGET RESTANT                       │
│        restant ← B - tokens_utilisés                                │
│        mode ← ChoisirMode(c, restant)                               │
│        coût ← CalculerCoût(c, mode)                                 │
│                                                                     │
│        # Dégrader si nécessaire                                     │
│        Si tokens_utilisés + coût > B :                              │
│           mode ← Dégrader(mode)  # Verbatim → Fingerprint → Header  │
│           coût ← Recalculer(mode)                                   │
│           Si tokens_utilisés + coût > B : continuer                 │
│                                                                     │
│        S ← S ∪ {c}, tokens_utilisés ← tokens_utilisés + coût        │
│                                                                     │
│  6. RETOURNER S trié par score décroissant                          │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Modes de Rendu Adaptatif

| Budget Restant | Mode        | Tokens | Contenu              |
| -------------- | ----------- | ------ | -------------------- |
| < 100          | Header      | 2-5    | `[type\|date\|wing]` |
| < 1000         | Fingerprint | ~15%   | Faits essentiels T1  |
| ≥ 1000         | Verbatim    | 100%   | Texte original T0    |

---

## Graphe Causal

### Relations Supportées

```
┌─────────────────────────────────────────────────────────────────────┐
│                      RELATIONS CAUSALES                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   BECAUSE (Parce que)        A ←────────── B                        │
│   "B explique pourquoi A"    Bug compris  Parce qu'on a analysé     │
│                              ───────────→   les logs                │
│                                                                     │
│   TRIGGERED (Déclenché)      A ←────────── B                        │
│   "B a déclenché A"          Migration    Après la réunion          │
│                              ───────────→   de décision             │
│                                                                     │
│   CONTRADICTS (Contradit)    A ←────────→ B                         │
│   "A et B se contredisent"   Option A     Option B                  │
│                              ───────────→   incompatible            │
│                                                                     │
│   UPDATES (Met à jour)       A ←────────── B                        │
│   "B remplace/actualise A"   Spec v1      Spec v2                   │
│                              ───────────→   (nouvelle version)      │
│                                                                     │
│   RESOLVES (Résout)          A ←────────── B                        │
│   "B résout le problème A"   Bug #123     Fix #124                  │
│                              ───────────→   (correction)            │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Détection Automatique

Les relations sont détectées automatiquement via des patterns linguistiques :

```go
causalPatterns := map[RelationType]*regexp.Regexp{
    RelTriggered:   regexp.MustCompile(`(?i)(?:following|after|in response to)`),
    RelBecause:     regexp.MustCompile(`(?i)(?:because|since|due to)`),
    RelContradicts: regexp.MustCompile(`(?i)(?:contradicts|however|but)`),
    RelUpdates:     regexp.MustCompile(`(?i)(?:updates|replaces|new version)`),
    RelResolves:    regexp.MustCompile(`(?i)(?:resolves|solves|fixes)`),
}
```

---

## Installation

### Prérequis

- Go 1.23+ (si compilation depuis source)
- SQLite3 (inclus)
- ~100MB d'espace disque pour le modèle d'embedding

### Depuis les Sources

```bash
# Cloner le repository
git clone https://github.com/benoitpetit/mira.git
cd mira

# Compiler
go build -o mira ./cmd/mira

# Vérifier
./mira --version
```

### Via Go Install

```bash
go install github.com/benoitpetit/mira/cmd/mira@latest
```

### Releases Binaires

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

---

## Démarrage Rapide

### 1. Initialisation

```bash
# Copier la configuration exemple
cp config.example.yaml config.yaml

# Éditer selon vos besoins
nano config.yaml
```

### 2. Démarrer le Serveur MCP

```bash
# Mode stdio (pour Claude Desktop, Cursor, etc.)
./mira

# Avec fichier de config personnalisé
./mira -config ./config.yaml
```

### 3. Utiliser les Outils MCP

#### Stocker une Mémoire

```json
{
  "tool": "mira_store",
  "arguments": {
    "content": "Nous avons décidé de migrer vers PostgreSQL pour la v2. Rejeté: MySQL (pas ACID), MongoDB (pas relationnel). Raison: ACID et expertise équipe. Validé par le CTO. Assigné à Jean.",
    "wing": "backend-team",
    "room": "database-migration"
  }
}
```

#### Récupérer du Contexte

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "Pourquoi avons-nous choisi PostgreSQL ?",
    "budget": 2000,
    "wing": "backend-team"
  }
}
```

**Réponse :**

```
=== MIRA CONTEXT ===
Requête: Pourquoi avons-nous choisi PostgreSQL ? | Budget: 2000
Wing: backend-team

--- [1] FINGERPRINT (45 tokens) ---
Décision: Migration vers PostgreSQL
Rejeté: MySQL, MongoDB
Raison: ACID, expertise équipe
Validé par: CTO
Assigné: Jean

--- [2] VERBATIM (120 tokens) ---
Nous avons décidé de migrer vers PostgreSQL pour la v2...
[contenu complet]

=== Total: 165/2000 tokens (8.3%) ===
```

#### Chaîne Causale

```json
{
  "tool": "mira_causal_chain",
  "arguments": {
    "id": "uuid-de-la-decision",
    "max_depth": 3,
    "include_consequences": true
  }
}
```

---

## Configuration

### Fichier config.yaml

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
  M: 16 # Voisins max par nœud
  Ml: 0.25 # Facteur de génération de niveau
  ef_construction: 200 # Taille liste construction
  ef_search: 50 # Taille liste recherche

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

archive_thresholds:
  session_note: 30
  debug_log: 7

mcp:
  name: "mira"
  version: "0.3.0"
  transport: "stdio"
  timeout_seconds: 30

# Export de métriques Prometheus
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

| Outil               | Description                             |
| ------------------- | --------------------------------------- |
| `mira_store`        | Stocke mémoire avec extraction T0→T1,T2 |
| `mira_recall`       | Récupère contexte optimal avec budget   |
| `mira_load`         | Charge verbatim complet par ID          |
| `mira_causal_chain` | Remonte la chaîne causale               |
| `mira_status`       | Statistiques système et santé           |
| `mira_timeline`     | Reconstruction chronologique filtrée    |
| `mira_archive`      | Archive et nettoie vieilles mémoires    |

Voir [API_REFERENCES.md](docs/API_REFERENCES.md) pour la référence API détaillée et des exemples d'utilisation.

### Endpoints de Health Check

Quand les métriques sont activées, MIRA expose des endpoints de santé :

```bash
# Health check complet (inclut DB, Vector Store, Embedder)
curl http://localhost:9090/health

# Liveness probe (Kubernetes)
curl http://localhost:9090/health/live

# Readiness probe (Kubernetes)
curl http://localhost:9090/health/ready

# Métriques Prometheus
curl http://localhost:9090/metrics
```

---

## Performance

### Complexités Algorithmiques

| Opération             | Complexité | Notes              |
| --------------------- | ---------- | ------------------ |
| Stockage T0,T1,T2     | O(1)       | Insertion atomique |
| Recherche vectorielle | O(log n)   | HNSW ANN           |
| Scoring CBA           | O(n)       | n = candidats      |
| Allocation            | O(n log n) | Max-heap           |
| BFS Graphe Causal     | O(V+E)     | V=nœuds, E=arêtes  |

### Performances Réelles

| Métrique            | Valeur                  |
| ------------------- | ----------------------- |
| Recherche HNSW      | ~1ms pour 100K vecteurs |
| Recherche SQLite    | ~50ms pour 10K vecteurs |
| Allocation complète | ~5ms avec cache         |
| Cosine similarity   | 50M ops/sec             |

### Optimisations en v0.3.0

- **Lazy Evaluation**: Calcul d'overlap uniquement pour les candidats prometteurs
- **Cache LRU**: 1000 entrées pour les embeddings de requête
- **Persistance HNSW**: Rechargement rapide de l'index au redémarrage
- **SQLite WAL Mode**: Performance lecture/écriture concurrente

---

## Architecture Technique

### Clean Architecture (Uncle Bob)

```
┌─────────────────────────────────────────────────────────────────────┐
│                    CLEAN ARCHITECTURE                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │  DOMAIN (Règles Enterprise)                                 │   │
│   │  • entities: Verbatim, Fingerprint, Embedding, Candidate   │    │
│   │  • valueobjects: MemoryType, RenderMode, RelationType      │    │
│   │  ✓ Aucune dépendance externe                               │    │
│   └─────────────────────────────────────────────────────────────┘   │
│                              ▲                                      │
│                              │ Dépendance                           │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │  USE CASES (Règles Application)                             │   │
│   │  • StoreMemory, RecallMemory (CBA), LoadMemory              │   │
│   │  • GetTimeline, GetStatus, GetCausalChain, Archive          │   │
│   │  • ports: Interfaces Repository                             │   │
│   │  ✓ Dépend uniquement du Domain                              │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              ▲                                      │
│                              │                                      │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │  INTERFACE ADAPTERS                                         │   │
│   │  • storage: SQLiteRepository                                │   │
│   │  • vector: HNSWStore, SQLiteVectorStore                     │   │
│   │  • extraction: ProseExtractor, CybertronEmbedder            │   │
│   │  • webhook, metrics                                         │   │
│   │  ✓ Implémente les ports                                     │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              ▲                                      │
│                              │                                      │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │  FRAMEWORKS & DRIVERS                                       │   │
│   │  • SQLite3, HNSW lib, Cybertron, MCP Server                 │   │
│   │  ✓ Détails techniques extérieurs                            │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Structure du Projet

```
mira/
├── cmd/mira/              # Point d'entrée
├── internal/
│   ├── domain/            # Couche Domaine
│   │   ├── entities/      # Entités métier
│   │   └── valueobjects/  # Objets valeur
│   ├── usecases/          # Couche Use Cases
│   │   ├── ports/         # Interfaces (Repository, Services)
│   │   └── interactors/   # Implémentations use cases
│   ├── adapters/          # Couche Adapters
│   │   ├── storage/       # SQLite repository
│   │   ├── vector/        # HNSW, SQLite vector store
│   │   ├── extraction/    # NLP, embeddings
│   │   ├── webhook/       # Notifications HTTP
│   │   └── metrics/       # Métriques Prometheus
│   ├── interfaces/        # Couche Interfaces
│   │   └── mcp/           # Contrôleur MCP
│   ├── config/            # Configuration
│   └── app/               # Racine de composition (DI)
│       ├── main.go        # Injection de dépendances
│       ├── health.go      # Health checks
│       ├── metrics.go     # Collecte de métriques
│       └── retry.go       # Logique de retry
├── docs/                  # Documentation
│   ├── WHITEPAPER.md      # Whitepaper technique
│   ├── API_EXAMPLES.md    # Exemples d'usage API
│   └── API_REFERENCES.md  # Référence API
├── config.example.yaml    # Configuration exemple
└── README_FR.md           # Ce fichier
```

---

## Développement

### Tests

```bash
# Tests unitaires
go test -v ./...

# Avec détection de race
go test -race ./...

# Benchmarks
go test -bench=. -benchmem ./...

# Couverture
go test -cover ./...
```

### Commandes Make

```bash
make build      # Compiler
make test       # Tests
make bench      # Benchmarks
make migrate    # Migrations DB
make clean      # Nettoyer
```



## Changelog

### v0.3.0 (2026-04-10)

**Release Majeure - Refactoring Complet**

#### ✅ Nouvelles Fonctionnalités

- **Clean Architecture**: Restructuration complète du codebase avec couches appropriées
- **Index Vectoriel HNSW**: Recherche approximative des plus proches voisins en O(log n)
- **Embeddings Cybertron**: Embeddings transformers réels (all-MiniLM-L6-v2)
- **Système de Métriques**: Métriques compatibles Prometheus avec 10 métriques
- **Notifications Webhook**: Callbacks HTTP avec signatures HMAC
- **Health Checks**: Sondes liveness/readiness compatibles Kubernetes
- **Circuit Breaker**: Pattern de résilience pour les webhooks
- **Retry Logic**: Backoff exponentiel pour opérations résilientes

#### ✅ Améliorations

- **Couverture de Tests**: 55.9% → 77.1% (ajout de 55+ tests)
- **Score de Qualité**: 70/100 → 88/100
- **Support Context**: Ajout de `context.Context` partout (40+ fichiers)
- **Lazy Evaluation**: Optimisation du calcul d'overlap CBA
- **Persistance HNSW**: Sauvegarde/chargement complet de la structure

#### ✅ Corrections de Bugs

- Chargement HNSW BuildFromStore
- Tri par similarité dans SQLiteVectorStore
- Implémentation GetFingerprintByID
- Routage des événements webhook
- Vérifications de causalité temporelle

### v0.2.0 (2026-04-09)

- Fondation HNSW
- Whitepaper technique

### v0.1.0 (2026-04-08)

- Version initiale

---

## Références

### Librairies Clés

- [tiktoken-go](https://github.com/pkoukk/tiktoken-go) - Tokenization OpenAI
- [prose](https://github.com/jdkato/prose) - NLP/NER en Go
- [cybertron](https://github.com/nlpodyssey/cybertron) - Embeddings transformers
- [hnsw](https://github.com/coder/hnsw) - Graphes HNSW
- [mcp-go](https://github.com/mark3labs/mcp-go) - Protocole MCP

### Modèle d'Embedding

- **Modèle:** sentence-transformers/all-MiniLM-L6-v2
- **Dimensions:** 384
- **Taille:** ~80MB
- **Performance:** ~1000 textes/sec sur CPU

---

<div align="center">

**MIRA** - _Memory with Information-theoretic Relevance Allocation_

_"La mémoire est la sève de l'intelligence artificielle."_

[📘 Référence API](docs/API_REFERENCES.md) • [📝 Changelog](CHANGELOG.md)

</div>
