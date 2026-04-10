# ADR 004: Causal Graph Model

## Status
Accepted

## Context
MIRA doit capturer et exploiter les relations causales entre mémoires (décisions, conséquences). Use cases:
- "Pourquoi avons-nous pris cette décision ?"
- "Quelles sont les conséquences de X ?"
- Traçabilité des décisions dans le temps

## Decision
Nous implémentons un graphe causal avec:

### Modèle de Données
```sql
-- Nœuds (mémoires avec métadonnées)
causal_nodes:
  - id: UUID
  - type: decision|fact|preference|...
  - summary: text
  - wing, room: text
  - timestamp: datetime

-- Arêtes (relations causales)
causal_edges:
  - id: UUID
  - source_id: UUID (cause)
  - target_id: UUID (effet)
  - relation_type: leads_to|depends_on|enables|...
  - confidence: float (0-1)
```

### Détection Automatique
L'extraction NLP détecte les patterns causaux:
- "Nous avons décidé X **parce que** Y"
- "**Suite à** Z, nous avons fait W"
- "**Grâce à** A, B est possible"

### Navigation
- `GetChain(id, maxDepth)`: Remonter aux causes
- `GetConsequences(id, maxDepth)`: Descendre aux effets
- BFS pour exploration

## Consequences

### Positives
- ✅ Traçabilité des décisions
- ✅ Explication du contexte décisionnel
- ✅ Détection d'impacts en cascade

### Négatives
- ➖ Détection NLP imparfaite (faux positifs)
- ➖ Graphe potentiellement dense (nombreuses connexions)
- ➖ Cycle possibles (A → B → C → A) à gérer

## Alternatives Considérées

1. **Event Sourcing complet**: Rejeté car trop complexe
2. **Simple tagging**: Rejeté car pas de notion de direction
3. **Knowledge Graph (RDF)**: Rejeté car overkill

## Évolutions Futures

- Validation manuelle des liens causaux
- Scoring de confiance basé sur le feedback utilisateur
- Visualisation graphique des chaînes causales
- Détection de cycles et alertes
