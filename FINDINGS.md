# Issue 50 implementation findings

- `CatalogEntry.Source` is already persisted by the catalog adapters.
- `sessionHitFromCatalog` currently drops that source, so `runResume` cannot
  select the correct runtime for a catalog hit and hardcodes Claude.
- `runResume` already stops before runtime discovery when `ResolveSession` or
  the consolidated store returns a match. Preserving `SessionHit.Source` and
  using it for catalog hits is the minimum root-cause fix; empty source remains
  Claude for legacy/stem-discovered hits.
- Genuine catalog/consolidated misses must retain the existing Codex,
  Antigravity, and Goose discovery fallback.

