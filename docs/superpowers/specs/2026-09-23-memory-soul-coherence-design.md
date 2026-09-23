# MIRA — Corrections, cohérence mémoire/soul et évolutions autonomes

## Objectif

Améliorer le cerveau mémoire/soul existant sans remplacer son architecture T0/T1/T2, son pipeline de recherche hybride ni son allocation CBA. Les clients gardent leurs outils d’action et de consultation ; les règles d’intégrité, de provenance, de consolidation et de rappel restent déterministes et internes à MIRA.

Le résultat attendu est une mémoire plus fiable : une observation ne devient pas automatiquement une vérité, une synthèse reste réversible, une identité est apprise depuis les réponses de l’agent uniquement, et le recall privilégie les informations prouvées et cohérentes.

## Périmètre et ordre

1. Provenance et rôles pour `soul_capture`.
2. Rétention réelle et jalons des snapshots soul.
3. Consolidation réversible et provenance des sources.
4. Causalité plus fiable et utilisation de `causal_lookback`/`causal_max_days`.
5. CBA relationnel et prise en compte de la qualité d’extraction.
6. Budget explicite entre identité, mémoire normative et preuves de travail.
7. Synchronisation des suppressions/archives avec l’index vectoriel et classification `identity` des événements soul.
8. Registre de croyances versionnées et boucle de qualité locale, activés après stabilisation des fondations.

Les modifications restent compatibles avec les mémoires existantes : les migrations ajoutent des colonnes ou tables et conservent les valeurs historiques par défaut.

## 1. Capture soul avec provenance

### Contrat

`CaptureRequest` accepte un ensemble de messages portant au minimum `role`, `content` et, lorsque disponible, `timestamp`/`session_id`. Le contrôleur MCP conserve la compatibilité avec le champ `conversation`, mais le traite comme contenu non attribué tant qu’aucun rôle assistant explicite n’est fourni.

L’extraction de traits, voix, style, valeurs et ton ne s’effectue que sur les messages `assistant`. Les messages `user`, `system` et `tool` peuvent enrichir le contexte de session, mais ne peuvent pas modifier directement l’identité de l’agent.

Chaque observation persistée garde une preuve bornée : identifiant de snapshot, message source, rôle, extrait, session et score de confiance. Les preuves sont tronquées à une limite configurée et ne doivent pas recopier une conversation entière.

### Compatibilité

Un appel historique à `soul_capture(conversation=...)` reste accepté. Il produit un snapshot à faible confiance ou un avertissement de provenance non attribuée ; il ne doit pas être présenté comme une observation agent confirmée.

## 2. Rétention des snapshots soul

`MaxHistoryVersions` devient une politique de stockage et non une simple limite de lecture. MIRA conserve :

- le snapshot courant ;
- les snapshots reliés à un changement de modèle, une dérive significative ou une correction explicite ;
- les versions nécessaires pour garder la lignée `derived_from_id` cohérente ;
- un nombre borné de versions intermédiaires récentes.

Les versions intermédiaires peuvent être compactées en un snapshot de synthèse, sans casser les références ni supprimer les jalons. L’opération est transactionnelle, idempotente et testée sur SQLite et PostgreSQL.

## 3. Consolidation réversible

Le schéma mémoire ajoute un état de cycle de vie (`active`, `superseded`, `archived`, `contested`) et un lien de provenance vers les sources. La consolidation crée une mémoire synthétique active, marque les notes sources `superseded`, puis conserve leurs T0/T1/T2 et leurs embeddings hors du recall normal.

Une synthèse ne peut être rappelée comme vérité sans conserver son origine. Une opération de révocation marque la synthèse `contested` et réactive les sources concernées. Les index dérivés (tags, causalité, vecteur) suivent le même état logique.

## 4. Causalité fiable

La détection actuelle par motif lexical reste un générateur de propositions. Une arête porte désormais :

- la relation ;
- un poids et une confiance ;
- les preuves textuelles ;
- la méthode de détection ;
- un état `proposed`, `confirmed` ou `rejected`.

Les candidats sont filtrés par wing, fenêtre de rappel (`causal_lookback`) et âge maximal (`causal_max_days`). Les sujets génériques (`Note`, `Fact`, etc.) ne suffisent plus à établir une relation. Une arête proposée doit partager un sujet/entity non générique ou satisfaire un seuil sémantique configurable.

Le rappel ne pénalise fortement que les relations confirmées. Les relations proposées restent disponibles comme signal faible et sont exposées comme incertaines dans les diagnostics.

## 5. CBA relationnel et qualité

Le score conserve les dimensions existantes mais ajoute :

`quality = extraction_confidence × validation_freshness × lifecycle_factor`

La densité ne repose plus uniquement sur `FactCount`. Les liens relationnels sont interprétés par type :

- `UPDATES` : préférer la version active et signaler l’ancienne comme remplacée ;
- `CONTRADICTS` : retourner les deux éléments avec un marqueur de conflit ;
- `BECAUSE`/`RESOLVES` : sélectionner un paquet cause-problème-résolution si le budget le permet ;
- relation proposée : bonus ou pénalité borné et inférieur à une relation confirmée.

Le paquet causal reste atomique pendant la sélection gloutonne : MIRA ne doit pas injecter une conséquence sans sa décision ou sa cause lorsque ceux-ci sont nécessaires à l’interprétation.

## 6. Soul et mémoire dans un budget commun

Le rendu de `soul_recall` réserve explicitement le budget en trois compartiments : identité normative, préférences et preuves de travail. Les preuves sont étiquetées comme non normatives et ne peuvent pas réécrire les règles d’identité.

Chaque compartiment possède un minimum et un maximum ; si le budget est trop faible, MIRA réduit les preuves avant les invariants d’identité. Le calcul de tokens inclut les en-têtes et séparateurs ajoutés par le rendu.

Les événements de changement d’identité sont stockés avec `kind=identity`, tout en gardant leur `type` d’extraction séparé.

## 7. Index et archivage

Toute archive, révocation ou suppression appelle une synchronisation vectorielle. Si l’index ne permet pas une suppression fiable, MIRA reconstruit l’index depuis le dépôt SQL avant de confirmer l’opération. Les tests vérifient qu’aucun identifiant archivé ne ressort d’une recherche.

## 8. Registre de croyances et boucle de qualité

Après les fondations, MIRA ajoute une vue dérivée de croyances versionnées : sujet, prédicat, valeur, validité temporelle, confiance, sources et statut (`active`, `superseded`, `contested`). Cette vue ne remplace pas T0/T1/T2 ; elle permet de rappeler une affirmation cohérente et de résoudre les contradictions sans perdre les textes sources.

Chaque mémoire injectée peut recevoir un signal local (`useful`, `stale`, `contradictory`, `irrelevant`). Les signaux sont agrégés par type, wing et agent pour calibrer progressivement les poids CBA. Aucun appel cloud ni apprentissage opaque n’est requis.

## Tests et critères d’acceptation

- Les appels soul avec conversation mixte n’attribuent aucun trait utilisateur à l’agent.
- Les preuves sont bornées, sérialisables et reliées au snapshot.
- La rétention ne dépasse pas la politique configurée tout en conservant les jalons et la lignée.
- Une consolidation puis révocation restaure les sources et empêche la synthèse contestée d’être rappelée comme active.
- Les relations causales hors fenêtre ou fondées sur un sujet générique ne sont pas confirmées.
- Les paquets relationnels respectent le budget et les types `UPDATES`/`CONTRADICTS`/`BECAUSE`.
- La confiance d’extraction et le cycle de vie modifient réellement le score de rappel.
- L’archivage SQL et l’index vectoriel restent cohérents.
- `go test -tags fts5 ./...` reste vert.

## Documentation et produit

Après chaque lot validé, mettre à jour `README.md`, `README_FR.md`, `SKILL.md`, `docs/ARCHITECTURE.md`, `docs/FEATURES.md`, la configuration exemple et la landing `mira-landing` lorsque les capacités ou les garanties publiques changent. Les docs doivent distinguer clairement les sources, les synthèses, l’identité et les preuves de travail.
