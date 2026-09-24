# MIRA — Durcissement du runtime mémoire et âme

## Décision

MIRA conserve son architecture T0/T1/T2, sa recherche hybride dense/lexicale et son allocation CBA. Le dépôt SQL devient explicitement la source de vérité pour le cycle de vie des souvenirs. Les index vectoriels, FTS5, tags, causalité et croyances sont des projections dérivées, réparables et filtrées par cet état.

L’objectif de cette itération est de rendre les garanties déjà annoncées vraies dans le code actuel, sans migration externe ni apprentissage opaque : une mémoire consolidée reste réversible, un archivage reste restaurable, une croyance est traçable, une causalité est bornée par la configuration, et l’âme ne mélange pas identité normative et preuves de travail.

## État constaté au départ

Le dépôt contient déjà une partie des fondations : les entités exposent un cycle de vie, le scoring lit une qualité d’extraction, les migrations de croyances existent et la capture soul possède une première notion de messages. Les écarts prioritaires sont :

1. certains chemins HNSW/embeddings ne réhydratent pas le cycle de vie et peuvent ressortir des souvenirs remplacés ou contestés ;
2. la consolidation committe la synthèse avant les transitions des sources et la révocation ne répare pas toutes les projections ;
3. l’archivage SQL supprime encore des données alors que le produit promet une restauration ;
4. les requêtes causales ignorent partiellement les filtres de relation/statut, la fenêtre configurée et les causes futures ;
5. la qualité, les relations et la calibration des croyances ne sont pas appliquées de manière uniforme au recall ;
6. snapshot, preuves soul, rétention et budget ne forment pas encore une politique atomique et strictement bornée ;
7. la documentation et la landing décrivent des garanties plus fortes que celles effectivement vérifiées.

## Modèle de données et invariants

### Cycle de vie mémoire

Les états autorisés sont `active`, `superseded`, `archived` et `contested`. Les colonnes SQL dédiées (`lifecycle_state`, `superseded_by`) sont la représentation canonique ; les métadonnées JSON historiques sont utilisées uniquement pour le backfill et la compatibilité de lecture.

Invariants :

- toute nouvelle mémoire est `active` ;
- `superseded` doit pointer vers une synthèse existante ;
- une mémoire `contested` n’est jamais sélectionnée comme vérité active ;
- `archived` reste stockée et peut être restaurée ;
- tout lecteur de candidats — FTS5, HNSW, fallback, rebuild et timeline — applique le même filtre par défaut ;
- une reconstruction depuis SQL ne peut pas réintroduire un état non actif dans l’index de recherche normal.

La migration est idempotente pour SQLite et PostgreSQL, initialise les anciennes lignes à `active`, puis recopie les valeurs valides issues du JSON. Les index nécessaires portent sur l’état et la relation de remplacement.

### Consolidation et révocation

Une consolidation exécute dans une transaction SQL : insertion de la synthèse T0/T1/T2, insertion de sa provenance, passage des sources à `superseded` et enregistrement de `superseded_by`. Les opérations dérivées sont ensuite synchronisées depuis l’état SQL ; une erreur de projection déclenche une reconstruction ou rend l’opération explicitement non confirmée.

La révocation est idempotente : la synthèse passe à `contested`, les sources directes redeviennent `active`, puis tags, causalité et vecteurs sont reconstruits ou mis à jour. Une seconde révocation ne modifie pas le résultat. La synthèse contestée reste consultable dans un mode diagnostic, mais ne consomme pas le contexte normal.

### Archivage

`ArchiveOldMemories` devient une transition d’état et non une suppression en cascade. Les compteurs d’archivage indiquent le nombre de souvenirs exclus du contexte actif, pas le nombre de lignes supprimées. Les embeddings et preuves sont conservés pour restauration et audit ; les index de recherche les excluent.

## Causalité et scoring

Le détecteur natif respecte `causal_lookback` et `causal_max_days`, ignore les causes postérieures à l’effet, et ne confirme pas une relation fondée uniquement sur un sujet générique. Chaque arête conserve relation, confiance, statut, preuve tronquée, détecteur et date de détection.

Les lectures de graphe filtrent `status = confirmed` par défaut et appliquent réellement la liste des types demandés. Les arêtes `proposed` restent visibles dans les diagnostics mais leur influence est bornée et inférieure à celle d’une relation confirmée.

Le score de rappel utilise une qualité bornée :

```text
quality = clamp(extraction_confidence)
        * clamp(validation_freshness)
        * lifecycle_factor
        * belief_calibration
```

Les valeurs manquantes gardent un défaut neutre pour les données historiques. Le pruning précoce utilise le score composite calculé, et non la seule similarité brute. Les règles relationnelles restent bornées : `UPDATES` favorise l’actif, `CONTRADICTS` conserve le couple avec un marqueur de conflit, `BECAUSE`/`RESOLVES` sélectionne un paquet atomique si le budget le permet.

## Âme, preuves et budget

La capture distingue les messages `assistant`, `user`, `system` et `tool`. Seuls les messages assistant peuvent produire une observation normative. Les preuves gardent snapshot, rôle, extrait borné, session, horodatage et confiance ; le contrat historique `conversation` reste accepté mais marqué non attribué.

La transaction snapshot + preuves est atomique. L’identifiant de modèle courant est préservé lorsqu’il n’est pas fourni dans la requête. La rétention possède une limite dure : courant, jalons nécessaires à la lignée et nombre borné d’intermédiaires ; les milestones ne peuvent plus croître sans limite.

Le rendu soul réserve explicitement le budget en trois compartiments : identité normative, préférences et preuves de travail. Les en-têtes et séparateurs sont comptés. Si le budget est trop petit, les preuves sont réduites avant les invariants d’identité. Les événements produits par les changements d’identité utilisent `kind=identity` sans perdre leur type d’extraction.

## Croyances et feedback local

Le registre de croyances devient une projection persistée des fingerprints structurés et du cycle de vie. Une croyance contient sujet, prédicat, valeur, validité temporelle, confiance, sources et statut. Les textes T0/T1/T2 restent la preuve primaire ; la vue croyance sert à résoudre les versions et contradictions.

Les feedbacks autorisés sont `useful`, `stale`, `contradictory` et `irrelevant`. Leur agrégation est locale, bornée, explicable et idempotente. Elle ne modifie jamais directement le contenu et n’applique au CBA qu’un facteur limité autour de 1 ; le comportement cold-start reste neutre.

## Compatibilité et limites

- Les noms d’outils existants restent compatibles ; les nouveaux champs sont optionnels.
- Les anciennes lignes sans qualité, croyance ou statut causal reçoivent des valeurs neutres/conservatrices.
- SQLite et PostgreSQL doivent exposer les mêmes invariants, même si leur SQL interne diffère.
- Aucun service cloud, aucun appel de modèle supplémentaire et aucune suppression destructive ne sont introduits.
- Les corrections de cohérence doivent rester testables avec les doubles et les tests SQLite déjà utilisés par le dépôt.

## Documentation et landing

Les documents publics distinguent désormais : observation source, fingerprint, embedding, croyance dérivée, synthèse consolidée, identité soul et preuve de travail. Ils décrivent les états de cycle de vie, la révocation, l’archivage restaurable, les limites de causalité et le budget partagé.

La landing montre le pipeline réel :

```text
capture avec provenance
  → T0/T1/T2 SQL
  → projections lifecycle/tags/causal/beliefs
  → recherche dense + lexicale
  → qualité + relations + calibration
  → CBA à budget borné
  → rendu identité/preuves explicite
```

## Critères d’acceptation

- `go test -tags fts5 ./...` reste vert.
- Une mémoire `superseded`, `archived` ou `contested` ne ressort pas du recall normal après un rebuild HNSW/FTS5.
- Consolidation puis révocation restaure les sources, marque la synthèse contestée et reste idempotente.
- L’archivage conserve les lignes et permet une restauration testée.
- Les filtres causaux par type/statut sont effectifs ; une cause future ou générique n’est pas confirmée.
- Le score, le pruning et le budget utilisent réellement les facteurs qualité/cycle de vie/croyance sans sortir de leurs bornes.
- Les traits soul ne sont jamais extraits des messages utilisateur, les preuves sont bornées et la rétention respecte sa limite dure.
- Les croyances et feedbacks sont persistés, traçables par sources et appliqués au rappel avec un facteur borné.
- Les README, docs API/architecture, configuration, skill et landing reflètent le comportement vérifié.
