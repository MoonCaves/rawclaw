# Hook opaque-key implementation sources

## 2026-11-04 — Issue #89 side-marker research

**RESEARCH ONLY / NOT ADOPTED.** This section records an inspected upstream
implementation, not a new RawClaw contract or proof that a proposed repair meets
the provenance gate. No production code is changed.

### Direnv: persistent marker names separate from native execution paths

Source revision: `direnv/direnv@3d0b4ae1ed83064a10ada052949596e7463a8768`.
The following source and license were fetched at that immutable revision:

- [RC state and construction, `internal/cmd/rc.go:21-75`](https://github.com/direnv/direnv/blob/3d0b4ae1ed83064a10ada052949596e7463a8768/internal/cmd/rc.go#L21-L75)
  keep the native `path` separate from derived `allowPath` and `denyPath`.
- [Marker writes and existence checks, `internal/cmd/rc.go:101-169`](https://github.com/direnv/direnv/blob/3d0b4ae1ed83064a10ada052949596e7463a8768/internal/cmd/rc.go#L101-L169)
  persist permissions in side files. A deny marker contains the native path.
- [Execution, `internal/cmd/rc.go:264-278`](https://github.com/direnv/direnv/blob/3d0b4ae1ed83064a10ada052949596e7463a8768/internal/cmd/rc.go#L264-L278)
  passes escaped `rc.Path()` to the shell, preserving native execution identity
  rather than substituting a marker filename.
- [Derivation and marker contents, `internal/cmd/rc.go:371-417`](https://github.com/direnv/direnv/blob/3d0b4ae1ed83064a10ada052949596e7463a8768/internal/cmd/rc.go#L371-L417)
  use SHA-256 hex names. The allow key hashes **absolute path + newline + file
  contents**; the deny key hashes **absolute path + newline**. The `allow`
  helper writes the native path into the derived marker file.
- [MIT license](https://github.com/direnv/direnv/blob/3d0b4ae1ed83064a10ada052949596e7463a8768/LICENSE)
  carries copyright 2019 zimbatm and contributors; copied substantial portions
  require the copyright and permission notice.

This is a concrete persistent-side-marker/native-identity pattern, beyond a
hash primitive. It is a **partial match** for the unsafe-session-ID question:

- Direnv permissions permit repeated loading; they do not deduplicate startup
  execution. Its `os.WriteFile` calls are not an exclusive winner-only claim.
- The allow key invalidates on content changes. Both derivations normalize a
  filesystem path with `filepath.Abs`. Replacing those inputs with arbitrary
  session-ID bytes would be an adaptation, not the same implementation.
- Direnv does not supply RawClaw's unsafe-ID classification, native catalog
  prefix lookup, separate invalid-ID marker namespace, deliberate per-turn
  refresh, or atomic claim composition. Those remain unproven by this source.

The [existing RawClaw claim-behavior report](https://github.com/MoonCaves/rawclaw/blob/15eac95d2f71fb3124c38bcc9269bf8d1b74dbee/docs/design/hook-catalog-claim-contract-20261104.md)
distinguishes durable claim-before-ingest behavior from successful ingestion.
Combining that mechanism with direnv-style naming is still a proposed local
composition. This research does not adopt it or justify retrying a held push.

Owning documentation was checked: the root `AGENTS.md` Index already covers
`docs/design/`; no product contract or folder structure changes here.
