# MIRA memory, recall, and soul corrections

## Goal

Correct the business logic defects found in MIRA's memory lifecycle, semantic recall, Soul profile evolution, and PostgreSQL retention path, while preserving backward compatibility for stored JSON and existing integrations.

## Design

1. **One derived-memory indexing pipeline**
   - Updates and consolidations rebuild tags, causal nodes, and causal relations through the same helper used by normal storage.
   - Updates validate content before mutating the existing record.
   - Tag storage and lookup are case-insensitive and normalized for new data.

2. **Recall correctness before pruning**
   - Score candidates before clustering so cluster representatives are chosen by the actual query score.
   - Use stable representative-based clustering to avoid single-link transitive chains.
   - Activate the configured early relevance threshold with a bounded fallback when all candidates are below it.
   - Filter invalid/future memories at the semantic boundary, guard optional logging, clamp recency, and make RRF ties deterministic.

3. **Soul observations and directives**
   - Count one observation per capture rather than keyword occurrences.
   - Make keyword extraction boundary-aware and basic-negation-aware.
   - Blend evolving communication, behavior, and emotional fields instead of replacing them on every capture.
   - Support both positive and negative natural-language directives, validate patch keys/types, and persist the change reason.
   - Allocate Soul recall budget between identity and clearly delimited non-normative memory evidence.

4. **Retention and encoding safety**
   - Implement PostgreSQL archive parity with SQLite, including dependent records and token accounting.
   - Make consolidation summaries rune-safe.

## Compatibility

- Existing snapshot JSON remains readable; the optional `change_reason` field is additive.
- The public storage interfaces remain unchanged where possible. The update interactor receives an optional causal detector so existing tests and embedders remain compatible.
- No migration is required for existing mixed-case tags because lookup uses case-insensitive matching; new tags are normalized.

## Verification

- Add focused regression tests before each implementation change.
- Run the complete Go suite with the `fts5` build tag, plus vet/build checks where supported.
- Verify the final Git state contains only the intended changes and push `main`.
