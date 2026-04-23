<div align="center">
  <img src="./logo.png" alt="MIRA Logo" width="800">
  
  # MIRA
  ### Memory with Information-theoretic Relevance Allocation
  
  **Système de Mémoire Long-Terme pour LLMs avec Allocation Optimale de Budget Contextuel**
  
  [![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
  [![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
  [![Version](https://img.shields.io/badge/Version-0.4.4-blue?style=flat-square)]()
  [![Tests](https://img.shields.io/badge/Tests-77%25-brightgreen?style=flat-square)]()
  
  *100% Local • Déterministe • O(n log n) • Clean Architecture*
  
  [Reference API](docs/API_REFERENCES.md) • [Changelog](CHANGELOG.md) • [Skill](SKILL.md) • [English](README.md)
  
</div>

---

## Table des Matieres

- [Qu'est-ce que MIRA ?](#quest-ce-que-mira-)
- [Extension SOUL : Préservation de l'Identité](#extension-soul--préservation-de-lidentité)
- [La Révolution de la Mémoire pour LLMs](#la-révolution-de-la-mémoire-pour-llms)
- [Fonctionnement](#fonctionnement)
- [Architecture à 3 Niveaux (T0/T1/T2)](#architecture-à-3-niveaux-t0t1t2)
- [L'Algorithme CBA](#lalgorithme-cba)
- [Pipeline de Recall Amélioré](#pipeline-de-recall-amélioré)
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

Les LLMs modernes souffrent d'un problème fondamental : **la fenêtre de contexte est limitée** (4K-128K tokens), mais les conversations et projets s'étendent sur des milliers d'interactions. Comment décider quoi garder dans le contexte ?

**Les approches traditionnelles echouent :**

- [x] RAG simple : Recuperation basee uniquement sur la similarite, ignore la densite d'information
- [x] Fenetre glissante : Perd les informations critiques du debut
- [x] Resume statique : Ne s'adapte pas a la requete actuelle
- [x] Vector DB basique : Complexite O(n), pas de gestion de budget

**MIRA apporte la solution :**

- [+] **Allocation de Budget Contextuel** : Optimise chaque token sur 6 dimensions
- [+] **Densite Informationnelle** : Privilegie les memoires riches en faits
- [+] **Coherence Temporelle** : Maintient la continuite narrative
- [+] **Graphe Causal** : Comprend les relations cause-effet
- [+] **Recherche O(log n)** : HNSW pour des millions de memoires
- [+] **Clean Architecture** : Maintenable, testable, extensible

---

## Extension SOUL : Préservation de l'Identite

MIRA repond *"Que sait l'agent ?"*. Mais un agent complet a besoin de plus : il doit savoir *"Qui est-il ?"*

[**SOUL**](https://github.com/benoitpetit/soul) (System for Observed Unique Legacy) est une **extension d'identite optionnelle** pour MIRA qui capture, stocke et rappelle la personnalite, la voix et les valeurs des agents IA au travers des sessions et des changements de modele.

Pour integrer SOUL dans MIRA, demarrez avec `--with-soul` ou definissez `soul.enabled: true` dans la configuration. Quand il est active, SOUL fournit **8 outils MCP supplementaires** (16 au total) pour :
- Capturer l'identite a partir des conversations
- Rappeler les prompts d'identite pour l'injection dans le contexte LLM
- Detecter la derive d'identite apres un changement de modele
- Generer des prompts de renforcement apres un changement de modele

SOUL est **opt-in et desactive par defaut**. MIRA fonctionne parfaitement seul (8 outils). Pour activer SOUL, utilisez le flag `--with-soul` ou definissez `soul.enabled: true` dans `config.yaml`. Quand il est active, ils partagent la meme base de donnees SQLite.

| Configuration | Outils | Ce que ca repond |
|---------------|--------|-----------------|
| MIRA seul | 8 `mira_*` | "Que sait l'agent ?" |
| MIRA + SOUL | 16 `mira_*` + `soul_*` | "Que sait l'agent ?" + "Qui est l'agent ?" |

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
| `decision`     | 0.001      | ~693 jours | No           | Decisions architecturales  |
| `fact`         | 0.005      | ~139 jours | No           | Connaissances, faits       |
| `preference`   | 0.01       | ~69 jours  | No           | Preferences utilisateur    |
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

## Pipeline de Recall Amélioré

Le recall de MIRA utilise un **pipeline de récupération multi-étapes** qui va bien au-delà de la simple similarité vectorielle :

```
Requête → Expansion → Dense (HNSW) + Lexical (FTS5) → Fusion RRF → Clustering → Boost par Tags → Seuil Adaptatif → Sélection Gloutonne CBA
```

### 1. Expansion de Requête
Avant l'embedding, MIRA génère des variantes sémantiquement proches de la requête (nettoyée, sans mots vides, mots-clés principaux) et **moyenne leurs embeddings**. Cela améliore la récupération cross-lingue et la robustesse aux variations de vocabulaire.

### 2. Recherche Hybride (Dense + Lexicale)
- **Dense** : recherche vectorielle HNSW en O(log n)
- **Lexicale** : recherche full-text SQLite FTS5 (auto-activée si disponible)
- **Fusion** : Reciprocal Rank Fusion (`k=60`) fusionne les deux classements en une seule liste de candidats

### 3. Clustering à la Recherche
Les candidats sont groupés par cosine similarity ≥ 0.88. Les quasi-duplicatas sont fusionnés, et seul le meilleur représentant par cluster passe au scoring. Cela évite de gaspiller le budget sur des mémoires redondantes.

### 4. Récupération par Tags
Une nouvelle table `memory_tags` indexe les entités, sujets et mots-clés extraits. Les candidats correspondant aux tags de la requête reçoivent un petit boost de pertinence additif.

### 5. Méthodes de Seuil Adaptatif
Au lieu d'un seuil fixe à 0.6, MIRA supporte désormais trois méthodes dynamiques :

| Methode | Description | Defaut |
|---------|-------------|--------|
| `iqr` | Premier quartile de la distribution des scores | Oui |
| `elbow` | Plus forte chute de derivee (methode du coude) | |
| `mean_stddev` | moyenne - ecart-type | |

Le seuil est clampé entre `0.15` (plancher) et `0.75` (plafond).

### 6. Reranker Heuristique (Optionnel)
Un reranker léger 100% Go score les top-k candidats via :
- Chevauchement lexical de type Jaccard
- Bonus de présence de phrase exacte
- Préférence d'équilibre de longueur

Mélangé avec la pertinence sémantique : `0.7*sémantique + 0.3*rerank`.

### 7. Vector Store de Fallback
Si HNSW n'est pas encore prêt (ex. reconstruction depuis zéro), un wrapper de fallback transparent redirige automatiquement les recherches vers le vector store SQLite. Le recall ne tombe jamais en panne.

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
    RelBecause:     regexp.MustCompile(`(?i)(?:because|since|due to|in reason of)`),
    RelContradicts: regexp.MustCompile(`(?i)(?:contradicts|in contradiction|however)`),
    RelUpdates:     regexp.MustCompile(`(?i)(?:updates|replaces)`),
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

# Exécuter uniquement les migrations
./mira -migrate
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
  version: "0.4.4"

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

# Configuration HNSW
hnsw:
  M: 32 # Voisins max par nœud (optimise pour le recall, voir v0.4.2)
  Ml: 0.25 # Facteur de génération de niveau
  ef_construction: 0 # Inactif — non supporte par la librairie hnsw sous-jacente
  ef_search: 100 # Taille liste recherche (optimise, voir v0.4.2)

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

# Configuration de recall amélioré
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

# Extension d'identite SOUL (desactivee par defaut)
soul:
  enabled: false

mcp:
  name: "mira"
  version: "0.4.4"
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
| `mira_clear_memory` | Supprime définitivement toutes les mémoires ou celles d'un room |

### Wings de Secours (Fallback Wings)

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

Si le wing principal n'a aucune mémoire pertinente, MIRA cherchera automatiquement dans les wings de secours dans l'ordre.

### Recherche Multilingue et Élargie

`mira_recall` accepte les requêtes dans n'importe quelle langue (anglais, français, espagnol, italien, allemand, etc.) grâce aux embeddings cross-lingues. Si la recherche sémantique initiale retourne peu de résultats — par exemple lorsqu'une requête dans une langue cherche des mémoires stockées dans une autre — MIRA élargit automatiquement la recherche avec des seuils relaxés et fusionne les résultats. Il n'est pas nécessaire de traduire les requêtes ni d'ajuster des paramètres.

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
| Recherche HNSW      | ~0.14 ms pour 10K vecteurs (O(log n), ~0.5 ms estimé à 100K) |
| Recherche SQLite    | ~50 ms pour 10K vecteurs |
| Allocation complète | ~35 ms pour 100 candidats |
| Cosine similarity   | ~3.3M ops/sec           |

### Optimisations en v0.3.3

- **Expansion de Requête** : Moyenne d'embeddings de variantes sémantiques pour une récupération cross-lingue robuste
- **Recherche Lexicale FTS5** : Recherche full-text SQLite intégrée avec triggers auto et backfill
- **Fusion Hybride RRF** : Reciprocal Rank Fusion (`k=60`) combinant résultats denses HNSW et lexicaux FTS5
- **Clustering à la Recherche** : Déduplication en temps réel par clustering cosine-similarity (seuil 0.88)
- **Récupération par Tags** : Table `memory_tags` avec boost automatique des tags dans le scoring CBA
- **Reranker Heuristique** : Reranker lexical léger optionnel pour améliorer la précision
- **Méthodes de Seuil Adaptatif** : Élagage dynamique de pertinence avec les stratégies `iqr`, `elbow` et `mean_stddev`
- **Vector Store de Fallback** : Fallback transparent HNSW → SQLite quand l'index n'est pas prêt
- **Outil Clear Memory** : Nouvel outil MCP `mira_clear_memory` pour la suppression globale ou par room
- **Résolution T0 Chaîne Causale** : `mira_causal_chain` résout désormais correctement les références `T0:` en IDs fingerprint
- **Visibilité des IDs dans les Sorties** : `mira_recall` et `mira_timeline` incluent maintenant les IDs mémoire pour chaîner les outils
- **Erreurs Auto-Correctrices pour LLM** : Les erreurs d'ID invalide sont actionnables et indiquent aux LLMs où obtenir des IDs valides

### Optimisations en v0.3.1

- **Lazy Evaluation**: Calcul d'overlap uniquement pour les candidats prometteurs
- **Cache LRU**: 1000 entrées pour les embeddings de requête
- **Persistance HNSW**: Rechargement rapide de l'index au redémarrage
- **SQLite WAL Mode**: Performance lecture/écriture concurrente
- **Données Scopées au Projet**: Base de données locale cachée dans `.mira/` au lieu d'un dossier global `mira_data/`
- **Auto-Gitignore**: Ajout automatique de `.mira/` au `.gitignore` du projet s'il existe
- **Seuil Adaptatif**: Baisse du seuil de pertinence pour les petits corpus (<10 mémoires)
- **Mapping Room par Défaut**: Assignation automatique des rooms standards selon le type de mémoire

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
│   │  • extraction: NativeExtractor, CybertronEmbedder           │   │
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
│   │   ├── vector/        # HNSW, SQLite vector store, overlap cache
│   │   ├── extraction/    # NLP, embeddings
│   │   ├── logging/       # Logging structuré
│   │   ├── webhook/       # Notifications HTTP
│   │   └── metrics/       # Métriques Prometheus
│   ├── interfaces/        # Couche Interfaces
│   │   └── mcp/           # Contrôleur MCP
│   ├── config/            # Configuration
│   └── app/               # Racine de composition (DI)
│       ├── main.go        # Injection de dépendances
│       ├── health.go      # Health checks
│       ├── health_test.go # Tests health checks
│       └── main_test.go   # Tests application
├── docs/                  # Documentation
│   ├── INDEX.md           # Point d'entrée documentation
│   ├── ARCHITECTURE.md    # Deep-dive technique
│   ├── FEATURES.md        # Catalogue complet des fonctionnalités
│   └── API_REFERENCES.md  # Référence API
├── SKILL.md               # Skill agent et guidelines memory loop
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
make build       # Compiler
make test        # Tests (avec race detector)
make test-short  # Tests rapides
make bench       # Benchmarks
make bench-full  # Benchmarks complets
make run         # Compiler et lancer avec config.yaml
make clean       # Nettoyer les artefacts et données
make lint        # Lancer les linters
make fmt         # Formater le code
make install     # Installer dans GOPATH/bin
make prepublish VERSION=x.y.z  # Préparer une release
```



## Changelog

### v0.4.4 (2026-04-23)

- **Integration SOUL opt-in** : MIRA fonctionne maintenant seul (8 outils) par defaut. L'extension d'identite SOUL doit etre explicitement activee via le flag `--with-soul` ou `soul.enabled: true` dans la configuration.
- Les echecs d'initialisation de SOUL sont non fatals — MIRA retombe graceieusement en mode 8 outils.

### v0.4.3 (2026-04-23)

- **Correction des noms de parametres MCP SOUL** : `agent` → `agent_id`, `model` → `model_id`, `from` → `from_model`, `to` → `to_model`.

### v0.4.2 (2026-04-17)

- **Defaults HNSW optimises** : `M` 16 → 32, `ef_search` 50 → 100 pour un meilleur recall.
- **Pool d'embeddings concurrent** : Remplacement du mutex global par un pool d'instances.
- **Pipeline de recall parallele** : Recherche HNSW dense + FTS5 lexicale executees en parallele.
- `ef_construction` documente comme inactif (non supporte par la librairie sous-jacente).

Voir [CHANGELOG.md](CHANGELOG.md) pour l'historique complet des releases.

---

## Références

### Librairies Clés

- [tiktoken-go](https://github.com/pkoukk/tiktoken-go) - Tokenization OpenAI
- Implémentation native Go - NLP/NER basé sur règles (remplace prose archivé)
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

[Reference API](docs/API_REFERENCES.md) • [Changelog](CHANGELOG.md)

</div>
