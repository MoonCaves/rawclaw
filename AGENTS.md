# RawClaw project guide

The current product-positioning draft starts at **Own your AI history** in `README.md`. Read that
draft before designing a feature or changing product copy, then read the constraints in
`ROADMAP.md`.

Keep the local keyword core sovereign: no required account, network, API key, model, daemon, or
hosted service. Add networked and model-backed capabilities through optional seams.

New transcript sources belong behind the existing Source adapter port. New embedding providers
belong behind the optional embedder seam. Preserve source provenance, keep interpretations separate
from the raw record, and do not fork the retrieval engine for one agent's file format.

## Index

README.md — product position, user promises, upgrade seams, setup, and usage.
ROADMAP.md — technical direction, architectural constraints, and non-goals.
CONTRIBUTING.md — build, test, lint, adapter, and pull-request guidance.
CHANGELOG.md — shipped user-visible behavior by release.
