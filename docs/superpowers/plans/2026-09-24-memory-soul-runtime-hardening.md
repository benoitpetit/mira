# Plan — Durcissement du runtime mémoire et âme

> Ce plan s’exécute dans le worktree `feat/memory-soul-coherence`. Chaque tâche commence par des tests ciblés en échec, puis une implémentation minimale, une vérification ciblée et un commit isolé.

## Objectif

Rendre les invariants de la spécification `docs/superpowers/specs/2026-09-24-memory-soul-runtime-hardening.md` effectifs dans MIRA, puis aligner les contrats publics et la landing page.

## Contraintes communes

- SQL reste la source de vérité ; aucune projection dérivée ne peut décider seule du cycle de vie.
- Préserver les interfaces publiques et les doubles de test autant que possible ; ajouter des capacités optionnelles plutôt que casser les mocks existants.
- Maintenir SQLite et PostgreSQL au même niveau fonctionnel.
- Utiliser `apply_patch` pour les modifications de fichiers.
- Après chaque tâche : `gofmt` sur les fichiers Go concernés, test ciblé, `git diff --check`.
- Ne jamais supprimer les artefacts préexistants `.codex-gocache/` et `.codex-tmp/`.

## Tâche 1 — Source de vérité lifecycle, lecteurs et archivage réversible

Fichiers probables :

- `internal/adapters/storage/lifecycle.go`
- `internal/adapters/storage/sqlite_repository.go`
- `internal/adapters/storage/postgres_repository.go`
- `internal/adapters/vector/hnsw_store_unix.go`
- `internal/adapters/vector/hnsw_store_windows.go`
- `internal/adapters/vector/bruteforce_store.go`
- `internal/adapters/vector/sqlite_vector_store.go`
- `internal/usecases/interactors/archive_memories.go`
- `internal/usecases/ports/repositories.go`
- migrations `014_memory_lifecycle` et tests storage/vector/archive

1. Écrire des tests qui construisent une mémoire active, remplacée, contestée et archivée, puis vérifient que `GetCandidatesWithEmbeddings`, FTS5, brute force et reconstruction n’exposent que l’état actif par défaut.
2. Écrire un test d’archivage qui vérifie la conservation T0/T1/T2, l’état `archived`, l’exclusion du recall et la restauration.
3. Corriger les SELECT de candidats et d’embeddings pour lire l’état canonique et le réhydrater dans `Verbatim`, avec compatibilité des anciennes métadonnées.
4. Centraliser le prédicat de visibilité active pour éviter des variantes entre SQLite, PostgreSQL, HNSW et fallback.
5. Transformer `ArchiveOldMemories` en transition d’état non destructive et clarifier ses compteurs.
6. Ajouter la synchronisation vectorielle : suppression logique lorsque possible, reconstruction depuis SQL lorsque le backend ne sait pas garantir la suppression.
7. Vérifier les tests storage/vector/archive, puis commit `fix: make lifecycle authoritative across memory indexes`.

## Tâche 2 — Consolidation transactionnelle et révocation complète

Fichiers probables :

- `internal/usecases/interactors/consolidate_memories.go`
- `internal/usecases/interactors/revoke_consolidation.go`
- `internal/usecases/ports/repositories.go`
- `internal/adapters/storage/lifecycle.go`
- `internal/adapters/storage/sqlite_repository.go`
- `internal/adapters/storage/postgres_repository.go`
- `internal/usecases/interactors/memory_derived_index.go`
- tests de consolidation, révocation et intégration SQLite

1. Écrire des tests d’invariant : échec d’indexation, échec de transition source, consolidation réussie, révocation répétée ; aucun état partiellement consolidé ne doit être présenté comme terminé.
2. Introduire une capacité transactionnelle de consolidation dans le port, ou un port optionnel explicitement détecté, pour insérer la synthèse, enregistrer sa provenance et muter toutes les sources dans la même transaction SQL.
3. Conserver les T0/T1/T2 des sources et éliminer le fallback destructif `ClearByIDs` pour les repositories de production.
4. Faire de la révocation une transaction idempotente qui passe la synthèse à `contested`, réactive les sources directes et nettoie la relation `superseded_by`.
5. Rejouer/reconstruire tags, causalité et vecteurs après la mutation SQL ; rendre l’erreur de projection visible sans mentir sur l’état de la base.
6. Vérifier les tests ciblés, `go test ./internal/usecases/interactors ./internal/adapters/storage ./internal/adapters/vector`, puis commit `fix: make consolidation lifecycle atomic and reversible`.

## Tâche 3 — Causalité et CBA qualité/relation

Fichiers probables :

- `internal/adapters/extraction/native_extractor.go`
- `internal/adapters/storage/sqlite_repository.go`
- `internal/adapters/storage/postgres_repository.go`
- `internal/domain/entities/causal.go`
- `internal/domain/entities/candidate.go`
- `internal/usecases/interactors/memory_derived_index.go`
- `internal/usecases/interactors/recall_memory.go`
- tests extraction, graph, recall et scoring

1. Ajouter des tests rouges pour : cause future, fenêtre `causal_max_days`, limite `causal_lookback`, sujet générique, statut proposé, filtres multi-relations et edges non confirmées.
2. Corriger le détecteur pour trier/borner les candidats, rejeter les causes futures et produire `proposed` lorsqu’aucune preuve suffisante ne permet `confirmed`.
3. Corriger les requêtes `GetParents`, `GetChildren`, `HasEdge`, `GetChain` et `GetConsequences` afin d’appliquer les relations demandées et le statut confirmé par défaut.
4. Remplacer le lookback codé en dur dans `memory_derived_index.go` par la configuration effectivement fournie au détecteur.
5. Ajouter au candidat les métadonnées de relation/croyance nécessaires, calculer une qualité bornée, et faire utiliser le score composite au pruning précoce.
6. Appliquer des règles bornées pour `UPDATES`, `CONTRADICTS`, `BECAUSE` et `RESOLVES`, sans dépasser `[0,1]` ni casser la sélection atomique des paquets.
7. Vérifier les tests ciblés, les tests de recall complets et commit `fix: enforce causal bounds and quality-aware recall`.

## Tâche 4 — Atomicité, rétention et budget de l’âme

Fichiers probables :

- `internal/agentmemory/runtime.go`
- `internal/agentmemory/provider.go`
- `internal/agentmemory/mcp.go`
- `internal/config/config.go`
- migrations `012_agent_memory_evidence` et `013_agent_memory_retention`
- tests runtime/config/MCP

1. Écrire des tests rouges pour snapshot + preuves atomiques, conservation du modèle précédent, rétention répétée, jalons bornés, budget à 32 tokens et comptage de ponctuation.
2. Faire exécuter la capture dans une transaction ou une primitive atomique du runtime : snapshot, preuves et rétention doivent avoir un résultat cohérent ; les écritures externes restent après commit.
3. Préserver `ModelIdentifier` lorsqu’une nouvelle capture n’en fournit pas ; normaliser les rôles et borner les extraits sans inclure user/system/tool dans les traits normatifs.
4. Rendre `CompactHistory` strictement bornée en conservant courant, lignée et jalons ; marquer/compacter les intermédiaires sans casser `derived_from_id`.
5. Implémenter les compartiments identité/préférences/preuves avec minimums, maximums et compte des séparateurs ; réduire les preuves en premier.
6. Corriger `estimateTokens` et classifier les notifications d’identité avec `kind=identity`.
7. Vérifier les tests agentmemory/config/MCP et commit `fix: make soul history and budget bounded`.

## Tâche 5 — Persistance et calibration des croyances

Fichiers probables :

- `internal/usecases/interactors/belief_registry.go`
- `internal/domain/entities/belief.go`
- `internal/usecases/ports/repositories.go`
- `internal/adapters/storage/sqlite_repository.go`
- `internal/adapters/storage/postgres_repository.go`
- migrations `016_beliefs_quality_feedback`
- `internal/usecases/interactors/recall_memory.go`
- contrôleurs/API et tests beliefs/recall

1. Écrire des tests rouges pour upsert idempotent, sources conservées, validité temporelle, résolution active/superseded/contested et feedback borné.
2. Ajouter un port persistant de croyances et son implémentation SQLite/PostgreSQL ; sérialiser les sources de façon stable et protéger les transitions de statut.
3. Faire dériver/upserter la croyance depuis les fingerprints structurés et le lifecycle sans remplacer les T0/T1/T2.
4. Rendre le feedback atomique et idempotent ; conserver les compteurs par croyance et type.
5. Connecter la calibration au recall avec facteur borné, neutre sans historique, et sans permettre au feedback de contourner les filtres lifecycle.
6. Exposer uniquement les opérations nécessaires à travers les frontières existantes, avec erreurs et statuts documentés.
7. Vérifier les tests ciblés et commit `feat: persist beliefs and calibrate recall locally`.

## Tâche 6 — Documentation, README et landing page

Fichiers du dépôt MIRA :

- `README.md`, `README_FR.md`, `SKILL.md`
- `docs/ARCHITECTURE.md`, `docs/FEATURES.md`, `docs/API_REFERENCES.md`
- `config.example.yaml`

Fichiers du dépôt landing :

- `mira-landing/components/landing/memory-system-section.tsx`
- `mira-landing/components/landing/allocator-section.tsx`
- `mira-landing/components/landing/product-sections.tsx`
- `mira-landing/app/page.tsx` uniquement si nécessaire

1. Écrire/mettre à jour les exemples et assertions publiques après vérification du comportement final.
2. Documenter la source SQL, lifecycle, consolidation/révocation, archivage restaurable, causalité bornée, qualité CBA, croyances, feedback et budget soul.
3. Ajouter un schéma de pipeline lisible dans l’architecture et une section de compatibilité/migrations.
4. Mettre à jour la landing pour afficher le pipeline réel capture → T0/T1/T2 → projections → recherche hybride → scoring → budget/rendu, sans promesse non testée.
5. Lancer `git diff --check`, tests Go complets et `npm run build` dans un worktree dédié de `mira-landing`.
6. Commit MIRA `docs: align public memory and soul guarantees` et commit landing équivalent.

## Vérification finale

Depuis le worktree MIRA :

```bash
gofmt -w <fichiers Go modifiés>
git diff --check
go test -tags fts5 ./...
```

Depuis le worktree landing :

```bash
npm run build
```

Avant de conclure, vérifier `git status --short`, relire les diffs des deux dépôts et ne déclarer la tâche terminée qu’avec les sorties fraîches de ces commandes.
