<div align="center">
  <img src="./logo.png" alt="MIRA Logo" width="800">

  # MIRA
  ### Memory with Information-theoretic Relevance Allocation

  **Système de Mémoire Long-Terme pour LLMs avec Allocation Optimale de Budget Contextuel**

  [![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
  [![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
  [![Version](https://img.shields.io/badge/Version-0.4.7-blue?style=flat-square)]()
  [![Tests](https://img.shields.io/badge/Tests-~70%25-yellow?style=flat-square)]()

  *100% Local • Déterministe (variance embedding < 1e-6) • Clean Architecture*

  [Référence API](docs/API_REFERENCES.md) • [Changelog](CHANGELOG.md) • [Skill](SKILL.md) • [English](README.md) • [Extension SOUL](https://github.com/benoitpetit/soul)

</div>

---

## Table des matières

- [Qu'est-ce que MIRA ?](#quest-ce-que-mira-)
- [Fonctionnement](#fonctionnement)
- [Architecture à 3 niveaux (T0/T1/T2)](#architecture-à-3-niveaux-t0t1t2)
- [L'algorithme CBA](#lalgorithme-cba)
- [Pipeline de recall amélioré](#pipeline-de-recall-amélioré)
- [Graphe causal](#graphe-causal)
- [Installation](#installation)
- [Démarrage rapide](#démarrage-rapide)
- [Configuration](#configuration)
- [API MCP](#api-mcp)
- [API REST](#api-rest)
- [Performance](#performance)
- [Architecture technique](#architecture-technique)
- [Développement](#développement)
- [Changelog](#changelog)

---

## Qu'est-ce que MIRA ?

**MIRA** est un système de mémoire long-terme conçu pour les **Large Language Models**. Au lieu d'une simple récupération par similarité, MIRA résout un problème d'optimisation : maximiser l'information utile dans un budget de tokens fixe.

Chaque mémoire est stockée sous trois formes — texte complet (T0), faits structurés (T1) et embedding 384 dimensions (T2) — permettant un rendu adaptatif selon le budget disponible.

**Les approches traditionnelles sont insuffisantes :**

- Le RAG simple récupère par similarité et ignore la densité d'information
- La fenêtre glissante perd les informations critiques du début
- Le résumé statique ne s'adapte pas à la requête courante
- La Vector DB basique a une complexité O(n) sans gestion de budget

**MIRA apporte :**

- **Allocation de Budget Contextuel (CBA)** — maximise l'information sur 6 dimensions de scoring
- **Triple représentation (T0/T1/T2)** — rendu adaptatif du texte complet jusqu'à un en-tête de 5 tokens
- **Recherche hybride** — HNSW O(log n) + SQLite FTS5, fusionné avec Reciprocal Rank Fusion
- **Graphe causal** — détection automatique des relations cause-effet entre les mémoires
- **Clean architecture** — hexagonale, testée, extensible

> **Besoin de persistance d'identité ?** L'extension optionnelle [SOUL](https://github.com/benoitpetit/soul) ajoute 8 outils MCP pour capturer et rappeler la personnalité d'un agent à travers les changements de modèle — activée par un simple flag `--with-soul`.

---

## Fonctionnement

### Stockage

```
Texte input  →  Extraction T1/T2  →  SQLite (T0 + T1) + Index HNSW (T2)
```

À la sauvegarde d'une mémoire, l'extracteur natif produit :

- **T1** — un fingerprint JSON structuré (~15% des tokens d'origine)
- **T2** — un embedding 384 dimensions pour la recherche sémantique

Les deux sont dérivés atomiquement et stockés aux côtés du verbatim original (T0).

### Recall

```
Requête  →  Embed  →  HNSW top-100 (+ FTS5)  →  Fusion RRF  →  Scoring CBA  →  Sélection gloutonne
```

L'algorithme CBA sélectionne les mémoires de façon gloutonne dans un budget de tokens, ajustant le mode de rendu de chaque mémoire (Verbatim / Fingerprint / Header) selon les tokens restants.

### Score composite CBA

**S(m) = ρ × δ × η × (1−σ) × τ × χ × 𝟙[ρ>θ]**

| Symbole | Dimension | Formule |
|---------|-----------|---------|
| ρ | Pertinence sémantique | cos(embedding_m, requête) |
| δ | Densité informationnelle | sigmoïde(faits / √tokens) |
| η | Poids temporel | exp(−λ × âge) |
| σ | Chevauchement max | similarité max avec mémoires déjà sélectionnées |
| τ | Boost session | +20% si dans la même fenêtre de 2h |
| χ | Pénalité causale | exp(−0.15 × liens causaux vers la sélection) |
| 𝟙[ρ>θ] | Seuil | exclure si ρ < 0.6 |

---

## Architecture à 3 niveaux (T0/T1/T2)

Le cerveau humain n'enregistre pas tout avec la même fidélité. MIRA imite cette hiérarchie.

### T0 — Verbatim (Mémoire épisodique)

Le texte original complet, stocké en UTF-8 (max 64 Ko). Utilisé quand le budget permet un contexte riche.

- **Stockage :** texte UTF-8 complet
- **Coût :** ~200 tokens

### T1 — Fingerprint (Mémoire sémantique)

Un JSON canonique structuré avec les faits, entités, décisions et relations extraits.

```json
{
  "type": "decision",
  "decision": "Migration vers PostgreSQL",
  "rejected": ["MySQL", "MongoDB"],
  "reason": ["robustesse ACID", "expertise équipe"],
  "assignee": "Jean",
  "deadline": "Sprint 5",
  "validated_by": "Sophie (PO)"
}
```

- **Stockage :** JSON canonique
- **Coût :** ~30 tokens (15% de T0)

### T2 — Embedding (Index de recherche)

Un vecteur float32 de 384 dimensions utilisé exclusivement pour la recherche HNSW. Jamais rendu dans le contexte.

- **Stockage :** float32[384]
- **Coût :** 0 token (recherche uniquement)

### Types de mémoire et décroissance

| Type | λ (jour⁻¹) | Demi-vie | Auto-archive | Usage |
|------|-----------|----------|--------------|-------|
| `decision` | 0.001 | ~693 jours | Non | Décisions architecturales |
| `fact` | 0.005 | ~139 jours | Non | Connaissances, faits |
| `preference` | 0.01 | ~69 jours | Non | Préférences utilisateur |
| `session_note` | 0.1 | ~7 jours | 30 jours | Notes de session |
| `debug_log` | 0.5 | ~1.4 jours | 7 jours | Logs de debug |

---

## L'algorithme CBA

### Algorithme (O(n²))

```
ENTRÉE :  Requête q, Budget B (tokens), Wing w, Room r
SORTIE :  Liste de mémoires avec mode de rendu

1. EMBEDDING
   e_q ← Embed(q)  — avec cache LRU (1000 entrées)

2. RECHERCHE VECTORIELLE
   C ← HNSW_Search(e_q, N=100, w, r)           // O(log n)
   Si HNSW non prêt : C ← SQLite_Search(...)    // Fallback

3. ÉLAGAGE PRÉCOCE
   C' ← { c ∈ C : ρ(c,q) > 0.6 }
   Si C' = ∅ : C' ← top-5(C) par ρ

4. SCORING INITIAL
   Pour chaque c ∈ C' :
      c.score ← ρ(c) × δ_sigmoïde(c) × η_récence(c)

5. SÉLECTION GLOUTONNE avec renormalisation dynamique
   S ← ∅, utilisé ← 0
   PQ ← MaxHeap(C')

   Tant que PQ ≠ ∅ et utilisé < B :
      c ← Pop(PQ)
      c.σ ← max_{s∈S} sim(c, s)
      c.χ ← exp(−0.15 × |liens_causaux(c, S)|)
      c.τ ← 1.2 si |temps(c) − temps(S)| < 2h sinon 1.0
      ajusté ← c.score × (1−c.σ) × c.χ × c.τ

      Si PQ[0].score × 0.8 > ajusté :
         Push(PQ, c) avec score ajusté ; continuer

      mode ← ChoisirMode(c, B − utilisé)
      coût ← Coût(c, mode)
      Si utilisé + coût > B : Dégrader(mode) ; Recalculer ; ignorer si toujours dépassé

      S ← S ∪ {c}, utilisé ← utilisé + coût

6. RETOURNER S trié par score décroissant
```

### Modes de rendu adaptatif

| Budget restant | Mode | Tokens | Contenu |
|---------------|------|--------|---------|
| < 100 | Header | 2–5 | `[type\|date\|wing]` |
| < 1000 | Fingerprint | ~15% | Faits essentiels T1 |
| ≥ 1000 | Verbatim | 100% | Texte complet T0 |

---

## Pipeline de recall amélioré

```
Requête → Expansion → Dense (HNSW) + Lexical (FTS5) → Fusion RRF → Clustering → Boost Tags → Seuil Adaptatif → Sélection Gloutonne CBA
```

### 1. Expansion de requête

MIRA génère des variantes sémantiquement proches de la requête (nettoyée, sans mots vides, mots-clés principaux) et **moyenne leurs embeddings**. Cela améliore la récupération cross-lingue et la robustesse aux variations de vocabulaire.

### 2. Recherche hybride (Dense + Lexicale)

- **Dense :** recherche vectorielle HNSW en O(log n)
- **Lexicale :** recherche full-text SQLite FTS5 (auto-activée si disponible)
- **Fusion :** Reciprocal Rank Fusion (`k=60`) fusionne les deux classements

### 3. Clustering à la recherche

Les candidats sont regroupés par cosine similarity ≥ 0.88. Les quasi-doublons sont fusionnés vers leur meilleur représentant, évitant de gaspiller le budget sur des mémoires redondantes.

### 4. Récupération par tags

La table `memory_tags` indexe les entités, sujets et mots-clés extraits. Les candidats correspondant aux tags de la requête reçoivent un petit boost de pertinence additif.

### 5. Seuil adaptatif

Au lieu d'un seuil fixe à 0.6, MIRA supporte trois méthodes dynamiques :

| Méthode | Description | Défaut |
|---------|-------------|--------|
| `iqr` | Premier quartile de la distribution des scores | Oui |
| `elbow` | Plus forte chute de dérivée | |
| `mean_stddev` | moyenne − écart-type | |

Le seuil est borné entre 0.15 (plancher) et 0.75 (plafond).

### 6. Reranker heuristique (optionnel)

Un reranker léger 100% Go combine signaux sémantiques et lexicaux :

- Chevauchement lexical de type Jaccard
- Bonus de présence de phrase exacte
- Préférence d'équilibre de longueur

Mélange : `0.7 × sémantique + 0.3 × rerank`

### 7. Vector Store de fallback

Si HNSW n'est pas encore prêt (ex. reconstruction depuis zéro), un wrapper transparent redirige automatiquement vers le vector store SQLite. Le recall ne tombe jamais en panne.

### 8. Compression contextuelle

Compression contextuelle à base de règles pour les verbatims `session_note`. Le texte résumé est stocké aux côtés de l'original et surfaced par le moteur de recall quand le budget de tokens est trop serré pour le verbatim complet.

- **Pas besoin de LLM** — déterministe, compression instantanée
- **Auto-compression** au moment du stockage (async, non-fatale) ou à la demande via `mira_compress`
- **Configurable** — définir le seuil `min_tokens` pour ignorer les notes courtes

```yaml
compression:
  auto_compress: false   # Auto-compresser au moment du stockage
  min_tokens: 100        # Seuil minimum de tokens pour se qualifier
```

---

## Graphe causal

### Relations supportées

| Relation | Signification | Déclenchée par |
|----------|--------------|----------------|
| `BECAUSE` | B explique pourquoi A | "because", "since", "due to" |
| `TRIGGERED` | B a déclenché A | "following", "after", "in response to" |
| `CONTRADICTS` | A et B sont incompatibles | "contradicts", "however" |
| `UPDATES` | B remplace A | "updates", "replaces" |
| `RESOLVES` | B résout le problème A | "resolves", "solves", "fixes" |

### Détection automatique

```go
causalPatterns := map[RelationType]*regexp.Regexp{
    RelTriggered:   regexp.MustCompile(`(?i)(?:following|after|in response to)`),
    RelBecause:     regexp.MustCompile(`(?i)(?:because|since|due to|in reason of)`),
    RelContradicts: regexp.MustCompile(`(?i)(?:contradicts|in contradiction|however)`),
    RelUpdates:     regexp.MustCompile(`(?i)(?:updates|replaces)`),
    RelResolves:    regexp.MustCompile(`(?i)(?:resolves|solves|fixes)`),
}
```

---

## Installation

### Prérequis

- Go 1.25+ (si compilation depuis les sources)
- SQLite3 (inclus)
- ~100 Mo d'espace disque pour le modèle d'embedding

### Depuis les sources

```bash
git clone https://github.com/benoitpetit/mira.git
cd mira
go build -o mira ./cmd/mira
./mira --version
```

### Via Go Install

```bash
go install github.com/benoitpetit/mira/cmd/mira@latest
```

### Releases binaires

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

## Démarrage rapide

### 1. Initialisation

```bash
cp config.example.yaml config.yaml
nano config.yaml
```

### 2. Démarrer le serveur MCP

```bash
# Mode stdio — pour Claude Desktop, Cursor, etc.
./mira server

# Avec un fichier de config personnalisé
./mira --config ./config.yaml server

# Avec un chemin de stockage personnalisé (aussi : MIRA_DATA_PATH)
./mira --storage-path /data/mira server

# Modes de transport MCP : stdio (défaut), sse
./mira server --transport sse --mcp-addr localhost:3001

# Activer l'API REST optionnelle
./mira server --with-api --api-addr :8080 --api-token mon-secret

# Activer l'extension SOUL
./mira server --with-soul

# Métriques Prometheus (défaut : :9090)
./mira server --prometheus-addr :9091

# Désactiver les métriques Prometheus
./mira server --no-metrics

# Extraction via Ollama (nécessite une instance Ollama en cours)
./mira server --with-llm --llm-endpoint http://localhost:11434
```

### 3. Commandes utilitaires

```bash
# Migrations de base de données
./mira migrate

# Santé du système (lisible ou JSON)
./mira doctor
./mira doctor --json

# Statut système (pour scripting/monitoring)
./mira status
./mira status --json

# Requête one-shot depuis le CLI
./mira query --query "Pourquoi avons-nous choisi PostgreSQL ?" --wing backend-team
./mira query -q "décisions API" --json

# Stocker une mémoire depuis le CLI
./mira store --content "PostgreSQL choisi pour la DB primaire" --wing backend-team --type decision

# Supprimer une mémoire par UUID
./mira delete 5a159ddf-bc11-46a6-8a0d-f39f25853cb4

# Exporter les mémoires en JSON
./mira export --wing backend-team --output memories.json

# Importer des mémoires depuis JSON (avec aperçu dry-run)
./mira import --file memories.json
./mira import --file memories.json --dry-run

# Optimiser un fichier d'historique de chat pour tenir dans un budget de tokens (sans LLM)
./mira optimize --file history.json --budget 2000
./mira optimize --file history.json --stats-only

# Validation et inspection de la configuration
./mira config validate
./mira config show
./mira config show --json
```

### 4. Utiliser les outils MCP

#### Stocker une mémoire

```json
{
  "tool": "mira_store",
  "arguments": {
    "content": "Nous avons décidé de migrer vers PostgreSQL pour la v2. Rejeté : MySQL (pas ACID), MongoDB (pas relationnel). Raison : ACID et expertise équipe. Validé par le CTO. Assigné à Jean.",
    "wing": "backend-team",
    "room": "database-migration"
  }
}
```

#### Récupérer du contexte

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
Requête : Pourquoi avons-nous choisi PostgreSQL ? | Budget : 2000
Wing : backend-team

--- [1] FINGERPRINT (45 tokens) ---
Décision : Migration vers PostgreSQL
Rejeté : MySQL, MongoDB
Raison : ACID, expertise équipe
Validé par : CTO
Assigné : Jean

--- [2] VERBATIM (120 tokens) ---
Nous avons décidé de migrer vers PostgreSQL pour la v2...

=== Total : 165/2000 tokens (8.3%) ===
```

#### Chaîne causale

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

### config.yaml

```yaml
system:
  version: "0.4.7"

storage:
  path: ".mira"
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

hnsw:
  M: 32
  Ml: 0.25
  ef_construction: 0   # inactif — non supporté par la librairie sous-jacente
  ef_search: 100

allocator:
  default_budget: 4000
  max_candidates: 100
  early_pruning_threshold: 0.6
  session_window_seconds: 7200
  session_boost_beta: 0.2
  session_boost_max: 1.2
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

overlap_cache:
  ttl_days: 30
  max_entries: 1000000

extraction:
  min_entity_length: 2
  causal_lookback: 50
  causal_max_days: 30

recall:
  adaptive_threshold_method: "iqr"
  adaptive_threshold_floor: 0.15
  adaptive_threshold_ceiling: 0.75
  enable_fts5: true
  fts5_limit: 100
  rrf_k: 60
  query_expansion:
    enabled: true
    num_variants: 3
    temperature: 0.3
  search_time_clustering:
    enabled: true
    similarity_threshold: 0.88
  reranker:
    enabled: false
    top_k: 30

# Extension d'identité SOUL (désactivée par défaut)
soul:
  enabled: false

mcp:
  name: "mira"
  version: "0.4.7"
  transport: "stdio"   # "stdio" pour Claude Desktop/Cursor, "sse" pour HTTP SSE
  address: "localhost:3001"
  timeout_seconds: 30

# API REST HTTP optionnelle
api:
  enabled: false
  address: ":8080"
  auth_token: ""
  read_timeout_seconds: 30
  write_timeout_seconds: 30

# Métriques Prometheus
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

### Outils disponibles

| Outil | Description |
|-------|-------------|
| `mira_store` | Stocker une mémoire avec extraction T0/T1/T2 |
| `mira_recall` | Récupérer le contexte optimal dans un budget de tokens |
| `mira_load` | Charger le verbatim complet par UUID |
| `mira_causal_chain` | Remonter la chaîne causale depuis une mémoire |
| `mira_status` | Statistiques système et santé |
| `mira_health` | Health check rapide (JSON) |
| `mira_timeline` | Reconstruction chronologique des mémoires |
| `mira_archive` | Archiver et nettoyer les vieilles mémoires |
| `mira_clear_memory` | Suppression permanente (globale ou par room) |
| `mira_compress` | Compression contextuelle à base de règles pour les session_notes |

### Wings de secours (Fallback Wings)

Quand un recall dans un wing principal ne retourne rien, `mira_recall` supporte des wings de secours séparés par des virgules :

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "stratégie de migration base de données",
    "budget": 2000,
    "wing": "backend-team",
    "fallback_wings": "platform-team,dba-team"
  }
}
```

### Recherche multilingue

`mira_recall` accepte les requêtes dans n'importe quelle langue grâce aux embeddings cross-lingues. Lorsqu'une requête dans une langue cherche des mémoires dans une autre, MIRA élargit automatiquement la recherche avec des seuils relaxés.

```json
{
  "tool": "mira_recall",
  "arguments": {
    "query": "règles de langue français anglais",
    "budget": 2000,
    "wing": "general"
  }
}
```

Voir [API_REFERENCES.md](docs/API_REFERENCES.md) pour la référence complète.

### Endpoints de health check

Quand les métriques sont activées, MIRA expose des endpoints de santé :

```bash
curl http://localhost:9090/health        # Vérification complète (DB, Vector Store, Embedder)
curl http://localhost:9090/health/live   # Liveness probe (Kubernetes)
curl http://localhost:9090/health/ready  # Readiness probe (Kubernetes)
curl http://localhost:9090/metrics       # Métriques Prometheus
```

---

## API REST

MIRA embarque une API REST HTTP optionnelle pour les scripts, tableaux de bord ou intégrations non-MCP. Désactivée par défaut.

### Activation

```bash
# Via flag CLI
./mira server --with-api --api-addr :8080 --api-token mon-secret

# Via config.yaml
api:
  enabled: true
  address: ":8080"
  auth_token: "mon-secret"
```

### Authentification

Quand `auth_token` est défini, chaque requête doit porter :

```
Authorization: Bearer mon-secret
```

L'endpoint `/openapi.json` est toujours public.

### Endpoints

| Méthode | Chemin | Description |
|---------|--------|-------------|
| `POST` | `/api/v1/memories` | Stocker une mémoire |
| `GET` | `/api/v1/memories/{id}` | Charger le verbatim complet par UUID |
| `PUT` | `/api/v1/memories/{id}` | Mettre à jour le contenu |
| `DELETE` | `/api/v1/memories/{id}` | Supprimer une mémoire |
| `POST` | `/api/v1/memories/recall` | Recall de contexte (pipeline CBA complet) |
| `POST` | `/api/v1/memories/search` | Recherche sémantique vectorielle |
| `POST` | `/api/v1/memories/consolidate` | Consolider les mémoires redondantes |
| `DELETE` | `/api/v1/memories` | Effacer les mémoires (global ou scopé) |
| `GET` | `/api/v1/timeline` | Timeline chronologique |
| `POST` | `/api/v1/archive` | Archiver les vieilles mémoires |
| `GET` | `/api/v1/causal/{id}` | Chaîne causale d'une mémoire |
| `GET` | `/api/v1/status` | Statut système (JSON) |
| `GET` | `/openapi.json` | Spécification OpenAPI 3.1 |

### Exemples rapides

```bash
# Stocker une mémoire
curl -s -X POST http://localhost:8080/api/v1/memories \
  -H "Authorization: Bearer mon-secret" \
  -H "Content-Type: application/json" \
  -d '{"content":"Nous avons choisi PostgreSQL pour v2","wing":"backend","type":"decision"}'

# Recall de contexte
curl -s -X POST http://localhost:8080/api/v1/memories/recall \
  -H "Authorization: Bearer mon-secret" \
  -H "Content-Type: application/json" \
  -d '{"query":"Pourquoi PostgreSQL ?","budget":2000,"wing":"backend"}'

# Obtenir la spec OpenAPI (sans auth)
curl -s http://localhost:8080/openapi.json | jq .info
```

Voir [docs/API_REFERENCES.md](docs/API_REFERENCES.md) pour la référence complète avec tous les schémas.

---

## Performance

### Complexité algorithmique

| Opération | Complexité | Notes |
|-----------|------------|-------|
| Stockage T0, T1, T2 | O(1) | Insertion atomique |
| Recherche vectorielle | O(log n) | HNSW ANN |
| Scoring CBA | O(n) | n = candidats |
| Allocation gloutonne | O(n²) | Avec renormalisation dynamique |
| BFS graphe causal | O(V+E) | V = nœuds, E = arêtes |

### Benchmarks

| Métrique | Valeur |
|----------|--------|
| Recherche HNSW | ~0.14 ms pour 10K vecteurs (benchmarké) |
| Recherche SQLite fallback | ~50 ms pour 10K vecteurs (estimation) |
| Allocation complète | ~35 ms pour 100 candidats (estimation) |
| Cosine similarity | ~3.3M ops/sec |

### Optimisations en v0.3.3

- **Expansion de requête** — moyenne d'embeddings de variantes pour une récupération cross-lingue robuste
- **Recherche lexicale FTS5** — recherche full-text SQLite avec triggers auto et backfill
- **Fusion hybride RRF** — Reciprocal Rank Fusion (`k=60`) combinant HNSW et FTS5
- **Clustering à la recherche** — déduplication en temps réel à cosine similarity ≥ 0.88
- **Récupération par tags** — table `memory_tags` avec boost automatique dans le scoring CBA
- **Reranker heuristique** — reranker lexical léger optionnel
- **Méthodes de seuil adaptatif** — élagage dynamique avec `iqr`, `elbow`, `mean_stddev`
- **Vector Store de fallback** — fallback transparent HNSW → SQLite quand l'index n'est pas prêt
- **Outil Clear Memory** — `mira_clear_memory` pour suppression globale ou par room
- **Résolution T0 chaîne causale** — `mira_causal_chain` résout les références `T0:` en IDs fingerprint
- **Visibilité des IDs** — `mira_recall` et `mira_timeline` incluent les IDs mémoire pour chaîner les outils

### Optimisations en v0.3.1

- **Lazy Evaluation** — calcul d'overlap uniquement pour les candidats prometteurs
- **Cache LRU** — 1000 entrées pour les embeddings de requête
- **Persistance HNSW** — rechargement rapide de l'index au redémarrage
- **SQLite WAL Mode** — performance lecture/écriture concurrente
- **Seuil adaptatif** — abaissement du seuil de pertinence pour les petits corpus (< 10 mémoires)
- **Mapping room par défaut** — assignation automatique des rooms standards selon le type

---

## Architecture technique

### Architecture hexagonale (Clean Architecture)

**Domain** — règles enterprise, aucune dépendance externe
- `entities` : Verbatim, Fingerprint, Embedding, Candidate
- `valueobjects` : MemoryType, RenderMode, RelationType

**Use Cases** — règles applicatives, dépend uniquement du Domain
- StoreMemory, RecallMemory (CBA), LoadMemory
- GetTimeline, GetStatus, GetCausalChain, Archive
- `ports` : interfaces Repository et services

**Interface Adapters** — implémente les ports
- `storage` : SQLiteRepository
- `vector` : HNSWStore, SQLiteVectorStore, overlap cache
- `extraction` : NativeExtractor, CybertronEmbedder
- `webhook`, `metrics`

**Frameworks & Drivers** — détails techniques extérieurs
- SQLite3, HNSW lib, Cybertron, MCP Server

### Structure du projet

```
mira/
├── cmd/mira/              # Point d'entrée (CLI cobra)
│   └── main.go            # Sous-commandes : server, migrate, doctor, query, export, import
├── internal/
│   ├── domain/
│   │   ├── entities/      # Entités métier
│   │   └── valueobjects/  # Objets valeur
│   ├── usecases/
│   │   ├── ports/         # Interfaces (Repository, Services)
│   │   └── interactors/   # Implémentations use cases
│   ├── adapters/
│   │   ├── storage/       # SQLite repository
│   │   ├── vector/        # HNSW, SQLite vector store, overlap cache
│   │   ├── extraction/    # NLP, embeddings
│   │   ├── logging/       # Logging structuré
│   │   ├── webhook/       # Notifications HTTP
│   │   └── metrics/       # Métriques Prometheus
│   ├── interfaces/
│   │   ├── mcp/           # Contrôleur MCP (stdio / SSE)
│   │   └── rest/          # API REST HTTP optionnelle (:8080)
│   ├── config/
│   └── app/               # Racine de composition (injection de dépendances)
├── docs/
│   ├── INDEX.md
│   ├── ARCHITECTURE.md
│   ├── FEATURES.md
│   └── API_REFERENCES.md
├── SKILL.md
├── config.example.yaml
└── README_FR.md
```

---

## Développement

### Tests

```bash
go test -v ./...                       # Tests unitaires
go test -race ./...                    # Avec détection de race
go test -bench=. -benchmem ./...      # Benchmarks
go test -cover ./...                   # Couverture
```

### Commandes Make

```bash
make build        # Compiler
make test         # Tests (avec race detector)
make test-short   # Tests rapides
make bench        # Benchmarks
make bench-full   # Benchmarks complets
make run          # Compiler et lancer avec config.yaml
make clean        # Nettoyer les artefacts et données
make lint         # Linters
make fmt          # Formater le code
make install      # Installer dans GOPATH/bin
make prepublish VERSION=x.y.z  # Préparer une release
```

## Changelog

Voir [CHANGELOG.md](CHANGELOG.md) pour l'historique complet des releases.

---

## Références

### Bibliothèques clés

- [tiktoken-go](https://github.com/pkoukk/tiktoken-go) — tokenisation OpenAI
- Implémentation Go native — NLP/NER à base de règles
- [cybertron](https://github.com/nlpodyssey/cybertron) — embeddings Transformer
- [hnsw](https://github.com/coder/hnsw) — graphes HNSW
- [mcp-go](https://github.com/mark3labs/mcp-go) — protocole MCP

### Modèle d'embedding

- **Modèle :** sentence-transformers/all-MiniLM-L6-v2
- **Dimensions :** 384
- **Taille :** ~80 Mo
- **Performance :** ~1000 textes/sec sur CPU

---

<div align="center">

**MIRA** — _Memory with Information-theoretic Relevance Allocation_

_« La mémoire est la sève de l'intelligence artificielle. »_

[Référence API](docs/API_REFERENCES.md) • [Changelog](CHANGELOG.md)

</div>
