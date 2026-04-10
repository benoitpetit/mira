# ADR 002: Context Budget Allocation (CBA) Algorithm

## Status
Accepted

## Context
MIRA doit sélectionner les mémoires les plus pertinentes pour un contexte donné tout en respectant une contrainte de budget (nombre de tokens). Le système doit:
- Maximiser la pertinence sémantique
- Minimiser la redondance (overlap)
- Privilégier les informations récentes
- Tenir compte des relations causales

## Decision
Nous implémentons l'algorithme CBA (Context Budget Allocation) avec les caractéristiques suivantes:

### Formule de Score
```
Score = ρ (Relevance) × δ (Density) × η (Recency) × (1 - Overlap) × CausalPenalty × SessionBoost
```

### Composantes
1. **Relevance (ρ)**: Similarité cosinus entre l'embedding de la requête et celui de la mémoire
2. **Density (δ)**: Nombre de faits normalisé par la racine carrée du nombre de tokens
3. **Recency (η)**: Décay exponentiel basé sur l'âge de la mémoire
4. **Overlap**: Similarité maximale avec les mémoires déjà sélectionnées
5. **CausalPenalty**: Pénalité pour les mémoires déjà liées causalement
6. **SessionBoost**: Bonus pour les mémoires de la même session

### Algorithme de Sélection
1. Scoring initial de tous les candidats
2. Filtrage par threshold de pertinence
3. Sélection gloutonne (greedy) avec recalcul dynamique des scores
4. Respecting du budget avec fallback sur des modes de rendu plus légers

## Consequences

### Positives
- ✅ Sélection optimisée pour le contexte LLM
- ✅ Évite la redondance d'information
- ✅ Priorise les décisions et faits récents

### Négatives
- ➖ Complexité O(n²) pour le calcul d'overlap
- ➖ Paramètres à tuner (thresholds, decay rates)
- ➖ Pas de garantie d'optimalité globale (greedy)

## Alternatives Considérées

1. **Top-K simple par similarité**: Rejeté car ignore la redondance
2. **Clustering puis sélection**: Rejeté car trop complexe
3. **Integer Programming**: Rejeté car overkill et lent

## Références
- Algorithme inspiré de la "Maximum Marginal Relevance" (MMR)
- Décay rates basés sur les travaux de Ebbinghaus sur l'oubli
