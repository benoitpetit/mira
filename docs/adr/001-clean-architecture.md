# ADR 001: Clean Architecture / Hexagonal Architecture

## Status
Accepted

## Context
MIRA est un système de mémoire long terme pour LLMs avec des exigences de:
- Maintenabilité sur le long terme
- Testabilité des composants métier
- Possibilité de changer les implémentations techniques (DB, modèles ML, etc.)

## Decision
Nous adoptons la Clean Architecture (aussi appelée Hexagonal Architecture ou Ports & Adapters) avec les couches suivantes:

1. **Domain** (centre): Entities et Value Objects purs
2. **Use Cases**: Interactors et Ports (interfaces)
3. **Adapters**: Implémentations concrètes (SQLite, Cybertron, etc.)
4. **Frameworks**: MCP server, configuration

### Structure des packages
```
internal/
├── domain/           # Entités et value objects
├── usecases/         # Interactors et ports
│   ├── interactors/  # Logique métier
│   └── ports/        # Interfaces
└── adapters/         # Implémentations
    ├── storage/
    ├── extraction/
    └── ...
```

## Consequences

### Positives
- ✅ Testabilité: les use cases peuvent être testés sans infrastructure
- ✅ Flexibilité: changement de DB ou modèle ML sans toucher au métier
- ✅ Indépendance: le domaine ne dépend d'aucune librairie externe

### Négatives
- ➖ Courbe d'apprentissage pour les nouveaux développeurs
- ➖ Plus de boilerplate (interfaces, adapters)
- ➖ Complexité accrue pour un projet simple

## Alternatives Considérées

1. **MVC traditionnel**: Rejeté car trop de couplage avec la DB
2. **Microservices**: Rejeté car overkill pour la taille du projet
3. **Serverless**: Rejeté car besoin de persistance locale et contrôle

## Références
- [The Clean Architecture by Robert C. Martin](https://blog.cleancoder.com/uncle-bob/2012/08/13/the-clean-architecture.html)
- [Hexagonal Architecture by A. Cockburn](https://alistair.cockburn.us/hexagonal-architecture/)
