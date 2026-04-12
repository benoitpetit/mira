# HNSW Benchmark Analysis & Optimization Report

## Current Performance Baseline

Based on benchmark results (AMD Ryzen 5 7535U):

```
BenchmarkHNSWSearch-12:                ~88µs/op, 24KB alloc, 293 allocs
BenchmarkHNSWAdd-12:                   ~262µs/op
Scalability (100→10K vectors):         1.47x (excellent)
```

## Critical Issues Identified

### 1. High Memory Allocation in Search (CRITICAL)
- **293 allocations per search** - causes GC pressure
- **24KB allocated per query** - excessive for read-heavy workloads
- Root cause: Dynamic slice growth, map lookups, temporary objects

### 2. Over-fetching Results (HIGH)
- Current: `limit*2` (fetches 2x more results than needed)
- Impact: Unnecessary distance calculations and memory usage
- With limit=10, fetches 20, then filters to 10

### 3. Suboptimal HNSW Parameters (MEDIUM)
- `EfSearch=50` - conservative, increases search time
- `M=16` - reasonable but could be tuned per dataset size
- No adaptive parameters based on dataset size

### 4. Map Lookups in Hot Path (MEDIUM)
- Two map lookups per result: `idToUUID` and `uuidToID`
- Maps are fast but add overhead in tight loops

## Optimization Plan

### Phase 1: Reduce Allocations (Immediate Impact)

1. **Pre-allocated ID slice with capacity**
   ```go
   // Before: dynamic growth
   var ids []uuid.UUID
   
   // After: pre-allocate
   ids := make([]uuid.UUID, 0, limit*2)
   ```

2. **Object pooling for temporary vectors**
   ```go
   var queryPool = sync.Pool{
       New: func() interface{} {
           return make([]float32, dimension)
       },
   }
   ```

3. **Reuse embedding buffer in BuildFromStore**

### Phase 2: Smart Fetch Limit (High Impact)

1. **Adaptive fetch ratio**: Start with 1.5x instead of 2x
2. **Early termination**: If enough results after filtering, stop
3. **Configurable precision vs speed trade-off**

### Phase 3: Parameter Tuning (Medium Impact)

1. **Dynamic EfSearch based on dataset size**:
   - <1000 vectors: ef=20
   - 1000-10000: ef=30
   - >10000: ef=50

2. **Expose configuration** to users for their use case

### Phase 4: Data Structure Optimization (Long-term)

1. **Replace map with slice** for ID mapping when possible
2. **Batch ID resolution** in single operation
3. **Consider sync.Map** for concurrent access patterns

## Expected Improvements

| Metric | Before | After (Target) | Improvement |
|--------|--------|----------------|-------------|
| Search Latency | 88µs | 60µs | 32% faster |
| Allocations | 293/op | 50/op | 83% reduction |
| Memory/Query | 24KB | 8KB | 67% reduction |
| Scalability | 1.47x | 1.4x | Maintained |

## Implementation Priority

1. **P0**: Pre-allocated slices (easy, high impact)
2. **P1**: Reduce fetch multiplier 2x→1.5x
3. **P2**: Configurable HNSW parameters
4. **P3**: Object pooling for high-throughput scenarios
