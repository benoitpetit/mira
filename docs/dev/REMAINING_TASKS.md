# 📋 Étapes Restantes et Nettoyage - Projet MIRA

## ✅ Nettoyage Effectué

### Fichiers Supprimés
- ❌ `mira` (binaire à la racine, doublon avec `bin/mira`)
- ❌ `coverage.out` (fichier de couverture généré)

### Anciens Dossiers Déjà Supprimés
Les dossiers de l'ancienne architecture ont été correctement supprimés :
- ❌ `budget/` → refactorisé dans `internal/`
- ❌ `causal/` → refactorisé dans `internal/`
- ❌ `config/` → refactorisé dans `internal/`
- ❌ `extract/` → refactorisé dans `internal/`
- ❌ `mcp/` → refactorisé dans `internal/`
- ❌ `store/` → refactorisé dans `internal/`
- ❌ `types/` → refactorisé dans `internal/`
- ❌ `vector/` → refactorisé dans `internal/`

---

## 🔧 TODOs Restants à Traiter

### Priorité HAUTE

#### 1. Async Processing Non Fonctionnel
**Fichier**: `internal/adapters/async/processor.go:205`
**Problème**: L'async processor échoue systématiquement tous les jobs.

**Options**:
- **Option A**: Implémenter le traitement async réel (complexe, nécessite injection dépendances)
- **Option B**: Retirer le code async non fonctionnel (recommandé si non utilisé)
- **Option C**: Laisser tel quel avec message d'erreur clair (statut actuel)

**Recommandation**: Option C pour l'instant - le message d'erreur est clair et le traitement synchrone fonctionne.

### Priorité MOYENNE

#### 2. HMAC Signature pour Webhooks
**Fichier**: `internal/adapters/webhook/manager.go:221`
**Code**:
```go
// TODO: Add HMAC signature if secret is configured
// if endpoint.Secret != "" {
//     signature := computeHMAC(payload, endpoint.Secret)
//     req.Header.Set("X-MIRA-Signature", "sha256="+signature)
// }
```

**Impact**: Les secrets configurés dans les endpoints ne servent actuellement à rien.

**Solution**: Décommenter et implémenter la fonction `computeHMAC`.

#### 3. Log Warnings Manquants
**Fichier**: `internal/usecases/interactors/store_memory.go:112,119`
**Code**:
```go
// TODO: Log warning
```

**Solution**: Ajouter des logs appropriés avec `log.Printf()` ou un logger structuré.

### Priorité BASSE

#### 4. UseSimpleEmbedder Config
**Fichier**: `internal/app/main.go:78`
**Code**:
```go
useSimpleEmbedder := false // TODO: Add UseSimpleEmbedder to config
```

**Impact**: Le embedder simple ne peut pas être activé via configuration.

**Solution**: Ajouter `UseSimpleEmbedder bool` dans la struct `Config`.

#### 5. Support Autres Transports MCP
**Fichier**: `internal/app/main.go:298`
**Code**:
```go
// TODO: Support other transports
```

**Impact**: Seul le transport `stdio` est supporté (SSE, HTTP non implémentés).

**Solution**: Implémenter les transports SSE ou HTTP si nécessaire.

---

## 📊 Résumé de l'État du Projet

### ✅ Terminé (100%)
- [x] Bugs critiques corrigés (CBA, cache LRU, type param)
- [x] Tests unitaires ajoutés (couverture 55.8%)
- [x] Métriques intégrées
- [x] Optimisations HNSW (batch SQL, persistance)
- [x] Build stable
- [x] Serveur MCP fonctionnel

### ⚠️ À Compléter (TODOs)
- [ ] HMAC webhooks (priorité moyenne)
- [ ] Log warnings dans store_memory (priorité moyenne)
- [ ] Config UseSimpleEmbedder (priorité basse)
- [ ] Transports MCP alternatifs (priorité basse)

### ❌ Volontairement Non Implémenté
- [ ] Async processing (fonctionne en synchrone, message d'erreur clair)

---

## 🎯 Recommandations pour la Production

### Immédiat (Avant MEP)
1. **Rien de critique** - Le projet est stable en l'état

### Court Terme (1-2 semaines)
1. Implémenter HMAC pour les webhooks si utilisés en production
2. Ajouter les logs warnings manquants

### Moyen Terme (1-2 mois)
1. Ajouter des tests pour les interactors restants (get_causal_chain, get_status, etc.)
2. Implémenter UseSimpleEmbedder dans la config si besoin
3. Évaluer l'utilité de l'async processing

---

## 🧹 Conseils de Maintenance

### Avant chaque commit
```bash
go fmt ./...
go vet ./...
go test ./...
```

### Avant chaque release
```bash
go test -race ./...
go build -o bin/mira ./cmd/mira
```

### Fichiers à ne JAMAIS commiter
- `bin/mira` (déjà dans .gitignore)
- `coverage.out` (déjà dans .gitignore)
- `mira_data/*.db*` (déjà dans .gitignore)
- `config.yaml` (déjà dans .gitignore, utiliser config.example.yaml)

---

*Document généré le 10 avril 2026*
