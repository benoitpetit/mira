# ADR 003: Vector Search Strategy

## Status
Accepted

## Context
MIRA doit rechercher efficacement les mémoires similaires sémantiquement. Les contraintes sont:
- Latence < 100ms pour la recherche
- Précision élevée (top-k pertinent)
- Évolution du nombre de mémoires (1K → 100K+)

## Decision
Nous utilisons une stratégie hybride avec deux implémentations:

### 1. HNSW (Hierarchical Navigable Small World)
**Usage**: Recherche approximative rapide O(log n)

**Caractéristiques**:
- Index en mémoire pour latence minimale
- Reconstruction au démarrage (ou chargement depuis disque)
- Paramètres: M=16, efConstruction=200, efSearch=50

### 2. SQLite Full Scan (Fallback)
**Usage**: Quand HNSW n'est pas disponible ou pour de petites bases

**Caractéristiques**:
- Scan linéaire O(n) mais simple et fiable
- Pas de dépendance supplémentaire
- Suffisant pour < 10K mémoires

### Architecture
```go
type VectorStore interface {
    Search(ctx context.Context, vector []float32, limit int, wing, room *string) ([]*entities.Candidate, error)
    AddCandidate(ctx context.Context, candidate *entities.Candidate) error
    Delete(ctx context.Context, id uuid.UUID) error
}
```

## Consequences

### Positives
- ✅ Flexibilité: fallback automatique si HNSW échoue
- ✅ Performance: O(log n) pour les grandes bases
- ✅ Simplicité: SQLite comme solution de secours

### Négatives
- ➖ Double implémentation à maintenir
- ➖ HNSW nécessite de la RAM (proportional au nombre de vecteurs)
- ➖ Pas de persistance native du graphe HNSW

## Alternatives Considérées

1. **FAISS (Facebook)**: Rejeté car dépendance C++ lourde
2. **Pinecone (cloud)**: Rejeté car besoin de fonctionnement offline
3. **pgvector**: Rejeté car ajoute une dépendance PostgreSQL
4. **Weaviate/Milvus**: Rejetés car trop complexes pour ce besoin

## Évolutions Futures

- Persistance du graphe HNSW sur disque
- Sharding pour très grandes bases (>1M vecteurs)
- Quantification pour réduire l'empreinte mémoire
