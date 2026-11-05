# Package attribution finding

- Kind: shrink (one comment line changed; no logic added).
- Location: `internal/source/opencode/opencode.go:1`.
- Evidence: at immutable RawClaw commit `9da5273c86bf9e07154b56febf8852791514bb7a`, the package comment names both OpenCode and Crush, while `ROADMAP.md` lists Crush as a planned reader and `AGENTS.md` lists implemented OpenCode separately.
- Historical attribution: https://github.com/MoonCaves/rawclaw/commit/d1d5df651affe4e66f62e25d796901ad1696bea1
- Proposed correction: remove only ` / Crush` from the first comment line. Preserve all executable bytes and fixtures.
- Limitation: historical intent and missing native fixtures do not establish universal historical incompatibility or runtime failure. Native compatibility remains open.
- Owning docs checked and intentionally unchanged. No user-visible behavior change or changelog entry.
