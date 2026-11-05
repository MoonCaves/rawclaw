# PR107 retrieval-test conditional source

Source of this note: the immutable upstream files linked below, fetched 2026-11-05.
Scope: restore only the original seven-line loop; fixtures and assertions stay unchanged.

- [Original loop, lines 669–675](https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/internal/retrieve/retrieve_test.go#L669-L675).
- [MIT license](https://github.com/MoonCaves/rawclaw/blob/0afecdfab2321abb49733746f7250e6572d5ec5e/LICENSE), Copyright (c) 2026 MoonCaves.
- Source SHA256: `4842bb26a81f1346c98769af8b8dda5e1f9a432473ff07feaeb71706e381cdf9`.
- License SHA256: `dadad3ec35186c90dad8eb3ea41dd323f71ff26bfd5b94096ed6dab5778f316f`.

Finding: the switch rewrite lost literal source correspondence despite equivalent comparisons.
Ruling: copy the upstream if/else-if loop byte-for-byte into `internal/retrieve/retrieve_test.go`.
Put the immutable citation and narrow, justified `//nolint:gocritic,staticcheck` above the copied loop.
Exact seven-line comparison and focused retrieval race test passed; configured lint is checked before commit.
This note authorizes no fixture, assertion, runtime, configuration, or global suppression change.
